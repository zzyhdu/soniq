package activities

import (
	"context"
	"errors"
	"strings"
	"testing"
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
