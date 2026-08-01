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
		RunE:          func(cmd *cobra.Command, _ []string) error { return run(cmd.Context()) },
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

// run wires the exporter up and blocks until parent is cancelled, a termination
// signal arrives, or the HTTP server fails. parent is normally
// context.Background(); tests pass a cancellable context to drive shutdown.
func run(parent context.Context) error {
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

	ctx, cancel := context.WithCancel(parent)
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

	// --once: a single synchronous cycle with no HTTP server at all.
	if once {
		snap := collectFirstCycle(ctx, collector, cfg.GetCollectionTimeout())
		otlpExp := setupOTLP(ctx, cfg, store)
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
	registerEndpoints(mux, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}), cfg.Server.URI)
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

	// Optional OTLP metric export. Created before the first cycle so the
	// collection goroutine below can register instruments as soon as it lands.
	otlpExp := setupOTLP(ctx, cfg, store)

	// The listener is already bound above (ADR-0002): the first collection
	// runs in the background so an unreachable cluster cannot delay /livez,
	// /readyz or /metrics by a whole collection timeout. Until the first
	// snapshot lands the store serves an empty one, so /metrics is valid and
	// simply reports nothing.
	go func() {
		collectFirstCycle(ctx, collector, cfg.GetCollectionTimeout())
		if otlpExp != nil {
			if err := otlpExp.EnsureInstruments(); err != nil {
				log.WithError(err).Warn("OTLP instrument registration failed")
			}
		}
		collector.Run(ctx)
	}()

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
	case <-parent.Done():
		log.Info("context cancelled, shutting down")
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

// registerEndpoints wires every route the exporter serves onto mux. It is the
// single source of truth for the route table: run() and the tests both call it,
// so a test that probes a path exercises the registration that ships. metrics
// may be nil, in which case only the health and probe routes are registered.
func registerEndpoints(mux *http.ServeMux, metrics http.Handler, uri string) {
	if metrics != nil {
		mux.Handle(uri, metrics)
	}
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/livez", staticOKHandler)
	mux.HandleFunc("/readyz", staticOKHandler)
}

// collectFirstCycle runs the startup collection cycle under its own deadline.
func collectFirstCycle(ctx context.Context, collector *pve.Collector, timeout time.Duration) *pve.Snapshot {
	cycleCtx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()
	return collector.CollectOnce(cycleCtx)
}

// setupOTLP builds the optional OTLP metric exporter. It returns nil when OTLP
// is disabled or fails to initialise — neither is fatal.
func setupOTLP(ctx context.Context, cfg *models.Config, store *pve.SnapshotStore) *pve.OTLPExporter {
	if !cfg.IsOTelMetricsEnabled() {
		return nil
	}
	exp, err := pve.NewOTLPExporter(ctx, cfg.OpenTelemetry.Metrics, store, version)
	if err != nil {
		log.WithError(err).Warn("OTLP metrics init failed, continuing without OTLP")
		return nil
	}
	if err := exp.EnsureInstruments(); err != nil {
		log.WithError(err).Warn("OTLP instrument registration failed")
	}
	return exp
}

// staticOKHandler always answers 200 — no snapshot state, no collection
// state, nothing that can make it fail. /livez and /readyz both use it: a
// probe wired here can never be the reason a healthy process gets restarted
// or pulled from rotation. /health remains the endpoint for anything that
// wants to know whether a cluster is actually reachable.
func staticOKHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
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
