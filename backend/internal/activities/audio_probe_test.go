package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
)

type audioProbeRecordingStoreSpy struct {
	recordings map[string]domain.Recording
	probes     []recordings.UpsertAudioProbeInput
}

func (s *audioProbeRecordingStoreSpy) Get(id string) (domain.Recording, bool, error) {
	if s.recordings == nil {
		return domain.Recording{}, false, nil
	}
	recording, ok := s.recordings[id]
	return recording, ok, nil
}

func (s *audioProbeRecordingStoreSpy) GetForWorkspace(input recordings.GetRecordingInput) (domain.Recording, bool, error) {
	recording, ok, err := s.Get(input.ID)
	if err != nil {
		return domain.Recording{}, false, err
	}
	if !ok || recording.WorkspaceID != input.WorkspaceID {
		return domain.Recording{}, false, nil
	}
	return recording, true, nil
}

func (s *audioProbeRecordingStoreSpy) UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
	recording, ok, err := s.Get(input.ID)
	if err != nil {
		return domain.Recording{}, err
	}
	if !ok {
		return domain.Recording{}, errors.New("recording not found")
	}
	recording.Status = input.Status
	return recording, nil
}

func (s *audioProbeRecordingStoreSpy) UpsertAudioProbe(input recordings.UpsertAudioProbeInput) (recordings.RecordingAudioProbe, error) {
	s.probes = append(s.probes, input)
	return recordings.RecordingAudioProbe{
		RecordingID:     input.RecordingID,
		DurationSeconds: input.DurationSeconds,
		FormatName:      input.FormatName,
		CodecName:       input.CodecName,
		SampleRate:      input.SampleRate,
		Channels:        input.Channels,
		BitRate:         input.BitRate,
		RawProbeJSON:    append([]byte(nil), input.RawProbeJSON...),
		ProbedAt:        input.ProbedAt,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}, nil
}

type localPathResolverSpy struct {
	paths []string
	path  string
	err   error
}

func (s *localPathResolverSpy) LocalPathForObject(key string) (string, error) {
	s.paths = append(s.paths, key)
	if s.err != nil {
		return "", s.err
	}
	return s.path, nil
}

type audioProbeRunnerSpy struct {
	paths  []string
	result AudioProbeResult
	err    error
}

func (s *audioProbeRunnerSpy) Probe(ctx context.Context, path string) (AudioProbeResult, error) {
	s.paths = append(s.paths, path)
	if s.err != nil {
		return AudioProbeResult{}, s.err
	}
	return s.result, nil
}

func TestRecordingProcessingActivitiesProbeRecordingAudioPersistsProbeMetadata(t *testing.T) {
	store := &audioProbeRecordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_probe": {
			ID:             "rec_probe",
			AudioObjectKey: "recordings/rec_probe/original.wav",
		},
	}}
	resolver := &localPathResolverSpy{path: "/tmp/soniq/recordings/rec_probe/original.wav"}
	probedAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	runner := &audioProbeRunnerSpy{result: AudioProbeResult{
		DurationSeconds: 12.5,
		FormatName:      "wav",
		CodecName:       "pcm_s16le",
		SampleRate:      16000,
		Channels:        1,
		BitRate:         256000,
		RawProbeJSON:    []byte(`{"format":{"duration":"12.5"}}`),
		ProbedAt:        probedAt,
	}}
	activities := NewRecordingProcessingActivitiesWithAudioProbe(store, resolver, runner)

	if err := activities.ProbeRecordingAudio(context.Background(), "rec_probe"); err != nil {
		t.Fatalf("ProbeRecordingAudio returned error: %v", err)
	}

	if len(resolver.paths) != 1 || resolver.paths[0] != "recordings/rec_probe/original.wav" {
		t.Fatalf("resolved paths = %+v, want original object key", resolver.paths)
	}
	if len(runner.paths) != 1 || runner.paths[0] != "/tmp/soniq/recordings/rec_probe/original.wav" {
		t.Fatalf("runner paths = %+v, want resolved local path", runner.paths)
	}
	if len(store.probes) != 1 {
		t.Fatalf("stored probes = %d, want 1", len(store.probes))
	}
	probe := store.probes[0]
	if probe.RecordingID != "rec_probe" || probe.FormatName != "wav" || probe.CodecName != "pcm_s16le" {
		t.Fatalf("stored probe = %+v, want ffprobe metadata", probe)
	}
	if probe.DurationSeconds != 12.5 || probe.SampleRate != 16000 || probe.Channels != 1 || probe.BitRate != 256000 {
		t.Fatalf("stored numeric fields = %+v, want ffprobe metadata", probe)
	}
	if string(probe.RawProbeJSON) != `{"format":{"duration":"12.5"}}` {
		t.Fatalf("RawProbeJSON = %s, want raw ffprobe json", probe.RawProbeJSON)
	}
	if !probe.ProbedAt.Equal(probedAt) {
		t.Fatalf("ProbedAt = %s, want %s", probe.ProbedAt, probedAt)
	}
}

func TestRecordingProcessingActivitiesProbeRecordingAudioRejectsMissingRecordingID(t *testing.T) {
	activities := NewRecordingProcessingActivitiesWithAudioProbe(&audioProbeRecordingStoreSpy{}, &localPathResolverSpy{}, &audioProbeRunnerSpy{})

	if err := activities.ProbeRecordingAudio(context.Background(), ""); err == nil {
		t.Fatal("ProbeRecordingAudio returned nil error, want missing recording id error")
	}
}

func TestRecordingProcessingActivitiesProbeRecordingAudioRequiresStore(t *testing.T) {
	activities := NewRecordingProcessingActivitiesWithAudioProbe(nil, &localPathResolverSpy{}, &audioProbeRunnerSpy{})

	if err := activities.ProbeRecordingAudio(context.Background(), "rec_probe"); err == nil {
		t.Fatal("ProbeRecordingAudio returned nil error, want store required error")
	}
}

func TestRecordingProcessingActivitiesProbeRecordingAudioRequiresExistingRecording(t *testing.T) {
	activities := NewRecordingProcessingActivitiesWithAudioProbe(&audioProbeRecordingStoreSpy{}, &localPathResolverSpy{}, &audioProbeRunnerSpy{})

	if err := activities.ProbeRecordingAudio(context.Background(), "rec_missing"); err == nil {
		t.Fatal("ProbeRecordingAudio returned nil error, want missing recording error")
	}
}

func TestRecordingProcessingActivitiesProbeRecordingAudioRequiresAudioObjectKey(t *testing.T) {
	store := &audioProbeRecordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_probe": {ID: "rec_probe"},
	}}
	activities := NewRecordingProcessingActivitiesWithAudioProbe(store, &localPathResolverSpy{}, &audioProbeRunnerSpy{})

	if err := activities.ProbeRecordingAudio(context.Background(), "rec_probe"); err == nil {
		t.Fatal("ProbeRecordingAudio returned nil error, want missing audio object key error")
	}
}

func TestRecordingProcessingActivitiesProbeRecordingAudioReturnsResolverError(t *testing.T) {
	store := &audioProbeRecordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_probe": {ID: "rec_probe", AudioObjectKey: "recordings/rec_probe/original.wav"},
	}}
	resolverErr := errors.New("resolve failed")
	activities := NewRecordingProcessingActivitiesWithAudioProbe(store, &localPathResolverSpy{err: resolverErr}, &audioProbeRunnerSpy{})

	if err := activities.ProbeRecordingAudio(context.Background(), "rec_probe"); !errors.Is(err, resolverErr) {
		t.Fatalf("ProbeRecordingAudio error = %v, want resolver error", err)
	}
}

func TestRecordingProcessingActivitiesProbeRecordingAudioReturnsRunnerErrorWithoutPersisting(t *testing.T) {
	store := &audioProbeRecordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_probe": {ID: "rec_probe", AudioObjectKey: "recordings/rec_probe/original.wav"},
	}}
	runnerErr := errors.New("ffprobe failed")
	activities := NewRecordingProcessingActivitiesWithAudioProbe(
		store,
		&localPathResolverSpy{path: "/tmp/soniq/recordings/rec_probe/original.wav"},
		&audioProbeRunnerSpy{err: runnerErr},
	)

	if err := activities.ProbeRecordingAudio(context.Background(), "rec_probe"); !errors.Is(err, runnerErr) {
		t.Fatalf("ProbeRecordingAudio error = %v, want runner error", err)
	}
	if len(store.probes) != 0 {
		t.Fatalf("stored probes = %d, want 0 after runner error", len(store.probes))
	}
}
