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

	row := s.db.QueryRow(
		context.Background(),
		`INSERT INTO recordings (id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, created_at, updated_at`,
		recording.ID,
		recording.Title,
		recording.Status,
		recording.WorkflowType,
		recording.Language,
		recording.AudioObjectKey,
		recording.AudioContentType,
		recording.AudioSizeBytes,
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
		`SELECT id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, created_at, updated_at
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

// UpdateStatus persists a recording status transition and returns the updated row.
func (s *PostgresStore) UpdateStatus(input UpdateRecordingStatusInput) (domain.Recording, error) {
	if err := validateStatusUpdateInput(input); err != nil {
		return domain.Recording{}, err
	}
	if s == nil || s.db == nil {
		return domain.Recording{}, fmt.Errorf("postgres recording store requires database executor")
	}

	updatedAt := time.Now().UTC()
	var recording domain.Recording
	row := s.db.QueryRow(
		context.Background(),
		`UPDATE recordings
SET status = $2, updated_at = $3
WHERE id = $1
RETURNING id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, created_at, updated_at`,
		input.ID,
		input.Status,
		updatedAt,
	)
	if err := scanRecording(row, &recording); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Recording{}, fmt.Errorf("recording not found: %s", input.ID)
		}
		return domain.Recording{}, fmt.Errorf("update recording status: %w", err)
	}
	return recording, nil
}

func scanRecording(row PostgresRow, recording *domain.Recording) error {
	return row.Scan(
		&recording.ID,
		&recording.Title,
		&recording.Status,
		&recording.WorkflowType,
		&recording.Language,
		&recording.AudioObjectKey,
		&recording.AudioContentType,
		&recording.AudioSizeBytes,
		&recording.CreatedAt,
		&recording.UpdatedAt,
	)
}

// UpsertAudioProbe stores or replaces ffprobe metadata for a recording.
func (s *PostgresStore) UpsertAudioProbe(input UpsertAudioProbeInput) (RecordingAudioProbe, error) {
	if err := validateAudioProbeInput(input); err != nil {
		return RecordingAudioProbe{}, err
	}
	if s == nil || s.db == nil {
		return RecordingAudioProbe{}, fmt.Errorf("postgres recording store requires database executor")
	}

	now := time.Now().UTC()
	var probe RecordingAudioProbe
	row := s.db.QueryRow(
		context.Background(),
		`INSERT INTO recording_audio_probes (recording_id, duration_seconds, format_name, codec_name, sample_rate, channels, bit_rate, raw_probe_json, probed_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (recording_id) DO UPDATE
SET duration_seconds = EXCLUDED.duration_seconds,
    format_name = EXCLUDED.format_name,
    codec_name = EXCLUDED.codec_name,
    sample_rate = EXCLUDED.sample_rate,
    channels = EXCLUDED.channels,
    bit_rate = EXCLUDED.bit_rate,
    raw_probe_json = EXCLUDED.raw_probe_json,
    probed_at = EXCLUDED.probed_at,
    updated_at = EXCLUDED.updated_at
RETURNING recording_id, duration_seconds, format_name, codec_name, sample_rate, channels, bit_rate, raw_probe_json, probed_at, created_at, updated_at`,
		input.RecordingID,
		input.DurationSeconds,
		input.FormatName,
		input.CodecName,
		input.SampleRate,
		input.Channels,
		input.BitRate,
		append([]byte(nil), input.RawProbeJSON...),
		input.ProbedAt,
		now,
		now,
	)
	if err := scanAudioProbe(row, &probe); err != nil {
		return RecordingAudioProbe{}, fmt.Errorf("upsert recording audio probe: %w", err)
	}
	return probe, nil
}

// GetAudioProbe returns persisted ffprobe metadata by recording id.
func (s *PostgresStore) GetAudioProbe(recordingID string) (RecordingAudioProbe, bool) {
	if s == nil || s.db == nil {
		return RecordingAudioProbe{}, false
	}

	var probe RecordingAudioProbe
	row := s.db.QueryRow(
		context.Background(),
		`SELECT recording_id, duration_seconds, format_name, codec_name, sample_rate, channels, bit_rate, raw_probe_json, probed_at, created_at, updated_at
FROM recording_audio_probes
WHERE recording_id = $1`,
		recordingID,
	)
	if err := scanAudioProbe(row, &probe); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RecordingAudioProbe{}, false
		}
		return RecordingAudioProbe{}, false
	}
	return probe, true
}

func scanAudioProbe(row PostgresRow, probe *RecordingAudioProbe) error {
	return row.Scan(
		&probe.RecordingID,
		&probe.DurationSeconds,
		&probe.FormatName,
		&probe.CodecName,
		&probe.SampleRate,
		&probe.Channels,
		&probe.BitRate,
		&probe.RawProbeJSON,
		&probe.ProbedAt,
		&probe.CreatedAt,
		&probe.UpdatedAt,
	)
}
