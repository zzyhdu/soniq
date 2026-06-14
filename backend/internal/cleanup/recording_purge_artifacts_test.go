package cleanup

import (
	"context"
	"errors"
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

func (s *purgeObjectStoreSpy) DeleteObject(ctx context.Context, key string) error {
	s.deletes = append(s.deletes, key)
	return s.err
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

func TestRecordingPurgeArtifactCleanerRunOnceReturnsClaimErrors(t *testing.T) {
	store := &purgeArtifactStoreSpy{err: errors.New("claim failed")}
	cleaner := NewRecordingPurgeArtifactCleaner(store, &purgeObjectStoreSpy{}, RecordingPurgeArtifactCleanerOptions{})

	if err := cleaner.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce returned nil error, want claim error")
	}
}
