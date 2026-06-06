package recordings

import (
	"strings"
	"testing"

	"github.com/zzyhdu/soniq/backend/internal/domain"
)

func TestMemoryStoreCreateAssignsRecordingDefaults(t *testing.T) {
	store := NewMemoryStore()

	recording, err := store.Create(CreateRecordingInput{
		Title:        "Weekly sync",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if !strings.HasPrefix(recording.ID, "rec_") {
		t.Fatalf("recording.ID = %q, want rec_ prefix", recording.ID)
	}
	if recording.Status != domain.RecordingStatusUploaded {
		t.Fatalf("recording.Status = %q, want uploaded", recording.Status)
	}
	if recording.Title != "Weekly sync" {
		t.Fatalf("recording.Title = %q, want Weekly sync", recording.Title)
	}
	if recording.WorkflowType != domain.WorkflowTypeMeeting {
		t.Fatalf("recording.WorkflowType = %q, want meeting", recording.WorkflowType)
	}
	if recording.Language != "en" {
		t.Fatalf("recording.Language = %q, want en", recording.Language)
	}
	if recording.CreatedAt.IsZero() {
		t.Fatal("recording.CreatedAt is zero, want timestamp")
	}
	if recording.UpdatedAt.IsZero() {
		t.Fatal("recording.UpdatedAt is zero, want timestamp")
	}
	if !recording.CreatedAt.Equal(recording.UpdatedAt) {
		t.Fatalf("CreatedAt = %s, UpdatedAt = %s, want equal on create", recording.CreatedAt, recording.UpdatedAt)
	}
}

func TestMemoryStoreCreatePreservesAudioMetadata(t *testing.T) {
	store := NewMemoryStore()

	recording, err := store.Create(CreateRecordingInput{
		Title:            "Weekly sync",
		WorkflowType:     domain.WorkflowTypeMeeting,
		Language:         "en",
		AudioObjectKey:   "recordings/rec_123/original.wav",
		AudioContentType: "audio/wav",
		AudioSizeBytes:   12345,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if recording.AudioObjectKey != "recordings/rec_123/original.wav" {
		t.Fatalf("AudioObjectKey = %q, want object key", recording.AudioObjectKey)
	}
	if recording.AudioContentType != "audio/wav" {
		t.Fatalf("AudioContentType = %q, want audio/wav", recording.AudioContentType)
	}
	if recording.AudioSizeBytes != 12345 {
		t.Fatalf("AudioSizeBytes = %d, want 12345", recording.AudioSizeBytes)
	}

	got, ok := store.Get(recording.ID)
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", recording.ID)
	}
	if got.AudioObjectKey != recording.AudioObjectKey || got.AudioContentType != recording.AudioContentType || got.AudioSizeBytes != recording.AudioSizeBytes {
		t.Fatalf("stored audio metadata = %+v, want %+v", got, recording)
	}
}

func TestMemoryStoreGetReturnsExistingRecording(t *testing.T) {
	store := NewMemoryStore()

	created, err := store.Create(CreateRecordingInput{
		Title:        "Lecture 1",
		WorkflowType: domain.WorkflowTypeLecture,
		Language:     "zh",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	got, ok := store.Get(created.ID)
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", created.ID)
	}
	if got != created {
		t.Fatalf("Get(%q) = %+v, want %+v", created.ID, got, created)
	}
}

func TestMemoryStoreGetReturnsFalseForUnknownRecording(t *testing.T) {
	store := NewMemoryStore()

	_, ok := store.Get("rec_missing")
	if ok {
		t.Fatal("Get(rec_missing) ok = true, want false")
	}
}

func TestMemoryStoreCreateRejectsInvalidWorkflowType(t *testing.T) {
	store := NewMemoryStore()

	_, err := store.Create(CreateRecordingInput{
		Title:        "Podcast",
		WorkflowType: domain.WorkflowType("podcast"),
		Language:     "en",
	})
	if err == nil {
		t.Fatal("Create returned nil error, want invalid workflow type error")
	}
}
