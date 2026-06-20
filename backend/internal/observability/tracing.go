package observability

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	temporalotel "go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"
)

const defaultOTLPHTTPEndpoint = "http://localhost:4318"

// TracingConfig configures OpenTelemetry tracing for one Soniq process.
type TracingConfig struct {
	Enabled      bool
	ServiceName  string
	Environment  string
	OTLPEndpoint string
}

// Tracing owns process tracing dependencies and shutdown.
type Tracing struct {
	enabled        bool
	serviceName    string
	tracerProvider trace.TracerProvider
	propagator     propagation.TextMapPropagator
	shutdown       func(context.Context) error
}

// NewTracing creates an OpenTelemetry tracer provider. Disabled tracing returns
// no-op tracers so callers do not need separate instrumentation branches.
func NewTracing(ctx context.Context, config TracingConfig) (*Tracing, error) {
	config = normalizeTracingConfig(config)
	if !config.Enabled {
		return newNoopTracing(config), nil
	}

	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(config.OTLPEndpoint))
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	return newTracingWithExporter(config, exporter, true), nil
}

func newTracingWithExporter(config TracingConfig, exporter sdktrace.SpanExporter, batched bool) *Tracing {
	config = normalizeTracingConfig(config)
	propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource.NewWithAttributes("",
			attribute.String("service.name", config.ServiceName),
			attribute.String("deployment.environment", config.Environment),
		)),
	}
	if batched {
		options = append(options, sdktrace.WithBatcher(exporter))
	} else {
		options = append(options, sdktrace.WithSyncer(exporter))
	}
	provider := sdktrace.NewTracerProvider(options...)
	return &Tracing{
		enabled:        config.Enabled,
		serviceName:    config.ServiceName,
		tracerProvider: provider,
		propagator:     propagator,
		shutdown:       provider.Shutdown,
	}
}

func newNoopTracing(config TracingConfig) *Tracing {
	config = normalizeTracingConfig(config)
	return &Tracing{
		enabled:        false,
		serviceName:    config.ServiceName,
		tracerProvider: noop.NewTracerProvider(),
		propagator:     propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
		shutdown:       func(context.Context) error { return nil },
	}
}

func normalizeTracingConfig(config TracingConfig) TracingConfig {
	config.ServiceName = strings.TrimSpace(config.ServiceName)
	if config.ServiceName == "" {
		config.ServiceName = "soniq"
	}
	config.Environment = strings.TrimSpace(config.Environment)
	if config.Environment == "" {
		config.Environment = "development"
	}
	config.OTLPEndpoint = strings.TrimSpace(config.OTLPEndpoint)
	if config.OTLPEndpoint == "" {
		config.OTLPEndpoint = defaultOTLPHTTPEndpoint
	}
	return config
}

// Enabled reports whether tracing exports real spans.
func (t *Tracing) Enabled() bool {
	return t != nil && t.enabled
}

// Tracer returns a named tracer from this process tracer provider.
func (t *Tracing) Tracer(name string) trace.Tracer {
	if t == nil || t.tracerProvider == nil {
		return noop.NewTracerProvider().Tracer(name)
	}
	return t.tracerProvider.Tracer(name)
}

// Propagator returns the text map propagator shared by HTTP and Temporal.
func (t *Tracing) Propagator() propagation.TextMapPropagator {
	if t == nil || t.propagator == nil {
		return propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	}
	return t.propagator
}

// TemporalInterceptor returns a Temporal OpenTelemetry tracing interceptor.
func (t *Tracing) TemporalInterceptor() (interceptor.Interceptor, error) {
	if !t.Enabled() {
		return nil, nil
	}
	return temporalotel.NewTracingInterceptor(temporalotel.TracerOptions{
		Tracer:                  t.Tracer(t.serviceName + "/temporal"),
		TextMapPropagator:       t.Propagator(),
		AllowInvalidParentSpans: true,
	})
}

// Shutdown flushes and releases tracing resources.
func (t *Tracing) Shutdown(ctx context.Context) error {
	if t == nil || t.shutdown == nil {
		return nil
	}
	if err := t.shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown tracing: %w", err)
	}
	return nil
}
