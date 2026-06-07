package workflows

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/zzyhdu/soniq/backend/internal/activities"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestRecordingProcessingWorkflowCompletesTranscriptionSummaryPipeline(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	registerRecordingProcessingActivityNames(env)

	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "meeting",
		Language:     "en",
	}

	env.OnActivity(activities.ValidateRecordingActivityName, mock.Anything, input).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.ProbeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.NormalizeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingTranscribingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.TranscribeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingSummarizingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.SummarizeRecordingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.CompleteRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(RecordingProcessingResult{
		RecordingID: "rec_test",
		Status:      "completed",
	}, nil).Once()

	env.ExecuteWorkflow(RecordingProcessingWorkflow, input)

	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error = %v, want nil", err)
	}

	var result RecordingProcessingResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("GetWorkflowResult() error = %v, want nil", err)
	}
	want := RecordingProcessingResult{
		RecordingID: "rec_test",
		Status:      "completed",
	}
	if result != want {
		t.Fatalf("workflow result = %#v, want %#v", result, want)
	}
	env.AssertExpectations(t)
}

func TestRecordingProcessingWorkflowMarksFailedWhenCompletionFails(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	registerRecordingProcessingActivityNames(env)

	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "meeting",
		Language:     "en",
	}
	completeErr := errors.New("complete failed")

	env.OnActivity(activities.ValidateRecordingActivityName, mock.Anything, input).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.ProbeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.NormalizeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingTranscribingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.TranscribeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingSummarizingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.SummarizeRecordingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.CompleteRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(RecordingProcessingResult{}, completeErr).Once()
	env.OnActivity(activities.FailRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()

	env.ExecuteWorkflow(RecordingProcessingWorkflow, input)

	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	err := env.GetWorkflowError()
	if err == nil {
		t.Fatalf("workflow error = nil, want completion error")
	}
	if !strings.Contains(err.Error(), "complete failed") {
		t.Fatalf("workflow error = %v, want original completion error", err)
	}
	env.AssertExpectations(t)
}

func TestRecordingProcessingWorkflowMarksFailedWhenProbeFails(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	registerRecordingProcessingActivityNames(env)

	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "meeting",
		Language:     "en",
	}
	probeErr := errors.New("probe failed")

	env.OnActivity(activities.ValidateRecordingActivityName, mock.Anything, input).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.ProbeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(probeErr).Once()
	env.OnActivity(activities.FailRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()

	env.ExecuteWorkflow(RecordingProcessingWorkflow, input)

	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	err := env.GetWorkflowError()
	if err == nil {
		t.Fatalf("workflow error = nil, want probe error")
	}
	if !strings.Contains(err.Error(), "probe failed") {
		t.Fatalf("workflow error = %v, want original probe error", err)
	}
	env.AssertExpectations(t)
}

func TestRecordingProcessingWorkflowMarksFailedWhenNormalizationFails(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	registerRecordingProcessingActivityNames(env)

	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "meeting",
		Language:     "en",
	}
	normalizeErr := errors.New("normalize failed")

	env.OnActivity(activities.ValidateRecordingActivityName, mock.Anything, input).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.ProbeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.NormalizeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(normalizeErr).Once()
	env.OnActivity(activities.FailRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()

	env.ExecuteWorkflow(RecordingProcessingWorkflow, input)

	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	err := env.GetWorkflowError()
	if err == nil {
		t.Fatalf("workflow error = nil, want normalization error")
	}
	if !strings.Contains(err.Error(), "normalize failed") {
		t.Fatalf("workflow error = %v, want original normalization error", err)
	}
	env.AssertExpectations(t)
}

func TestRecordingProcessingWorkflowMarksFailedWhenTranscriptionFails(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	registerRecordingProcessingActivityNames(env)

	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "meeting",
		Language:     "en",
	}
	transcriptionErr := errors.New("transcription failed")

	env.OnActivity(activities.ValidateRecordingActivityName, mock.Anything, input).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.ProbeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.NormalizeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingTranscribingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.TranscribeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(transcriptionErr).Once()
	env.OnActivity(activities.FailRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()

	env.ExecuteWorkflow(RecordingProcessingWorkflow, input)

	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	err := env.GetWorkflowError()
	if err == nil {
		t.Fatalf("workflow error = nil, want transcription error")
	}
	if !strings.Contains(err.Error(), "transcription failed") {
		t.Fatalf("workflow error = %v, want original transcription error", err)
	}
	env.AssertExpectations(t)
}

func TestRecordingProcessingWorkflowMarksFailedWhenSummarizationFails(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	registerRecordingProcessingActivityNames(env)

	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "meeting",
		Language:     "en",
	}
	summarizationErr := errors.New("summarization failed")

	env.OnActivity(activities.ValidateRecordingActivityName, mock.Anything, input).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.ProbeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.NormalizeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingTranscribingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.TranscribeRecordingAudioActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingSummarizingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.SummarizeRecordingActivityName, mock.Anything, input.RecordingID).Return(summarizationErr).Once()
	env.OnActivity(activities.FailRecordingProcessingActivityName, mock.Anything, input.RecordingID).Return(nil).Once()

	env.ExecuteWorkflow(RecordingProcessingWorkflow, input)

	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	err := env.GetWorkflowError()
	if err == nil {
		t.Fatalf("workflow error = nil, want summarization error")
	}
	if !strings.Contains(err.Error(), "summarization failed") {
		t.Fatalf("workflow error = %v, want original summarization error", err)
	}
	env.AssertExpectations(t)
}

func TestRecordingProcessingActivityTimeoutsCoverRealProviders(t *testing.T) {
	if got, want := longRunningActivityTimeout, 6*time.Minute; got < want {
		t.Fatalf("longRunningActivityTimeout = %s, want at least %s", got, want)
	}
	if got, want := shortActivityTimeout, time.Minute; got != want {
		t.Fatalf("shortActivityTimeout = %s, want %s", got, want)
	}
}

func registerRecordingProcessingActivityNames(env *testsuite.TestWorkflowEnvironment) {
	env.RegisterActivityWithOptions(func(context.Context, activities.RecordingProcessingInput) error { return nil }, activity.RegisterOptions{Name: activities.ValidateRecordingActivityName})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil }, activity.RegisterOptions{Name: activities.MarkRecordingProcessingActivityName})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil }, activity.RegisterOptions{Name: activities.ProbeRecordingAudioActivityName})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil }, activity.RegisterOptions{Name: activities.NormalizeRecordingAudioActivityName})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil }, activity.RegisterOptions{Name: activities.MarkRecordingTranscribingActivityName})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil }, activity.RegisterOptions{Name: activities.TranscribeRecordingAudioActivityName})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil }, activity.RegisterOptions{Name: activities.MarkRecordingSummarizingActivityName})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil }, activity.RegisterOptions{Name: activities.SummarizeRecordingActivityName})
	env.RegisterActivityWithOptions(func(context.Context, string) (RecordingProcessingResult, error) {
		return RecordingProcessingResult{}, nil
	}, activity.RegisterOptions{Name: activities.CompleteRecordingProcessingActivityName})
	env.RegisterActivityWithOptions(func(context.Context, string) error { return nil }, activity.RegisterOptions{Name: activities.FailRecordingProcessingActivityName})
}
