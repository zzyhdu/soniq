package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/zzyhdu/soniq/backend/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRequestLoggingMiddlewarePropagatesRequestIDAndWritesAccessLog(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := chi.NewRouter()
	router.Use(requestLoggingMiddleware(logger, nil))
	router.Get("/workspaces/{workspace_id}/recordings/{recording_id}", func(w http.ResponseWriter, r *http.Request) {
		setRequestLogUserID(r.Context(), "usr_1")
		setRequestLogWorkspaceID(r.Context(), "wsp_1")
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_1/recordings/rec_1?token=secret", nil)
	request.Header.Set(observability.RequestIDHeader, "req_test")
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if got := response.Header().Get(observability.RequestIDHeader); got != "req_test" {
		t.Fatalf("response request id = %q, want req_test", got)
	}
	entries := decodeJSONLogEntries(t, logs.String())
	entry := findLogEvent(t, entries, "http_request_completed")
	assertLogField(t, entry, "request_id", "req_test")
	assertLogField(t, entry, "method", http.MethodGet)
	assertLogField(t, entry, "route", "/workspaces/{workspace_id}/recordings/{recording_id}")
	assertLogNumber(t, entry, "status", http.StatusNoContent)
	assertLogField(t, entry, "user_id", "usr_1")
	assertLogField(t, entry, "workspace_id", "wsp_1")
	if _, ok := entry["duration_ms"]; !ok {
		t.Fatalf("duration_ms missing from log entry: %#v", entry)
	}
	logOutput := logs.String()
	if strings.Contains(logOutput, "token=secret") || strings.Contains(logOutput, "Bearer secret") {
		t.Fatalf("access log leaked sensitive request data: %s", logOutput)
	}
}

func TestRequestLoggingMiddlewareGeneratesRequestID(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := chi.NewRouter()
	router.Use(requestLoggingMiddleware(logger, nil))
	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	requestID := response.Header().Get(observability.RequestIDHeader)
	if !strings.HasPrefix(requestID, "req_") {
		t.Fatalf("generated response request id = %q, want req_ prefix", requestID)
	}
	entry := findLogEvent(t, decodeJSONLogEntries(t, logs.String()), "http_request_completed")
	assertLogField(t, entry, "request_id", requestID)
	assertLogField(t, entry, "route", "/healthz")
}

func TestRequestLoggingMiddlewareLogsOnlyFiveHundredErrorsAsAPIErrors(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := chi.NewRouter()
	router.Use(requestLoggingMiddleware(logger, nil))
	router.Get("/server-error", func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "database unavailable")
	})
	router.Get("/bad-request", func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusBadRequest, errorCodeValidationFailed, "invalid json")
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/server-error", nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/bad-request", nil))

	entries := decodeJSONLogEntries(t, logs.String())
	apiErrors := filterLogEvent(entries, "api_error")
	if got, want := len(apiErrors), 1; got != want {
		t.Fatalf("api_error log count = %d, want %d; entries=%#v", got, want, entries)
	}
	assertLogNumber(t, apiErrors[0], "status", http.StatusInternalServerError)
	assertLogField(t, apiErrors[0], "error_code", string(errorCodeInternalError))
	assertLogField(t, apiErrors[0], "error", "database unavailable")
}

func TestRequestLoggingMiddlewareKeepsAPIErrorDetailsThroughTracingMiddleware(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	defer tracerProvider.Shutdown(t.Context()) //nolint:errcheck
	router := chi.NewRouter()
	router.Use(requestLoggingMiddleware(logger, nil))
	router.Use(requestTracingMiddleware(HTTPTracingConfig{
		Tracer:     tracerProvider.Tracer("test"),
		Propagator: propagation.TraceContext{},
	}))
	router.Get("/server-error", func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "database unavailable")
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/server-error", nil))

	entry := findLogEvent(t, decodeJSONLogEntries(t, logs.String()), "api_error")
	assertLogNumber(t, entry, "status", http.StatusInternalServerError)
	assertLogField(t, entry, "error_code", string(errorCodeInternalError))
	assertLogField(t, entry, "error", "database unavailable")
}

func TestRequestLoggingMiddlewareRecordsHTTPMetricsWithRouteTemplate(t *testing.T) {
	metrics := observability.NewMetrics()
	router := chi.NewRouter()
	router.Use(requestLoggingMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), metrics))
	router.Get("/workspaces/{workspace_id}/recordings/{recording_id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	router.Method(http.MethodGet, "/metrics", metrics.Handler())

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/workspaces/wsp_1/recordings/rec_1", nil))
	metricsResponse := httptest.NewRecorder()
	router.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if metricsResponse.Code != http.StatusOK {
		t.Fatalf("metrics status code = %d, want %d", metricsResponse.Code, http.StatusOK)
	}
	body := metricsResponse.Body.String()
	if !strings.Contains(body, `soniq_http_requests_total{method="GET",route="/workspaces/{workspace_id}/recordings/{recording_id}",status="204"} 1`) {
		t.Fatalf("metrics output missing templated request counter:\n%s", body)
	}
	if !strings.Contains(body, `soniq_http_request_duration_seconds_bucket{method="GET",route="/workspaces/{workspace_id}/recordings/{recording_id}",le=`) {
		t.Fatalf("metrics output missing templated duration histogram:\n%s", body)
	}
	if strings.Contains(body, "wsp_1") || strings.Contains(body, "rec_1") {
		t.Fatalf("metrics output leaked high-cardinality IDs:\n%s", body)
	}
}

func TestRequestTracingMiddlewareRecordsRouteAndContextAttributes(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	defer tracerProvider.Shutdown(t.Context()) //nolint:errcheck
	router := chi.NewRouter()
	router.Use(requestLoggingMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), nil))
	router.Use(requestTracingMiddleware(HTTPTracingConfig{
		Tracer:     tracerProvider.Tracer("test"),
		Propagator: propagation.TraceContext{},
	}))
	router.Get("/workspaces/{workspace_id}/recordings/{recording_id}", func(w http.ResponseWriter, r *http.Request) {
		setRequestLogWorkspaceID(r.Context(), "wsp_1")
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/workspaces/wsp_1/recordings/rec_1?token=secret", nil)
	request.Header.Set(observability.RequestIDHeader, "req_trace")
	router.ServeHTTP(httptest.NewRecorder(), request)

	spans := spanRecorder.Ended()
	if got, want := len(spans), 1; got != want {
		t.Fatalf("ended spans = %d, want %d", got, want)
	}
	span := spans[0]
	if span.Name() != "GET /workspaces/{workspace_id}/recordings/{recording_id}" {
		t.Fatalf("span name = %q, want templated route", span.Name())
	}
	attrs := span.Attributes()
	assertTraceAttributeString(t, attrs, "request_id", "req_trace")
	assertTraceAttributeString(t, attrs, "http.route", "/workspaces/{workspace_id}/recordings/{recording_id}")
	assertTraceAttributeString(t, attrs, "workspace_id", "wsp_1")
	assertTraceAttributeString(t, attrs, "recording_id", "rec_1")
	assertTraceAttributeInt(t, attrs, "http.response.status_code", http.StatusNoContent)
	if containsTraceAttributeValue(attrs, "token=secret") {
		t.Fatalf("trace attributes leaked query secret: %#v", attrs)
	}
}

func decodeJSONLogEntries(t *testing.T, output string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func assertTraceAttributeString(t *testing.T, attrs []attribute.KeyValue, key string, want string) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key {
			if got := attr.Value.AsString(); got != want {
				t.Fatalf("trace attribute %s = %q, want %q", key, got, want)
			}
			return
		}
	}
	t.Fatalf("trace attribute %q missing from %#v", key, attrs)
}

func assertTraceAttributeInt(t *testing.T, attrs []attribute.KeyValue, key string, want int) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key {
			if got := int(attr.Value.AsInt64()); got != want {
				t.Fatalf("trace attribute %s = %d, want %d", key, got, want)
			}
			return
		}
	}
	t.Fatalf("trace attribute %q missing from %#v", key, attrs)
}

func containsTraceAttributeValue(attrs []attribute.KeyValue, value string) bool {
	for _, attr := range attrs {
		if strings.Contains(attr.Value.Emit(), value) {
			return true
		}
	}
	return false
}

func findLogEvent(t *testing.T, entries []map[string]any, event string) map[string]any {
	t.Helper()
	for _, entry := range entries {
		if entry["event"] == event {
			return entry
		}
	}
	t.Fatalf("event %q not found in entries %#v", event, entries)
	return nil
}

func filterLogEvent(entries []map[string]any, event string) []map[string]any {
	filtered := []map[string]any{}
	for _, entry := range entries {
		if entry["event"] == event {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func assertLogField(t *testing.T, entry map[string]any, key string, want string) {
	t.Helper()
	if got, ok := entry[key].(string); !ok || got != want {
		t.Fatalf("%s = %#v, want %q", key, entry[key], want)
	}
}

func assertLogNumber(t *testing.T, entry map[string]any, key string, want int) {
	t.Helper()
	got, ok := entry[key].(float64)
	if !ok || int(got) != want {
		t.Fatalf("%s = %#v, want %d", key, entry[key], want)
	}
}
