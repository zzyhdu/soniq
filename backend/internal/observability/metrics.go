package observability

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	MetricsResultSuccess = "success"
	MetricsResultError   = "error"
	MetricsResultUnknown = "unknown"

	MetricsRecordingStatusCompleted = "completed"
	MetricsRecordingStatusFailed    = "failed"
	MetricsRecordingStatusUnknown   = "unknown"
)

// Metrics owns the Prometheus registry and collectors for Soniq processes.
type Metrics struct {
	registry                            *prometheus.Registry
	httpRequestsTotal                   *prometheus.CounterVec
	httpRequestDuration                 *prometheus.HistogramVec
	workerActivitiesTotal               *prometheus.CounterVec
	workerActivityDuration              *prometheus.HistogramVec
	recordingTerminalStatusUpdatesTotal *prometheus.CounterVec
	purgeArtifactsClaimed               prometheus.Counter
	purgeArtifactsDeleted               prometheus.Counter
	purgeArtifactsFailed                prometheus.Counter
	purgeCleanupRunDuration             *prometheus.HistogramVec
}

// NewMetrics builds a Soniq metrics registry with process collectors registered.
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
		workerActivitiesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "soniq_worker_activities_total",
			Help: "Total Temporal activities executed by Soniq workers.",
		}, []string{"activity", "result"}),
		workerActivityDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "soniq_worker_activity_duration_seconds",
			Help:    "Temporal activity execution duration for Soniq workers.",
			Buckets: prometheus.DefBuckets,
		}, []string{"activity"}),
		recordingTerminalStatusUpdatesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "soniq_recording_terminal_status_updates_total",
			Help: "Total recording terminal status updates observed by Soniq workers.",
		}, []string{"status"}),
		purgeArtifactsClaimed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "soniq_purge_artifacts_claimed_total",
			Help: "Total purge artifact cleanup rows claimed by Soniq workers.",
		}),
		purgeArtifactsDeleted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "soniq_purge_artifacts_deleted_total",
			Help: "Total purge artifacts successfully deleted by Soniq workers.",
		}),
		purgeArtifactsFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "soniq_purge_artifacts_failed_total",
			Help: "Total purge artifact cleanup attempts that failed in Soniq workers.",
		}),
		purgeCleanupRunDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "soniq_purge_cleanup_run_duration_seconds",
			Help:    "Purge artifact cleanup run duration for Soniq workers.",
			Buckets: prometheus.DefBuckets,
		}, []string{"result"}),
	}
	registry.MustRegister(
		metrics.httpRequestsTotal,
		metrics.httpRequestDuration,
		metrics.workerActivitiesTotal,
		metrics.workerActivityDuration,
		metrics.recordingTerminalStatusUpdatesTotal,
		metrics.purgeArtifactsClaimed,
		metrics.purgeArtifactsDeleted,
		metrics.purgeArtifactsFailed,
		metrics.purgeCleanupRunDuration,
	)
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

// ObserveWorkerActivity records one completed Temporal activity execution.
func (m *Metrics) ObserveWorkerActivity(activity string, result string, duration time.Duration) {
	if m == nil {
		return
	}
	activity = metricLabelValue(activity, "unknown")
	result = metricResultLabel(result)
	m.workerActivitiesTotal.WithLabelValues(activity, result).Inc()
	m.workerActivityDuration.WithLabelValues(activity).Observe(duration.Seconds())
}

// ObserveRecordingTerminalStatus records one persisted recording terminal status update.
func (m *Metrics) ObserveRecordingTerminalStatus(status string) {
	if m == nil {
		return
	}
	m.recordingTerminalStatusUpdatesTotal.WithLabelValues(metricRecordingStatusLabel(status)).Inc()
}

// ObservePurgeArtifactsClaimed records claimed purge artifact cleanup rows.
func (m *Metrics) ObservePurgeArtifactsClaimed(count int) {
	if m == nil || count <= 0 {
		return
	}
	m.purgeArtifactsClaimed.Add(float64(count))
}

// ObservePurgeArtifactDeleted records one successfully deleted purge artifact.
func (m *Metrics) ObservePurgeArtifactDeleted() {
	if m == nil {
		return
	}
	m.purgeArtifactsDeleted.Inc()
}

// ObservePurgeArtifactFailed records one failed purge artifact cleanup attempt.
func (m *Metrics) ObservePurgeArtifactFailed() {
	if m == nil {
		return
	}
	m.purgeArtifactsFailed.Inc()
}

// ObservePurgeCleanupRun records one purge cleanup run.
func (m *Metrics) ObservePurgeCleanupRun(result string, duration time.Duration) {
	if m == nil {
		return
	}
	m.purgeCleanupRunDuration.WithLabelValues(metricResultLabel(result)).Observe(duration.Seconds())
}

func metricLabelValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func metricResultLabel(result string) string {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case MetricsResultSuccess:
		return MetricsResultSuccess
	case MetricsResultError:
		return MetricsResultError
	default:
		return MetricsResultUnknown
	}
}

func metricRecordingStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case MetricsRecordingStatusCompleted:
		return MetricsRecordingStatusCompleted
	case MetricsRecordingStatusFailed:
		return MetricsRecordingStatusFailed
	default:
		return MetricsRecordingStatusUnknown
	}
}
