package recordings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	storedb "github.com/zzyhdu/soniq/backend/internal/db"
	"github.com/zzyhdu/soniq/backend/internal/domain"
)

// PostgresStore persists recordings in a Postgres recordings table.
type PostgresStore struct {
	db storedb.PostgresExecutor
}

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
RETURNING id, workspace_id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, created_at, updated_at`,
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
		`SELECT id, workspace_id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, created_at, updated_at
FROM recordings
WHERE id = $1`,
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
		`SELECT id, workspace_id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, created_at, updated_at
FROM recordings
WHERE workspace_id = $1
  AND id = $2`,
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
		`SELECT id, workspace_id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, created_at, updated_at
FROM recordings
WHERE workspace_id = $1
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
	var row storedb.PostgresRow
	if input.WorkspaceID == "" {
		row = s.db.QueryRow(
			context.Background(),
			`UPDATE recordings
SET status = $2, updated_at = $3
WHERE id = $1
RETURNING id, workspace_id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, created_at, updated_at`,
			input.ID,
			input.Status,
			updatedAt,
		)
	} else {
		row = s.db.QueryRow(
			context.Background(),
			`UPDATE recordings
SET status = $3, updated_at = $4
WHERE workspace_id = $1
  AND id = $2
RETURNING id, workspace_id, title, status, workflow_type, language, audio_object_key, audio_content_type, audio_size_bytes, created_at, updated_at`,
			input.WorkspaceID,
			input.ID,
			input.Status,
			updatedAt,
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

func scanRecording(row storedb.PostgresRow, recording *domain.Recording) error {
	return row.Scan(
		&recording.ID,
		&recording.WorkspaceID,
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
