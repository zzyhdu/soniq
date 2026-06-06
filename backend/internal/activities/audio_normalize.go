package activities

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	normalizedAudioContentType = "audio/wav"
	normalizedAudioFormatName  = "wav"
	normalizedAudioCodecName   = "pcm_s16le"
	normalizedAudioSampleRate  = 16000
	normalizedAudioChannels    = 1
)

// AudioNormalizeRunner normalizes an audio file into Soniq's stable local ASR input target.
type AudioNormalizeRunner interface {
	Normalize(ctx context.Context, input AudioNormalizeRequest) (AudioNormalizeResult, error)
}

// AudioNormalizeRequest contains local filesystem paths for audio normalization.
type AudioNormalizeRequest struct {
	InputPath  string
	OutputPath string
}

// AudioNormalizeResult contains the normalized audio target metadata.
type AudioNormalizeResult struct {
	OutputPath   string
	ContentType  string
	FormatName   string
	CodecName    string
	SampleRate   int
	Channels     int
	NormalizedAt time.Time
}

// CommandRunner is the command execution seam used by FFmpegNormalizeRunner.
type CommandRunner interface {
	Run(ctx context.Context, binary string, args ...string) ([]byte, error)
}

// ExecCommandRunner runs local commands with exec.CommandContext.
type ExecCommandRunner struct{}

// Run executes a command and returns combined stdout/stderr output.
func (ExecCommandRunner) Run(ctx context.Context, binary string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, binary, args...).CombinedOutput()
}

// FFmpegNormalizeRunner runs ffmpeg to produce Soniq's normalized WAV/PCM target.
type FFmpegNormalizeRunner struct {
	Binary        string
	CommandRunner CommandRunner
}

// Normalize writes a normalized WAV/PCM copy of the input audio to the output path.
func (r FFmpegNormalizeRunner) Normalize(ctx context.Context, input AudioNormalizeRequest) (AudioNormalizeResult, error) {
	inputPath := strings.TrimSpace(input.InputPath)
	if inputPath == "" {
		return AudioNormalizeResult{}, errors.New("normalize input path is required")
	}
	outputPath := strings.TrimSpace(input.OutputPath)
	if outputPath == "" {
		return AudioNormalizeResult{}, errors.New("normalize output path is required")
	}

	binary := strings.TrimSpace(r.Binary)
	if binary == "" {
		binary = "ffmpeg"
	}
	commandRunner := r.CommandRunner
	if commandRunner == nil {
		commandRunner = ExecCommandRunner{}
	}

	args := []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputPath,
		"-ac", fmt.Sprint(normalizedAudioChannels),
		"-ar", fmt.Sprint(normalizedAudioSampleRate),
		"-c:a", normalizedAudioCodecName,
		outputPath,
	}
	output, err := commandRunner.Run(ctx, binary, args...)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return AudioNormalizeResult{}, fmt.Errorf("run ffmpeg: %w", err)
		}
		return AudioNormalizeResult{}, fmt.Errorf("run ffmpeg: %w: %s", err, message)
	}

	return AudioNormalizeResult{
		OutputPath:   outputPath,
		ContentType:  normalizedAudioContentType,
		FormatName:   normalizedAudioFormatName,
		CodecName:    normalizedAudioCodecName,
		SampleRate:   normalizedAudioSampleRate,
		Channels:     normalizedAudioChannels,
		NormalizedAt: time.Now().UTC(),
	}, nil
}
