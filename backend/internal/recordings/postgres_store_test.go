package recordings

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
)

type postgresQueryCall struct {
	query string
	args  []any
}

type postgresExecutorSpy struct {
	calls []postgresQueryCall
	rows  []*postgresRowStub
}

func newPostgresExecutorSpy(rows ...*postgresRowStub) *postgresExecutorSpy {
	return &postgresExecutorSpy{rows: rows}
}

func (s *postgresExecutorSpy) QueryRow(ctx context.Context, query string, args ...any) interface{ Scan(dest ...any) error } {
	s.calls = append(s.calls, postgresQueryCall{query: query, args: append([]any(nil), args...)})
	if len(s.rows) == 0 {
		return &postgresRowStub{err: sql.ErrNoRows}
	}
	row := s.rows[0]
	s.rows = s.rows[1:]
	return row
}

type postgresRowStub struct {
	values []any
	err    error
}

func postgresRow(values ...any) *postgresRowStub {
	return &postgresRowStub{values: values}
}

func postgresErrorRow(err error) *postgresRowStub {
	return &postgresRowStub{err: err}
}

func (r *postgresRowStub) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return sql.ErrNoRows
	}
	for i, value := range r.values {
		switch target := dest[i].(type) {
		case *string:
			*target = value.(string)
		case *domain.RecordingStatus:
			*target = value.(domain.RecordingStatus)
		case *domain.WorkflowType:
			*target = value.(domain.WorkflowType)
		case *time.Time:
			*target = value.(time.Time)
		case *int64:
			*target = value.(int64)
		case *int:
			*target = value.(int)
		case *float64:
			*target = value.(float64)
		case *[]byte:
			*target = append((*target)[:0], value.([]byte)...)
		default:
			return sql.ErrNoRows
		}
	}
	return nil
}

func TestPostgresStoreCreateInsertsRecording(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"Weekly sync",
		domain.RecordingStatusUploaded,
		domain.WorkflowTypeMeeting,
		"en",
		"",
		"",
		int64(0),
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	recording, err := store.Create(CreateRecordingInput{
		Title:        "Weekly sync",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if recording.ID != "rec_pg" {
		t.Fatalf("recording.ID = %q, want rec_pg", recording.ID)
	}
	if recording.Status != domain.RecordingStatusUploaded {
		t.Fatalf("recording.Status = %q, want uploaded", recording.Status)
	}
	if recording.WorkflowType != domain.WorkflowTypeMeeting {
		t.Fatalf("recording.WorkflowType = %q, want meeting", recording.WorkflowType)
	}
	if recording.CreatedAt != createdAt || recording.UpdatedAt != updatedAt {
		t.Fatalf("timestamps = %s/%s, want %s/%s", recording.CreatedAt, recording.UpdatedAt, createdAt, updatedAt)
	}
	if got, want := len(db.calls), 1; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	if !strings.Contains(strings.ToLower(db.calls[0].query), "insert into recordings") {
		t.Fatalf("query = %q, want insert into recordings", db.calls[0].query)
	}
	if got, want := len(db.calls[0].args), 10; got != want {
		t.Fatalf("insert args = %d, want %d", got, want)
	}
	if id, ok := db.calls[0].args[0].(string); !ok || !strings.HasPrefix(id, "rec_") {
		t.Fatalf("first insert arg = %#v, want generated rec_ id", db.calls[0].args[0])
	}
}

func TestPostgresStoreCreatePreservesAudioMetadata(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"Weekly sync",
		domain.RecordingStatusUploaded,
		domain.WorkflowTypeMeeting,
		"en",
		"recordings/rec_pg/original.wav",
		"audio/wav",
		int64(12345),
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	recording, err := store.Create(CreateRecordingInput{
		Title:            "Weekly sync",
		WorkflowType:     domain.WorkflowTypeMeeting,
		Language:         "en",
		AudioObjectKey:   "recordings/rec_pg/original.wav",
		AudioContentType: "audio/wav",
		AudioSizeBytes:   12345,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if recording.AudioObjectKey != "recordings/rec_pg/original.wav" {
		t.Fatalf("AudioObjectKey = %q, want object key", recording.AudioObjectKey)
	}
	if recording.AudioContentType != "audio/wav" {
		t.Fatalf("AudioContentType = %q, want audio/wav", recording.AudioContentType)
	}
	if recording.AudioSizeBytes != 12345 {
		t.Fatalf("AudioSizeBytes = %d, want 12345", recording.AudioSizeBytes)
	}
	if got, want := len(db.calls[0].args), 10; got != want {
		t.Fatalf("insert args = %d, want %d", got, want)
	}
	if got, want := db.calls[0].args[5], "recordings/rec_pg/original.wav"; got != want {
		t.Fatalf("audio object key arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[6], "audio/wav"; got != want {
		t.Fatalf("audio content type arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[7], int64(12345); got != want {
		t.Fatalf("audio size arg = %v, want %v", got, want)
	}
}

func TestPostgresStoreGetReturnsExistingRecording(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"Lecture 1",
		domain.RecordingStatusUploaded,
		domain.WorkflowTypeLecture,
		"zh",
		"",
		"",
		int64(0),
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	recording, ok := store.Get("rec_pg")
	if !ok {
		t.Fatal("Get(rec_pg) ok = false, want true")
	}
	if recording.ID != "rec_pg" || recording.Title != "Lecture 1" || recording.WorkflowType != domain.WorkflowTypeLecture {
		t.Fatalf("recording = %+v, want persisted recording", recording)
	}
	if got, want := len(db.calls), 1; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	if !strings.Contains(strings.ToLower(db.calls[0].query), "select") || !strings.Contains(strings.ToLower(db.calls[0].query), "from recordings") {
		t.Fatalf("query = %q, want select from recordings", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "rec_pg"; got != want {
		t.Fatalf("query id arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreGetReturnsFalseForMissingRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	_, ok := store.Get("rec_missing")
	if ok {
		t.Fatal("Get(rec_missing) ok = true, want false")
	}
}

func TestPostgresStoreCreateRejectsInvalidWorkflowTypeBeforeInsert(t *testing.T) {
	db := newPostgresExecutorSpy()
	store := NewPostgresStore(db)

	_, err := store.Create(CreateRecordingInput{
		Title:        "Podcast",
		WorkflowType: domain.WorkflowType("podcast"),
		Language:     "en",
	})
	if err == nil {
		t.Fatal("Create returned nil error, want invalid workflow type error")
	}
	if got, want := len(db.calls), 0; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
}

func TestPostgresStoreUpdateStatusUpdatesAndReturnsRecording(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"Weekly sync",
		domain.RecordingStatusProcessing,
		domain.WorkflowTypeMeeting,
		"en",
		"recordings/rec_pg/original.wav",
		"audio/wav",
		int64(12345),
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	recording, err := store.UpdateStatus(UpdateRecordingStatusInput{
		ID:     "rec_pg",
		Status: domain.RecordingStatusProcessing,
	})
	if err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}

	if recording.ID != "rec_pg" {
		t.Fatalf("recording.ID = %q, want rec_pg", recording.ID)
	}
	if recording.Status != domain.RecordingStatusProcessing {
		t.Fatalf("recording.Status = %q, want processing", recording.Status)
	}
	if recording.AudioObjectKey != "recordings/rec_pg/original.wav" || recording.AudioContentType != "audio/wav" || recording.AudioSizeBytes != 12345 {
		t.Fatalf("audio metadata = %+v, want preserved audio metadata", recording)
	}
	if recording.UpdatedAt != updatedAt {
		t.Fatalf("UpdatedAt = %s, want %s", recording.UpdatedAt, updatedAt)
	}
	if got, want := len(db.calls), 1; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "update recordings") || !strings.Contains(query, "set status") || !strings.Contains(query, "returning") {
		t.Fatalf("query = %q, want update recordings set status returning", db.calls[0].query)
	}
	if got, want := len(db.calls[0].args), 3; got != want {
		t.Fatalf("update args = %d, want %d", got, want)
	}
	if got, want := db.calls[0].args[0], "rec_pg"; got != want {
		t.Fatalf("id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[1], domain.RecordingStatusProcessing; got != want {
		t.Fatalf("status arg = %q, want %q", got, want)
	}
	if _, ok := db.calls[0].args[2].(time.Time); !ok {
		t.Fatalf("updated_at arg = %#v, want time.Time", db.calls[0].args[2])
	}
}

func TestPostgresStoreUpdateStatusReturnsErrorForMissingRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	_, err := store.UpdateStatus(UpdateRecordingStatusInput{
		ID:     "rec_missing",
		Status: domain.RecordingStatusProcessing,
	})
	if err == nil {
		t.Fatal("UpdateStatus returned nil error, want missing recording error")
	}
}

func TestPostgresStoreUpsertAudioProbeInsertsOrUpdatesProbe(t *testing.T) {
	probedAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	createdAt := probedAt
	updatedAt := probedAt.Add(time.Second)
	raw := []byte(`{"format":{"duration":"12.5"}}`)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_probe",
		12.5,
		"wav",
		"pcm_s16le",
		16000,
		1,
		256000,
		raw,
		probedAt,
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	probe, err := store.UpsertAudioProbe(UpsertAudioProbeInput{
		RecordingID:     "rec_probe",
		DurationSeconds: 12.5,
		FormatName:      "wav",
		CodecName:       "pcm_s16le",
		SampleRate:      16000,
		Channels:        1,
		BitRate:         256000,
		RawProbeJSON:    raw,
		ProbedAt:        probedAt,
	})
	if err != nil {
		t.Fatalf("UpsertAudioProbe returned error: %v", err)
	}

	if probe.RecordingID != "rec_probe" || probe.FormatName != "wav" || probe.CodecName != "pcm_s16le" {
		t.Fatalf("probe = %+v, want persisted audio probe", probe)
	}
	if probe.DurationSeconds != 12.5 || probe.SampleRate != 16000 || probe.Channels != 1 || probe.BitRate != 256000 {
		t.Fatalf("probe numeric fields = %+v, want persisted values", probe)
	}
	if string(probe.RawProbeJSON) != string(raw) {
		t.Fatalf("RawProbeJSON = %s, want %s", probe.RawProbeJSON, raw)
	}
	if got, want := len(db.calls), 1; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "insert into recording_audio_probes") || !strings.Contains(query, "on conflict") || !strings.Contains(query, "returning") {
		t.Fatalf("query = %q, want insert into recording_audio_probes on conflict returning", db.calls[0].query)
	}
	if got, want := len(db.calls[0].args), 11; got != want {
		t.Fatalf("upsert args = %d, want %d", got, want)
	}
	if got, want := db.calls[0].args[0], "rec_probe"; got != want {
		t.Fatalf("recording_id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[7], raw; string(got.([]byte)) != string(want) {
		t.Fatalf("raw json arg = %s, want %s", got, want)
	}
}

func TestPostgresStoreGetAudioProbeReturnsExistingProbe(t *testing.T) {
	probedAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	createdAt := probedAt
	updatedAt := probedAt.Add(time.Second)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_probe",
		12.5,
		"wav",
		"pcm_s16le",
		16000,
		1,
		256000,
		[]byte(`{"format":{"duration":"12.5"}}`),
		probedAt,
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	probe, ok := store.GetAudioProbe("rec_probe")
	if !ok {
		t.Fatal("GetAudioProbe(rec_probe) ok = false, want true")
	}
	if probe.RecordingID != "rec_probe" || probe.FormatName != "wav" || probe.CodecName != "pcm_s16le" {
		t.Fatalf("probe = %+v, want persisted audio probe", probe)
	}
	if got, want := len(db.calls), 1; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "select") || !strings.Contains(query, "from recording_audio_probes") {
		t.Fatalf("query = %q, want select from recording_audio_probes", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "rec_probe"; got != want {
		t.Fatalf("recording_id arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreGetAudioProbeReturnsFalseForMissingRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	_, ok := store.GetAudioProbe("rec_missing")
	if ok {
		t.Fatal("GetAudioProbe(rec_missing) ok = true, want false")
	}
}

func TestPostgresStoreUpsertTranscriptInsertsOrUpdatesTranscript(t *testing.T) {
	transcribedAt := time.Date(2026, 6, 6, 2, 3, 4, 0, time.UTC)
	createdAt := transcribedAt
	updatedAt := transcribedAt.Add(time.Second)
	raw := []byte(`{"text":"hello world"}`)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_transcript",
		"fake_transcription",
		"fake-whisper-v1",
		"en",
		"hello world",
		raw,
		transcribedAt,
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	transcript, err := store.UpsertTranscript(UpsertTranscriptInput{
		RecordingID:   "rec_transcript",
		Provider:      "fake_transcription",
		Model:         "fake-whisper-v1",
		Language:      "en",
		Text:          "hello world",
		RawResultJSON: raw,
		TranscribedAt: transcribedAt,
		Segments:      []UpsertTranscriptSegmentInput{{SegmentIndex: 0, Text: "hello world"}},
	})
	if err != nil {
		t.Fatalf("UpsertTranscript returned error: %v", err)
	}
	if transcript.RecordingID != "rec_transcript" || transcript.Provider != "fake_transcription" || transcript.Text != "hello world" {
		t.Fatalf("transcript = %+v, want persisted transcript", transcript)
	}
	if string(transcript.RawResultJSON) != string(raw) {
		t.Fatalf("RawResultJSON = %s, want %s", transcript.RawResultJSON, raw)
	}
	if got, want := len(db.calls), 3; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "insert into recording_transcripts") || !strings.Contains(query, "on conflict") || !strings.Contains(query, "returning") {
		t.Fatalf("query = %q, want insert into recording_transcripts on conflict returning", db.calls[0].query)
	}
	if got, want := len(db.calls[0].args), 9; got != want {
		t.Fatalf("transcript upsert args = %d, want %d", got, want)
	}
	if got, want := db.calls[0].args[0], "rec_transcript"; got != want {
		t.Fatalf("recording_id arg = %q, want %q", got, want)
	}
	if !strings.Contains(strings.ToLower(db.calls[1].query), "delete from recording_transcript_segments") {
		t.Fatalf("second query = %q, want delete transcript segments", db.calls[1].query)
	}
	if !strings.Contains(strings.ToLower(db.calls[2].query), "insert into recording_transcript_segments") {
		t.Fatalf("third query = %q, want insert transcript segment", db.calls[2].query)
	}
}

func TestPostgresStoreGetTranscriptReturnsExistingTranscript(t *testing.T) {
	transcribedAt := time.Date(2026, 6, 6, 2, 3, 4, 0, time.UTC)
	createdAt := transcribedAt
	updatedAt := transcribedAt.Add(time.Second)
	raw := []byte(`{"text":"hello world"}`)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_transcript",
		"fake_transcription",
		"fake-whisper-v1",
		"en",
		"hello world",
		raw,
		transcribedAt,
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	transcript, ok := store.GetTranscript("rec_transcript")
	if !ok {
		t.Fatal("GetTranscript(rec_transcript) ok = false, want true")
	}
	if transcript.RecordingID != "rec_transcript" || transcript.Provider != "fake_transcription" || transcript.Text != "hello world" {
		t.Fatalf("transcript = %+v, want persisted transcript", transcript)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "select") || !strings.Contains(query, "from recording_transcripts") {
		t.Fatalf("query = %q, want select from recording_transcripts", db.calls[0].query)
	}
}

func TestPostgresStoreGetTranscriptReturnsFalseForMissingRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)
	_, ok := store.GetTranscript("rec_missing")
	if ok {
		t.Fatal("GetTranscript(rec_missing) ok = true, want false")
	}
}

func TestPostgresStoreListTranscriptSegmentsReturnsExistingSegments(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 2, 3, 4, 0, time.UTC)
	db := newPostgresExecutorSpy(
		postgresRow("rec_transcript-seg-000000", "rec_transcript", 0, 0, 1200, "speaker_1", "hello", 0.95, createdAt),
		postgresRow("rec_transcript-seg-000001", "rec_transcript", 1, 1200, 2400, "speaker_1", "world", 0.96, createdAt),
		postgresErrorRow(sql.ErrNoRows),
	)
	store := NewPostgresStore(db)

	segments := store.ListTranscriptSegments("rec_transcript")
	if got, want := len(segments), 2; got != want {
		t.Fatalf("segments = %d, want %d", got, want)
	}
	if segments[0].ID != "rec_transcript-seg-000000" || segments[0].Text != "hello" {
		t.Fatalf("first segment = %+v, want persisted first segment", segments[0])
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "from recording_transcript_segments") || !strings.Contains(query, "segment_index") {
		t.Fatalf("query = %q, want select transcript segments ordered by segment_index", db.calls[0].query)
	}
}

func TestPostgresStoreUpsertSummaryInsertsOrUpdatesSummary(t *testing.T) {
	summarizedAt := time.Date(2026, 6, 6, 3, 4, 5, 0, time.UTC)
	createdAt := summarizedAt
	updatedAt := summarizedAt.Add(time.Second)
	raw := []byte(`{"overview":"weekly sync summary"}`)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_summary",
		"fake_llm",
		"fake-summary-v1",
		domain.WorkflowTypeMeeting,
		"Weekly sync",
		"weekly sync summary",
		"# Weekly sync\n\n- hello world",
		raw,
		summarizedAt,
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	summary, err := store.UpsertSummary(UpsertSummaryInput{
		RecordingID:     "rec_summary",
		Provider:        "fake_llm",
		Model:           "fake-summary-v1",
		Type:            domain.WorkflowTypeMeeting,
		Title:           "Weekly sync",
		Overview:        "weekly sync summary",
		ContentMarkdown: "# Weekly sync\n\n- hello world",
		RawResultJSON:   raw,
		SummarizedAt:    summarizedAt,
	})
	if err != nil {
		t.Fatalf("UpsertSummary returned error: %v", err)
	}
	if summary.RecordingID != "rec_summary" || summary.Provider != "fake_llm" || summary.Overview != "weekly sync summary" {
		t.Fatalf("summary = %+v, want persisted summary", summary)
	}
	if string(summary.RawResultJSON) != string(raw) {
		t.Fatalf("RawResultJSON = %s, want %s", summary.RawResultJSON, raw)
	}
	if got, want := len(db.calls), 1; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "insert into recording_summaries") || !strings.Contains(query, "on conflict") || !strings.Contains(query, "returning") {
		t.Fatalf("query = %q, want insert into recording_summaries on conflict returning", db.calls[0].query)
	}
	if got, want := len(db.calls[0].args), 11; got != want {
		t.Fatalf("summary upsert args = %d, want %d", got, want)
	}
}

func TestPostgresStoreGetSummaryReturnsExistingSummary(t *testing.T) {
	summarizedAt := time.Date(2026, 6, 6, 3, 4, 5, 0, time.UTC)
	createdAt := summarizedAt
	updatedAt := summarizedAt.Add(time.Second)
	raw := []byte(`{"overview":"weekly sync summary"}`)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_summary",
		"fake_llm",
		"fake-summary-v1",
		domain.WorkflowTypeMeeting,
		"Weekly sync",
		"weekly sync summary",
		"# Weekly sync\n\n- hello world",
		raw,
		summarizedAt,
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	summary, ok := store.GetSummary("rec_summary")
	if !ok {
		t.Fatal("GetSummary(rec_summary) ok = false, want true")
	}
	if summary.RecordingID != "rec_summary" || summary.Provider != "fake_llm" || summary.Overview != "weekly sync summary" {
		t.Fatalf("summary = %+v, want persisted summary", summary)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "select") || !strings.Contains(query, "from recording_summaries") {
		t.Fatalf("query = %q, want select from recording_summaries", db.calls[0].query)
	}
}

func TestPostgresStoreGetSummaryReturnsFalseForMissingRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)
	_, ok := store.GetSummary("rec_missing")
	if ok {
		t.Fatal("GetSummary(rec_missing) ok = true, want false")
	}
}
