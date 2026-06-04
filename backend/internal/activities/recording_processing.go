package activities

import (
	"context"
	"errors"

	"github.com/zzyhdu/soniq/backend/internal/domain"
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

// ValidateRecordingActivity validates the minimal recording processing input.
func ValidateRecordingActivity(ctx context.Context, input RecordingProcessingInput) error {
	if input.RecordingID == "" {
		return errors.New("recording id is required")
	}
	if !domain.IsValidWorkflowType(string(input.WorkflowType)) {
		return errors.New("workflow type is invalid")
	}
	return nil
}

// MarkRecordingProcessingActivity is a placeholder activity that will later persist processing state.
func MarkRecordingProcessingActivity(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	return nil
}

// CompleteRecordingProcessingActivity is a placeholder activity that returns a completed skeleton result.
func CompleteRecordingProcessingActivity(ctx context.Context, recordingID string) (RecordingProcessingResult, error) {
	if recordingID == "" {
		return RecordingProcessingResult{}, errors.New("recording id is required")
	}
	return RecordingProcessingResult{
		RecordingID: recordingID,
		Status:      domain.RecordingStatusCompleted,
	}, nil
}
