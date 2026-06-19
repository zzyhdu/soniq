package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthzReturnsOKJSON(t *testing.T) {
	router := NewRouter()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	contentType := response.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	var body struct {
		Status  string `json:"status"`
		Service string `json:"service"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
	if body.Service != "soniq-api" {
		t.Fatalf("service = %q, want soniq-api", body.Service)
	}
}

func TestNewRouterWithStorePreservesHealthzEndpoint(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestOpenAPIEndpointServesContract(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	contentType := response.Header().Get("Content-Type")
	if contentType != "application/yaml; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/yaml; charset=utf-8", contentType)
	}
	body := response.Body.String()
	if !strings.Contains(body, "openapi: 3.1.0") {
		t.Fatalf("body does not contain OpenAPI version")
	}
	if !strings.Contains(body, "/recordings/upload:") {
		t.Fatalf("body does not contain recordings upload path")
	}
}

func TestMetricsEndpointServesPrometheusMetrics(t *testing.T) {
	router := NewRouter()
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	contentType := response.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", contentType)
	}
	body := response.Body.String()
	if !strings.Contains(body, `soniq_http_requests_total{method="GET",route="/healthz",status="200"} 1`) {
		t.Fatalf("metrics output missing healthz request counter:\n%s", body)
	}
}

func TestAPIConsoleEndpointServesScalarPage(t *testing.T) {
	router := NewRouterWithStore(newFakeRecordingStore())
	request := httptest.NewRequest(http.MethodGet, "/api-console", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	contentType := response.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/html; charset=utf-8", contentType)
	}
	body := response.Body.String()
	if !strings.Contains(body, "@scalar/api-reference") {
		t.Fatalf("body does not contain Scalar API reference")
	}
	if !strings.Contains(body, "/openapi.yaml") {
		t.Fatalf("body does not contain OpenAPI URL")
	}
}

func TestHealthzRejectsNonGET(t *testing.T) {
	router := NewRouter()
	request := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}
