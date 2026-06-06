package recordings

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
)

// CreateRecordingInput contains the metadata needed to create a recording skeleton.
type CreateRecordingInput struct {
	Title            string
	WorkflowType     domain.WorkflowType
	Language         string
	AudioObjectKey   string
	AudioContentType string
	AudioSizeBytes   int64
}

// UpdateRecordingStatusInput contains the state transition to persist for a recording.
type UpdateRecordingStatusInput struct {
	ID     string
	Status domain.RecordingStatus
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

// MemoryStore is a thread-safe in-memory recording store for local skeleton workflows.
type MemoryStore struct {
	mu          sync.RWMutex
	recordings  map[string]domain.Recording
	audioProbes map[string]RecordingAudioProbe
}

// NewMemoryStore creates an empty in-memory recording store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		recordings:  make(map[string]domain.Recording),
		audioProbes: make(map[string]RecordingAudioProbe),
	}
}

// Create stores a new recording with skeleton defaults.
func (s *MemoryStore) Create(input CreateRecordingInput) (domain.Recording, error) {
	if !domain.IsValidWorkflowType(string(input.WorkflowType)) {
		return domain.Recording{}, fmt.Errorf("invalid workflow type: %s", input.WorkflowType)
	}

	now := time.Now().UTC()
	recording := domain.Recording{
		ID:               newRecordingID(),
		Title:            input.Title,
		Status:           domain.RecordingStatusUploaded,
		WorkflowType:     input.WorkflowType,
		Language:         input.Language,
		AudioObjectKey:   input.AudioObjectKey,
		AudioContentType: input.AudioContentType,
		AudioSizeBytes:   input.AudioSizeBytes,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordings[recording.ID] = recording

	return recording, nil
}

// Get returns a recording by id.
func (s *MemoryStore) Get(id string) (domain.Recording, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	recording, ok := s.recordings[id]
	return recording, ok
}

// UpdateStatus updates a recording's processing status while preserving existing metadata.
func (s *MemoryStore) UpdateStatus(input UpdateRecordingStatusInput) (domain.Recording, error) {
	if err := validateStatusUpdateInput(input); err != nil {
		return domain.Recording{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	recording, ok := s.recordings[input.ID]
	if !ok {
		return domain.Recording{}, fmt.Errorf("recording not found: %s", input.ID)
	}
	recording.Status = input.Status
	recording.UpdatedAt = time.Now().UTC()
	s.recordings[input.ID] = recording

	return recording, nil
}

// UpsertAudioProbe stores or replaces ffprobe metadata for a recording.
func (s *MemoryStore) UpsertAudioProbe(input UpsertAudioProbeInput) (RecordingAudioProbe, error) {
	if err := validateAudioProbeInput(input); err != nil {
		return RecordingAudioProbe{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	probe := RecordingAudioProbe{
		RecordingID:     input.RecordingID,
		DurationSeconds: input.DurationSeconds,
		FormatName:      input.FormatName,
		CodecName:       input.CodecName,
		SampleRate:      input.SampleRate,
		Channels:        input.Channels,
		BitRate:         input.BitRate,
		RawProbeJSON:    append([]byte(nil), input.RawProbeJSON...),
		ProbedAt:        input.ProbedAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if existing, ok := s.audioProbes[input.RecordingID]; ok {
		probe.CreatedAt = existing.CreatedAt
	}
	s.audioProbes[input.RecordingID] = probe
	return probe, nil
}

// GetAudioProbe returns persisted ffprobe metadata by recording id.
func (s *MemoryStore) GetAudioProbe(recordingID string) (RecordingAudioProbe, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	probe, ok := s.audioProbes[recordingID]
	probe.RawProbeJSON = append([]byte(nil), probe.RawProbeJSON...)
	return probe, ok
}

func validateStatusUpdateInput(input UpdateRecordingStatusInput) error {
	if input.ID == "" {
		return fmt.Errorf("recording id is required")
	}
	switch input.Status {
	case domain.RecordingStatusProcessing, domain.RecordingStatusTranscribing, domain.RecordingStatusSummarizing, domain.RecordingStatusCompleted, domain.RecordingStatusFailed:
		return nil
	default:
		return fmt.Errorf("unsupported recording status update: %s", input.Status)
	}
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
