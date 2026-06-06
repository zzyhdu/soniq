package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
)

// RecordingProcessingInput is the input shared by the recording processing workflow and activity stubs.
type RecordingProcessingInput struct {
	RecordingID  string
	WorkflowType domain.WorkflowType
	Language     string
}

// RecordingProcessingResult is the skeleton result returned after processing completes.
type RecordingProcessingResult struct {
	RecordingID string
	Status      domain.RecordingStatus
}

// RecordingStore is the persistence seam used by recording processing activities.
type RecordingStore interface {
	Get(id string) (domain.Recording, bool)
	UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error)
	UpsertAudioProbe(input recordings.UpsertAudioProbeInput) (recordings.RecordingAudioProbe, error)
}

// LocalObjectPathResolver resolves stored object keys to local filesystem paths.
type LocalObjectPathResolver interface {
	LocalPathForObject(key string) (string, error)
}

// AudioProbeRunner probes an audio file and returns normalized metadata.
type AudioProbeRunner interface {
	Probe(ctx context.Context, path string) (AudioProbeResult, error)
}

// AudioProbeResult contains normalized ffprobe metadata for persistence.
type AudioProbeResult struct {
	DurationSeconds float64
	FormatName      string
	CodecName       string
	SampleRate      int
	Channels        int
	BitRate         int
	RawProbeJSON    []byte
	ProbedAt        time.Time
}

// FFProbeRunner runs the ffprobe binary.
type FFProbeRunner struct {
	Binary string
}

// RecordingProcessingActivities contains store-backed Temporal activity methods.
type RecordingProcessingActivities struct {
	store        RecordingStore
	pathResolver LocalObjectPathResolver
	probeRunner  AudioProbeRunner
}

// NewRecordingProcessingActivities creates store-backed recording processing activities.
func NewRecordingProcessingActivities(store RecordingStore) *RecordingProcessingActivities {
	return &RecordingProcessingActivities{store: store}
}

// NewRecordingProcessingActivitiesWithAudioProbe creates recording processing activities with audio probe dependencies.
func NewRecordingProcessingActivitiesWithAudioProbe(store RecordingStore, resolver LocalObjectPathResolver, runner AudioProbeRunner) *RecordingProcessingActivities {
	return &RecordingProcessingActivities{store: store, pathResolver: resolver, probeRunner: runner}
}

// ValidateRecordingActivity validates the minimal recording processing input.
//
// This package-level function is the current stateless Temporal activity used
// by the existing workflow/worker wiring. The store-backed
// RecordingProcessingActivities methods below are the next wiring target.
func ValidateRecordingActivity(ctx context.Context, input RecordingProcessingInput) error {
	if input.RecordingID == "" {
		return errors.New("recording id is required")
	}
	if !domain.IsValidWorkflowType(string(input.WorkflowType)) {
		return errors.New("workflow type is invalid")
	}
	return nil
}

// MarkRecordingProcessingActivity is the current stateless compatibility activity.
// Store-backed status persistence lives in RecordingProcessingActivities.MarkRecordingProcessing.
func MarkRecordingProcessingActivity(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	return nil
}

// CompleteRecordingProcessingActivity is the current stateless compatibility activity.
// Store-backed status persistence lives in RecordingProcessingActivities.CompleteRecordingProcessing.
func CompleteRecordingProcessingActivity(ctx context.Context, recordingID string) (RecordingProcessingResult, error) {
	if recordingID == "" {
		return RecordingProcessingResult{}, errors.New("recording id is required")
	}
	return RecordingProcessingResult{
		RecordingID: recordingID,
		Status:      domain.RecordingStatusCompleted,
	}, nil
}

// FailRecordingProcessingActivity is the current stateless compatibility activity.
// Store-backed status persistence lives in RecordingProcessingActivities.FailRecordingProcessing.
func FailRecordingProcessingActivity(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	return nil
}

// ProbeRecordingAudioActivity is the current stateless compatibility activity.
// Store-backed audio probing lives in RecordingProcessingActivities.ProbeRecordingAudio.
func ProbeRecordingAudioActivity(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	return nil
}

// ValidateRecording validates processing input and confirms the recording exists.
func (a *RecordingProcessingActivities) ValidateRecording(ctx context.Context, input RecordingProcessingInput) error {
	if err := ValidateRecordingActivity(ctx, input); err != nil {
		return err
	}
	if a == nil || a.store == nil {
		return errors.New("recording store is required")
	}
	if _, ok := a.store.Get(input.RecordingID); !ok {
		return fmt.Errorf("recording not found: %s", input.RecordingID)
	}
	return nil
}

// MarkRecordingProcessing persists the processing status transition.
func (a *RecordingProcessingActivities) MarkRecordingProcessing(ctx context.Context, recordingID string) error {
	_, err := a.updateStatus(recordingID, domain.RecordingStatusProcessing)
	return err
}

// CompleteRecordingProcessing persists completion and returns the workflow result.
func (a *RecordingProcessingActivities) CompleteRecordingProcessing(ctx context.Context, recordingID string) (RecordingProcessingResult, error) {
	updated, err := a.updateStatus(recordingID, domain.RecordingStatusCompleted)
	if err != nil {
		return RecordingProcessingResult{}, err
	}
	return RecordingProcessingResult{
		RecordingID: updated.ID,
		Status:      updated.Status,
	}, nil
}

// FailRecordingProcessing persists a failed status transition.
func (a *RecordingProcessingActivities) FailRecordingProcessing(ctx context.Context, recordingID string) error {
	_, err := a.updateStatus(recordingID, domain.RecordingStatusFailed)
	return err
}

// ProbeRecordingAudio probes the original uploaded audio and persists ffprobe metadata.
func (a *RecordingProcessingActivities) ProbeRecordingAudio(ctx context.Context, recordingID string) error {
	if recordingID == "" {
		return errors.New("recording id is required")
	}
	if a == nil || a.store == nil {
		return errors.New("recording store is required")
	}
	if a.pathResolver == nil {
		return errors.New("audio object path resolver is required")
	}
	if a.probeRunner == nil {
		return errors.New("audio probe runner is required")
	}

	recording, ok := a.store.Get(recordingID)
	if !ok {
		return fmt.Errorf("recording not found: %s", recordingID)
	}
	if strings.TrimSpace(recording.AudioObjectKey) == "" {
		return fmt.Errorf("recording audio object key is required: %s", recordingID)
	}

	path, err := a.pathResolver.LocalPathForObject(recording.AudioObjectKey)
	if err != nil {
		return fmt.Errorf("resolve recording audio object path: %w", err)
	}
	probe, err := a.probeRunner.Probe(ctx, path)
	if err != nil {
		return fmt.Errorf("probe recording audio: %w", err)
	}
	_, err = a.store.UpsertAudioProbe(recordings.UpsertAudioProbeInput{
		RecordingID:     recordingID,
		DurationSeconds: probe.DurationSeconds,
		FormatName:      probe.FormatName,
		CodecName:       probe.CodecName,
		SampleRate:      probe.SampleRate,
		Channels:        probe.Channels,
		BitRate:         probe.BitRate,
		RawProbeJSON:    append([]byte(nil), probe.RawProbeJSON...),
		ProbedAt:        probe.ProbedAt,
	})
	if err != nil {
		return fmt.Errorf("persist recording audio probe: %w", err)
	}
	return nil
}

func (r FFProbeRunner) Probe(ctx context.Context, path string) (AudioProbeResult, error) {
	binary := strings.TrimSpace(r.Binary)
	if binary == "" {
		binary = "ffprobe"
	}
	output, err := exec.CommandContext(ctx, binary, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", path).Output()
	if err != nil {
		return AudioProbeResult{}, fmt.Errorf("run ffprobe: %w", err)
	}
	return parseFFProbeOutput(output, time.Now().UTC())
}

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecType  string `json:"codec_type"`
	CodecName  string `json:"codec_name"`
	SampleRate string `json:"sample_rate"`
	Channels   int    `json:"channels"`
	BitRate    string `json:"bit_rate"`
}

type ffprobeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`
}

func parseFFProbeOutput(raw []byte, probedAt time.Time) (AudioProbeResult, error) {
	var parsed ffprobeOutput
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return AudioProbeResult{}, fmt.Errorf("parse ffprobe json: %w", err)
	}

	result := AudioProbeResult{
		FormatName:   parsed.Format.FormatName,
		BitRate:      parseIntOrZero(parsed.Format.BitRate),
		RawProbeJSON: append([]byte(nil), raw...),
		ProbedAt:     probedAt,
	}
	if parsed.Format.Duration != "" {
		duration, err := strconv.ParseFloat(parsed.Format.Duration, 64)
		if err != nil {
			return AudioProbeResult{}, fmt.Errorf("parse ffprobe duration: %w", err)
		}
		result.DurationSeconds = duration
	}

	for _, stream := range parsed.Streams {
		if stream.CodecType != "audio" {
			continue
		}
		result.CodecName = stream.CodecName
		result.SampleRate = parseIntOrZero(stream.SampleRate)
		result.Channels = stream.Channels
		if stream.BitRate != "" {
			result.BitRate = parseIntOrZero(stream.BitRate)
		}
		break
	}
	if strings.TrimSpace(result.FormatName) == "" {
		return AudioProbeResult{}, errors.New("ffprobe format_name is required")
	}
	return result, nil
}

func parseIntOrZero(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func (a *RecordingProcessingActivities) updateStatus(recordingID string, status domain.RecordingStatus) (domain.Recording, error) {
	if recordingID == "" {
		return domain.Recording{}, errors.New("recording id is required")
	}
	if a == nil || a.store == nil {
		return domain.Recording{}, errors.New("recording store is required")
	}
	updated, err := a.store.UpdateStatus(recordings.UpdateRecordingStatusInput{
		ID:     recordingID,
		Status: status,
	})
	if err != nil {
		return domain.Recording{}, err
	}
	return updated, nil
}
