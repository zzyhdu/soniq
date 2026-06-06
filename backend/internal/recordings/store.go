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

// MemoryStore is a thread-safe in-memory recording store for local skeleton workflows.
type MemoryStore struct {
	mu         sync.RWMutex
	recordings map[string]domain.Recording
}

// NewMemoryStore creates an empty in-memory recording store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		recordings: make(map[string]domain.Recording),
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

func validateStatusUpdateInput(input UpdateRecordingStatusInput) error {
	if input.ID == "" {
		return fmt.Errorf("recording id is required")
	}
	switch input.Status {
	case domain.RecordingStatusProcessing, domain.RecordingStatusCompleted, domain.RecordingStatusFailed:
		return nil
	default:
		return fmt.Errorf("unsupported recording status update: %s", input.Status)
	}
}

func newRecordingID() string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("rec_%d", time.Now().UnixNano())
	}
	return "rec_" + hex.EncodeToString(bytes[:])
}
