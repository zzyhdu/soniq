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
)

func TestRequestLoggingMiddlewarePropagatesRequestIDAndWritesAccessLog(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := chi.NewRouter()
	router.Use(requestLoggingMiddleware(logger))
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
	router.Use(requestLoggingMiddleware(logger))
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
	router.Use(requestLoggingMiddleware(logger))
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
