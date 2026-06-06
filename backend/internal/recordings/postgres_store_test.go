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
