package workflows

import (
	"errors"
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
	recording := activities.RecordingReference{
		WorkspaceID: input.WorkspaceID,
		RecordingID: input.RecordingID,
	}

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
	if err := workflow.ExecuteActivity(shortActivityCtx, activities.MarkRecordingProcessingActivityName, recording).Get(shortActivityCtx, nil); err != nil {
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(longRunningActivityCtx, activities.PrepareRecordingAudioActivityName, input.RecordingID).Get(longRunningActivityCtx, nil); err != nil {
		_ = failRecording(shortActivityCtx, recording, "prepare audio", err)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(shortActivityCtx, activities.MarkRecordingTranscribingActivityName, recording).Get(shortActivityCtx, nil); err != nil {
		_ = failRecording(shortActivityCtx, recording, "mark transcribing", err)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(longRunningActivityCtx, activities.TranscribeRecordingAudioActivityName, input.RecordingID).Get(longRunningActivityCtx, nil); err != nil {
		_ = failRecording(shortActivityCtx, recording, "transcribe audio", err)
		return RecordingProcessingResult{}, err
	}
	if input.DeleteOriginalAudioAfterTranscription {
		if err := workflow.ExecuteActivity(shortActivityCtx, activities.DeleteOriginalRecordingAudioActivityName, input.RecordingID).Get(shortActivityCtx, nil); err != nil {
			_ = failRecording(shortActivityCtx, recording, "delete original audio", err)
			return RecordingProcessingResult{}, err
		}
	}
	if err := workflow.ExecuteActivity(shortActivityCtx, activities.MarkRecordingSummarizingActivityName, recording).Get(shortActivityCtx, nil); err != nil {
		_ = failRecording(shortActivityCtx, recording, "mark summarizing", err)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(longRunningActivityCtx, activities.SummarizeRecordingActivityName, input.RecordingID).Get(longRunningActivityCtx, nil); err != nil {
		_ = failRecording(shortActivityCtx, recording, "summarize recording", err)
		return RecordingProcessingResult{}, err
	}
	if err := workflow.ExecuteActivity(longRunningActivityCtx, activities.GenerateMindMapActivityName, input.RecordingID).Get(longRunningActivityCtx, nil); err != nil {
		_ = failRecording(shortActivityCtx, recording, "generate mind map", err)
		return RecordingProcessingResult{}, err
	}

	var result RecordingProcessingResult
	if err := workflow.ExecuteActivity(shortActivityCtx, activities.CompleteRecordingProcessingActivityName, recording).Get(shortActivityCtx, &result); err != nil {
		_ = failRecording(shortActivityCtx, recording, "complete processing", err)
		return RecordingProcessingResult{}, err
	}

	return result, nil
}

func failRecording(ctx workflow.Context, recording activities.RecordingReference, step string, err error) error {
	return workflow.ExecuteActivity(ctx, activities.FailRecordingProcessingActivityName, activities.RecordingFailure{
		WorkspaceID: recording.WorkspaceID,
		RecordingID: recording.RecordingID,
		Reason:      recordingFailureReason(step, err),
	}).Get(ctx, nil)
}

func recordingFailureReason(step string, err error) string {
	var activityErr *temporal.ActivityError
	if errors.As(err, &activityErr) && activityErr.Unwrap() != nil {
		err = activityErr.Unwrap()
	}
	var applicationErr *temporal.ApplicationError
	if errors.As(err, &applicationErr) {
		return step + ": " + applicationErr.Message()
	}
	return step + ": " + err.Error()
}
