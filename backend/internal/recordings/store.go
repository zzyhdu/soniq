package recordings

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
)

// CreateRecordingInput contains the metadata needed to create a recording skeleton.
type CreateRecordingInput struct {
	WorkspaceID      string
	Title            string
	WorkflowType     domain.WorkflowType
	Language         string
	AudioObjectKey   string
	AudioContentType string
	AudioSizeBytes   int64
}

// GetRecordingInput identifies a recording within a workspace.
type GetRecordingInput struct {
	WorkspaceID string
	ID          string
}

// ListRecordingsInput contains filters for listing recordings in a workspace.
type ListRecordingsInput struct {
	WorkspaceID string
	Limit       int
}

// ListDeletedRecordingsInput contains filters for listing soft-deleted recordings in a workspace.
type ListDeletedRecordingsInput struct {
	WorkspaceID string
	Limit       int
}

// UpdateRecordingInput contains editable recording metadata.
type UpdateRecordingInput struct {
	WorkspaceID string
	ID          string
	Title       string
}

// UpdateRecordingStatusInput contains the state transition to persist for a recording.
type UpdateRecordingStatusInput struct {
	WorkspaceID   string
	ID            string
	Status        domain.RecordingStatus
	FailureReason string
}

// RetryRecordingInput identifies a failed recording that should be prepared for retry.
type RetryRecordingInput struct {
	WorkspaceID string
	ID          string
}

// SoftDeleteRecordingInput identifies an active recording to move to Trash.
type SoftDeleteRecordingInput struct {
	WorkspaceID     string
	ID              string
	DeletedByUserID string
}

// RestoreRecordingInput identifies a soft-deleted recording to restore.
type RestoreRecordingInput struct {
	WorkspaceID string
	ID          string
}

// PurgeRecordingInput identifies a soft-deleted recording to permanently purge.
type PurgeRecordingInput struct {
	WorkspaceID string
	ID          string
}

// RecordingPurgeArtifactStatus is the cleanup state for a purged recording artifact.
type RecordingPurgeArtifactStatus string

const (
	RecordingPurgeArtifactStatusPending  RecordingPurgeArtifactStatus = "pending"
	RecordingPurgeArtifactStatusDeleting RecordingPurgeArtifactStatus = "deleting"
	RecordingPurgeArtifactStatusDeleted  RecordingPurgeArtifactStatus = "deleted"
	RecordingPurgeArtifactStatusFailed   RecordingPurgeArtifactStatus = "failed"
)

const (
	RecordingPurgeArtifactKindOriginalAudio   = "original_audio"
	RecordingPurgeArtifactKindNormalizedAudio = "normalized_audio"
)

// RecordingPurgeArtifact tracks object-storage cleanup after a recording purge.
type RecordingPurgeArtifact struct {
	ID            string
	RecordingID   string
	WorkspaceID   string
	ObjectKey     string
	ArtifactKind  string
	Status        RecordingPurgeArtifactStatus
	AttemptCount  int
	NextAttemptAt time.Time
	LastError     string
	DeletedAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PurgeRecordingResult contains artifact cleanup rows created by a purge.
type PurgeRecordingResult struct {
	Artifacts []RecordingPurgeArtifact
}

// ClaimPurgeArtifactsInput controls a cleanup worker claim batch.
type ClaimPurgeArtifactsInput struct {
	Limit int
}

// MarkPurgeArtifactDeletedInput identifies an artifact cleanup row that succeeded.
type MarkPurgeArtifactDeletedInput struct {
	ID string
}

// MarkPurgeArtifactFailedInput records a failed artifact cleanup attempt.
type MarkPurgeArtifactFailedInput struct {
	ID            string
	LastError     string
	NextAttemptAt time.Time
}

// RecordingAudioProbe contains ffprobe metadata for a recording's original audio.
type RecordingAudioProbe struct {
	RecordingID     string
	DurationSeconds float64
	FormatName      string
	CodecName       string
	SampleRate      int
	Channels        int
	BitRate         int
	RawProbeJSON    []byte
	ProbedAt        time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UpsertAudioProbeInput contains audio probe metadata to persist for a recording.
type UpsertAudioProbeInput struct {
	RecordingID     string
	DurationSeconds float64
	FormatName      string
	CodecName       string
	SampleRate      int
	Channels        int
	BitRate         int
	RawProbeJSON    []byte
	ProbedAt        time.Time
}

// RecordingNormalizedAudio contains metadata for a recording's normalized audio artifact.
type RecordingNormalizedAudio struct {
	RecordingID     string
	ObjectKey       string
	ContentType     string
	SizeBytes       int64
	FormatName      string
	CodecName       string
	SampleRate      int
	Channels        int
	DurationSeconds float64
	NormalizedAt    time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UpsertNormalizedAudioInput contains normalized audio metadata to persist for a recording.
type UpsertNormalizedAudioInput struct {
	RecordingID     string
	ObjectKey       string
	ContentType     string
	SizeBytes       int64
	FormatName      string
	CodecName       string
	SampleRate      int
	Channels        int
	DurationSeconds float64
	NormalizedAt    time.Time
}

// RecordingTranscript contains the latest transcript for a recording.
type RecordingTranscript struct {
	RecordingID   string
	Provider      string
	Model         string
	Language      string
	Text          string
	RawResultJSON []byte
	TranscribedAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// RecordingTranscriptSegment contains one transcript segment for a recording.
type RecordingTranscriptSegment struct {
	ID           string
	RecordingID  string
	SegmentIndex int
	StartMS      int
	EndMS        int
	SpeakerLabel string
	Text         string
	Confidence   float64
	CreatedAt    time.Time
}

// UpsertTranscriptSegmentInput contains a transcript segment to persist.
type UpsertTranscriptSegmentInput struct {
	SegmentIndex int
	StartMS      int
	EndMS        int
	SpeakerLabel string
	Text         string
	Confidence   float64
}

// UpsertTranscriptInput contains transcript data to persist for a recording.
type UpsertTranscriptInput struct {
	RecordingID   string
	Provider      string
	Model         string
	Language      string
	Text          string
	RawResultJSON []byte
	TranscribedAt time.Time
	Segments      []UpsertTranscriptSegmentInput
}

// RecordingSummary contains the latest summary for a recording.
type RecordingSummary struct {
	RecordingID     string
	Provider        string
	Model           string
	Type            domain.WorkflowType
	Title           string
	Overview        string
	ContentMarkdown string
	RawResultJSON   []byte
	SummarizedAt    time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UpsertSummaryInput contains summary data to persist for a recording.
type UpsertSummaryInput struct {
	RecordingID     string
	Provider        string
	Model           string
	Type            domain.WorkflowType
	Title           string
	Overview        string
	ContentMarkdown string
	RawResultJSON   []byte
	SummarizedAt    time.Time
}

// RecordingMindMap contains the latest mind map for a recording.
type RecordingMindMap struct {
	RecordingID     string
	Provider        string
	Model           string
	Title           string
	RootJSON        []byte
	ContentMarkdown string
	RawResultJSON   []byte
	GeneratedAt     time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UpsertMindMapInput contains mind map data to persist for a recording.
type UpsertMindMapInput struct {
	RecordingID     string
	Provider        string
	Model           string
	Title           string
	RootJSON        []byte
	ContentMarkdown string
	RawResultJSON   []byte
	GeneratedAt     time.Time
}

func validateStatusUpdateInput(input UpdateRecordingStatusInput) error {
	if input.ID == "" {
		return fmt.Errorf("recording id is required")
	}
	switch input.Status {
	case domain.RecordingStatusUploaded, domain.RecordingStatusProcessing, domain.RecordingStatusTranscribing, domain.RecordingStatusSummarizing, domain.RecordingStatusCompleted, domain.RecordingStatusFailed:
		return nil
	default:
		return fmt.Errorf("unsupported recording status update: %s", input.Status)
	}
}

func validateRetryRecordingInput(input RetryRecordingInput) error {
	if input.WorkspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if input.ID == "" {
		return fmt.Errorf("recording id is required")
	}
	return nil
}

func validateUpdateRecordingInput(input UpdateRecordingInput) error {
	if input.WorkspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if input.ID == "" {
		return fmt.Errorf("recording id is required")
	}
	if strings.TrimSpace(input.Title) == "" {
		return fmt.Errorf("title is required")
	}
	return nil
}

func validateSoftDeleteRecordingInput(input SoftDeleteRecordingInput) error {
	if input.WorkspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if input.ID == "" {
		return fmt.Errorf("recording id is required")
	}
	if input.DeletedByUserID == "" {
		return fmt.Errorf("deleted by user id is required")
	}
	return nil
}

func validateListDeletedRecordingsInput(input ListDeletedRecordingsInput) error {
	if input.WorkspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if input.Limit < 0 {
		return fmt.Errorf("recording trash limit must not be negative")
	}
	return nil
}

func validateRestoreRecordingInput(input RestoreRecordingInput) error {
	if input.WorkspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if input.ID == "" {
		return fmt.Errorf("recording id is required")
	}
	return nil
}

func validatePurgeRecordingInput(input PurgeRecordingInput) error {
	if input.WorkspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if input.ID == "" {
		return fmt.Errorf("recording id is required")
	}
	return nil
}

func validateClaimPurgeArtifactsInput(input ClaimPurgeArtifactsInput) error {
	if input.Limit < 0 {
		return fmt.Errorf("purge artifact claim limit must not be negative")
	}
	return nil
}

func validateMarkPurgeArtifactDeletedInput(input MarkPurgeArtifactDeletedInput) error {
	if input.ID == "" {
		return fmt.Errorf("purge artifact id is required")
	}
	return nil
}

func validateMarkPurgeArtifactFailedInput(input MarkPurgeArtifactFailedInput) error {
	if input.ID == "" {
		return fmt.Errorf("purge artifact id is required")
	}
	if input.NextAttemptAt.IsZero() {
		return fmt.Errorf("next attempt timestamp is required")
	}
	return nil
}

func validateCreateRecordingInput(input CreateRecordingInput) error {
	if input.WorkspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if !domain.IsValidWorkflowType(string(input.WorkflowType)) {
		return fmt.Errorf("invalid workflow type: %s", input.WorkflowType)
	}
	if input.AudioSizeBytes < 0 {
		return fmt.Errorf("audio size must not be negative")
	}
	return nil
}

func validateGetRecordingInput(input GetRecordingInput) error {
	if input.WorkspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if input.ID == "" {
		return fmt.Errorf("recording id is required")
	}
	return nil
}

func validateListRecordingsInput(input ListRecordingsInput) error {
	if input.WorkspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if input.Limit < 0 {
		return fmt.Errorf("recording list limit must not be negative")
	}
	return nil
}

func validateAudioProbeInput(input UpsertAudioProbeInput) error {
	if input.RecordingID == "" {
		return fmt.Errorf("recording id is required")
	}
	if input.FormatName == "" {
		return fmt.Errorf("audio probe format name is required")
	}
	if len(input.RawProbeJSON) == 0 {
		return fmt.Errorf("audio probe raw json is required")
	}
	if input.ProbedAt.IsZero() {
		return fmt.Errorf("audio probe timestamp is required")
	}
	return nil
}

func validateNormalizedAudioInput(input UpsertNormalizedAudioInput) error {
	if input.RecordingID == "" {
		return fmt.Errorf("recording id is required")
	}
	if input.ObjectKey == "" {
		return fmt.Errorf("normalized audio object key is required")
	}
	if input.ContentType == "" {
		return fmt.Errorf("normalized audio content type is required")
	}
	if input.SizeBytes <= 0 {
		return fmt.Errorf("normalized audio size must be positive")
	}
	if input.FormatName == "" {
		return fmt.Errorf("normalized audio format name is required")
	}
	if input.CodecName == "" {
		return fmt.Errorf("normalized audio codec name is required")
	}
	if input.SampleRate <= 0 {
		return fmt.Errorf("normalized audio sample rate must be positive")
	}
	if input.Channels <= 0 {
		return fmt.Errorf("normalized audio channels must be positive")
	}
	if input.NormalizedAt.IsZero() {
		return fmt.Errorf("normalized audio timestamp is required")
	}
	return nil
}

func validateTranscriptInput(input UpsertTranscriptInput) error {
	if input.RecordingID == "" {
		return fmt.Errorf("recording id is required")
	}
	if input.Provider == "" {
		return fmt.Errorf("transcript provider is required")
	}
	if input.Text == "" {
		return fmt.Errorf("transcript text is required")
	}
	if len(input.RawResultJSON) == 0 {
		return fmt.Errorf("transcript raw json is required")
	}
	if input.TranscribedAt.IsZero() {
		return fmt.Errorf("transcript timestamp is required")
	}
	return nil
}

func validateSummaryInput(input UpsertSummaryInput) error {
	if input.RecordingID == "" {
		return fmt.Errorf("recording id is required")
	}
	if input.Provider == "" {
		return fmt.Errorf("summary provider is required")
	}
	if !domain.IsValidWorkflowType(string(input.Type)) {
		return fmt.Errorf("summary type is invalid: %s", input.Type)
	}
	if input.Overview == "" {
		return fmt.Errorf("summary overview is required")
	}
	if input.ContentMarkdown == "" {
		return fmt.Errorf("summary markdown is required")
	}
	if len(input.RawResultJSON) == 0 {
		return fmt.Errorf("summary raw json is required")
	}
	if input.SummarizedAt.IsZero() {
		return fmt.Errorf("summary timestamp is required")
	}
	return nil
}

func validateMindMapInput(input UpsertMindMapInput) error {
	if input.RecordingID == "" {
		return fmt.Errorf("recording id is required")
	}
	if input.Provider == "" {
		return fmt.Errorf("mind map provider is required")
	}
	if len(input.RootJSON) == 0 {
		return fmt.Errorf("mind map root json is required")
	}
	if input.ContentMarkdown == "" {
		return fmt.Errorf("mind map markdown is required")
	}
	if len(input.RawResultJSON) == 0 {
		return fmt.Errorf("mind map raw json is required")
	}
	if input.GeneratedAt.IsZero() {
		return fmt.Errorf("mind map timestamp is required")
	}
	return nil
}

func transcriptSegmentID(recordingID string, index int) string {
	return fmt.Sprintf("%s-seg-%06d", recordingID, index)
}

func newRecordingID() string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("rec_%d", time.Now().UnixNano())
	}
	return "rec_" + hex.EncodeToString(bytes[:])
}

func purgeArtifactID(recordingID string, objectKey string) string {
	sum := sha256.Sum256([]byte(recordingID + "\x00" + objectKey))
	return "rpa_" + hex.EncodeToString(sum[:8])
}
