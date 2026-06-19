package observability

import (
	"net/http/httptest"
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

func collectMetricsBody(t *testing.T, metrics *Metrics) string {
	t.Helper()
	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	if response.Code != 200 {
		t.Fatalf("metrics status = %d, want 200", response.Code)
	}
	return response.Body.String()
}
