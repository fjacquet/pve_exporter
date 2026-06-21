// Package telemetry manages the optional OTLP tracer provider lifecycle.
package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Config configures the tracer provider.
type Config struct {
	Endpoint       string
	Insecure       bool
	SamplingRate   float64
	ServiceName    string
	ServiceVersion string
}

// Manager owns the tracer provider and exposes a nil-safe accessor.
type Manager struct {
	enabled  bool
	provider *sdktrace.TracerProvider
	cfg      Config
}

// NewManager returns an uninitialized Manager.
func NewManager(cfg Config) *Manager { return &Manager{cfg: cfg} }

// Initialize sets up the OTLP trace exporter and installs the global provider.
func (m *Manager) Initialize(ctx context.Context) error {
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(m.cfg.Endpoint)}
	if m.cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exp, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return err
	}
	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(m.cfg.ServiceName),
		semconv.ServiceVersion(m.cfg.ServiceVersion),
	))
	if err != nil {
		return err
	}
	m.provider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(m.cfg.SamplingRate)),
	)
	otel.SetTracerProvider(m.provider)
	m.enabled = true
	return nil
}

// Shutdown flushes and stops the tracer provider.
func (m *Manager) Shutdown(ctx context.Context) error {
	if m.provider == nil {
		return nil
	}
	return m.provider.Shutdown(ctx)
}

// IsEnabled reports whether tracing was initialized.
func (m *Manager) IsEnabled() bool { return m.enabled }

// TracerProvider returns the active provider, or a no-op provider when disabled.
func (m *Manager) TracerProvider() trace.TracerProvider {
	if m.provider == nil {
		return otel.GetTracerProvider()
	}
	return m.provider
}
