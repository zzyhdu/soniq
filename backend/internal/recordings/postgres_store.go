package recordings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	storedb "github.com/zzyhdu/soniq/backend/internal/db"
	"github.com/zzyhdu/soniq/backend/internal/domain"
)

// PostgresStore persists recordings in a Postgres recordings table.
type PostgresStore struct {
	db storedb.PostgresExecutor
}

const recordingColumns = `id, workspace_id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, failure_reason, completed_at, failed_at, deleted_at, deleted_by_user_id, created_at, updated_at`

// NewPostgresStore creates a Postgres-backed recording store.
func NewPostgresStore(db storedb.PostgresExecutor) *PostgresStore {
	return &PostgresStore{db: db}
}

// Create inserts a new recording with skeleton defaults and returns the persisted row.
func (s *PostgresStore) Create(input CreateRecordingInput) (domain.Recording, error) {
	if err := validateCreateRecordingInput(input); err != nil {
		return domain.Recording{}, err
	}
	if s == nil || s.db == nil {
		return domain.Recording{}, fmt.Errorf("postgres recording store requires database executor")
	}

	now := time.Now().UTC()
	recording := domain.Recording{
		ID:               newRecordingID(),
		WorkspaceID:      input.WorkspaceID,
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
		`INSERT INTO recordings (id, workspace_id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING `+recordingColumns,
		recording.ID,
		recording.WorkspaceID,
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
func (s *PostgresStore) Get(id string) (domain.Recording, bool, error) {
	if s == nil || s.db == nil {
		return domain.Recording{}, false, fmt.Errorf("postgres recording store requires database executor")
	}

	var recording domain.Recording
	row := s.db.QueryRow(
		context.Background(),
		`SELECT `+recordingColumns+`
FROM recordings
WHERE id = $1
  AND deleted_at IS NULL`,
		id,
	)
	if err := scanRecording(row, &recording); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Recording{}, false, nil
		}
		return domain.Recording{}, false, fmt.Errorf("get recording: %w", err)
	}
	return recording, true, nil
}

// GetForWorkspace returns a recording by id only if it belongs to the workspace.
func (s *PostgresStore) GetForWorkspace(input GetRecordingInput) (domain.Recording, bool, error) {
	if err := validateGetRecordingInput(input); err != nil {
		return domain.Recording{}, false, err
	}
	if s == nil || s.db == nil {
		return domain.Recording{}, false, fmt.Errorf("postgres recording store requires database executor")
	}

	var recording domain.Recording
	row := s.db.QueryRow(
		context.Background(),
		`SELECT `+recordingColumns+`
FROM recordings
WHERE workspace_id = $1
  AND id = $2
  AND deleted_at IS NULL`,
		input.WorkspaceID,
		input.ID,
	)
	if err := scanRecording(row, &recording); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Recording{}, false, nil
		}
		return domain.Recording{}, false, fmt.Errorf("get recording for workspace: %w", err)
	}
	return recording, true, nil
}

// ListByWorkspace returns recent recordings for a workspace.
func (s *PostgresStore) ListByWorkspace(input ListRecordingsInput) ([]domain.Recording, error) {
	if err := validateListRecordingsInput(input); err != nil {
		return nil, err
	}
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("postgres recording store requires database executor")
	}

	limit := input.Limit
	if limit == 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := s.db.Query(
		context.Background(),
		`SELECT `+recordingColumns+`
FROM recordings
WHERE workspace_id = $1
  AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT $2`,
		input.WorkspaceID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list recordings by workspace: %w", err)
	}
	defer rows.Close()

	recordings := []domain.Recording{}
	for rows.Next() {
		var recording domain.Recording
		if err := scanRecording(rows, &recording); err != nil {
			return nil, fmt.Errorf("scan workspace recording: %w", err)
		}
		recordings = append(recordings, recording)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recordings by workspace rows: %w", err)
	}
	return recordings, nil
}

// UpdateForWorkspace updates editable recording metadata within a workspace.
func (s *PostgresStore) UpdateForWorkspace(input UpdateRecordingInput) (domain.Recording, bool, error) {
	if err := validateUpdateRecordingInput(input); err != nil {
		return domain.Recording{}, false, err
	}
	if s == nil || s.db == nil {
		return domain.Recording{}, false, fmt.Errorf("postgres recording store requires database executor")
	}

	updatedAt := time.Now().UTC()
	var recording domain.Recording
	row := s.db.QueryRow(
		context.Background(),
		`UPDATE recordings
SET title = $3,
    updated_at = $4
WHERE workspace_id = $1
  AND id = $2
  AND deleted_at IS NULL
RETURNING `+recordingColumns,
		input.WorkspaceID,
		input.ID,
		strings.TrimSpace(input.Title),
		updatedAt,
	)
	if err := scanRecording(row, &recording); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Recording{}, false, nil
		}
		return domain.Recording{}, false, fmt.Errorf("update recording metadata: %w", err)
	}
	return recording, true, nil
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
	failureReason := failureReasonForStatus(input.Status, input.FailureReason)
	var recording domain.Recording
	var row storedb.PostgresRow
	if input.WorkspaceID == "" {
		row = s.db.QueryRow(
			context.Background(),
			`UPDATE recordings
SET status = $2,
    failure_reason = CASE WHEN $2::text = 'failed' THEN $4 ELSE '' END,
    completed_at = CASE WHEN $2::text = 'completed' THEN $3::timestamptz ELSE NULL::timestamptz END,
    failed_at = CASE WHEN $2::text = 'failed' THEN $3::timestamptz ELSE NULL::timestamptz END,
    updated_at = $3
WHERE id = $1
  AND deleted_at IS NULL
RETURNING `+recordingColumns,
			input.ID,
			input.Status,
			updatedAt,
			failureReason,
		)
	} else {
		row = s.db.QueryRow(
			context.Background(),
			`UPDATE recordings
SET status = $3,
    failure_reason = CASE WHEN $3::text = 'failed' THEN $5 ELSE '' END,
    completed_at = CASE WHEN $3::text = 'completed' THEN $4::timestamptz ELSE NULL::timestamptz END,
    failed_at = CASE WHEN $3::text = 'failed' THEN $4::timestamptz ELSE NULL::timestamptz END,
    updated_at = $4
WHERE workspace_id = $1
  AND id = $2
  AND deleted_at IS NULL
RETURNING `+recordingColumns,
			input.WorkspaceID,
			input.ID,
			input.Status,
			updatedAt,
			failureReason,
		)
	}
	if err := scanRecording(row, &recording); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Recording{}, fmt.Errorf("recording not found: %s", input.ID)
		}
		return domain.Recording{}, fmt.Errorf("update recording status: %w", err)
	}
	return recording, nil
}

// ResetForRetry clears failure metadata and moves a failed recording back to uploaded.
func (s *PostgresStore) ResetForRetry(input RetryRecordingInput) (domain.Recording, error) {
	if err := validateRetryRecordingInput(input); err != nil {
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
SET status = 'uploaded',
    failure_reason = '',
    completed_at = NULL,
    failed_at = NULL,
    updated_at = $3
WHERE workspace_id = $1
  AND id = $2
  AND status = 'failed'
  AND deleted_at IS NULL
RETURNING `+recordingColumns,
		input.WorkspaceID,
		input.ID,
		updatedAt,
	)
	if err := scanRecording(row, &recording); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Recording{}, fmt.Errorf("recording not found or not failed: %s", input.ID)
		}
		return domain.Recording{}, fmt.Errorf("reset recording for retry: %w", err)
	}
	return recording, nil
}

// SoftDeleteForWorkspace marks an active recording as deleted within a workspace.
func (s *PostgresStore) SoftDeleteForWorkspace(input SoftDeleteRecordingInput) (domain.Recording, bool, error) {
	if err := validateSoftDeleteRecordingInput(input); err != nil {
		return domain.Recording{}, false, err
	}
	if s == nil || s.db == nil {
		return domain.Recording{}, false, fmt.Errorf("postgres recording store requires database executor")
	}

	deletedAt := time.Now().UTC()
	var recording domain.Recording
	row := s.db.QueryRow(
		context.Background(),
		`UPDATE recordings
SET deleted_at = $3,
    deleted_by_user_id = $4,
    updated_at = $3
WHERE workspace_id = $1
  AND id = $2
  AND deleted_at IS NULL
RETURNING `+recordingColumns,
		input.WorkspaceID,
		input.ID,
		deletedAt,
		input.DeletedByUserID,
	)
	if err := scanRecording(row, &recording); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Recording{}, false, nil
		}
		return domain.Recording{}, false, fmt.Errorf("soft delete recording: %w", err)
	}
	return recording, true, nil
}

// ListDeletedByWorkspace returns soft-deleted recordings for a workspace.
func (s *PostgresStore) ListDeletedByWorkspace(input ListDeletedRecordingsInput) ([]domain.Recording, error) {
	if err := validateListDeletedRecordingsInput(input); err != nil {
		return nil, err
	}
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("postgres recording store requires database executor")
	}

	limit := input.Limit
	if limit == 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := s.db.Query(
		context.Background(),
		`SELECT `+recordingColumns+`
FROM recordings
WHERE workspace_id = $1
  AND deleted_at IS NOT NULL
ORDER BY deleted_at DESC, id DESC
LIMIT $2`,
		input.WorkspaceID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list deleted recordings by workspace: %w", err)
	}
	defer rows.Close()

	recordings := []domain.Recording{}
	for rows.Next() {
		var recording domain.Recording
		if err := scanRecording(rows, &recording); err != nil {
			return nil, fmt.Errorf("scan deleted workspace recording: %w", err)
		}
		recordings = append(recordings, recording)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list deleted recordings by workspace rows: %w", err)
	}
	return recordings, nil
}

// RestoreForWorkspace clears deletion metadata for a soft-deleted recording.
func (s *PostgresStore) RestoreForWorkspace(input RestoreRecordingInput) (domain.Recording, bool, error) {
	if err := validateRestoreRecordingInput(input); err != nil {
		return domain.Recording{}, false, err
	}
	if s == nil || s.db == nil {
		return domain.Recording{}, false, fmt.Errorf("postgres recording store requires database executor")
	}

	updatedAt := time.Now().UTC()
	var recording domain.Recording
	row := s.db.QueryRow(
		context.Background(),
		`UPDATE recordings
SET deleted_at = NULL,
    deleted_by_user_id = NULL,
    updated_at = $3
WHERE workspace_id = $1
  AND id = $2
  AND deleted_at IS NOT NULL
RETURNING `+recordingColumns,
		input.WorkspaceID,
		input.ID,
		updatedAt,
	)
	if err := scanRecording(row, &recording); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return domain.Recording{}, false, nil
		}
		return domain.Recording{}, false, fmt.Errorf("restore recording: %w", err)
	}
	return recording, true, nil
}

// PurgeForWorkspace permanently deletes a soft-deleted recording and records artifact cleanup intents.
func (s *PostgresStore) PurgeForWorkspace(input PurgeRecordingInput) (PurgeRecordingResult, bool, error) {
	if err := validatePurgeRecordingInput(input); err != nil {
		return PurgeRecordingResult{}, false, err
	}
	if s == nil || s.db == nil {
		return PurgeRecordingResult{}, false, fmt.Errorf("postgres recording store requires database executor")
	}
	transactor, ok := s.db.(storedb.PostgresTransactor)
	if !ok {
		return PurgeRecordingResult{}, false, fmt.Errorf("postgres recording store requires transaction support")
	}

	ctx := context.Background()
	tx, err := transactor.Begin(ctx)
	if err != nil {
		return PurgeRecordingResult{}, false, fmt.Errorf("begin purge recording transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	var recordingID string
	var workspaceID string
	var originalObjectKey string
	var normalizedObjectKey string
	row := tx.QueryRow(
		ctx,
		`SELECT r.id, r.workspace_id, r.audio_object_key, COALESCE(n.object_key, '')
FROM recordings r
LEFT JOIN recording_normalized_audios n ON n.recording_id = r.id
WHERE r.workspace_id = $1
  AND r.id = $2
  AND r.deleted_at IS NOT NULL
FOR UPDATE OF r`,
		input.WorkspaceID,
		input.ID,
	)
	if err := row.Scan(&recordingID, &workspaceID, &originalObjectKey, &normalizedObjectKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return PurgeRecordingResult{}, false, nil
		}
		return PurgeRecordingResult{}, false, fmt.Errorf("select recording for purge: %w", err)
	}

	now := time.Now().UTC()
	result := PurgeRecordingResult{Artifacts: []RecordingPurgeArtifact{}}
	for _, artifact := range purgeArtifactsForRecording(recordingID, workspaceID, originalObjectKey, normalizedObjectKey) {
		inserted, err := insertPurgeArtifact(ctx, tx, artifact, now)
		if err != nil {
			return PurgeRecordingResult{}, false, err
		}
		result.Artifacts = append(result.Artifacts, inserted)
	}

	childDeletes := []string{
		`DELETE FROM recording_mind_maps WHERE recording_id = $1`,
		`DELETE FROM recording_transcript_segments WHERE recording_id = $1`,
		`DELETE FROM recording_summaries WHERE recording_id = $1`,
		`DELETE FROM recording_transcripts WHERE recording_id = $1`,
		`DELETE FROM recording_audio_probes WHERE recording_id = $1`,
		`DELETE FROM recording_normalized_audios WHERE recording_id = $1`,
	}
	for _, query := range childDeletes {
		if err := tx.Exec(ctx, query, recordingID); err != nil {
			return PurgeRecordingResult{}, false, fmt.Errorf("delete recording child rows: %w", err)
		}
	}
	if err := tx.QueryRow(ctx, `DELETE FROM recordings WHERE id = $1 RETURNING id`, recordingID).Scan(&recordingID); err != nil {
		return PurgeRecordingResult{}, false, fmt.Errorf("delete recording row: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PurgeRecordingResult{}, false, fmt.Errorf("commit purge recording transaction: %w", err)
	}
	committed = true
	return result, true, nil
}

// ClaimPurgeArtifacts claims retryable purge artifact cleanup rows.
func (s *PostgresStore) ClaimPurgeArtifacts(input ClaimPurgeArtifactsInput) ([]RecordingPurgeArtifact, error) {
	if err := validateClaimPurgeArtifactsInput(input); err != nil {
		return nil, err
	}
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("postgres recording store requires database executor")
	}

	limit := input.Limit
	if limit == 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	now := time.Now().UTC()
	rows, err := s.db.Query(
		context.Background(),
		`WITH claim AS (
  SELECT id
  FROM recording_purge_artifacts
  WHERE deleted_at IS NULL
    AND (
      status IN ('pending', 'failed')
      OR (status = 'deleting' AND updated_at <= $1::timestamptz - INTERVAL '15 minutes')
    )
    AND next_attempt_at <= $1::timestamptz
  ORDER BY next_attempt_at ASC, created_at ASC, id ASC
  LIMIT $2
  FOR UPDATE SKIP LOCKED
)
UPDATE recording_purge_artifacts artifact
SET status = 'deleting',
    updated_at = $1
FROM claim
WHERE artifact.id = claim.id
RETURNING artifact.id, artifact.recording_id, artifact.workspace_id, artifact.object_key, artifact.artifact_kind, artifact.status, artifact.attempt_count, artifact.next_attempt_at, artifact.last_error, artifact.created_at, artifact.updated_at, artifact.deleted_at`,
		now,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("claim purge artifacts: %w", err)
	}
	defer rows.Close()

	artifacts := []RecordingPurgeArtifact{}
	for rows.Next() {
		var artifact RecordingPurgeArtifact
		if err := scanPurgeArtifact(rows, &artifact); err != nil {
			return nil, fmt.Errorf("scan purge artifact: %w", err)
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim purge artifacts rows: %w", err)
	}
	return artifacts, nil
}

// MarkPurgeArtifactDeleted marks an artifact cleanup row completed.
func (s *PostgresStore) MarkPurgeArtifactDeleted(input MarkPurgeArtifactDeletedInput) (bool, error) {
	if err := validateMarkPurgeArtifactDeletedInput(input); err != nil {
		return false, err
	}
	if s == nil || s.db == nil {
		return false, fmt.Errorf("postgres recording store requires database executor")
	}

	now := time.Now().UTC()
	var id string
	row := s.db.QueryRow(
		context.Background(),
		`UPDATE recording_purge_artifacts
SET status = 'deleted',
    last_error = '',
    deleted_at = $2,
    updated_at = $2
WHERE id = $1
RETURNING id`,
		input.ID,
		now,
	)
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("mark purge artifact deleted: %w", err)
	}
	return true, nil
}

// MarkPurgeArtifactFailed records a failed artifact cleanup attempt.
func (s *PostgresStore) MarkPurgeArtifactFailed(input MarkPurgeArtifactFailedInput) (bool, error) {
	if err := validateMarkPurgeArtifactFailedInput(input); err != nil {
		return false, err
	}
	if s == nil || s.db == nil {
		return false, fmt.Errorf("postgres recording store requires database executor")
	}

	now := time.Now().UTC()
	var id string
	row := s.db.QueryRow(
		context.Background(),
		`UPDATE recording_purge_artifacts
SET status = 'failed',
    attempt_count = attempt_count + 1,
    last_error = $2,
    next_attempt_at = $3,
    updated_at = $4
WHERE id = $1
  AND deleted_at IS NULL
RETURNING id`,
		input.ID,
		truncatePurgeArtifactError(input.LastError),
		input.NextAttemptAt,
		now,
	)
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("mark purge artifact failed: %w", err)
	}
	return true, nil
}

func purgeArtifactsForRecording(recordingID string, workspaceID string, originalObjectKey string, normalizedObjectKey string) []RecordingPurgeArtifact {
	artifacts := []RecordingPurgeArtifact{}
	seen := map[string]struct{}{}
	appendArtifact := func(kind string, objectKey string) {
		objectKey = strings.TrimSpace(objectKey)
		if objectKey == "" {
			return
		}
		if _, ok := seen[objectKey]; ok {
			return
		}
		seen[objectKey] = struct{}{}
		artifacts = append(artifacts, RecordingPurgeArtifact{
			ID:           purgeArtifactID(recordingID, objectKey),
			RecordingID:  recordingID,
			WorkspaceID:  workspaceID,
			ObjectKey:    objectKey,
			ArtifactKind: kind,
			Status:       RecordingPurgeArtifactStatusPending,
		})
	}
	appendArtifact(RecordingPurgeArtifactKindOriginalAudio, originalObjectKey)
	appendArtifact(RecordingPurgeArtifactKindNormalizedAudio, normalizedObjectKey)
	return artifacts
}

func insertPurgeArtifact(ctx context.Context, tx storedb.PostgresTx, artifact RecordingPurgeArtifact, now time.Time) (RecordingPurgeArtifact, error) {
	var inserted RecordingPurgeArtifact
	row := tx.QueryRow(
		ctx,
		`INSERT INTO recording_purge_artifacts (id, recording_id, workspace_id, object_key, artifact_kind, status, next_attempt_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, 'pending', $6, $6, $6)
ON CONFLICT (recording_id, object_key) DO UPDATE
SET workspace_id = EXCLUDED.workspace_id,
    artifact_kind = EXCLUDED.artifact_kind,
    status = 'pending',
    next_attempt_at = EXCLUDED.next_attempt_at,
    last_error = '',
    deleted_at = NULL,
    updated_at = EXCLUDED.updated_at
RETURNING id, recording_id, workspace_id, object_key, artifact_kind, status, attempt_count, next_attempt_at, last_error, created_at, updated_at, deleted_at`,
		artifact.ID,
		artifact.RecordingID,
		artifact.WorkspaceID,
		artifact.ObjectKey,
		artifact.ArtifactKind,
		now,
	)
	if err := scanPurgeArtifact(row, &inserted); err != nil {
		return RecordingPurgeArtifact{}, fmt.Errorf("insert purge artifact: %w", err)
	}
	return inserted, nil
}

func truncatePurgeArtifactError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 2000 {
		return value
	}
	return value[:2000]
}

// UpsertNormalizedAudio stores or replaces normalized audio metadata for a recording.
func (s *PostgresStore) UpsertNormalizedAudio(input UpsertNormalizedAudioInput) (RecordingNormalizedAudio, error) {
	if err := validateNormalizedAudioInput(input); err != nil {
		return RecordingNormalizedAudio{}, err
	}
	if s == nil || s.db == nil {
		return RecordingNormalizedAudio{}, fmt.Errorf("postgres recording store requires database executor")
	}

	now := time.Now().UTC()
	var normalized RecordingNormalizedAudio
	row := s.db.QueryRow(
		context.Background(),
		`INSERT INTO recording_normalized_audios (recording_id, object_key, content_type, size_bytes, format_name, codec_name, sample_rate, channels, duration_seconds, normalized_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (recording_id) DO UPDATE
SET object_key = EXCLUDED.object_key,
    content_type = EXCLUDED.content_type,
    size_bytes = EXCLUDED.size_bytes,
    format_name = EXCLUDED.format_name,
    codec_name = EXCLUDED.codec_name,
    sample_rate = EXCLUDED.sample_rate,
    channels = EXCLUDED.channels,
    duration_seconds = EXCLUDED.duration_seconds,
    normalized_at = EXCLUDED.normalized_at,
    updated_at = EXCLUDED.updated_at
RETURNING recording_id, object_key, content_type, size_bytes, format_name, codec_name, sample_rate, channels, duration_seconds, normalized_at, created_at, updated_at`,
		input.RecordingID,
		input.ObjectKey,
		input.ContentType,
		input.SizeBytes,
		input.FormatName,
		input.CodecName,
		input.SampleRate,
		input.Channels,
		input.DurationSeconds,
		input.NormalizedAt,
		now,
		now,
	)
	if err := scanNormalizedAudio(row, &normalized); err != nil {
		return RecordingNormalizedAudio{}, fmt.Errorf("upsert recording normalized audio: %w", err)
	}
	return normalized, nil
}

// GetNormalizedAudio returns normalized audio metadata by recording id.
func (s *PostgresStore) GetNormalizedAudio(recordingID string) (RecordingNormalizedAudio, bool, error) {
	if s == nil || s.db == nil {
		return RecordingNormalizedAudio{}, false, fmt.Errorf("postgres recording store requires database executor")
	}
	var normalized RecordingNormalizedAudio
	row := s.db.QueryRow(
		context.Background(),
		`SELECT recording_id, object_key, content_type, size_bytes, format_name, codec_name, sample_rate, channels, duration_seconds, normalized_at, created_at, updated_at
FROM recording_normalized_audios
WHERE recording_id = $1`,
		recordingID,
	)
	if err := scanNormalizedAudio(row, &normalized); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return RecordingNormalizedAudio{}, false, nil
		}
		return RecordingNormalizedAudio{}, false, fmt.Errorf("get recording normalized audio: %w", err)
	}
	return normalized, true, nil
}

// UpsertTranscript stores or replaces the latest transcript and its segments for a recording.
func (s *PostgresStore) UpsertTranscript(input UpsertTranscriptInput) (RecordingTranscript, error) {
	if err := validateTranscriptInput(input); err != nil {
		return RecordingTranscript{}, err
	}
	if s == nil || s.db == nil {
		return RecordingTranscript{}, fmt.Errorf("postgres recording store requires database executor")
	}

	now := time.Now().UTC()
	var transcript RecordingTranscript
	row := s.db.QueryRow(
		context.Background(),
		`INSERT INTO recording_transcripts (recording_id, provider, model, language, text, raw_result_json, transcribed_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (recording_id) DO UPDATE
SET provider = EXCLUDED.provider,
    model = EXCLUDED.model,
    language = EXCLUDED.language,
    text = EXCLUDED.text,
    raw_result_json = EXCLUDED.raw_result_json,
    transcribed_at = EXCLUDED.transcribed_at,
    updated_at = EXCLUDED.updated_at
RETURNING recording_id, provider, model, language, text, raw_result_json, transcribed_at, created_at, updated_at`,
		input.RecordingID,
		input.Provider,
		input.Model,
		input.Language,
		input.Text,
		append([]byte(nil), input.RawResultJSON...),
		input.TranscribedAt,
		now,
		now,
	)
	if err := scanTranscript(row, &transcript); err != nil {
		return RecordingTranscript{}, fmt.Errorf("upsert recording transcript: %w", err)
	}

	if err := s.db.QueryRow(context.Background(), `DELETE FROM recording_transcript_segments WHERE recording_id = $1`, input.RecordingID).Scan(); err != nil && !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, pgx.ErrNoRows) {
		return RecordingTranscript{}, fmt.Errorf("delete recording transcript segments: %w", err)
	}
	for i, segment := range input.Segments {
		idx := segment.SegmentIndex
		if idx < 0 {
			idx = i
		}
		if err := s.db.QueryRow(
			context.Background(),
			`INSERT INTO recording_transcript_segments (id, recording_id, segment_index, start_ms, end_ms, speaker_label, text, confidence, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			transcriptSegmentID(input.RecordingID, idx),
			input.RecordingID,
			idx,
			segment.StartMS,
			segment.EndMS,
			segment.SpeakerLabel,
			segment.Text,
			segment.Confidence,
			now,
		).Scan(); err != nil && !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, pgx.ErrNoRows) {
			return RecordingTranscript{}, fmt.Errorf("insert recording transcript segment: %w", err)
		}
	}
	return transcript, nil
}

// GetTranscript returns the latest transcript by recording id.
func (s *PostgresStore) GetTranscript(recordingID string) (RecordingTranscript, bool, error) {
	if s == nil || s.db == nil {
		return RecordingTranscript{}, false, fmt.Errorf("postgres recording store requires database executor")
	}
	var transcript RecordingTranscript
	row := s.db.QueryRow(
		context.Background(),
		`SELECT recording_id, provider, model, language, text, raw_result_json, transcribed_at, created_at, updated_at
FROM recording_transcripts
WHERE recording_id = $1`,
		recordingID,
	)
	if err := scanTranscript(row, &transcript); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return RecordingTranscript{}, false, nil
		}
		return RecordingTranscript{}, false, fmt.Errorf("get recording transcript: %w", err)
	}
	return transcript, true, nil
}

// ListTranscriptSegments returns transcript segments by recording id ordered by segment_index.
func (s *PostgresStore) ListTranscriptSegments(recordingID string) ([]RecordingTranscriptSegment, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("postgres recording store requires database executor")
	}
	rows, err := s.db.Query(
		context.Background(),
		`SELECT id, recording_id, segment_index, start_ms, end_ms, speaker_label, text, confidence, created_at
FROM recording_transcript_segments
WHERE recording_id = $1
ORDER BY segment_index`,
		recordingID,
	)
	if err != nil {
		return nil, fmt.Errorf("list recording transcript segments: %w", err)
	}
	defer rows.Close()

	segments := make([]RecordingTranscriptSegment, 0)
	for rows.Next() {
		var segment RecordingTranscriptSegment
		if err := scanTranscriptSegment(rows, &segment); err != nil {
			return nil, fmt.Errorf("scan recording transcript segment: %w", err)
		}
		segments = append(segments, segment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recording transcript segments: %w", err)
	}
	return segments, nil
}

// UpsertSummary stores or replaces the latest summary for a recording.
func (s *PostgresStore) UpsertSummary(input UpsertSummaryInput) (RecordingSummary, error) {
	if err := validateSummaryInput(input); err != nil {
		return RecordingSummary{}, err
	}
	if s == nil || s.db == nil {
		return RecordingSummary{}, fmt.Errorf("postgres recording store requires database executor")
	}
	now := time.Now().UTC()
	var summary RecordingSummary
	row := s.db.QueryRow(
		context.Background(),
		`INSERT INTO recording_summaries (recording_id, provider, model, type, title, overview, content_markdown, raw_result_json, summarized_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (recording_id) DO UPDATE
SET provider = EXCLUDED.provider,
    model = EXCLUDED.model,
    type = EXCLUDED.type,
    title = EXCLUDED.title,
    overview = EXCLUDED.overview,
    content_markdown = EXCLUDED.content_markdown,
    raw_result_json = EXCLUDED.raw_result_json,
    summarized_at = EXCLUDED.summarized_at,
    updated_at = EXCLUDED.updated_at
RETURNING recording_id, provider, model, type, title, overview, content_markdown, raw_result_json, summarized_at, created_at, updated_at`,
		input.RecordingID,
		input.Provider,
		input.Model,
		input.Type,
		input.Title,
		input.Overview,
		input.ContentMarkdown,
		append([]byte(nil), input.RawResultJSON...),
		input.SummarizedAt,
		now,
		now,
	)
	if err := scanSummary(row, &summary); err != nil {
		return RecordingSummary{}, fmt.Errorf("upsert recording summary: %w", err)
	}
	return summary, nil
}

// GetSummary returns the latest summary by recording id.
func (s *PostgresStore) GetSummary(recordingID string) (RecordingSummary, bool, error) {
	if s == nil || s.db == nil {
		return RecordingSummary{}, false, fmt.Errorf("postgres recording store requires database executor")
	}
	var summary RecordingSummary
	row := s.db.QueryRow(
		context.Background(),
		`SELECT recording_id, provider, model, type, title, overview, content_markdown, raw_result_json, summarized_at, created_at, updated_at
FROM recording_summaries
WHERE recording_id = $1`,
		recordingID,
	)
	if err := scanSummary(row, &summary); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return RecordingSummary{}, false, nil
		}
		return RecordingSummary{}, false, fmt.Errorf("get recording summary: %w", err)
	}
	return summary, true, nil
}

// UpsertMindMap stores or replaces the latest mind map for a recording.
func (s *PostgresStore) UpsertMindMap(input UpsertMindMapInput) (RecordingMindMap, error) {
	if err := validateMindMapInput(input); err != nil {
		return RecordingMindMap{}, err
	}
	if s == nil || s.db == nil {
		return RecordingMindMap{}, fmt.Errorf("postgres recording store requires database executor")
	}
	now := time.Now().UTC()
	var mindMap RecordingMindMap
	row := s.db.QueryRow(
		context.Background(),
		`INSERT INTO recording_mind_maps (recording_id, provider, model, title, root_json, content_markdown, raw_result_json, generated_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (recording_id) DO UPDATE
SET provider = EXCLUDED.provider,
    model = EXCLUDED.model,
    title = EXCLUDED.title,
    root_json = EXCLUDED.root_json,
    content_markdown = EXCLUDED.content_markdown,
    raw_result_json = EXCLUDED.raw_result_json,
    generated_at = EXCLUDED.generated_at,
    updated_at = EXCLUDED.updated_at
RETURNING recording_id, provider, model, title, root_json, content_markdown, raw_result_json, generated_at, created_at, updated_at`,
		input.RecordingID,
		input.Provider,
		input.Model,
		input.Title,
		append([]byte(nil), input.RootJSON...),
		input.ContentMarkdown,
		append([]byte(nil), input.RawResultJSON...),
		input.GeneratedAt,
		now,
		now,
	)
	if err := scanMindMap(row, &mindMap); err != nil {
		return RecordingMindMap{}, fmt.Errorf("upsert recording mind map: %w", err)
	}
	return mindMap, nil
}

// GetMindMap returns the latest mind map by recording id.
func (s *PostgresStore) GetMindMap(recordingID string) (RecordingMindMap, bool, error) {
	if s == nil || s.db == nil {
		return RecordingMindMap{}, false, fmt.Errorf("postgres recording store requires database executor")
	}
	var mindMap RecordingMindMap
	row := s.db.QueryRow(
		context.Background(),
		`SELECT recording_id, provider, model, title, root_json, content_markdown, raw_result_json, generated_at, created_at, updated_at
FROM recording_mind_maps
WHERE recording_id = $1`,
		recordingID,
	)
	if err := scanMindMap(row, &mindMap); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return RecordingMindMap{}, false, nil
		}
		return RecordingMindMap{}, false, fmt.Errorf("get recording mind map: %w", err)
	}
	return mindMap, true, nil
}

func scanNormalizedAudio(row storedb.PostgresRow, normalized *RecordingNormalizedAudio) error {
	return row.Scan(
		&normalized.RecordingID,
		&normalized.ObjectKey,
		&normalized.ContentType,
		&normalized.SizeBytes,
		&normalized.FormatName,
		&normalized.CodecName,
		&normalized.SampleRate,
		&normalized.Channels,
		&normalized.DurationSeconds,
		&normalized.NormalizedAt,
		&normalized.CreatedAt,
		&normalized.UpdatedAt,
	)
}

func scanTranscript(row storedb.PostgresRow, transcript *RecordingTranscript) error {
	return row.Scan(&transcript.RecordingID, &transcript.Provider, &transcript.Model, &transcript.Language, &transcript.Text, &transcript.RawResultJSON, &transcript.TranscribedAt, &transcript.CreatedAt, &transcript.UpdatedAt)
}

func scanTranscriptSegment(row storedb.PostgresRow, segment *RecordingTranscriptSegment) error {
	return row.Scan(&segment.ID, &segment.RecordingID, &segment.SegmentIndex, &segment.StartMS, &segment.EndMS, &segment.SpeakerLabel, &segment.Text, &segment.Confidence, &segment.CreatedAt)
}

func scanSummary(row storedb.PostgresRow, summary *RecordingSummary) error {
	return row.Scan(&summary.RecordingID, &summary.Provider, &summary.Model, &summary.Type, &summary.Title, &summary.Overview, &summary.ContentMarkdown, &summary.RawResultJSON, &summary.SummarizedAt, &summary.CreatedAt, &summary.UpdatedAt)
}

func scanMindMap(row storedb.PostgresRow, mindMap *RecordingMindMap) error {
	return row.Scan(&mindMap.RecordingID, &mindMap.Provider, &mindMap.Model, &mindMap.Title, &mindMap.RootJSON, &mindMap.ContentMarkdown, &mindMap.RawResultJSON, &mindMap.GeneratedAt, &mindMap.CreatedAt, &mindMap.UpdatedAt)
}

func scanPurgeArtifact(row storedb.PostgresRow, artifact *RecordingPurgeArtifact) error {
	var deletedAt sql.NullTime
	var status string
	if err := row.Scan(
		&artifact.ID,
		&artifact.RecordingID,
		&artifact.WorkspaceID,
		&artifact.ObjectKey,
		&artifact.ArtifactKind,
		&status,
		&artifact.AttemptCount,
		&artifact.NextAttemptAt,
		&artifact.LastError,
		&artifact.CreatedAt,
		&artifact.UpdatedAt,
		&deletedAt,
	); err != nil {
		return err
	}
	artifact.DeletedAt = nil
	if deletedAt.Valid {
		deleted := deletedAt.Time
		artifact.DeletedAt = &deleted
	}
	artifact.Status = RecordingPurgeArtifactStatus(status)
	return nil
}

func scanRecording(row storedb.PostgresRow, recording *domain.Recording) error {
	var completedAt sql.NullTime
	var failedAt sql.NullTime
	var deletedAt sql.NullTime
	var deletedByUserID sql.NullString
	if err := row.Scan(
		&recording.ID,
		&recording.WorkspaceID,
		&recording.Title,
		&recording.Status,
		&recording.WorkflowType,
		&recording.Language,
		&recording.AudioObjectKey,
		&recording.AudioContentType,
		&recording.AudioSizeBytes,
		&recording.FailureReason,
		&completedAt,
		&failedAt,
		&deletedAt,
		&deletedByUserID,
		&recording.CreatedAt,
		&recording.UpdatedAt,
	); err != nil {
		return err
	}
	recording.CompletedAt = nil
	if completedAt.Valid {
		completed := completedAt.Time
		recording.CompletedAt = &completed
	}
	recording.FailedAt = nil
	if failedAt.Valid {
		failed := failedAt.Time
		recording.FailedAt = &failed
	}
	recording.DeletedAt = nil
	if deletedAt.Valid {
		deleted := deletedAt.Time
		recording.DeletedAt = &deleted
	}
	recording.DeletedByUserID = ""
	if deletedByUserID.Valid {
		recording.DeletedByUserID = deletedByUserID.String
	}
	return nil
}

func failureReasonForStatus(status domain.RecordingStatus, reason string) string {
	if status != domain.RecordingStatusFailed {
		return ""
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "recording processing failed"
	}
	return reason
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
func (s *PostgresStore) GetAudioProbe(recordingID string) (RecordingAudioProbe, bool, error) {
	if s == nil || s.db == nil {
		return RecordingAudioProbe{}, false, fmt.Errorf("postgres recording store requires database executor")
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
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return RecordingAudioProbe{}, false, nil
		}
		return RecordingAudioProbe{}, false, fmt.Errorf("get recording audio probe: %w", err)
	}
	return probe, true, nil
}

func scanAudioProbe(row storedb.PostgresRow, probe *RecordingAudioProbe) error {
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
