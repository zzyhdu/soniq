package workflows

import (
	"time"

	"github.com/zzyhdu/soniq/backend/internal/activities"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// RecordingProcessingInput is the workflow input for processing a recording.
type RecordingProcessingInput = activities.RecordingProcessingInput

// RecordingProcessingResult is the workflow result returned after the skeleton pipeline completes.
type RecordingProcessingResult = activities.RecordingProcessingResult

// RecordingProcessingWorkflow orchestrates the initial recording processing skeleton.
func RecordingProcessingWorkflow(ctx workflow.Context, input RecordingProcessingInput) (RecordingProcessingResult, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	})

	if err := workflow.ExecuteActivity(ctx, activities.ValidateRecordingActivity, input).Get(ctx, nil); err != nil {
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(ctx, activities.MarkRecordingProcessingActivity, input.RecordingID).Get(ctx, nil); err != nil {
		return RecordingProcessingResult{}, err
	}

	var result RecordingProcessingResult
	if err := workflow.ExecuteActivity(ctx, activities.CompleteRecordingProcessingActivity, input.RecordingID).Get(ctx, &result); err != nil {
		_ = workflow.ExecuteActivity(ctx, activities.FailRecordingProcessingActivity, input.RecordingID).Get(ctx, nil)
		return RecordingProcessingResult{}, err
	}

	return result, nil
}
