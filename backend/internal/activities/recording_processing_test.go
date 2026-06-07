package activities

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
)

type recordingStoreSpy struct {
	recordings map[string]domain.Recording
	updates    []recordings.UpdateRecordingStatusInput
	updateErr  error
}

func (s *recordingStoreSpy) Get(id string) (domain.Recording, bool, error) {
	if s.recordings == nil {
		return domain.Recording{}, false, nil
	}
	recording, ok := s.recordings[id]
	return recording, ok, nil
}

func (s *recordingStoreSpy) UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
	s.updates = append(s.updates, input)
	if s.updateErr != nil {
		return domain.Recording{}, s.updateErr
	}
	recording, ok, err := s.Get(input.ID)
	if err != nil {
		return domain.Recording{}, err
	}
	if !ok {
		return domain.Recording{}, errors.New("recording not found")
	}
	recording.Status = input.Status
	return recording, nil
}

func (s *recordingStoreSpy) UpsertAudioProbe(input recordings.UpsertAudioProbeInput) (recordings.RecordingAudioProbe, error) {
	return recordings.RecordingAudioProbe{RecordingID: input.RecordingID}, nil
}

func TestFakeSummaryProviderTruncatesChineseOverviewWithoutBreakingUTF8(t *testing.T) {
	result, err := FakeSummaryProvider{}.Summarize(context.Background(), SummaryRequest{
		RecordingID:    "rec_zh",
		Title:          "中文摘要",
		TranscriptText: strings.Repeat("界", 121),
	})
	if err != nil {
		t.Fatalf("Summarize returned error: %v", err)
	}
	if !utf8.ValidString(result.Overview) {
		t.Fatalf("overview contains invalid UTF-8: %q", result.Overview)
	}
	if got, want := len([]rune(result.Overview)), 120; got != want {
		t.Fatalf("overview runes = %d, want %d", got, want)
	}
}

func TestRecordingProcessingActivitiesValidateRecordingAcceptsExistingRecording(t *testing.T) {
	store := &recordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_test": {ID: "rec_test", Status: domain.RecordingStatusUploaded},
	}}
	activities := NewRecordingProcessingActivities(store)

	err := activities.ValidateRecording(context.Background(), RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("ValidateRecording() error = %v, want nil", err)
	}
}

func TestRecordingProcessingActivitiesValidateRecordingRejectsMissingRecordingID(t *testing.T) {
	activities := NewRecordingProcessingActivities(&recordingStoreSpy{})

	err := activities.ValidateRecording(context.Background(), RecordingProcessingInput{
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})
	if err == nil {
		t.Fatal("ValidateRecording() error = nil, want missing recording id error")
	}
}

func TestRecordingProcessingActivitiesValidateRecordingRejectsInvalidWorkflowType(t *testing.T) {
	activities := NewRecordingProcessingActivities(&recordingStoreSpy{})

	err := activities.ValidateRecording(context.Background(), RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "podcast",
		Language:     "en",
	})
	if err == nil {
		t.Fatal("ValidateRecording() error = nil, want invalid workflow type error")
	}
}

func TestRecordingProcessingActivitiesValidateRecordingRequiresExistingRecording(t *testing.T) {
	activities := NewRecordingProcessingActivities(&recordingStoreSpy{})

	err := activities.ValidateRecording(context.Background(), RecordingProcessingInput{
		RecordingID:  "rec_missing",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})
	if err == nil {
		t.Fatal("ValidateRecording() error = nil, want missing recording error")
	}
}

func TestRecordingProcessingActivitiesMarkRecordingProcessingUpdatesStatus(t *testing.T) {
	store := &recordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_test": {ID: "rec_test", Status: domain.RecordingStatusUploaded},
	}}
	activities := NewRecordingProcessingActivities(store)

	if err := activities.MarkRecordingProcessing(context.Background(), "rec_test"); err != nil {
		t.Fatalf("MarkRecordingProcessing() error = %v, want nil", err)
	}

	want := recordings.UpdateRecordingStatusInput{ID: "rec_test", Status: domain.RecordingStatusProcessing}
	if len(store.updates) != 1 || store.updates[0] != want {
		t.Fatalf("updates = %+v, want [%+v]", store.updates, want)
	}
}

func TestRecordingProcessingActivitiesMarkRecordingTranscribingUpdatesStatus(t *testing.T) {
	store := &recordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_test": {ID: "rec_test", Status: domain.RecordingStatusProcessing},
	}}
	activities := NewRecordingProcessingActivities(store)

	if err := activities.MarkRecordingTranscribing(context.Background(), "rec_test"); err != nil {
		t.Fatalf("MarkRecordingTranscribing() error = %v, want nil", err)
	}

	want := recordings.UpdateRecordingStatusInput{ID: "rec_test", Status: domain.RecordingStatusTranscribing}
	if len(store.updates) != 1 || store.updates[0] != want {
		t.Fatalf("updates = %+v, want [%+v]", store.updates, want)
	}
}

func TestRecordingProcessingActivitiesMarkRecordingSummarizingUpdatesStatus(t *testing.T) {
	store := &recordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_test": {ID: "rec_test", Status: domain.RecordingStatusTranscribing},
	}}
	activities := NewRecordingProcessingActivities(store)

	if err := activities.MarkRecordingSummarizing(context.Background(), "rec_test"); err != nil {
		t.Fatalf("MarkRecordingSummarizing() error = %v, want nil", err)
	}

	want := recordings.UpdateRecordingStatusInput{ID: "rec_test", Status: domain.RecordingStatusSummarizing}
	if len(store.updates) != 1 || store.updates[0] != want {
		t.Fatalf("updates = %+v, want [%+v]", store.updates, want)
	}
}

func TestRecordingProcessingActivitiesCompleteRecordingProcessingUpdatesStatusAndReturnsResult(t *testing.T) {
	store := &recordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_test": {ID: "rec_test", Status: domain.RecordingStatusProcessing},
	}}
	activities := NewRecordingProcessingActivities(store)

	result, err := activities.CompleteRecordingProcessing(context.Background(), "rec_test")
	if err != nil {
		t.Fatalf("CompleteRecordingProcessing() error = %v, want nil", err)
	}

	wantUpdate := recordings.UpdateRecordingStatusInput{ID: "rec_test", Status: domain.RecordingStatusCompleted}
	if len(store.updates) != 1 || store.updates[0] != wantUpdate {
		t.Fatalf("updates = %+v, want [%+v]", store.updates, wantUpdate)
	}
	wantResult := RecordingProcessingResult{RecordingID: "rec_test", Status: domain.RecordingStatusCompleted}
	if result != wantResult {
		t.Fatalf("result = %+v, want %+v", result, wantResult)
	}
}

func TestRecordingProcessingActivitiesFailRecordingProcessingUpdatesStatus(t *testing.T) {
	store := &recordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_test": {ID: "rec_test", Status: domain.RecordingStatusProcessing},
	}}
	activities := NewRecordingProcessingActivities(store)

	if err := activities.FailRecordingProcessing(context.Background(), "rec_test"); err != nil {
		t.Fatalf("FailRecordingProcessing() error = %v, want nil", err)
	}

	want := recordings.UpdateRecordingStatusInput{ID: "rec_test", Status: domain.RecordingStatusFailed}
	if len(store.updates) != 1 || store.updates[0] != want {
		t.Fatalf("updates = %+v, want [%+v]", store.updates, want)
	}
}
