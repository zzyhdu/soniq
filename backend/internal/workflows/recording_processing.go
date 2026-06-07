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

const (
	shortActivityTimeout       = time.Minute
	longRunningActivityTimeout = 10 * time.Minute
)

// RecordingProcessingWorkflow orchestrates the recording processing pipeline.
func RecordingProcessingWorkflow(ctx workflow.Context, input RecordingProcessingInput) (RecordingProcessingResult, error) {
	shortActivityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: shortActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	})
	longRunningActivityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: longRunningActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	})

	if err := workflow.ExecuteActivity(shortActivityCtx, activities.ValidateRecordingActivityName, input).Get(shortActivityCtx, nil); err != nil {
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(shortActivityCtx, activities.MarkRecordingProcessingActivityName, input.RecordingID).Get(shortActivityCtx, nil); err != nil {
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(shortActivityCtx, activities.ProbeRecordingAudioActivityName, input.RecordingID).Get(shortActivityCtx, nil); err != nil {
		_ = workflow.ExecuteActivity(shortActivityCtx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(shortActivityCtx, nil)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(shortActivityCtx, activities.NormalizeRecordingAudioActivityName, input.RecordingID).Get(shortActivityCtx, nil); err != nil {
		_ = workflow.ExecuteActivity(shortActivityCtx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(shortActivityCtx, nil)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(shortActivityCtx, activities.MarkRecordingTranscribingActivityName, input.RecordingID).Get(shortActivityCtx, nil); err != nil {
		_ = workflow.ExecuteActivity(shortActivityCtx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(shortActivityCtx, nil)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(longRunningActivityCtx, activities.TranscribeRecordingAudioActivityName, input.RecordingID).Get(longRunningActivityCtx, nil); err != nil {
		_ = workflow.ExecuteActivity(shortActivityCtx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(shortActivityCtx, nil)
		return RecordingProcessingResult{}, err
	}
	if input.DeleteOriginalAudioAfterTranscription {
		if err := workflow.ExecuteActivity(shortActivityCtx, activities.DeleteOriginalRecordingAudioActivityName, input.RecordingID).Get(shortActivityCtx, nil); err != nil {
			_ = workflow.ExecuteActivity(shortActivityCtx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(shortActivityCtx, nil)
			return RecordingProcessingResult{}, err
		}
	}
	if err := workflow.ExecuteActivity(shortActivityCtx, activities.MarkRecordingSummarizingActivityName, input.RecordingID).Get(shortActivityCtx, nil); err != nil {
		_ = workflow.ExecuteActivity(shortActivityCtx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(shortActivityCtx, nil)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(longRunningActivityCtx, activities.SummarizeRecordingActivityName, input.RecordingID).Get(longRunningActivityCtx, nil); err != nil {
		_ = workflow.ExecuteActivity(shortActivityCtx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(shortActivityCtx, nil)
		return RecordingProcessingResult{}, err
	}

	var result RecordingProcessingResult
	if err := workflow.ExecuteActivity(shortActivityCtx, activities.CompleteRecordingProcessingActivityName, input.RecordingID).Get(shortActivityCtx, &result); err != nil {
		_ = workflow.ExecuteActivity(shortActivityCtx, activities.FailRecordingProcessingActivityName, input.RecordingID).Get(shortActivityCtx, nil)
		return RecordingProcessingResult{}, err
	}

	return result, nil
}
