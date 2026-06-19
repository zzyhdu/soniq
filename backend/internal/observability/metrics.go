package observability

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns the API Prometheus registry and collectors.
type Metrics struct {
	registry            *prometheus.Registry
	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
}

// NewMetrics builds a Soniq metrics registry with API collectors registered.
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	metrics := &Metrics{
		registry: registry,
		httpRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "soniq_http_requests_total",
			Help: "Total HTTP requests handled by the Soniq API.",
		}, []string{"route", "method", "status"}),
		httpRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "soniq_http_request_duration_seconds",
			Help:    "HTTP request duration for the Soniq API.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "method"}),
	}
	registry.MustRegister(metrics.httpRequestsTotal, metrics.httpRequestDuration)
	return metrics
}

// Handler returns an HTTP handler suitable for Prometheus scraping.
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		m = NewMetrics()
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ObserveHTTPRequest records one completed API request.
func (m *Metrics) ObserveHTTPRequest(route string, method string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	route = metricLabelValue(route, "unmatched")
	method = metricLabelValue(method, "UNKNOWN")
	statusLabel := strconv.Itoa(status)
	m.httpRequestsTotal.WithLabelValues(route, method, statusLabel).Inc()
	m.httpRequestDuration.WithLabelValues(route, method).Observe(duration.Seconds())
}

func metricLabelValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
