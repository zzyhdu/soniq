package processing

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.temporal.io/sdk/client"
)

func TestTemporalRecordingProcessorStartsRecordingWorkflow(t *testing.T) {
	starter := &workflowStarterSpy{}
	processor := NewTemporalRecordingProcessor(starter, TemporalRecordingProcessorConfig{
		TaskQueue:                             "soniq-audio-pipeline",
		DeleteOriginalAudioAfterTranscription: true,
	})
	recording := domain.Recording{
		WorkspaceID:  "wsp_default",
		ID:           "rec_123",
		Title:        "Weekly sync",
		Status:       domain.RecordingStatusUploaded,
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	ctx := context.WithValue(context.Background(), testContextKey{}, "trace-context")
	if err := processor.Enqueue(ctx, recording); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}

	if got, want := len(starter.calls), 1; got != want {
		t.Fatalf("ExecuteWorkflow calls = %d, want %d", got, want)
	}
	call := starter.calls[0]
	if call.ctx != ctx {
		t.Fatal("ExecuteWorkflow context was not passed through from Enqueue")
	}
	if call.options.ID != "recording-processing-rec_123" {
		t.Fatalf("workflow ID = %q, want recording-processing-rec_123", call.options.ID)
	}
	if call.options.TaskQueue != "soniq-audio-pipeline" {
		t.Fatalf("task queue = %q, want soniq-audio-pipeline", call.options.TaskQueue)
	}
	if !sameFunction(call.workflow, workflows.RecordingProcessingWorkflow) {
		t.Fatalf("workflow = %T, want RecordingProcessingWorkflow", call.workflow)
	}
	if got, want := len(call.args), 1; got != want {
		t.Fatalf("workflow args = %d, want %d", got, want)
	}
	input, ok := call.args[0].(workflows.RecordingProcessingInput)
	if !ok {
		t.Fatalf("workflow arg type = %T, want RecordingProcessingInput", call.args[0])
	}
	if input.RecordingID != recording.ID {
		t.Fatalf("input recording ID = %q, want %q", input.RecordingID, recording.ID)
	}
	if input.WorkspaceID != recording.WorkspaceID {
		t.Fatalf("input workspace ID = %q, want %q", input.WorkspaceID, recording.WorkspaceID)
	}
	if input.WorkflowType != recording.WorkflowType {
		t.Fatalf("input workflow_type = %q, want %q", input.WorkflowType, recording.WorkflowType)
	}
	if input.Language != recording.Language {
		t.Fatalf("input language = %q, want %q", input.Language, recording.Language)
	}
	if !input.DeleteOriginalAudioAfterTranscription {
		t.Fatal("input DeleteOriginalAudioAfterTranscription = false, want true")
	}
}

func TestTemporalRecordingProcessorReturnsStartError(t *testing.T) {
	startErr := errors.New("temporal start failed")
	starter := &workflowStarterSpy{err: startErr}
	processor := NewTemporalRecordingProcessor(starter, TemporalRecordingProcessorConfig{
		TaskQueue: "soniq-audio-pipeline",
	})

	err := processor.Enqueue(context.Background(), domain.Recording{
		ID:           "rec_123",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})

	if !errors.Is(err, startErr) {
		t.Fatalf("Enqueue error = %v, want %v", err, startErr)
	}
}

func TestTemporalRecordingProcessorRejectsEmptyTaskQueue(t *testing.T) {
	starter := &workflowStarterSpy{}
	processor := NewTemporalRecordingProcessor(starter, TemporalRecordingProcessorConfig{})

	err := processor.Enqueue(context.Background(), domain.Recording{
		ID:           "rec_123",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})

	if err == nil {
		t.Fatal("Enqueue error = nil, want error")
	}
	if got, want := len(starter.calls), 0; got != want {
		t.Fatalf("ExecuteWorkflow calls = %d, want %d", got, want)
	}
}

type testContextKey struct{}

func sameFunction(a, b interface{}) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

type workflowStarterSpy struct {
	calls []workflowStartCall
	err   error
}

type workflowStartCall struct {
	ctx      context.Context
	options  client.StartWorkflowOptions
	workflow interface{}
	args     []interface{}
}

func (s *workflowStarterSpy) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	s.calls = append(s.calls, workflowStartCall{
		ctx:      ctx,
		options:  options,
		workflow: workflow,
		args:     args,
	})
	return nil, s.err
}
