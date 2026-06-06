package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/zzyhdu/soniq/backend/internal/activities"
	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.temporal.io/sdk/activity"
)

func TestRegisterRecordingProcessingRegistersWorkflowAndActivities(t *testing.T) {
	worker := &recordingWorkerSpy{}
	store := &workerRecordingStoreSpy{}

	registerRecordingProcessing(worker, store, localPathResolverTestStub{}, audioProbeRunnerTestStub{})

	if got, want := len(worker.workflows), 1; got != want {
		t.Fatalf("registered workflows = %d, want %d", got, want)
	}
	if !sameFunction(worker.workflows[0], workflows.RecordingProcessingWorkflow) {
		t.Fatalf("registered workflow = %T, want RecordingProcessingWorkflow", worker.workflows[0])
	}

	wantActivityNames := []string{
		activities.ValidateRecordingActivityName,
		activities.MarkRecordingProcessingActivityName,
		activities.ProbeRecordingAudioActivityName,
		activities.CompleteRecordingProcessingActivityName,
		activities.FailRecordingProcessingActivityName,
	}
	if got, want := len(worker.activities), len(wantActivityNames); got != want {
		t.Fatalf("registered activities = %d, want %d", got, want)
	}
	for i, wantName := range wantActivityNames {
		if got := worker.activities[i].options.Name; got != wantName {
			t.Fatalf("activity %d name = %q, want %q", i, got, wantName)
		}
	}
}

func sameFunction(a, b interface{}) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

type localPathResolverTestStub struct{}

func (localPathResolverTestStub) LocalPathForObject(key string) (string, error) {
	return key, nil
}

type audioProbeRunnerTestStub struct{}

func (audioProbeRunnerTestStub) Probe(ctx context.Context, path string) (activities.AudioProbeResult, error) {
	return activities.AudioProbeResult{}, nil
}

type registeredActivity struct {
	activity interface{}
	options  activity.RegisterOptions
}

type recordingWorkerSpy struct {
	workflows  []interface{}
	activities []registeredActivity
}

func (s *recordingWorkerSpy) RegisterWorkflow(workflow interface{}) {
	s.workflows = append(s.workflows, workflow)
}

func (s *recordingWorkerSpy) RegisterActivity(activityFn interface{}) {
	s.RegisterActivityWithOptions(activityFn, activity.RegisterOptions{})
}

func (s *recordingWorkerSpy) RegisterActivityWithOptions(activityFn interface{}, options activity.RegisterOptions) {
	s.activities = append(s.activities, registeredActivity{activity: activityFn, options: options})
}

type workerRecordingStoreSpy struct{}

func (s *workerRecordingStoreSpy) Get(id string) (domain.Recording, bool) {
	return domain.Recording{ID: id}, true
}

func (s *workerRecordingStoreSpy) UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
	return domain.Recording{ID: input.ID, Status: input.Status}, nil
}

func (s *workerRecordingStoreSpy) UpsertAudioProbe(input recordings.UpsertAudioProbeInput) (recordings.RecordingAudioProbe, error) {
	return recordings.RecordingAudioProbe{RecordingID: input.RecordingID}, nil
}
