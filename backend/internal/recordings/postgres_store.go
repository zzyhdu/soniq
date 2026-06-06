package recordings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
)

// PostgresRow is the subset of database row behavior used by PostgresStore.
type PostgresRow interface {
	Scan(dest ...any) error
}

// PostgresExecutor is the subset of database behavior used by PostgresStore.
type PostgresExecutor interface {
	QueryRow(ctx context.Context, query string, args ...any) interface{ Scan(dest ...any) error }
}

// PostgresStore persists recordings in a Postgres recordings table.
type PostgresStore struct {
	db PostgresExecutor
}

// NewPostgresStore creates a Postgres-backed recording store.
func NewPostgresStore(db PostgresExecutor) *PostgresStore {
	return &PostgresStore{db: db}
}

// Create inserts a new recording with skeleton defaults and returns the persisted row.
func (s *PostgresStore) Create(input CreateRecordingInput) (domain.Recording, error) {
	if !domain.IsValidWorkflowType(string(input.WorkflowType)) {
		return domain.Recording{}, fmt.Errorf("invalid workflow type: %s", input.WorkflowType)
	}
	if s == nil || s.db == nil {
		return domain.Recording{}, fmt.Errorf("postgres recording store requires database executor")
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

	row := s.db.QueryRow(
		context.Background(),
		`INSERT INTO recordings (id, title, status, workflow_type, language, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, title, status, workflow_type, language, created_at, updated_at`,
		recording.ID,
		recording.Title,
		recording.Status,
		recording.WorkflowType,
		recording.Language,
		recording.CreatedAt,
		recording.UpdatedAt,
	)
	if err := scanRecording(row, &recording); err != nil {
		return domain.Recording{}, fmt.Errorf("insert recording: %w", err)
	}
	return recording, nil
}

// Get returns a recording by id.
func (s *PostgresStore) Get(id string) (domain.Recording, bool) {
	if s == nil || s.db == nil {
		return domain.Recording{}, false
	}

	var recording domain.Recording
	row := s.db.QueryRow(
		context.Background(),
		`SELECT id, title, status, workflow_type, language, created_at, updated_at
FROM recordings
WHERE id = $1`,
		id,
	)
	if err := scanRecording(row, &recording); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Recording{}, false
		}
		return domain.Recording{}, false
	}
	return recording, true
}

func scanRecording(row PostgresRow, recording *domain.Recording) error {
	return row.Scan(
		&recording.ID,
		&recording.Title,
		&recording.Status,
		&recording.WorkflowType,
		&recording.Language,
		&recording.CreatedAt,
		&recording.UpdatedAt,
	)
}
