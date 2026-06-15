package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadyzReturnsReadyWhenAllChecksPass(t *testing.T) {
	router := NewRouterWithReadiness(fakeReadinessChecker{report: ReadinessReport{
		Checks: map[string]ReadinessCheck{
			"postgres":       ReadinessCheckOK(),
			"migrations":     ReadinessCheckOK(),
			"temporal":       ReadinessCheckOK(),
			"object_storage": ReadinessCheckOK(),
		},
	}})
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
		Errors map[string]string `json:"errors,omitempty"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Status != "ready" {
		t.Fatalf("status = %q, want ready", body.Status)
	}
	for _, checkName := range []string{"postgres", "migrations", "temporal", "object_storage"} {
		if body.Checks[checkName] != "ok" {
			t.Fatalf("check %s = %q, want ok", checkName, body.Checks[checkName])
		}
	}
	if len(body.Errors) != 0 {
		t.Fatalf("errors = %+v, want empty", body.Errors)
	}
}

func TestReadyzReturnsUnavailableWhenAnyCheckFails(t *testing.T) {
	router := NewRouterWithReadiness(fakeReadinessChecker{report: ReadinessReport{
		Checks: map[string]ReadinessCheck{
			"postgres":       ReadinessCheckOK(),
			"migrations":     ReadinessCheckFailed("schema migration version 5 is below required 6"),
			"temporal":       ReadinessCheckOK(),
			"object_storage": ReadinessCheckFailed("object storage root is not writable"),
		},
	}})
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
		Errors map[string]string `json:"errors,omitempty"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Status != "not_ready" {
		t.Fatalf("status = %q, want not_ready", body.Status)
	}
	if body.Checks["migrations"] != "error" || body.Checks["object_storage"] != "error" {
		t.Fatalf("checks = %+v, want failed migrations and object_storage", body.Checks)
	}
	if body.Errors["migrations"] == "" || body.Errors["object_storage"] == "" {
		t.Fatalf("errors = %+v, want failed check messages", body.Errors)
	}
	output := response.Body.String()
	for _, sensitive := range []string{"postgres://", "secret", "/home/yangsan"} {
		if strings.Contains(output, sensitive) {
			t.Fatalf("readyz response leaked sensitive value %q: %s", sensitive, output)
		}
	}
}

func TestReadyzRejectsNonGET(t *testing.T) {
	router := NewRouter()
	request := httptest.NewRequest(http.MethodPost, "/readyz", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

type fakeReadinessChecker struct {
	report ReadinessReport
}

func (c fakeReadinessChecker) CheckReadiness(context.Context) ReadinessReport {
	return c.report
}
