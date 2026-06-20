package observability

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMetricsRecordsWorkerActivityAndRecordingTerminalStatus(t *testing.T) {
	metrics := NewMetrics()

	metrics.ObserveWorkerActivity("TranscribeRecordingAudio", MetricsResultSuccess, 25*time.Millisecond)
	metrics.ObserveRecordingTerminalStatus(MetricsRecordingStatusCompleted)

	body := collectMetricsBody(t, metrics)
	if !strings.Contains(body, `soniq_worker_activities_total{activity="TranscribeRecordingAudio",result="success"} 1`) {
		t.Fatalf("metrics output missing worker activity counter:\n%s", body)
	}
	if !strings.Contains(body, `soniq_worker_activity_duration_seconds_bucket{activity="TranscribeRecordingAudio",le=`) {
		t.Fatalf("metrics output missing worker activity duration histogram:\n%s", body)
	}
	if !strings.Contains(body, `soniq_recording_terminal_status_updates_total{status="completed"} 1`) {
		t.Fatalf("metrics output missing recording terminal status counter:\n%s", body)
	}
}

func TestMetricsRecordsPurgeCleanup(t *testing.T) {
	metrics := NewMetrics()

	metrics.ObservePurgeArtifactsClaimed(2)
	metrics.ObservePurgeArtifactDeleted()
	metrics.ObservePurgeArtifactFailed()
	metrics.ObservePurgeCleanupRun(MetricsResultError, 10*time.Millisecond)

	body := collectMetricsBody(t, metrics)
	for _, want := range []string{
		`soniq_purge_artifacts_claimed_total 2`,
		`soniq_purge_artifacts_deleted_total 1`,
		`soniq_purge_artifacts_failed_total 1`,
		`soniq_purge_cleanup_run_duration_seconds_bucket{result="error",le=`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, body)
		}
	}
}

func TestTemporalSDKMetricsHandlerRecordsTemporalMetrics(t *testing.T) {
	metrics := NewMetrics()
	handler := metrics.TemporalSDKMetricsHandler().WithTags(map[string]string{
		"namespace":     "default",
		"task_queue":    "soniq-audio-pipeline",
		"activity_type": "TranscribeRecordingAudio",
	})

	handler.Counter("temporal_activity_execution_failed").Inc(2)
	handler.Gauge("temporal_worker_task_slots_available").Update(3)
	handler.Timer("temporal_activity_execution_latency").Record(25 * time.Millisecond)

	body := collectMetricsBody(t, metrics)
	for _, want := range []string{
		`temporal_activity_execution_failed{`,
		`activity_type="TranscribeRecordingAudio"`,
		`namespace="default"`,
		`task_queue="soniq-audio-pipeline"`,
		`} 2`,
		`temporal_worker_task_slots_available{`,
		`} 3`,
		`temporal_activity_execution_latency_bucket{`,
		`le=`,
		`le="600"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "rec_") || strings.Contains(body, "wsp_") {
		t.Fatalf("Temporal metrics output leaked high-cardinality app IDs:\n%s", body)
	}
}

func TestTemporalSDKMetricsHandlerMergesTags(t *testing.T) {
	metrics := NewMetrics()
	handler := metrics.TemporalSDKMetricsHandler().
		WithTags(map[string]string{"namespace": "default", "task_queue": "original"}).
		WithTags(map[string]string{"task_queue": "soniq-audio-pipeline", "workflow_type": "RecordingProcessingWorkflow"})

	handler.Counter("temporal_workflow_completed").Inc(1)

	body := collectMetricsBody(t, metrics)
	for _, want := range []string{
		`temporal_workflow_completed{`,
		`namespace="default"`,
		`task_queue="soniq-audio-pipeline"`,
		`workflow_type="RecordingProcessingWorkflow"`,
		`} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, body)
		}
	}
}

func TestTemporalSDKMetricsHandlerRenamesConflictingMetricKinds(t *testing.T) {
	metrics := NewMetrics()
	handler := metrics.TemporalSDKMetricsHandler()

	handler.Counter("temporal_conflicting_metric").Inc(1)
	handler.Gauge("temporal_conflicting_metric").Update(2)
	handler.Timer("temporal_conflicting_metric").Record(time.Second)

	body := collectMetricsBody(t, metrics)
	for _, want := range []string{
		`temporal_conflicting_metric{`,
		`} 1`,
		`temporal_conflicting_metric_gauge{`,
		`} 2`,
		`temporal_conflicting_metric_seconds_bucket{`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, body)
		}
	}
}

func TestSoniqOverviewDashboardUsesTemporalSDKMetricNames(t *testing.T) {
	metrics := NewMetrics()
	handler := metrics.TemporalSDKMetricsHandler().WithTags(map[string]string{
		"namespace":     "default",
		"task_queue":    "soniq-audio-pipeline",
		"activity_type": "TranscribeRecordingAudio",
		"poller_type":   "workflow_task",
		"worker_type":   "WorkflowWorker",
	})

	handler.Counter("temporal_workflow_task_queue_poll_succeed").Inc(1)
	handler.Timer("temporal_activity_schedule_to_start_latency").Record(time.Second)
	handler.Gauge("temporal_worker_task_slots_available").Update(1)

	metricsBody := collectMetricsBody(t, metrics)
	dashboardBody := readSoniqOverviewDashboard(t)
	for _, metricName := range []string{
		"temporal_workflow_task_queue_poll_succeed",
		"temporal_activity_schedule_to_start_latency_bucket",
		"temporal_worker_task_slots_available",
	} {
		if !strings.Contains(metricsBody, metricName) {
			t.Fatalf("Temporal SDK metrics output missing %q:\n%s", metricName, metricsBody)
		}
		if !strings.Contains(dashboardBody, metricName) {
			t.Fatalf("Grafana dashboard missing query for %q:\n%s", metricName, dashboardBody)
		}
	}
}

func TestTemporalSDKMetricsHandlerReusesCollectorsAcrossHandlers(t *testing.T) {
	metrics := NewMetrics()

	metrics.TemporalSDKMetricsHandler().Counter("temporal_worker_start").Inc(1)
	metrics.TemporalSDKMetricsHandler().Counter("temporal_worker_start").Inc(1)

	body := collectMetricsBody(t, metrics)
	if !strings.Contains(body, `temporal_worker_start{`) || !strings.Contains(body, `} 2`) {
		t.Fatalf("metrics output missing reused Temporal counter:\n%s", body)
	}
}

func readSoniqOverviewDashboard(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "deploy", "observability", "grafana", "dashboards", "soniq-api-overview.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse dashboard JSON: %v", err)
	}
	return string(body)
}

func collectMetricsBody(t *testing.T, metrics *Metrics) string {
	t.Helper()
	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	if response.Code != 200 {
		t.Fatalf("metrics status = %d, want 200", response.Code)
	}
	return response.Body.String()
}
