package workflows

import (
	"testing"

	"github.com/stretchr/testify/mock"
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

	env.OnActivity(ValidateRecordingActivity, mock.Anything, input).Return(nil).Once()
	env.OnActivity(MarkRecordingProcessingActivity, mock.Anything, input).Return(nil).Once()
	env.OnActivity(CompleteRecordingProcessingActivity, mock.Anything, input).Return(nil).Once()

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
