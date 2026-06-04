package workflows

import (
	"context"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"go.temporal.io/sdk/workflow"
)

// RecordingProcessingInput is the workflow input for processing a recording.
type RecordingProcessingInput struct {
	RecordingID  string
	WorkflowType domain.WorkflowType
	Language     string
}

// RecordingProcessingResult is the workflow result returned after the skeleton pipeline completes.
type RecordingProcessingResult struct {
	RecordingID string
	Status      domain.RecordingStatus
}

// RecordingProcessingWorkflow orchestrates the initial recording processing skeleton.
func RecordingProcessingWorkflow(ctx workflow.Context, input RecordingProcessingInput) (RecordingProcessingResult, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute,
	})

	if err := workflow.ExecuteActivity(ctx, ValidateRecordingActivity, input).Get(ctx, nil); err != nil {
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(ctx, MarkRecordingProcessingActivity, input).Get(ctx, nil); err != nil {
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(ctx, CompleteRecordingProcessingActivity, input).Get(ctx, nil); err != nil {
		return RecordingProcessingResult{}, err
	}

	return RecordingProcessingResult{
		RecordingID: input.RecordingID,
		Status:      domain.RecordingStatusCompleted,
	}, nil
}

// ValidateRecordingActivity is a placeholder validation activity for the Temporal skeleton.
func ValidateRecordingActivity(context.Context, RecordingProcessingInput) error {
	return nil
}

// MarkRecordingProcessingActivity is a placeholder activity that will later persist processing state.
func MarkRecordingProcessingActivity(context.Context, RecordingProcessingInput) error {
	return nil
}

// CompleteRecordingProcessingActivity is a placeholder activity that will later persist completion state.
func CompleteRecordingProcessingActivity(context.Context, RecordingProcessingInput) error {
	return nil
}
