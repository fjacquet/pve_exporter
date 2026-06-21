// Command pve_exporter exports Proxmox VE metrics to Prometheus and OTLP.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/fjacquet/pve_exporter/internal/config"
	"github.com/fjacquet/pve_exporter/internal/logging"
	"github.com/fjacquet/pve_exporter/internal/models"
	"github.com/fjacquet/pve_exporter/internal/pve"
	"github.com/fjacquet/pve_exporter/internal/telemetry"
	"github.com/fjacquet/pve_exporter/internal/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

var (
	configFile string
	debug      bool
	once       bool
	apiTrace   bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:           "pve_exporter",
		Short:         "Proxmox VE Prometheus + OTLP exporter",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(_ *cobra.Command, _ []string) error { return run() },
	}
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Path to configuration file (required)")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&once, "once", false, "Run a single collection cycle and exit")
	rootCmd.PersistentFlags().BoolVar(&apiTrace, "trace", false, "Log every PVE API response body")
	_ = rootCmd.MarkPersistentFlagRequired("config")

	if err := rootCmd.Execute(); err != nil {
		log.WithError(err).Fatal("exporter failed")
	}
}

// server holds the long-lived components needed for config hot reload.
type server struct {
	configPath string
	safeCfg    *models.SafeConfig
	collector  *pve.Collector
	trace      bool
}

// ReloadConfig reloads config and rebuilds clients if the target set changed.
func (s *server) ReloadConfig(path string) error {
	changed, err := s.safeCfg.ReloadConfig(path)
	if err != nil {
		return err
	}
	if changed {
		log.Info("cluster set changed, rebuilding clients")
		s.collector.SetTargets(buildTargets(s.safeCfg.Get(), s.trace))
	}
	return nil
}

// buildTargets constructs a client per configured cluster.
func buildTargets(cfg *models.Config, trace bool) []pve.Target {
	targets := make([]pve.Target, 0, len(cfg.Clusters))
	for _, cl := range cfg.Clusters {
		targets = append(targets, pve.Target{
			Cfg:    cl,
			Client: pve.NewClient(cl, trace),
		})
	}
	return targets
}

func run() error {
	utils.LoadDotEnv(configFile)

	cfg, err := models.LoadConfig(configFile, utils.ResolveSecrets)
	if err != nil {
		return err
	}

	if err := logging.PrepareLogs(cfg.Server.LogName); err != nil {
		return err
	}
	if debug {
		log.SetLevel(log.DebugLevel)
	}
	log.WithField("version", version).Info("starting pve_exporter")

	safeCfg := models.NewSafeConfig(cfg, utils.ResolveSecrets)
	store := pve.NewSnapshotStore()
	collector := pve.NewCollector(
		buildTargets(cfg, apiTrace),
		store,
		cfg.GetCollectionInterval(),
		cfg.GetCollectionTimeout(),
		cfg.Collectors,
		cfg.GetMaxConcurrentTargets(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Optional tracing.
	var tracer *telemetry.Manager
	if cfg.IsOTelTracingEnabled() {
		tracer = telemetry.NewManager(telemetry.Config{
			Endpoint:       cfg.OpenTelemetry.Tracing.Endpoint,
			Insecure:       cfg.OpenTelemetry.Tracing.Insecure,
			SamplingRate:   cfg.OpenTelemetry.Tracing.SamplingRate,
			ServiceName:    "pve-exporter",
			ServiceVersion: version,
		})
		if err := tracer.Initialize(ctx); err != nil {
			log.WithError(err).Warn("tracing init failed, continuing without traces")
			tracer = nil
		}
	}

	// First collection cycle synchronously so /metrics is populated immediately.
	startCtx, startCancel := context.WithTimeout(ctx, cfg.GetCollectionTimeout()+5*time.Second)
	snap := collector.CollectOnce(startCtx)
	startCancel()

	// Optional OTLP metric export.
	var otlpExp *pve.OTLPExporter
	if cfg.IsOTelMetricsEnabled() {
		otlpExp, err = pve.NewOTLPExporter(ctx, cfg.OpenTelemetry.Metrics, store, version)
		if err != nil {
			log.WithError(err).Warn("OTLP metrics init failed, continuing without OTLP")
			otlpExp = nil
		} else if err := otlpExp.EnsureInstruments(); err != nil {
			log.WithError(err).Warn("OTLP instrument registration failed")
		}
	}

	if once {
		if debug {
			dumpSamples(snap)
		}
		shutdown(ctx, nil, otlpExp, tracer)
		return nil
	}

	// Register Prometheus collector.
	registry := prometheus.NewRegistry()
	registry.MustRegister(pve.NewPromCollector(store))

	mux := http.NewServeMux()
	mux.Handle(cfg.Server.URI, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	httpServer := &http.Server{
		Addr:              cfg.GetServerAddress(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.WithField("addr", httpServer.Addr).Info("serving metrics")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Background collection loop.
	go collector.Run(ctx)

	// Keep OTLP instruments in sync with newly-seen metric names.
	if otlpExp != nil {
		go syncInstruments(ctx, otlpExp, cfg.GetCollectionInterval())
	}

	// Config hot reload.
	srv := &server{configPath: configFile, safeCfg: safeCfg, collector: collector, trace: apiTrace}
	config.SetupSIGHUPHandler(configFile, srv.ReloadConfig)
	watcher, werr := config.WatchConfigFile(configFile, srv.ReloadConfig)
	if werr != nil {
		log.WithError(werr).Warn("config file watch disabled")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case s := <-sig:
		log.WithField("signal", s.String()).Info("shutting down")
	case err := <-serverErr:
		log.WithError(err).Error("HTTP server error")
	}

	if watcher != nil {
		_ = watcher.Close()
	}
	cancel()
	shutdown(context.Background(), httpServer, otlpExp, tracer)
	return nil
}

// syncInstruments periodically re-registers OTLP instruments for new metrics.
func syncInstruments(ctx context.Context, exp *pve.OTLPExporter, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := exp.EnsureInstruments(); err != nil {
				log.WithError(err).Debug("OTLP instrument sync failed")
			}
		}
	}
}

// shutdown gracefully stops the HTTP server, OTLP exporter and tracer.
func shutdown(ctx context.Context, httpServer *http.Server, otlpExp *pve.OTLPExporter, tracer *telemetry.Manager) {
	shutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if httpServer != nil {
		if err := httpServer.Shutdown(shutCtx); err != nil {
			log.WithError(err).Warn("HTTP server shutdown error")
		}
	}
	if otlpExp != nil {
		if err := otlpExp.Shutdown(shutCtx); err != nil {
			log.WithError(err).Warn("OTLP shutdown error")
		}
	}
	if tracer != nil {
		if err := tracer.Shutdown(shutCtx); err != nil {
			log.WithError(err).Warn("tracer shutdown error")
		}
	}
}

// dumpSamples prints all collected samples in sorted exposition-like form, for
// --once --debug validation against docs/metrics.md.
func dumpSamples(snap *pve.Snapshot) {
	var lines []string
	for _, name := range snap.MetricNames() {
		for _, s := range snap.SamplesFor(name) {
			var parts []string
			for _, l := range s.Labels {
				parts = append(parts, fmt.Sprintf("%s=%q", l.Name, l.Value))
			}
			lines = append(lines, fmt.Sprintf("%s{%s} %g", s.Name, strings.Join(parts, ","), s.Value))
		}
	}
	sort.Strings(lines)
	for _, l := range lines {
		fmt.Println(l)
	}
}
