package observability

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.temporal.io/sdk/client"
)

const temporalMissingLabelValue = "none"

var (
	temporalMetricInvalidNameChar = regexp.MustCompile(`[^a-zA-Z0-9_:]`)
	temporalMetricInvalidFirst    = regexp.MustCompile(`^[^a-zA-Z_:]`)
)

var temporalSDKMetricLabels = []string{
	"namespace",
	"task_queue",
	"workflow_type",
	"activity_type",
	"poller_type",
	"worker_type",
	"operation",
	"status_code",
	"failure_reason",
	"cause",
	"client_name",
	"nexus_service",
	"nexus_operation",
}

var temporalSDKTimerBuckets = []float64{
	0.005,
	0.01,
	0.025,
	0.05,
	0.1,
	0.25,
	0.5,
	1,
	2.5,
	5,
	10,
	30,
	60,
	120,
	300,
	600,
}

// TemporalSDKMetricsHandler adapts Temporal SDK metrics into this process'
// Prometheus registry.
func (m *Metrics) TemporalSDKMetricsHandler() client.MetricsHandler {
	if m == nil || m.temporalSDKMetrics == nil {
		return client.MetricsNopHandler
	}
	return temporalSDKMetricsHandler{state: m.temporalSDKMetrics, tags: map[string]string{}}
}

type temporalSDKMetricsState struct {
	registry *prometheus.Registry
	mu       sync.Mutex

	counters   map[string]*prometheus.CounterVec
	gauges     map[string]*prometheus.GaugeVec
	histograms map[string]*prometheus.HistogramVec
	kinds      map[string]temporalSDKMetricKind
}

func newTemporalSDKMetricsState(registry *prometheus.Registry) *temporalSDKMetricsState {
	return &temporalSDKMetricsState{
		registry:   registry,
		counters:   map[string]*prometheus.CounterVec{},
		gauges:     map[string]*prometheus.GaugeVec{},
		histograms: map[string]*prometheus.HistogramVec{},
		kinds:      map[string]temporalSDKMetricKind{},
	}
}

type temporalSDKMetricKind string

const (
	temporalSDKMetricKindCounter   temporalSDKMetricKind = "counter"
	temporalSDKMetricKindGauge     temporalSDKMetricKind = "gauge"
	temporalSDKMetricKindHistogram temporalSDKMetricKind = "histogram"
)

type temporalSDKMetricsHandler struct {
	state *temporalSDKMetricsState
	tags  map[string]string
}

func (h temporalSDKMetricsHandler) WithTags(tags map[string]string) client.MetricsHandler {
	merged := make(map[string]string, len(h.tags)+len(tags))
	for key, value := range h.tags {
		merged[key] = value
	}
	for key, value := range tags {
		merged[key] = value
	}
	return temporalSDKMetricsHandler{state: h.state, tags: merged}
}

func (h temporalSDKMetricsHandler) Counter(name string) client.MetricsCounter {
	if h.state == nil {
		return temporalSDKCounter{}
	}
	counter := h.state.counter(name)
	return temporalSDKCounter{counter: counter, labelValues: temporalLabelValues(h.tags)}
}

func (h temporalSDKMetricsHandler) Gauge(name string) client.MetricsGauge {
	if h.state == nil {
		return temporalSDKGauge{}
	}
	gauge := h.state.gauge(name)
	return temporalSDKGauge{gauge: gauge, labelValues: temporalLabelValues(h.tags)}
}

func (h temporalSDKMetricsHandler) Timer(name string) client.MetricsTimer {
	if h.state == nil {
		return temporalSDKTimer{}
	}
	histogram := h.state.histogram(name)
	return temporalSDKTimer{histogram: histogram, labelValues: temporalLabelValues(h.tags)}
}

func (s *temporalSDKMetricsState) counter(name string) *prometheus.CounterVec {
	rawMetricName := temporalMetricName(name)
	s.mu.Lock()
	defer s.mu.Unlock()

	metricName := s.metricNameForKindLocked(rawMetricName, temporalSDKMetricKindCounter)
	if counter := s.counters[metricName]; counter != nil {
		return counter
	}
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricName,
		Help: "Temporal SDK counter " + metricName + ".",
	}, temporalSDKMetricLabels)
	s.registry.MustRegister(counter)
	s.counters[metricName] = counter
	return counter
}

func (s *temporalSDKMetricsState) gauge(name string) *prometheus.GaugeVec {
	rawMetricName := temporalMetricName(name)
	s.mu.Lock()
	defer s.mu.Unlock()

	metricName := s.metricNameForKindLocked(rawMetricName, temporalSDKMetricKindGauge)
	if gauge := s.gauges[metricName]; gauge != nil {
		return gauge
	}
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metricName,
		Help: "Temporal SDK gauge " + metricName + ".",
	}, temporalSDKMetricLabels)
	s.registry.MustRegister(gauge)
	s.gauges[metricName] = gauge
	return gauge
}

func (s *temporalSDKMetricsState) histogram(name string) *prometheus.HistogramVec {
	rawMetricName := temporalMetricName(name)
	s.mu.Lock()
	defer s.mu.Unlock()

	metricName := s.metricNameForKindLocked(rawMetricName, temporalSDKMetricKindHistogram)
	if histogram := s.histograms[metricName]; histogram != nil {
		return histogram
	}
	histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    metricName,
		Help:    "Temporal SDK timer " + metricName + ".",
		Buckets: temporalSDKTimerBuckets,
	}, temporalSDKMetricLabels)
	s.registry.MustRegister(histogram)
	s.histograms[metricName] = histogram
	return histogram
}

type temporalSDKCounter struct {
	counter     *prometheus.CounterVec
	labelValues []string
}

func (c temporalSDKCounter) Inc(delta int64) {
	if c.counter == nil || delta <= 0 {
		return
	}
	c.counter.WithLabelValues(c.labelValues...).Add(float64(delta))
}

type temporalSDKGauge struct {
	gauge       *prometheus.GaugeVec
	labelValues []string
}

func (g temporalSDKGauge) Update(value float64) {
	if g.gauge == nil {
		return
	}
	g.gauge.WithLabelValues(g.labelValues...).Set(value)
}

type temporalSDKTimer struct {
	histogram   *prometheus.HistogramVec
	labelValues []string
}

func (t temporalSDKTimer) Record(duration time.Duration) {
	if t.histogram == nil {
		return
	}
	t.histogram.WithLabelValues(t.labelValues...).Observe(duration.Seconds())
}

func temporalMetricName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "temporal_sdk_unknown"
	}
	name = temporalMetricInvalidNameChar.ReplaceAllString(name, "_")
	if temporalMetricInvalidFirst.MatchString(name) {
		name = "temporal_sdk_" + name
	}
	return name
}

func (s *temporalSDKMetricsState) metricNameForKindLocked(metricName string, kind temporalSDKMetricKind) string {
	if s.kinds == nil {
		s.kinds = map[string]temporalSDKMetricKind{}
	}
	if existing, ok := s.kinds[metricName]; !ok || existing == kind {
		s.kinds[metricName] = kind
		return metricName
	}

	conflictName := metricName + temporalSDKMetricKindSuffix(kind)
	if existing, ok := s.kinds[conflictName]; ok && existing != kind {
		conflictName = metricName + "_temporal_" + string(kind)
	}
	s.kinds[conflictName] = kind
	return conflictName
}

func temporalSDKMetricKindSuffix(kind temporalSDKMetricKind) string {
	switch kind {
	case temporalSDKMetricKindCounter:
		return "_counter"
	case temporalSDKMetricKindGauge:
		return "_gauge"
	case temporalSDKMetricKindHistogram:
		return "_seconds"
	default:
		return "_metric"
	}
}

func temporalLabelValues(tags map[string]string) []string {
	values := make([]string, 0, len(temporalSDKMetricLabels))
	for _, label := range temporalSDKMetricLabels {
		values = append(values, metricLabelValue(tags[label], temporalMissingLabelValue))
	}
	return values
}
