package activities

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
)

type normalizeCommandCall struct {
	binary string
	args   []string
}

type normalizeCommandRunnerSpy struct {
	calls  []normalizeCommandCall
	output []byte
	err    error
}

func (s *normalizeCommandRunnerSpy) Run(ctx context.Context, binary string, args ...string) ([]byte, error) {
	s.calls = append(s.calls, normalizeCommandCall{binary: binary, args: append([]string(nil), args...)})
	return append([]byte(nil), s.output...), s.err
}

func TestFFmpegNormalizeRunnerRejectsMissingInputPath(t *testing.T) {
	commandRunner := &normalizeCommandRunnerSpy{}
	runner := FFmpegNormalizeRunner{Binary: "ffmpeg-test", CommandRunner: commandRunner}

	_, err := runner.Normalize(context.Background(), AudioNormalizeRequest{OutputPath: "/tmp/normalized.wav"})
	if err == nil {
		t.Fatal("Normalize returned nil error, want missing input path error")
	}
	if len(commandRunner.calls) != 0 {
		t.Fatalf("command calls = %d, want 0", len(commandRunner.calls))
	}
}

func TestFFmpegNormalizeRunnerRejectsMissingOutputPath(t *testing.T) {
	commandRunner := &normalizeCommandRunnerSpy{}
	runner := FFmpegNormalizeRunner{Binary: "ffmpeg-test", CommandRunner: commandRunner}

	_, err := runner.Normalize(context.Background(), AudioNormalizeRequest{InputPath: "/tmp/original.wav"})
	if err == nil {
		t.Fatal("Normalize returned nil error, want missing output path error")
	}
	if len(commandRunner.calls) != 0 {
		t.Fatalf("command calls = %d, want 0", len(commandRunner.calls))
	}
}

func TestFFmpegNormalizeRunnerInvokesFFmpegWithStableTarget(t *testing.T) {
	commandRunner := &normalizeCommandRunnerSpy{}
	runner := FFmpegNormalizeRunner{Binary: "ffmpeg-test", CommandRunner: commandRunner}

	result, err := runner.Normalize(context.Background(), AudioNormalizeRequest{
		InputPath:  "/tmp/soniq/original.wav",
		OutputPath: "/tmp/soniq/normalized.wav",
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if len(commandRunner.calls) != 1 {
		t.Fatalf("command calls = %d, want 1", len(commandRunner.calls))
	}
	call := commandRunner.calls[0]
	if call.binary != "ffmpeg-test" {
		t.Fatalf("binary = %q, want ffmpeg-test", call.binary)
	}
	wantSubsequence := []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "/tmp/soniq/original.wav",
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "pcm_s16le",
		"/tmp/soniq/normalized.wav",
	}
	if !containsSubsequence(call.args, wantSubsequence) {
		t.Fatalf("args = %#v, want subsequence %#v", call.args, wantSubsequence)
	}
	if result.OutputPath != "/tmp/soniq/normalized.wav" {
		t.Fatalf("OutputPath = %q, want output path", result.OutputPath)
	}
	if result.ContentType != "audio/wav" || result.FormatName != "wav" || result.CodecName != "pcm_s16le" {
		t.Fatalf("result target metadata = %+v, want wav pcm_s16le metadata", result)
	}
	if result.SampleRate != 16000 || result.Channels != 1 {
		t.Fatalf("result sample metadata = %+v, want 16000 Hz mono", result)
	}
	if result.NormalizedAt.IsZero() {
		t.Fatal("NormalizedAt is zero, want timestamp")
	}
}

func TestFFmpegNormalizeRunnerIncludesStderrOnFailure(t *testing.T) {
	commandRunner := &normalizeCommandRunnerSpy{output: []byte("ffmpeg stderr: invalid data"), err: errors.New("exit status 1")}
	runner := FFmpegNormalizeRunner{Binary: "ffmpeg-test", CommandRunner: commandRunner}

	_, err := runner.Normalize(context.Background(), AudioNormalizeRequest{
		InputPath:  "/tmp/soniq/original.wav",
		OutputPath: "/tmp/soniq/normalized.wav",
	})
	if err == nil {
		t.Fatal("Normalize returned nil error, want ffmpeg failure")
	}
	if !strings.Contains(err.Error(), "ffmpeg stderr: invalid data") {
		t.Fatalf("Normalize error = %v, want stderr context", err)
	}
}

func containsSubsequence(values []string, subsequence []string) bool {
	if len(subsequence) == 0 {
		return true
	}
	j := 0
	for _, value := range values {
		if value == subsequence[j] {
			j++
			if j == len(subsequence) {
				return true
			}
		}
	}
	return false
}

type normalizeRecordingStoreSpy struct {
	recordings       map[string]domain.Recording
	probes           []recordings.UpsertAudioProbeInput
	normalizedAudios []recordings.UpsertNormalizedAudioInput
}

func (s *normalizeRecordingStoreSpy) Get(id string) (domain.Recording, bool, error) {
	recording, ok := s.recordings[id]
	return recording, ok, nil
}

func (s *normalizeRecordingStoreSpy) GetForWorkspace(input recordings.GetRecordingInput) (domain.Recording, bool, error) {
	recording, ok, err := s.Get(input.ID)
	if err != nil {
		return domain.Recording{}, false, err
	}
	if !ok || recording.WorkspaceID != input.WorkspaceID {
		return domain.Recording{}, false, nil
	}
	return recording, true, nil
}

func (s *normalizeRecordingStoreSpy) UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
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

func (s *normalizeRecordingStoreSpy) UpsertAudioProbe(input recordings.UpsertAudioProbeInput) (recordings.RecordingAudioProbe, error) {
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
	}, nil
}

func (s *normalizeRecordingStoreSpy) UpsertNormalizedAudio(input recordings.UpsertNormalizedAudioInput) (recordings.RecordingNormalizedAudio, error) {
	s.normalizedAudios = append(s.normalizedAudios, input)
	return recordings.RecordingNormalizedAudio{RecordingID: input.RecordingID, ObjectKey: input.ObjectKey}, nil
}

func (s *normalizeRecordingStoreSpy) GetNormalizedAudio(recordingID string) (recordings.RecordingNormalizedAudio, bool, error) {
	for _, input := range s.normalizedAudios {
		if input.RecordingID == recordingID {
			return recordings.RecordingNormalizedAudio{RecordingID: input.RecordingID, ObjectKey: input.ObjectKey}, true, nil
		}
	}
	return recordings.RecordingNormalizedAudio{}, false, nil
}

type audioNormalizeRunnerSpy struct {
	requests []AudioNormalizeRequest
	result   AudioNormalizeResult
	err      error
	write    []byte
}

type normalizeObjectStoreSpy struct {
	objects map[string]string
	gets    []string
	puts    []storage.PutObjectInput
	err     error
}

func (s *normalizeObjectStoreSpy) PutObject(ctx context.Context, input storage.PutObjectInput) (storage.PutObjectResult, error) {
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return storage.PutObjectResult{}, err
	}
	if s.objects == nil {
		s.objects = map[string]string{}
	}
	s.objects[input.Key] = string(body)
	s.puts = append(s.puts, storage.PutObjectInput{
		Key:         input.Key,
		ContentType: input.ContentType,
	})
	return storage.PutObjectResult{Key: input.Key, SizeBytes: int64(len(body))}, nil
}

func (s *normalizeObjectStoreSpy) GetObject(ctx context.Context, key string) (storage.GetObjectResult, error) {
	if s.err != nil {
		return storage.GetObjectResult{}, s.err
	}
	s.gets = append(s.gets, key)
	return storage.GetObjectResult{Key: key, Body: io.NopCloser(strings.NewReader(s.objects[key])), SizeBytes: int64(len(s.objects[key]))}, nil
}

func (s *normalizeObjectStoreSpy) PresignGetObject(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return "https://objects.example.test/" + key, nil
}

func (s *normalizeObjectStoreSpy) DeleteObject(ctx context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

func (s *audioNormalizeRunnerSpy) Normalize(ctx context.Context, input AudioNormalizeRequest) (AudioNormalizeResult, error) {
	s.requests = append(s.requests, input)
	if s.err != nil {
		return AudioNormalizeResult{}, s.err
	}
	if len(s.write) > 0 {
		if err := os.WriteFile(input.OutputPath, s.write, 0o644); err != nil {
			return AudioNormalizeResult{}, err
		}
	}
	result := s.result
	if result.OutputPath == "" {
		result.OutputPath = input.OutputPath
	}
	if result.ContentType == "" {
		result.ContentType = "audio/wav"
	}
	if result.FormatName == "" {
		result.FormatName = "wav"
	}
	if result.CodecName == "" {
		result.CodecName = "pcm_s16le"
	}
	if result.SampleRate == 0 {
		result.SampleRate = 16000
	}
	if result.Channels == 0 {
		result.Channels = 1
	}
	if result.NormalizedAt.IsZero() {
		result.NormalizedAt = time.Date(2026, 6, 6, 5, 6, 7, 0, time.UTC)
	}
	return result, nil
}

func TestRecordingProcessingActivitiesPrepareRecordingAudioUsesObjectStoreStagingOnce(t *testing.T) {
	store := &normalizeRecordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_normalize": {ID: "rec_normalize", AudioObjectKey: "recordings/rec_normalize/original.wav"},
	}}
	objectStore := &normalizeObjectStoreSpy{objects: map[string]string{
		"recordings/rec_normalize/original.wav": "original-audio",
	}}
	probeRunner := &audioProbeRunnerSpy{result: AudioProbeResult{FormatName: "wav", CodecName: "pcm_s16le"}}
	runner := &audioNormalizeRunnerSpy{write: []byte("normalized-audio")}
	activities := &RecordingProcessingActivities{store: store, normalizedAudioStore: store, objectStore: objectStore, probeRunner: probeRunner, normalizeRunner: runner}

	if err := activities.PrepareRecordingAudio(context.Background(), "rec_normalize"); err != nil {
		t.Fatalf("PrepareRecordingAudio returned error: %v", err)
	}
	if len(objectStore.gets) != 1 || objectStore.gets[0] != "recordings/rec_normalize/original.wav" {
		t.Fatalf("get objects = %+v, want one original audio download", objectStore.gets)
	}
	if len(probeRunner.paths) != 1 {
		t.Fatalf("probe runner paths = %d, want 1", len(probeRunner.paths))
	}
	if len(runner.requests) != 1 {
		t.Fatalf("normalize runner requests = %d, want 1", len(runner.requests))
	}
	if probeRunner.paths[0] != runner.requests[0].InputPath {
		t.Fatalf("probe path = %q, normalize input = %q, want shared staged file", probeRunner.paths[0], runner.requests[0].InputPath)
	}
	if runner.requests[0].InputPath == "" || runner.requests[0].OutputPath == "" || runner.requests[0].InputPath == runner.requests[0].OutputPath {
		t.Fatalf("normalize request paths = %+v, want distinct temporary paths", runner.requests[0])
	}
	if got := objectStore.objects["recordings/rec_normalize/normalized.wav"]; got != "normalized-audio" {
		t.Fatalf("uploaded normalized audio = %q, want normalized-audio", got)
	}
	if len(objectStore.puts) != 1 || objectStore.puts[0].Key != "recordings/rec_normalize/normalized.wav" || objectStore.puts[0].ContentType != "audio/wav" {
		t.Fatalf("put objects = %+v, want normalized audio upload", objectStore.puts)
	}
	if len(store.normalizedAudios) != 1 || store.normalizedAudios[0].SizeBytes != int64(len("normalized-audio")) {
		t.Fatalf("normalized rows = %+v, want uploaded normalized size", store.normalizedAudios)
	}
}

func TestRecordingProcessingActivitiesPrepareRecordingAudioPersistsProbeAndNormalizedMetadata(t *testing.T) {
	store := &normalizeRecordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_normalize": {ID: "rec_normalize", AudioObjectKey: "recordings/rec_normalize/original.wav"},
	}}
	objectStore := &normalizeObjectStoreSpy{objects: map[string]string{
		"recordings/rec_normalize/original.wav": "original-audio",
	}}
	probedAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	probeRunner := &audioProbeRunnerSpy{result: AudioProbeResult{
		DurationSeconds: 12.5,
		FormatName:      "wav",
		CodecName:       "pcm_s16le",
		SampleRate:      16000,
		Channels:        1,
		BitRate:         256000,
		RawProbeJSON:    []byte(`{"format":{"duration":"12.5"}}`),
		ProbedAt:        probedAt,
	}}
	runner := &audioNormalizeRunnerSpy{write: []byte("normalized-audio")}
	activities := &RecordingProcessingActivities{store: store, normalizedAudioStore: store, objectStore: objectStore, probeRunner: probeRunner, normalizeRunner: runner}

	if err := activities.PrepareRecordingAudio(context.Background(), "rec_normalize"); err != nil {
		t.Fatalf("PrepareRecordingAudio returned error: %v", err)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("normalize runner requests = %d, want 1", len(runner.requests))
	}
	if len(objectStore.gets) != 1 || objectStore.gets[0] != "recordings/rec_normalize/original.wav" {
		t.Fatalf("get objects = %+v, want original audio download", objectStore.gets)
	}
	if len(probeRunner.paths) != 1 || probeRunner.paths[0] != runner.requests[0].InputPath {
		t.Fatalf("probe path = %+v normalize request = %+v, want shared staged input", probeRunner.paths, runner.requests[0])
	}
	if runner.requests[0].InputPath == "" || runner.requests[0].OutputPath == "" || runner.requests[0].InputPath == runner.requests[0].OutputPath {
		t.Fatalf("normalize request = %+v, want distinct temporary paths", runner.requests[0])
	}
	if len(store.probes) != 1 {
		t.Fatalf("stored probes = %d, want 1", len(store.probes))
	}
	probe := store.probes[0]
	if probe.RecordingID != "rec_normalize" || probe.FormatName != "wav" || probe.CodecName != "pcm_s16le" {
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
	if len(store.normalizedAudios) != 1 {
		t.Fatalf("normalized rows = %d, want 1", len(store.normalizedAudios))
	}
	persisted := store.normalizedAudios[0]
	if persisted.RecordingID != "rec_normalize" || persisted.ObjectKey != "recordings/rec_normalize/normalized.wav" {
		t.Fatalf("persisted normalized audio = %+v, want recording id and normalized object key", persisted)
	}
	if persisted.ContentType != "audio/wav" || persisted.FormatName != "wav" || persisted.CodecName != "pcm_s16le" {
		t.Fatalf("persisted target metadata = %+v, want wav pcm_s16le", persisted)
	}
	if persisted.SampleRate != 16000 || persisted.Channels != 1 || persisted.SizeBytes != int64(len("normalized-audio")) {
		t.Fatalf("persisted numeric metadata = %+v, want 16k mono with output size", persisted)
	}
	if got := objectStore.objects["recordings/rec_normalize/normalized.wav"]; got != "normalized-audio" {
		t.Fatalf("uploaded normalized audio = %q, want normalized-audio", got)
	}
}

func TestRecordingProcessingActivitiesPrepareRecordingAudioRejectsMissingRecordingID(t *testing.T) {
	activities := &RecordingProcessingActivities{store: &normalizeRecordingStoreSpy{}, normalizedAudioStore: &normalizeRecordingStoreSpy{}, objectStore: &normalizeObjectStoreSpy{}, probeRunner: &audioProbeRunnerSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}
	if err := activities.PrepareRecordingAudio(context.Background(), ""); err == nil {
		t.Fatal("PrepareRecordingAudio returned nil error, want missing recording id error")
	}
}

func TestRecordingProcessingActivitiesPrepareRecordingAudioRequiresDependencies(t *testing.T) {
	store := &normalizeRecordingStoreSpy{recordings: map[string]domain.Recording{"rec_normalize": {ID: "rec_normalize", AudioObjectKey: "recordings/rec_normalize/original.wav"}}}
	tests := []struct {
		name       string
		activities *RecordingProcessingActivities
	}{
		{name: "store", activities: &RecordingProcessingActivities{normalizedAudioStore: store, objectStore: &normalizeObjectStoreSpy{}, probeRunner: &audioProbeRunnerSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}},
		{name: "normalized store", activities: &RecordingProcessingActivities{store: store, objectStore: &normalizeObjectStoreSpy{}, probeRunner: &audioProbeRunnerSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}},
		{name: "object store", activities: &RecordingProcessingActivities{store: store, normalizedAudioStore: store, probeRunner: &audioProbeRunnerSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}},
		{name: "probe runner", activities: &RecordingProcessingActivities{store: store, normalizedAudioStore: store, objectStore: &normalizeObjectStoreSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}},
		{name: "normalize runner", activities: &RecordingProcessingActivities{store: store, normalizedAudioStore: store, objectStore: &normalizeObjectStoreSpy{}, probeRunner: &audioProbeRunnerSpy{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.activities.PrepareRecordingAudio(context.Background(), "rec_normalize"); err == nil {
				t.Fatal("PrepareRecordingAudio returned nil error, want missing dependency error")
			}
		})
	}
}

func TestRecordingProcessingActivitiesPrepareRecordingAudioRequiresExistingRecording(t *testing.T) {
	activities := &RecordingProcessingActivities{store: &normalizeRecordingStoreSpy{}, normalizedAudioStore: &normalizeRecordingStoreSpy{}, objectStore: &normalizeObjectStoreSpy{}, probeRunner: &audioProbeRunnerSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}
	if err := activities.PrepareRecordingAudio(context.Background(), "rec_missing"); err == nil {
		t.Fatal("PrepareRecordingAudio returned nil error, want missing recording error")
	}
}

func TestRecordingProcessingActivitiesPrepareRecordingAudioRequiresAudioObjectKey(t *testing.T) {
	store := &normalizeRecordingStoreSpy{recordings: map[string]domain.Recording{"rec_normalize": {ID: "rec_normalize"}}}
	activities := &RecordingProcessingActivities{store: store, normalizedAudioStore: store, objectStore: &normalizeObjectStoreSpy{}, probeRunner: &audioProbeRunnerSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}
	if err := activities.PrepareRecordingAudio(context.Background(), "rec_normalize"); err == nil {
		t.Fatal("PrepareRecordingAudio returned nil error, want missing audio object key error")
	}
}

func TestRecordingProcessingActivitiesPrepareRecordingAudioReturnsObjectStoreReadError(t *testing.T) {
	store := &normalizeRecordingStoreSpy{recordings: map[string]domain.Recording{"rec_normalize": {ID: "rec_normalize", AudioObjectKey: "recordings/rec_normalize/original.wav"}}}
	readErr := errors.New("read object failed")
	activities := &RecordingProcessingActivities{store: store, normalizedAudioStore: store, objectStore: &normalizeObjectStoreSpy{err: readErr}, probeRunner: &audioProbeRunnerSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}

	if err := activities.PrepareRecordingAudio(context.Background(), "rec_normalize"); !errors.Is(err, readErr) {
		t.Fatalf("PrepareRecordingAudio error = %v, want object store read error", err)
	}
}

func TestRecordingProcessingActivitiesPrepareRecordingAudioReturnsProbeRunnerErrorWithoutPersisting(t *testing.T) {
	store := &normalizeRecordingStoreSpy{recordings: map[string]domain.Recording{"rec_normalize": {ID: "rec_normalize", AudioObjectKey: "recordings/rec_normalize/original.wav"}}}
	runnerErr := errors.New("ffprobe failed")
	activities := &RecordingProcessingActivities{store: store, normalizedAudioStore: store, objectStore: &normalizeObjectStoreSpy{objects: map[string]string{"recordings/rec_normalize/original.wav": "original-audio"}}, probeRunner: &audioProbeRunnerSpy{err: runnerErr}, normalizeRunner: &audioNormalizeRunnerSpy{}}

	if err := activities.PrepareRecordingAudio(context.Background(), "rec_normalize"); !errors.Is(err, runnerErr) {
		t.Fatalf("PrepareRecordingAudio error = %v, want runner error", err)
	}
	if len(store.probes) != 0 {
		t.Fatalf("stored probes = %d, want 0 after probe error", len(store.probes))
	}
	if len(store.normalizedAudios) != 0 {
		t.Fatalf("normalized rows = %d, want 0 after probe error", len(store.normalizedAudios))
	}
}

func TestRecordingProcessingActivitiesPrepareRecordingAudioReturnsNormalizeRunnerErrorWithoutPersistingNormalizedAudio(t *testing.T) {
	store := &normalizeRecordingStoreSpy{recordings: map[string]domain.Recording{"rec_normalize": {ID: "rec_normalize", AudioObjectKey: "recordings/rec_normalize/original.wav"}}}
	runnerErr := errors.New("ffmpeg failed")
	activities := &RecordingProcessingActivities{store: store, normalizedAudioStore: store, objectStore: &normalizeObjectStoreSpy{objects: map[string]string{"recordings/rec_normalize/original.wav": "original-audio"}}, probeRunner: &audioProbeRunnerSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{err: runnerErr}}

	if err := activities.PrepareRecordingAudio(context.Background(), "rec_normalize"); !errors.Is(err, runnerErr) {
		t.Fatalf("PrepareRecordingAudio error = %v, want runner error", err)
	}
	if len(store.probes) != 1 {
		t.Fatalf("stored probes = %d, want 1 before normalize error", len(store.probes))
	}
	if len(store.normalizedAudios) != 0 {
		t.Fatalf("normalized rows = %d, want 0 after runner error", len(store.normalizedAudios))
	}
}
