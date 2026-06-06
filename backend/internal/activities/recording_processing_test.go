package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
)

type recordingStoreSpy struct {
	recordings map[string]domain.Recording
	updates    []recordings.UpdateRecordingStatusInput
	updateErr  error
}

func (s *recordingStoreSpy) Get(id string) (domain.Recording, bool) {
	if s.recordings == nil {
		return domain.Recording{}, false
	}
	recording, ok := s.recordings[id]
	return recording, ok
}

func (s *recordingStoreSpy) UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
	s.updates = append(s.updates, input)
	if s.updateErr != nil {
		return domain.Recording{}, s.updateErr
	}
	recording, ok := s.Get(input.ID)
	if !ok {
		return domain.Recording{}, errors.New("recording not found")
	}
	recording.Status = input.Status
	return recording, nil
}

func (s *recordingStoreSpy) UpsertAudioProbe(input recordings.UpsertAudioProbeInput) (recordings.RecordingAudioProbe, error) {
	return recordings.RecordingAudioProbe{RecordingID: input.RecordingID}, nil
}

func TestValidateRecordingActivityAcceptsValidInput(t *testing.T) {
	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	}

	if err := ValidateRecordingActivity(context.Background(), input); err != nil {
		t.Fatalf("ValidateRecordingActivity() error = %v, want nil", err)
	}
}

func TestValidateRecordingActivityRejectsMissingRecordingID(t *testing.T) {
	input := RecordingProcessingInput{
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	}

	if err := ValidateRecordingActivity(context.Background(), input); err == nil {
		t.Fatalf("ValidateRecordingActivity() error = nil, want error")
	}
}

func TestValidateRecordingActivityRejectsInvalidWorkflowType(t *testing.T) {
	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "podcast",
		Language:     "en",
	}

	if err := ValidateRecordingActivity(context.Background(), input); err == nil {
		t.Fatalf("ValidateRecordingActivity() error = nil, want error")
	}
}

func TestMarkRecordingProcessingActivityAcceptsRecordingID(t *testing.T) {
	if err := MarkRecordingProcessingActivity(context.Background(), "rec_test"); err != nil {
		t.Fatalf("MarkRecordingProcessingActivity() error = %v, want nil", err)
	}
}

func TestMarkRecordingProcessingActivityRejectsMissingRecordingID(t *testing.T) {
	if err := MarkRecordingProcessingActivity(context.Background(), ""); err == nil {
		t.Fatalf("MarkRecordingProcessingActivity() error = nil, want error")
	}
}

func TestCompleteRecordingProcessingActivityReturnsCompletedResult(t *testing.T) {
	result, err := CompleteRecordingProcessingActivity(context.Background(), "rec_test")
	if err != nil {
		t.Fatalf("CompleteRecordingProcessingActivity() error = %v, want nil", err)
	}

	want := RecordingProcessingResult{
		RecordingID: "rec_test",
		Status:      domain.RecordingStatusCompleted,
	}
	if result != want {
		t.Fatalf("CompleteRecordingProcessingActivity() = %#v, want %#v", result, want)
	}
}

func TestCompleteRecordingProcessingActivityRejectsMissingRecordingID(t *testing.T) {
	if _, err := CompleteRecordingProcessingActivity(context.Background(), ""); err == nil {
		t.Fatalf("CompleteRecordingProcessingActivity() error = nil, want error")
	}
}

func TestFailRecordingProcessingActivityAcceptsRecordingID(t *testing.T) {
	if err := FailRecordingProcessingActivity(context.Background(), "rec_test"); err != nil {
		t.Fatalf("FailRecordingProcessingActivity() error = %v, want nil", err)
	}
}

func TestFailRecordingProcessingActivityRejectsMissingRecordingID(t *testing.T) {
	if err := FailRecordingProcessingActivity(context.Background(), ""); err == nil {
		t.Fatalf("FailRecordingProcessingActivity() error = nil, want error")
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
