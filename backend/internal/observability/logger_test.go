package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewLoggerWritesJSONWithServiceAndLevel(t *testing.T) {
	var output bytes.Buffer
	logger, err := NewLogger(LoggerConfig{
		Service: "soniq-api",
		Format:  "json",
		Level:   "debug",
		Output:  &output,
	})
	if err != nil {
		t.Fatalf("NewLogger returned error: %v", err)
	}

	logger.Debug("test event", "event", "logger_test")

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode log entry: %v; output=%s", err, output.String())
	}
	if entry["service"] != "soniq-api" {
		t.Fatalf("service = %v, want soniq-api", entry["service"])
	}
	if entry["event"] != "logger_test" {
		t.Fatalf("event = %v, want logger_test", entry["event"])
	}
	if entry["level"] != "DEBUG" {
		t.Fatalf("level = %v, want DEBUG", entry["level"])
	}
}

func TestNewLoggerRejectsInvalidConfig(t *testing.T) {
	if _, err := NewLogger(LoggerConfig{Format: "yaml", Level: "info"}); err == nil {
		t.Fatal("NewLogger accepted invalid format, want error")
	}
	if _, err := NewLogger(LoggerConfig{Format: "json", Level: "verbose"}); err == nil {
		t.Fatal("NewLogger accepted invalid level, want error")
	}
}

func TestRequestIDHelpers(t *testing.T) {
	generated := NewRequestID()
	if !strings.HasPrefix(generated, "req_") {
		t.Fatalf("generated request id = %q, want req_ prefix", generated)
	}

	requestID, ok := RequestIDFromHeader(" req_test ")
	if !ok || requestID != "req_test" {
		t.Fatalf("RequestIDFromHeader = %q/%v, want req_test/true", requestID, ok)
	}
	if requestID, ok := RequestIDFromHeader("bad\nid"); ok || requestID != "" {
		t.Fatalf("RequestIDFromHeader accepted control characters: %q/%v", requestID, ok)
	}

	ctx := ContextWithRequestID(context.Background(), "req_context")
	got, ok := RequestIDFromContext(ctx)
	if !ok || got != "req_context" {
		t.Fatalf("RequestIDFromContext = %q/%v, want req_context/true", got, ok)
	}
}
