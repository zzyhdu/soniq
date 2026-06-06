package recordings

import (
	"strings"
	"testing"
	"time"

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

func TestMemoryStoreUpdateStatusPreservesMetadataAndAdvancesUpdatedAt(t *testing.T) {
	store := NewMemoryStore()

	created, err := store.Create(CreateRecordingInput{
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

	updated, err := store.UpdateStatus(UpdateRecordingStatusInput{
		ID:     created.ID,
		Status: domain.RecordingStatusProcessing,
	})
	if err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}

	if updated.Status != domain.RecordingStatusProcessing {
		t.Fatalf("Status = %q, want processing", updated.Status)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("UpdatedAt = %s, want after %s", updated.UpdatedAt, created.UpdatedAt)
	}
	if updated.Title != created.Title || updated.WorkflowType != created.WorkflowType || updated.Language != created.Language {
		t.Fatalf("updated recording core metadata = %+v, want preserved from %+v", updated, created)
	}
	if updated.AudioObjectKey != created.AudioObjectKey || updated.AudioContentType != created.AudioContentType || updated.AudioSizeBytes != created.AudioSizeBytes {
		t.Fatalf("updated audio metadata = %+v, want preserved from %+v", updated, created)
	}

	stored, ok := store.Get(created.ID)
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", created.ID)
	}
	if stored != updated {
		t.Fatalf("stored recording = %+v, want updated %+v", stored, updated)
	}
}

func TestMemoryStoreUpdateStatusReturnsErrorForMissingRecording(t *testing.T) {
	store := NewMemoryStore()

	_, err := store.UpdateStatus(UpdateRecordingStatusInput{
		ID:     "rec_missing",
		Status: domain.RecordingStatusProcessing,
	})
	if err == nil {
		t.Fatal("UpdateStatus returned nil error, want missing recording error")
	}
}

func TestMemoryStoreUpsertAndGetAudioProbe(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)

	probe, err := store.UpsertAudioProbe(UpsertAudioProbeInput{
		RecordingID:     "rec_probe",
		DurationSeconds: 12.5,
		FormatName:      "wav",
		CodecName:       "pcm_s16le",
		SampleRate:      16000,
		Channels:        1,
		BitRate:         256000,
		RawProbeJSON:    []byte(`{"format":{"duration":"12.5"}}`),
		ProbedAt:        now,
	})
	if err != nil {
		t.Fatalf("UpsertAudioProbe returned error: %v", err)
	}

	if probe.RecordingID != "rec_probe" {
		t.Fatalf("RecordingID = %q, want rec_probe", probe.RecordingID)
	}
	if probe.DurationSeconds != 12.5 || probe.FormatName != "wav" || probe.CodecName != "pcm_s16le" {
		t.Fatalf("probe core fields = %+v, want persisted metadata", probe)
	}
	if probe.SampleRate != 16000 || probe.Channels != 1 || probe.BitRate != 256000 {
		t.Fatalf("probe audio fields = %+v, want persisted audio stream metadata", probe)
	}
	if string(probe.RawProbeJSON) != `{"format":{"duration":"12.5"}}` {
		t.Fatalf("RawProbeJSON = %s, want raw ffprobe json", probe.RawProbeJSON)
	}
	if !probe.ProbedAt.Equal(now) {
		t.Fatalf("ProbedAt = %s, want %s", probe.ProbedAt, now)
	}
	if probe.CreatedAt.IsZero() || probe.UpdatedAt.IsZero() {
		t.Fatalf("timestamps = %s/%s, want non-zero", probe.CreatedAt, probe.UpdatedAt)
	}

	got, ok := store.GetAudioProbe("rec_probe")
	if !ok {
		t.Fatal("GetAudioProbe(rec_probe) ok = false, want true")
	}
	if got.RecordingID != probe.RecordingID || got.FormatName != probe.FormatName || got.CodecName != probe.CodecName {
		t.Fatalf("stored probe = %+v, want %+v", got, probe)
	}
	if string(got.RawProbeJSON) != string(probe.RawProbeJSON) {
		t.Fatalf("stored RawProbeJSON = %s, want %s", got.RawProbeJSON, probe.RawProbeJSON)
	}
}

func TestMemoryStoreUpsertAudioProbeReplacesFieldsAndPreservesCreatedAt(t *testing.T) {
	store := NewMemoryStore()
	firstProbeAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	secondProbeAt := firstProbeAt.Add(time.Minute)

	first, err := store.UpsertAudioProbe(UpsertAudioProbeInput{
		RecordingID:     "rec_probe",
		DurationSeconds: 10,
		FormatName:      "wav",
		CodecName:       "pcm_s16le",
		SampleRate:      16000,
		Channels:        1,
		BitRate:         256000,
		RawProbeJSON:    []byte(`{"first":true}`),
		ProbedAt:        firstProbeAt,
	})
	if err != nil {
		t.Fatalf("first UpsertAudioProbe returned error: %v", err)
	}

	updated, err := store.UpsertAudioProbe(UpsertAudioProbeInput{
		RecordingID:     "rec_probe",
		DurationSeconds: 20.25,
		FormatName:      "mp3",
		CodecName:       "mp3",
		SampleRate:      44100,
		Channels:        2,
		BitRate:         192000,
		RawProbeJSON:    []byte(`{"second":true}`),
		ProbedAt:        secondProbeAt,
	})
	if err != nil {
		t.Fatalf("second UpsertAudioProbe returned error: %v", err)
	}

	if !updated.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("CreatedAt = %s, want preserved %s", updated.CreatedAt, first.CreatedAt)
	}
	if !updated.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("UpdatedAt = %s, want after %s", updated.UpdatedAt, first.UpdatedAt)
	}
	if updated.DurationSeconds != 20.25 || updated.FormatName != "mp3" || updated.CodecName != "mp3" {
		t.Fatalf("updated probe fields = %+v, want replaced metadata", updated)
	}
	if string(updated.RawProbeJSON) != `{"second":true}` {
		t.Fatalf("RawProbeJSON = %s, want second json", updated.RawProbeJSON)
	}
}

func TestMemoryStoreGetAudioProbeReturnsFalseForMissingRecording(t *testing.T) {
	store := NewMemoryStore()

	_, ok := store.GetAudioProbe("rec_missing")
	if ok {
		t.Fatal("GetAudioProbe(rec_missing) ok = true, want false")
	}
}
