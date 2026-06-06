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
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.temporal.io/sdk/client"
)

func TestBuildHandlerInjectsTemporalRecordingProcessor(t *testing.T) {
	temporalClient := &temporalClientSpy{}
	store := newBuildHandlerRecordingStoreSpy()
	storeFactory := &recordingStoreFactorySpy{store: store}
	cfg := config.Config{
		APIAddress:        ":0",
		PostgresDSN:       "postgres://custom_user:***@db:5432/custom?sslmode=disable",
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
	}, storeFactory.Open)
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()
	if got, want := len(storeFactory.calls), 1; got != want {
		t.Fatalf("recording store factory calls = %d, want %d", got, want)
	}
	if storeFactory.calls[0] != cfg.PostgresDSN {
		t.Fatalf("recording store factory DSN = %q, want %q", storeFactory.calls[0], cfg.PostgresDSN)
	}

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
	store := newBuildHandlerRecordingStoreSpy()
	storeFactory := &recordingStoreFactorySpy{store: store}

	_, cleanup, err := buildHandler(context.Background(), config.Config{TemporalTaskQueue: "soniq-audio-pipeline", PostgresDSN: "postgres://custom_user:***@db:5432/custom?sslmode=disable"}, func(context.Context, config.Config) (temporalWorkflowClient, error) {
		return temporalClient, nil
	}, storeFactory.Open)
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}

	cleanup()

	if !temporalClient.closed {
		t.Fatal("temporal client closed = false, want true")
	}
	if !store.closed {
		t.Fatal("recording store closed = false, want true")
	}
}

func sameFunction(a, b interface{}) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

type recordingStoreFactorySpy struct {
	store *buildHandlerRecordingStoreSpy
	calls []string
}

func (s *recordingStoreFactorySpy) Open(ctx context.Context, dsn string) (recordingStoreClient, error) {
	s.calls = append(s.calls, dsn)
	return s.store, nil
}

type buildHandlerRecordingStoreSpy struct {
	store  *recordings.MemoryStore
	closed bool
}

func newBuildHandlerRecordingStoreSpy() *buildHandlerRecordingStoreSpy {
	return &buildHandlerRecordingStoreSpy{store: recordings.NewMemoryStore()}
}

func (s *buildHandlerRecordingStoreSpy) Create(input recordings.CreateRecordingInput) (domain.Recording, error) {
	return s.store.Create(input)
}

func (s *buildHandlerRecordingStoreSpy) Get(id string) (domain.Recording, bool) {
	return s.store.Get(id)
}

func (s *buildHandlerRecordingStoreSpy) Close() {
	s.closed = true
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
