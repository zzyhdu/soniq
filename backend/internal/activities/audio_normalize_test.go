package activities

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
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
	return recordings.RecordingAudioProbe{RecordingID: input.RecordingID}, nil
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

type normalizePathResolverSpy struct {
	paths map[string]string
	keys  []string
	err   error
}

func (s *normalizePathResolverSpy) LocalPathForObject(key string) (string, error) {
	s.keys = append(s.keys, key)
	if s.err != nil {
		return "", s.err
	}
	if path, ok := s.paths[key]; ok {
		return path, nil
	}
	return "/tmp/soniq/" + key, nil
}

type audioNormalizeRunnerSpy struct {
	requests []AudioNormalizeRequest
	result   AudioNormalizeResult
	err      error
	write    []byte
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

func TestRecordingProcessingActivitiesNormalizeRecordingAudioPersistsMetadata(t *testing.T) {
	store := &normalizeRecordingStoreSpy{recordings: map[string]domain.Recording{
		"rec_normalize": {ID: "rec_normalize", AudioObjectKey: "recordings/rec_normalize/original.wav"},
	}}
	resolver := &normalizePathResolverSpy{paths: map[string]string{
		"recordings/rec_normalize/original.wav":   t.TempDir() + "/original.wav",
		"recordings/rec_normalize/normalized.wav": t.TempDir() + "/normalized.wav",
	}}
	runner := &audioNormalizeRunnerSpy{write: []byte("normalized-audio")}
	activities := &RecordingProcessingActivities{store: store, normalizedAudioStore: store, pathResolver: resolver, normalizeRunner: runner}

	if err := activities.NormalizeRecordingAudio(context.Background(), "rec_normalize"); err != nil {
		t.Fatalf("NormalizeRecordingAudio returned error: %v", err)
	}
	if len(resolver.keys) != 2 {
		t.Fatalf("resolved keys = %+v, want original and normalized keys", resolver.keys)
	}
	if resolver.keys[0] != "recordings/rec_normalize/original.wav" || resolver.keys[1] != "recordings/rec_normalize/normalized.wav" {
		t.Fatalf("resolved keys = %+v, want original then normalized object key", resolver.keys)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("normalize runner requests = %d, want 1", len(runner.requests))
	}
	if runner.requests[0].InputPath != resolver.paths["recordings/rec_normalize/original.wav"] || runner.requests[0].OutputPath != resolver.paths["recordings/rec_normalize/normalized.wav"] {
		t.Fatalf("normalize request = %+v, want resolved input/output paths", runner.requests[0])
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
}

func TestRecordingProcessingActivitiesNormalizeRecordingAudioRejectsMissingRecordingID(t *testing.T) {
	activities := &RecordingProcessingActivities{store: &normalizeRecordingStoreSpy{}, pathResolver: &normalizePathResolverSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}
	if err := activities.NormalizeRecordingAudio(context.Background(), ""); err == nil {
		t.Fatal("NormalizeRecordingAudio returned nil error, want missing recording id error")
	}
}

func TestRecordingProcessingActivitiesNormalizeRecordingAudioRequiresDependencies(t *testing.T) {
	store := &normalizeRecordingStoreSpy{recordings: map[string]domain.Recording{"rec_normalize": {ID: "rec_normalize", AudioObjectKey: "recordings/rec_normalize/original.wav"}}}
	tests := []struct {
		name       string
		activities *RecordingProcessingActivities
	}{
		{name: "store", activities: &RecordingProcessingActivities{normalizedAudioStore: store, pathResolver: &normalizePathResolverSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}},
		{name: "normalized store", activities: &RecordingProcessingActivities{store: store, pathResolver: &normalizePathResolverSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}},
		{name: "path resolver", activities: &RecordingProcessingActivities{store: store, normalizedAudioStore: store, normalizeRunner: &audioNormalizeRunnerSpy{}}},
		{name: "normalize runner", activities: &RecordingProcessingActivities{store: store, normalizedAudioStore: store, pathResolver: &normalizePathResolverSpy{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.activities.NormalizeRecordingAudio(context.Background(), "rec_normalize"); err == nil {
				t.Fatal("NormalizeRecordingAudio returned nil error, want missing dependency error")
			}
		})
	}
}

func TestRecordingProcessingActivitiesNormalizeRecordingAudioRequiresExistingRecording(t *testing.T) {
	activities := &RecordingProcessingActivities{store: &normalizeRecordingStoreSpy{}, normalizedAudioStore: &normalizeRecordingStoreSpy{}, pathResolver: &normalizePathResolverSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}
	if err := activities.NormalizeRecordingAudio(context.Background(), "rec_missing"); err == nil {
		t.Fatal("NormalizeRecordingAudio returned nil error, want missing recording error")
	}
}

func TestRecordingProcessingActivitiesNormalizeRecordingAudioRequiresAudioObjectKey(t *testing.T) {
	store := &normalizeRecordingStoreSpy{recordings: map[string]domain.Recording{"rec_normalize": {ID: "rec_normalize"}}}
	activities := &RecordingProcessingActivities{store: store, normalizedAudioStore: store, pathResolver: &normalizePathResolverSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{}}
	if err := activities.NormalizeRecordingAudio(context.Background(), "rec_normalize"); err == nil {
		t.Fatal("NormalizeRecordingAudio returned nil error, want missing audio object key error")
	}
}

func TestRecordingProcessingActivitiesNormalizeRecordingAudioReturnsRunnerErrorWithoutPersisting(t *testing.T) {
	store := &normalizeRecordingStoreSpy{recordings: map[string]domain.Recording{"rec_normalize": {ID: "rec_normalize", AudioObjectKey: "recordings/rec_normalize/original.wav"}}}
	runnerErr := errors.New("ffmpeg failed")
	activities := &RecordingProcessingActivities{store: store, normalizedAudioStore: store, pathResolver: &normalizePathResolverSpy{}, normalizeRunner: &audioNormalizeRunnerSpy{err: runnerErr}}

	if err := activities.NormalizeRecordingAudio(context.Background(), "rec_normalize"); !errors.Is(err, runnerErr) {
		t.Fatalf("NormalizeRecordingAudio error = %v, want runner error", err)
	}
	if len(store.normalizedAudios) != 0 {
		t.Fatalf("normalized rows = %d, want 0 after runner error", len(store.normalizedAudios))
	}
}
