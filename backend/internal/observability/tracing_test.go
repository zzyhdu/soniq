package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNewTracingDisabledReturnsNoopTracing(t *testing.T) {
	tracing, err := NewTracing(context.Background(), TracingConfig{
		ServiceName: "soniq-api",
	})
	if err != nil {
		t.Fatalf("NewTracing() error = %v, want nil", err)
	}
	if tracing.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}
	if interceptor, err := tracing.TemporalInterceptor(); err != nil || interceptor != nil {
		t.Fatalf("TemporalInterceptor() = (%v, %v), want nil interceptor and nil error", interceptor, err)
	}
	if err := tracing.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}
}

func TestTracingRecordsServiceResourceAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tracing := newTracingWithExporter(TracingConfig{
		Enabled:     true,
		ServiceName: "soniq-api",
		Environment: "test",
	}, exporter, false)
	ctx := context.Background()
	_, span := tracing.Tracer("test").Start(ctx, "upload")
	span.End()

	spans := exporter.GetSpans()
	if got, want := len(spans), 1; got != want {
		t.Fatalf("spans = %d, want %d", got, want)
	}
	attrs := spans[0].Resource.Attributes()
	assertAttribute(t, attrs, "service.name", "soniq-api")
	assertAttribute(t, attrs, "deployment.environment", "test")
}

func TestTracingCreatesTemporalInterceptorWhenEnabled(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tracing := newTracingWithExporter(TracingConfig{
		Enabled:     true,
		ServiceName: "soniq-worker",
		Environment: "test",
	}, exporter, false)

	interceptor, err := tracing.TemporalInterceptor()
	if err != nil {
		t.Fatalf("TemporalInterceptor() error = %v, want nil", err)
	}
	if interceptor == nil {
		t.Fatal("TemporalInterceptor() = nil, want interceptor")
	}
}

func assertAttribute(t *testing.T, attrs []attribute.KeyValue, key string, want string) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key {
			if got := attr.Value.AsString(); got != want {
				t.Fatalf("%s = %q, want %q", key, got, want)
			}
			return
		}
	}
	t.Fatalf("attribute %q missing from %#v", key, attrs)
}
