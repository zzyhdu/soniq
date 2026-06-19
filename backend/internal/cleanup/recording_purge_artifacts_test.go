package cleanup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
)

var errCleanupObjectDelete = errors.New("delete object failed")

type purgeArtifactStoreSpy struct {
	claimed []recordings.RecordingPurgeArtifact
	deleted []string
	failed  []recordings.MarkPurgeArtifactFailedInput
	err     error
}

func (s *purgeArtifactStoreSpy) ClaimPurgeArtifacts(input recordings.ClaimPurgeArtifactsInput) ([]recordings.RecordingPurgeArtifact, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]recordings.RecordingPurgeArtifact(nil), s.claimed...), nil
}

func (s *purgeArtifactStoreSpy) MarkPurgeArtifactDeleted(input recordings.MarkPurgeArtifactDeletedInput) (bool, error) {
	s.deleted = append(s.deleted, input.ID)
	return true, nil
}

func (s *purgeArtifactStoreSpy) MarkPurgeArtifactFailed(input recordings.MarkPurgeArtifactFailedInput) (bool, error) {
	s.failed = append(s.failed, input)
	return true, nil
}

type purgeObjectStoreSpy struct {
	deletes []string
	err     error
}

func (s *purgeObjectStoreSpy) PutObject(ctx context.Context, input storage.PutObjectInput) (storage.PutObjectResult, error) {
	return storage.PutObjectResult{Key: input.Key}, nil
}

func (s *purgeObjectStoreSpy) GetObject(ctx context.Context, key string) (storage.GetObjectResult, error) {
	return storage.GetObjectResult{Key: key, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func (s *purgeObjectStoreSpy) PresignGetObject(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return "", nil
}

func (s *purgeObjectStoreSpy) DeleteObject(ctx context.Context, key string) error {
	s.deletes = append(s.deletes, key)
	return s.err
}

type purgeMetricsSpy struct {
	claimed      int
	deleted      int
	failed       int
	runResults   []string
	runDurations []time.Duration
}

func (s *purgeMetricsSpy) ObservePurgeArtifactsClaimed(count int) {
	s.claimed += count
}

func (s *purgeMetricsSpy) ObservePurgeArtifactDeleted() {
	s.deleted++
}

func (s *purgeMetricsSpy) ObservePurgeArtifactFailed() {
	s.failed++
}

func (s *purgeMetricsSpy) ObservePurgeCleanupRun(result string, duration time.Duration) {
	s.runResults = append(s.runResults, result)
	s.runDurations = append(s.runDurations, duration)
}

func TestRecordingPurgeArtifactCleanerRunOnceDeletesClaimedArtifacts(t *testing.T) {
	store := &purgeArtifactStoreSpy{claimed: []recordings.RecordingPurgeArtifact{{
		ID:           "rpa_1",
		ObjectKey:    "workspaces/wsp_default/recordings/rec/original.wav",
		AttemptCount: 0,
	}}}
	objectStore := &purgeObjectStoreSpy{}
	cleaner := NewRecordingPurgeArtifactCleaner(store, objectStore, RecordingPurgeArtifactCleanerOptions{BatchSize: 10})

	if err := cleaner.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if got, want := objectStore.deletes, []string{"workspaces/wsp_default/recordings/rec/original.wav"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("deleted object keys = %+v, want %+v", got, want)
	}
	if got, want := store.deleted, []string{"rpa_1"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("deleted artifact ids = %+v, want %+v", got, want)
	}
	if len(store.failed) != 0 {
		t.Fatalf("failed marks = %+v, want none", store.failed)
	}
}

func TestRecordingPurgeArtifactCleanerRunOnceRecordsMetrics(t *testing.T) {
	store := &purgeArtifactStoreSpy{claimed: []recordings.RecordingPurgeArtifact{{
		ID:           "rpa_1",
		ObjectKey:    "workspaces/wsp_default/recordings/rec/original.wav",
		AttemptCount: 0,
	}}}
	objectStore := &purgeObjectStoreSpy{}
	metrics := &purgeMetricsSpy{}
	cleaner := NewRecordingPurgeArtifactCleaner(store, objectStore, RecordingPurgeArtifactCleanerOptions{
		BatchSize: 10,
		Metrics:   metrics,
	})

	if err := cleaner.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	if metrics.claimed != 1 {
		t.Fatalf("claimed metrics = %d, want 1", metrics.claimed)
	}
	if metrics.deleted != 1 {
		t.Fatalf("deleted metrics = %d, want 1", metrics.deleted)
	}
	if metrics.failed != 0 {
		t.Fatalf("failed metrics = %d, want 0", metrics.failed)
	}
	if got, want := metrics.runResults, []string{"success"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("run results = %+v, want %+v", got, want)
	}
	if len(metrics.runDurations) != 1 || metrics.runDurations[0] < 0 {
		t.Fatalf("run durations = %+v, want one non-negative duration", metrics.runDurations)
	}
}

func TestRecordingPurgeArtifactCleanerRunOnceMarksFailedArtifactsRetryable(t *testing.T) {
	store := &purgeArtifactStoreSpy{claimed: []recordings.RecordingPurgeArtifact{{
		ID:           "rpa_1",
		ObjectKey:    "workspaces/wsp_default/recordings/rec/original.wav",
		AttemptCount: 1,
	}}}
	objectStore := &purgeObjectStoreSpy{err: errCleanupObjectDelete}
	cleaner := NewRecordingPurgeArtifactCleaner(store, objectStore, RecordingPurgeArtifactCleanerOptions{BatchSize: 10})
	before := time.Now().UTC()

	err := cleaner.RunOnce(context.Background())
	if err == nil {
		t.Fatal("RunOnce returned nil error, want delete error")
	}
	if got, want := len(store.failed), 1; got != want {
		t.Fatalf("failed marks = %d, want %d", got, want)
	}
	if store.failed[0].ID != "rpa_1" || store.failed[0].LastError == "" {
		t.Fatalf("failed mark = %+v, want artifact failure", store.failed[0])
	}
	if !store.failed[0].NextAttemptAt.After(before) {
		t.Fatalf("NextAttemptAt = %s, want future retry time", store.failed[0].NextAttemptAt)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("deleted marks = %+v, want none", store.deleted)
	}
}

func TestRecordingPurgeArtifactCleanerRunOnceRecordsFailureMetrics(t *testing.T) {
	store := &purgeArtifactStoreSpy{claimed: []recordings.RecordingPurgeArtifact{{
		ID:           "rpa_1",
		ObjectKey:    "workspaces/wsp_default/recordings/rec/original.wav",
		AttemptCount: 1,
	}}}
	objectStore := &purgeObjectStoreSpy{err: errCleanupObjectDelete}
	metrics := &purgeMetricsSpy{}
	cleaner := NewRecordingPurgeArtifactCleaner(store, objectStore, RecordingPurgeArtifactCleanerOptions{
		BatchSize: 10,
		Metrics:   metrics,
	})

	err := cleaner.RunOnce(context.Background())
	if err == nil {
		t.Fatal("RunOnce returned nil error, want delete error")
	}

	if metrics.claimed != 1 {
		t.Fatalf("claimed metrics = %d, want 1", metrics.claimed)
	}
	if metrics.deleted != 0 {
		t.Fatalf("deleted metrics = %d, want 0", metrics.deleted)
	}
	if metrics.failed != 1 {
		t.Fatalf("failed metrics = %d, want 1", metrics.failed)
	}
	if got, want := metrics.runResults, []string{"error"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("run results = %+v, want %+v", got, want)
	}
}

func TestRecordingPurgeArtifactCleanerRunOnceWritesStructuredLogs(t *testing.T) {
	artifact := recordings.RecordingPurgeArtifact{
		ID:           "rpa_1",
		RecordingID:  "rec_1",
		WorkspaceID:  "wsp_1",
		ObjectKey:    "workspaces/wsp_1/recordings/rec_1/original.wav",
		ArtifactKind: recordings.RecordingPurgeArtifactKindOriginalAudio,
		AttemptCount: 1,
	}
	store := &purgeArtifactStoreSpy{claimed: []recordings.RecordingPurgeArtifact{artifact}}
	objectStore := &purgeObjectStoreSpy{err: errCleanupObjectDelete}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	cleaner := NewRecordingPurgeArtifactCleaner(store, objectStore, RecordingPurgeArtifactCleanerOptions{
		BatchSize: 10,
		Logger:    logger,
	})

	err := cleaner.RunOnce(context.Background())
	if err == nil {
		t.Fatal("RunOnce returned nil error, want delete error")
	}

	entries := decodeCleanupLogEntries(t, logs.String())
	claimed := findCleanupLogEvent(t, entries, "purge_artifact_cleanup_claimed")
	if got, want := int(claimed["artifact_count"].(float64)), 1; got != want {
		t.Fatalf("artifact_count = %d, want %d", got, want)
	}
	failed := findCleanupLogEvent(t, entries, "purge_artifact_cleanup_failed")
	assertCleanupLogField(t, failed, "artifact_id", "rpa_1")
	assertCleanupLogField(t, failed, "recording_id", "rec_1")
	assertCleanupLogField(t, failed, "workspace_id", "wsp_1")
	assertCleanupLogField(t, failed, "artifact_kind", recordings.RecordingPurgeArtifactKindOriginalAudio)
	assertCleanupLogField(t, failed, "error", errCleanupObjectDelete.Error())
	if strings.Contains(logs.String(), artifact.ObjectKey) {
		t.Fatalf("cleanup log leaked object key: %s", logs.String())
	}
}

func TestRecordingPurgeArtifactCleanerRunOnceReturnsClaimErrors(t *testing.T) {
	store := &purgeArtifactStoreSpy{err: errors.New("claim failed")}
	cleaner := NewRecordingPurgeArtifactCleaner(store, &purgeObjectStoreSpy{}, RecordingPurgeArtifactCleanerOptions{})

	if err := cleaner.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce returned nil error, want claim error")
	}
}

func decodeCleanupLogEntries(t *testing.T, output string) []map[string]any {
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

func findCleanupLogEvent(t *testing.T, entries []map[string]any, event string) map[string]any {
	t.Helper()
	for _, entry := range entries {
		if entry["event"] == event {
			return entry
		}
	}
	t.Fatalf("event %q not found in entries %#v", event, entries)
	return nil
}

func assertCleanupLogField(t *testing.T, entry map[string]any, key string, want string) {
	t.Helper()
	if got, ok := entry[key].(string); !ok || got != want {
		t.Fatalf("%s = %#v, want %q", key, entry[key], want)
	}
}
