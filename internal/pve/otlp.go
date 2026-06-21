package pve

import (
	"context"
	"sync"
	"time"

	"github.com/fjacquet/pve_exporter/internal/models"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const otlpServiceName = "pve-exporter"

// OTLPExporter mirrors the snapshot to OTLP using observable gauges driven by a
// periodic reader. Each distinct metric name is registered once; its callback
// reads the latest snapshot on every push.
type OTLPExporter struct {
	provider   *sdkmetric.MeterProvider
	meter      metric.Meter
	store      *SnapshotStore
	mu         sync.Mutex
	registered map[string]struct{}
}

// NewOTLPExporter builds an exporter pushing on oc.Interval.
func NewOTLPExporter(ctx context.Context, oc models.OTelExportConfig, store *SnapshotStore, version string) (*OTLPExporter, error) {
	opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(oc.Endpoint)}
	if oc.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	exp, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	interval := 10 * time.Second
	if d, err := time.ParseDuration(oc.Interval); err == nil && d > 0 {
		interval = d
	}
	reader := sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(interval))
	return newOTLPExporterWithReader(ctx, reader, store, version)
}

// newOTLPExporterWithReader builds an exporter around an arbitrary reader so
// tests can inject a ManualReader.
func newOTLPExporterWithReader(ctx context.Context, reader sdkmetric.Reader, store *SnapshotStore, version string) (*OTLPExporter, error) {
	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(otlpServiceName),
		semconv.ServiceVersion(version),
	))
	if err != nil {
		return nil, err
	}
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)
	return &OTLPExporter{
		provider:   provider,
		meter:      provider.Meter(otlpServiceName),
		store:      store,
		registered: make(map[string]struct{}),
	}, nil
}

// EnsureInstruments registers an observable gauge for each metric name seen in
// the current snapshot that is not yet registered. It is idempotent and safe to
// call after every collection cycle.
func (e *OTLPExporter) EnsureInstruments() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	snap := e.store.Load()
	for _, name := range snap.MetricNames() {
		if _, ok := e.registered[name]; ok {
			continue
		}
		metricName := name
		_, err := e.meter.Float64ObservableGauge(metricName,
			metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
				for _, s := range e.store.Load().SamplesFor(metricName) {
					o.Observe(s.Value, metric.WithAttributes(attrsFor(s.Labels)...))
				}
				return nil
			}),
		)
		if err != nil {
			return err
		}
		e.registered[name] = struct{}{}
	}
	return nil
}

// Shutdown flushes and stops the meter provider.
func (e *OTLPExporter) Shutdown(ctx context.Context) error {
	return e.provider.Shutdown(ctx)
}

// attrsFor converts a label slice to OTel attributes.
func attrsFor(labels []Label) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, len(labels))
	for i, l := range labels {
		attrs[i] = attribute.String(l.Name, l.Value)
	}
	return attrs
}
