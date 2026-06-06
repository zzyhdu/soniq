package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
)

// RecordingProcessingInput is the input shared by the recording processing workflow and activity stubs.
type RecordingProcessingInput struct {
	RecordingID  string
	WorkflowType domain.WorkflowType
	Language     string
}

// RecordingProcessingResult is the skeleton result returned after processing completes.
type RecordingProcessingResult struct {
	RecordingID string
	Status      domain.RecordingStatus
}

// RecordingStore is the persistence seam used by recording processing activities.
type RecordingStore interface {
	Get(id string) (domain.Recording, bool)
	UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error)
}

// RecordingProcessingActivities contains store-backed Temporal activity methods.
type RecordingProcessingActivities struct {
	store RecordingStore
}

// NewRecordingProcessingActivities creates store-backed recording processing activities.
func NewRecordingProcessingActivities(store RecordingStore) *RecordingProcessingActivities {
	return &RecordingProcessingActivities{store: store}
}

// ValidateRecordingActivity validates the minimal recording processing input.
//
// This package-level function is the current stateless Temporal activity used
// by the existing workflow/worker wiring. The store-backed
// RecordingProcessingActivities methods below are the next wiring target.
func ValidateRecordingActivity(ctx context.Context, input RecordingProcessingInput) error {
	if input.RecordingID == "" {
		return errors.New("recording id is required")
	}
	if !domain.IsValidWorkflowType(string(input.WorkflowType)) {
		return errors.New("workflow type is invalid")
	}
	return nil
}

// MarkRecordingProcessingActivity is the current stateless compatibility activity.
// Store-backed status persistence lives in RecordingProcessingActivities.MarkRecordingProcessing.
func MarkRecordingProcessingActivity(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	return nil
}

// CompleteRecordingProcessingActivity is the current stateless compatibility activity.
// Store-backed status persistence lives in RecordingProcessingActivities.CompleteRecordingProcessing.
func CompleteRecordingProcessingActivity(ctx context.Context, recordingID string) (RecordingProcessingResult, error) {
	if recordingID == "" {
		return RecordingProcessingResult{}, errors.New("recording id is required")
	}
	return RecordingProcessingResult{
		RecordingID: recordingID,
		Status:      domain.RecordingStatusCompleted,
	}, nil
}

// ValidateRecording validates processing input and confirms the recording exists.
func (a *RecordingProcessingActivities) ValidateRecording(ctx context.Context, input RecordingProcessingInput) error {
	if err := ValidateRecordingActivity(ctx, input); err != nil {
		return err
	}
	if a == nil || a.store == nil {
		return errors.New("recording store is required")
	}
	if _, ok := a.store.Get(input.RecordingID); !ok {
		return fmt.Errorf("recording not found: %s", input.RecordingID)
	}
	return nil
}

// MarkRecordingProcessing persists the processing status transition.
func (a *RecordingProcessingActivities) MarkRecordingProcessing(ctx context.Context, recordingID string) error {
	_, err := a.updateStatus(recordingID, domain.RecordingStatusProcessing)
	return err
}

// CompleteRecordingProcessing persists completion and returns the workflow result.
func (a *RecordingProcessingActivities) CompleteRecordingProcessing(ctx context.Context, recordingID string) (RecordingProcessingResult, error) {
	updated, err := a.updateStatus(recordingID, domain.RecordingStatusCompleted)
	if err != nil {
		return RecordingProcessingResult{}, err
	}
	return RecordingProcessingResult{
		RecordingID: updated.ID,
		Status:      updated.Status,
	}, nil
}

// FailRecordingProcessing persists a failed status transition.
func (a *RecordingProcessingActivities) FailRecordingProcessing(ctx context.Context, recordingID string) error {
	_, err := a.updateStatus(recordingID, domain.RecordingStatusFailed)
	return err
}

func (a *RecordingProcessingActivities) updateStatus(recordingID string, status domain.RecordingStatus) (domain.Recording, error) {
	if recordingID == "" {
		return domain.Recording{}, errors.New("recording id is required")
	}
	if a == nil || a.store == nil {
		return domain.Recording{}, errors.New("recording store is required")
	}
	updated, err := a.store.UpdateStatus(recordings.UpdateRecordingStatusInput{
		ID:     recordingID,
		Status: status,
	})
	if err != nil {
		return domain.Recording{}, err
	}
	return updated, nil
}
