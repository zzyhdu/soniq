package activities

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestFFProbeRunnerReturnsClearErrorForMissingAudioFile(t *testing.T) {
	runner := FFProbeRunner{Binary: "ffprobe"}
	missingPath := filepath.Join(t.TempDir(), "missing.wav")

	_, err := runner.Probe(context.Background(), missingPath)

	if err == nil || !strings.Contains(err.Error(), "audio file not found") {
		t.Fatalf("Probe error = %v, want missing file error", err)
	}
}

func TestFFProbeRunnerIncludesCommandOutputOnFailure(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "audio.wav")
	if err := os.WriteFile(audioPath, []byte("not audio"), 0o644); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}
	ffprobePath := filepath.Join(dir, "ffprobe")
	if err := os.WriteFile(ffprobePath, []byte("#!/bin/sh\necho invalid audio >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write ffprobe fixture: %v", err)
	}
	runner := FFProbeRunner{Binary: ffprobePath}

	_, err := runner.Probe(context.Background(), audioPath)

	if err == nil || !strings.Contains(err.Error(), "invalid audio") {
		t.Fatalf("Probe error = %v, want ffprobe output context", err)
	}
}
