package main

import (
	"reflect"
	"testing"

	"github.com/zzyhdu/soniq/backend/internal/activities"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.temporal.io/sdk/activity"
)

func TestRegisterRecordingProcessingRegistersWorkflowAndActivities(t *testing.T) {
	worker := &recordingWorkerSpy{}

	registerRecordingProcessing(worker)

	if got, want := len(worker.workflows), 1; got != want {
		t.Fatalf("registered workflows = %d, want %d", got, want)
	}
	if !sameFunction(worker.workflows[0], workflows.RecordingProcessingWorkflow) {
		t.Fatalf("registered workflow = %T, want RecordingProcessingWorkflow", worker.workflows[0])
	}

	wantActivities := []interface{}{
		activities.ValidateRecordingActivity,
		activities.MarkRecordingProcessingActivity,
		activities.CompleteRecordingProcessingActivity,
	}
	if got, want := len(worker.activities), len(wantActivities); got != want {
		t.Fatalf("registered activities = %d, want %d", got, want)
	}
	for i, wantActivity := range wantActivities {
		if !sameFunction(worker.activities[i], wantActivity) {
			t.Fatalf("activity %d = %T, want %T", i, worker.activities[i], wantActivity)
		}
	}
}

func sameFunction(a, b interface{}) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

type recordingWorkerSpy struct {
	workflows  []interface{}
	activities []interface{}
}

func (s *recordingWorkerSpy) RegisterWorkflow(workflow interface{}) {
	s.workflows = append(s.workflows, workflow)
}

func (s *recordingWorkerSpy) RegisterActivity(activity interface{}) {
	s.activities = append(s.activities, activity)
}

func (s *recordingWorkerSpy) RegisterActivityWithOptions(activity interface{}, options activity.RegisterOptions) {
	s.RegisterActivity(activity)
}
