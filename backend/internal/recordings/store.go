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
	Title        string
	WorkflowType domain.WorkflowType
	Language     string
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
		ID:           newRecordingID(),
		Title:        input.Title,
		Status:       domain.RecordingStatusUploaded,
		WorkflowType: input.WorkflowType,
		Language:     input.Language,
		CreatedAt:    now,
		UpdatedAt:    now,
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

func newRecordingID() string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("rec_%d", time.Now().UnixNano())
	}
	return "rec_" + hex.EncodeToString(bytes[:])
}
