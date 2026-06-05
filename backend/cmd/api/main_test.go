package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/zzyhdu/soniq/backend/internal/config"
	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.temporal.io/sdk/client"
)

func TestBuildHandlerInjectsTemporalRecordingProcessor(t *testing.T) {
	temporalClient := &temporalClientSpy{}
	cfg := config.Config{
		APIAddress:        ":0",
		TemporalAddress:   "temporal.example:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "soniq-audio-pipeline",
	}

	handler, cleanup, err := buildHandler(context.Background(), cfg, func(ctx context.Context, cfg config.Config) (temporalWorkflowClient, error) {
		if cfg.TemporalAddress != "temporal.example:7233" {
			t.Fatalf("TemporalAddress = %q, want temporal.example:7233", cfg.TemporalAddress)
		}
		if cfg.TemporalNamespace != "default" {
			t.Fatalf("TemporalNamespace = %q, want default", cfg.TemporalNamespace)
		}
		return temporalClient, nil
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	request := httptest.NewRequest(http.MethodPost, "/recordings", strings.NewReader(`{"title":"Weekly sync","workflow_type":"meeting","language":"en"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	var recording domain.Recording
	if err := json.NewDecoder(response.Body).Decode(&recording); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got, want := len(temporalClient.calls), 1; got != want {
		t.Fatalf("ExecuteWorkflow calls = %d, want %d", got, want)
	}
	call := temporalClient.calls[0]
	if call.options.ID != "recording-processing-"+recording.ID {
		t.Fatalf("workflow ID = %q, want recording-processing-%s", call.options.ID, recording.ID)
	}
	if call.options.TaskQueue != "soniq-audio-pipeline" {
		t.Fatalf("task queue = %q, want soniq-audio-pipeline", call.options.TaskQueue)
	}
	if !sameFunction(call.workflow, workflows.RecordingProcessingWorkflow) {
		t.Fatalf("workflow = %T, want RecordingProcessingWorkflow", call.workflow)
	}
	input, ok := call.args[0].(workflows.RecordingProcessingInput)
	if !ok {
		t.Fatalf("workflow arg type = %T, want RecordingProcessingInput", call.args[0])
	}
	if input.RecordingID != recording.ID {
		t.Fatalf("input recording ID = %q, want %q", input.RecordingID, recording.ID)
	}
	if input.WorkflowType != domain.WorkflowTypeMeeting {
		t.Fatalf("input workflow_type = %q, want meeting", input.WorkflowType)
	}
	if input.Language != "en" {
		t.Fatalf("input language = %q, want en", input.Language)
	}
}

func TestBuildHandlerCleanupClosesTemporalClient(t *testing.T) {
	temporalClient := &temporalClientSpy{}

	_, cleanup, err := buildHandler(context.Background(), config.Config{TemporalTaskQueue: "soniq-audio-pipeline"}, func(context.Context, config.Config) (temporalWorkflowClient, error) {
		return temporalClient, nil
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}

	cleanup()

	if !temporalClient.closed {
		t.Fatal("temporal client closed = false, want true")
	}
}

func sameFunction(a, b interface{}) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

type temporalClientSpy struct {
	calls  []workflowStartCall
	closed bool
}

type workflowStartCall struct {
	options  client.StartWorkflowOptions
	workflow interface{}
	args     []interface{}
}

func (s *temporalClientSpy) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	s.calls = append(s.calls, workflowStartCall{
		options:  options,
		workflow: workflow,
		args:     args,
	})
	return nil, nil
}

func (s *temporalClientSpy) Close() {
	s.closed = true
}
