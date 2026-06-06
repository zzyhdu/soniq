package workflows

import (
	"time"

	"github.com/zzyhdu/soniq/backend/internal/activities"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// RecordingProcessingInput is the workflow input for processing a recording.
type RecordingProcessingInput = activities.RecordingProcessingInput

// RecordingProcessingResult is the workflow result returned after processing completes.
type RecordingProcessingResult = activities.RecordingProcessingResult

// RecordingProcessingWorkflow orchestrates the recording processing pipeline.
func RecordingProcessingWorkflow(ctx workflow.Context, input RecordingProcessingInput) (RecordingProcessingResult, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	})

	if err := workflow.ExecuteActivity(ctx, activities.ValidateRecordingActivityName, input).Get(ctx, nil); err != nil {
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(ctx, activities.MarkRecordingProcessingActivityName, input.RecordingID).Get(ctx, nil); err != nil {
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(ctx, activities.ProbeRecordingAudioActivityName, input.RecordingID).Get(ctx, nil); err != nil {
		_ = workflow.ExecuteActivity(ctx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(ctx, nil)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(ctx, activities.MarkRecordingTranscribingActivityName, input.RecordingID).Get(ctx, nil); err != nil {
		_ = workflow.ExecuteActivity(ctx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(ctx, nil)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(ctx, activities.TranscribeRecordingAudioActivityName, input.RecordingID).Get(ctx, nil); err != nil {
		_ = workflow.ExecuteActivity(ctx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(ctx, nil)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(ctx, activities.MarkRecordingSummarizingActivityName, input.RecordingID).Get(ctx, nil); err != nil {
		_ = workflow.ExecuteActivity(ctx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(ctx, nil)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(ctx, activities.SummarizeRecordingActivityName, input.RecordingID).Get(ctx, nil); err != nil {
		_ = workflow.ExecuteActivity(ctx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(ctx, nil)
		return RecordingProcessingResult{}, err
	}

	var result RecordingProcessingResult
	if err := workflow.ExecuteActivity(ctx, activities.CompleteRecordingProcessingActivityName, input.RecordingID).Get(ctx, &result); err != nil {
		_ = workflow.ExecuteActivity(ctx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(ctx, nil)
		return RecordingProcessingResult{}, err
	}

	return result, nil
}
