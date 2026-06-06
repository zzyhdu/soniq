package workflows

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/zzyhdu/soniq/backend/internal/activities"
	"go.temporal.io/sdk/testsuite"
)

func TestRecordingProcessingWorkflowCompletesSkeletonPipeline(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "meeting",
		Language:     "en",
	}

	env.OnActivity(activities.ValidateRecordingActivity, mock.Anything, input).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingProcessingActivity, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.ProbeRecordingAudioActivity, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.CompleteRecordingProcessingActivity, mock.Anything, input.RecordingID).Return(RecordingProcessingResult{
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

	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "meeting",
		Language:     "en",
	}
	completeErr := errors.New("complete failed")

	env.OnActivity(activities.ValidateRecordingActivity, mock.Anything, input).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingProcessingActivity, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.ProbeRecordingAudioActivity, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.CompleteRecordingProcessingActivity, mock.Anything, input.RecordingID).Return(RecordingProcessingResult{}, completeErr).Once()
	env.OnActivity(activities.FailRecordingProcessingActivity, mock.Anything, input.RecordingID).Return(nil).Once()

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

	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "meeting",
		Language:     "en",
	}
	probeErr := errors.New("probe failed")

	env.OnActivity(activities.ValidateRecordingActivity, mock.Anything, input).Return(nil).Once()
	env.OnActivity(activities.MarkRecordingProcessingActivity, mock.Anything, input.RecordingID).Return(nil).Once()
	env.OnActivity(activities.ProbeRecordingAudioActivity, mock.Anything, input.RecordingID).Return(probeErr).Once()
	env.OnActivity(activities.FailRecordingProcessingActivity, mock.Anything, input.RecordingID).Return(nil).Once()

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
