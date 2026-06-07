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

	registerRecordingProcessing(worker, store, localPathResolverTestStub{}, audioProbeRunnerTestStub{}, audioNormalizeRunnerTestStub{})

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
		activities.NormalizeRecordingAudioActivityName,
		activities.MarkRecordingTranscribingActivityName,
		activities.TranscribeRecordingAudioActivityName,
		activities.MarkRecordingSummarizingActivityName,
		activities.SummarizeRecordingActivityName,
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

type audioNormalizeRunnerTestStub struct{}

func (audioNormalizeRunnerTestStub) Normalize(ctx context.Context, input activities.AudioNormalizeRequest) (activities.AudioNormalizeResult, error) {
	return activities.AudioNormalizeResult{}, nil
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

func (s *workerRecordingStoreSpy) UpsertNormalizedAudio(input recordings.UpsertNormalizedAudioInput) (recordings.RecordingNormalizedAudio, error) {
	return recordings.RecordingNormalizedAudio{RecordingID: input.RecordingID, ObjectKey: input.ObjectKey}, nil
}

func (s *workerRecordingStoreSpy) GetNormalizedAudio(recordingID string) (recordings.RecordingNormalizedAudio, bool) {
	return recordings.RecordingNormalizedAudio{RecordingID: recordingID, ObjectKey: "recordings/rec/normalized.wav"}, true
}

func (s *workerRecordingStoreSpy) UpsertTranscript(input recordings.UpsertTranscriptInput) (recordings.RecordingTranscript, error) {
	return recordings.RecordingTranscript{RecordingID: input.RecordingID}, nil
}

func (s *workerRecordingStoreSpy) GetTranscript(recordingID string) (recordings.RecordingTranscript, bool) {
	return recordings.RecordingTranscript{RecordingID: recordingID}, true
}

func (s *workerRecordingStoreSpy) UpsertSummary(input recordings.UpsertSummaryInput) (recordings.RecordingSummary, error) {
	return recordings.RecordingSummary{RecordingID: input.RecordingID}, nil
}
