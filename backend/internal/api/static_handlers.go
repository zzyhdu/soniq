package api

import (
	"encoding/json"
	"net/http"

	apidocs "github.com/zzyhdu/soniq/backend/doc"
)

func openAPIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serveEmbeddedFile(w, apidocs.OpenAPI, "application/yaml; charset=utf-8")
}

func apiConsoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serveEmbeddedFile(w, apidocs.APIConsole, "text/html; charset=utf-8")
}

func serveEmbeddedFile(w http.ResponseWriter, body []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(body)
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(struct {
		Status  string `json:"status"`
		Service string `json:"service"`
	}{
		Status:  "ok",
		Service: "soniq-api",
	})
}
