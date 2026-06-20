package recordings

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	storedb "github.com/zzyhdu/soniq/backend/internal/db"
	"github.com/zzyhdu/soniq/backend/internal/domain"
)

type postgresQueryCall struct {
	query string
	args  []any
}

type postgresExecutorSpy struct {
	calls     []postgresQueryCall
	rows      []*postgresRowStub
	queryErr  error
	beginErr  error
	commits   int
	rollbacks int
}

func newPostgresExecutorSpy(rows ...*postgresRowStub) *postgresExecutorSpy {
	return &postgresExecutorSpy{rows: rows}
}

func (s *postgresExecutorSpy) QueryRow(ctx context.Context, query string, args ...any) storedb.PostgresRow {
	s.calls = append(s.calls, postgresQueryCall{query: query, args: append([]any(nil), args...)})
	if len(s.rows) == 0 {
		return &postgresRowStub{err: sql.ErrNoRows}
	}
	row := s.rows[0]
	s.rows = s.rows[1:]
	return row
}

func (s *postgresExecutorSpy) Query(ctx context.Context, query string, args ...any) (storedb.PostgresRows, error) {
	s.calls = append(s.calls, postgresQueryCall{query: query, args: append([]any(nil), args...)})
	if s.queryErr != nil {
		return nil, s.queryErr
	}
	rows := s.rows
	s.rows = nil
	return &postgresRowsStub{rows: rows}, nil
}

func (s *postgresExecutorSpy) Begin(ctx context.Context) (storedb.PostgresTx, error) {
	s.calls = append(s.calls, postgresQueryCall{query: "BEGIN"})
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	return &postgresTxSpy{executor: s}, nil
}

type postgresTxSpy struct {
	executor *postgresExecutorSpy
}

func (tx *postgresTxSpy) QueryRow(ctx context.Context, query string, args ...any) storedb.PostgresRow {
	return tx.executor.QueryRow(ctx, query, args...)
}

func (tx *postgresTxSpy) Query(ctx context.Context, query string, args ...any) (storedb.PostgresRows, error) {
	return tx.executor.Query(ctx, query, args...)
}

func (tx *postgresTxSpy) Exec(ctx context.Context, query string, args ...any) error {
	tx.executor.calls = append(tx.executor.calls, postgresQueryCall{query: query, args: append([]any(nil), args...)})
	return nil
}

func (tx *postgresTxSpy) Commit(ctx context.Context) error {
	tx.executor.calls = append(tx.executor.calls, postgresQueryCall{query: "COMMIT"})
	tx.executor.commits++
	return nil
}

func (tx *postgresTxSpy) Rollback(ctx context.Context) error {
	tx.executor.calls = append(tx.executor.calls, postgresQueryCall{query: "ROLLBACK"})
	tx.executor.rollbacks++
	return nil
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
	values := r.values
	if len(dest) == 16 && len(values) == 11 {
		values = append(append(append([]any{}, values[:9]...), "", nil, nil, nil, ""), values[9:]...)
	}
	if len(dest) == 16 && len(values) == 14 {
		values = append(append(append([]any{}, values[:12]...), nil, ""), values[12:]...)
	}
	if len(dest) == 14 && len(values) == 11 {
		values = append(append(append([]any{}, values[:9]...), "", nil, nil), values[9:]...)
	}
	if len(dest) != len(values) {
		return sql.ErrNoRows
	}
	for i, value := range values {
		switch target := dest[i].(type) {
		case *string:
			*target = value.(string)
		case *domain.RecordingStatus:
			*target = value.(domain.RecordingStatus)
		case *domain.WorkflowType:
			*target = value.(domain.WorkflowType)
		case *time.Time:
			*target = value.(time.Time)
		case *sql.NullTime:
			if value == nil {
				*target = sql.NullTime{}
			} else {
				*target = sql.NullTime{Time: value.(time.Time), Valid: true}
			}
		case *sql.NullString:
			if value == nil {
				*target = sql.NullString{}
			} else {
				*target = sql.NullString{String: value.(string), Valid: true}
			}
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

type postgresRowsStub struct {
	rows   []*postgresRowStub
	index  int
	closed bool
}

func (r *postgresRowsStub) Close() {
	r.closed = true
}

func (r *postgresRowsStub) Next() bool {
	return r.index < len(r.rows)
}

func (r *postgresRowsStub) Scan(dest ...any) error {
	if r.index >= len(r.rows) {
		return sql.ErrNoRows
	}
	row := r.rows[r.index]
	r.index++
	return row.Scan(dest...)
}

func (r *postgresRowsStub) Err() error {
	return nil
}

func queriesFromCalls(calls []postgresQueryCall) []string {
	queries := make([]string, 0, len(calls))
	for _, call := range calls {
		queries = append(queries, call.query)
	}
	return queries
}

func TestPostgresStoreCreateInsertsRecording(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"wsp_default",
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
		WorkspaceID:  "wsp_default",
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
	if recording.WorkspaceID != "wsp_default" {
		t.Fatalf("recording.WorkspaceID = %q, want wsp_default", recording.WorkspaceID)
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
	if got, want := len(db.calls[0].args), 11; got != want {
		t.Fatalf("insert args = %d, want %d", got, want)
	}
	if id, ok := db.calls[0].args[0].(string); !ok || !strings.HasPrefix(id, "rec_") {
		t.Fatalf("first insert arg = %#v, want generated rec_ id", db.calls[0].args[0])
	}
	if got, want := db.calls[0].args[1], "wsp_default"; got != want {
		t.Fatalf("workspace id arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreCreatePreservesAudioMetadata(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"wsp_default",
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
		WorkspaceID:      "wsp_default",
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
	if got, want := len(db.calls[0].args), 11; got != want {
		t.Fatalf("insert args = %d, want %d", got, want)
	}
	if got, want := db.calls[0].args[6], "recordings/rec_pg/original.wav"; got != want {
		t.Fatalf("audio object key arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[7], "audio/wav"; got != want {
		t.Fatalf("audio content type arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[8], int64(12345); got != want {
		t.Fatalf("audio size arg = %v, want %v", got, want)
	}
}

func TestPostgresStoreGetReturnsExistingRecording(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"wsp_default",
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

	recording, ok, err := store.Get("rec_pg")
	if err != nil {
		t.Fatalf("Get(rec_pg) returned error: %v", err)
	}
	if !ok {
		t.Fatal("Get(rec_pg) ok = false, want true")
	}
	if recording.ID != "rec_pg" || recording.Title != "Lecture 1" || recording.WorkflowType != domain.WorkflowTypeLecture {
		t.Fatalf("recording = %+v, want persisted recording", recording)
	}
	if recording.WorkspaceID != "wsp_default" {
		t.Fatalf("recording.WorkspaceID = %q, want wsp_default", recording.WorkspaceID)
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

	_, ok, err := store.Get("rec_missing")
	if err != nil {
		t.Fatalf("Get(rec_missing) returned error: %v", err)
	}
	if ok {
		t.Fatal("Get(rec_missing) ok = true, want false")
	}
}

func TestPostgresStoreGetReturnsDatabaseErrors(t *testing.T) {
	dbErr := errors.New("database unavailable")
	db := newPostgresExecutorSpy(postgresErrorRow(dbErr))
	store := NewPostgresStore(db)

	_, ok, err := store.Get("rec_db_error")
	if ok {
		t.Fatal("Get(rec_db_error) ok = true, want false")
	}
	if !errors.Is(err, dbErr) {
		t.Fatalf("Get error = %v, want wrapped database error", err)
	}
}

func TestPostgresStoreCreateRejectsInvalidWorkflowTypeBeforeInsert(t *testing.T) {
	db := newPostgresExecutorSpy()
	store := NewPostgresStore(db)

	_, err := store.Create(CreateRecordingInput{
		WorkspaceID:  "wsp_default",
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

func TestPostgresStoreCreateRejectsMissingWorkspaceIDBeforeInsert(t *testing.T) {
	db := newPostgresExecutorSpy()
	store := NewPostgresStore(db)

	_, err := store.Create(CreateRecordingInput{
		Title:        "Weekly sync",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	})
	if err == nil {
		t.Fatal("Create returned nil error, want missing workspace id error")
	}
	if got, want := len(db.calls), 0; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
}

func TestPostgresStoreGetForWorkspaceReturnsExistingRecording(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"wsp_default",
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

	recording, ok, err := store.GetForWorkspace(GetRecordingInput{
		WorkspaceID: "wsp_default",
		ID:          "rec_pg",
	})
	if err != nil {
		t.Fatalf("GetForWorkspace returned error: %v", err)
	}
	if !ok {
		t.Fatal("GetForWorkspace ok = false, want true")
	}
	if recording.ID != "rec_pg" || recording.WorkspaceID != "wsp_default" {
		t.Fatalf("recording = %+v, want workspace-scoped recording", recording)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "where workspace_id = $1") || !strings.Contains(query, "and id = $2") || !strings.Contains(query, "deleted_at is null") {
		t.Fatalf("query = %q, want workspace-scoped get", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "wsp_default"; got != want {
		t.Fatalf("workspace id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[1], "rec_pg"; got != want {
		t.Fatalf("recording id arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreGetForWorkspaceReturnsFalseForCrossWorkspaceRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	_, ok, err := store.GetForWorkspace(GetRecordingInput{
		WorkspaceID: "wsp_default",
		ID:          "rec_other",
	})
	if err != nil {
		t.Fatalf("GetForWorkspace returned error: %v", err)
	}
	if ok {
		t.Fatal("GetForWorkspace ok = true, want false")
	}
}

func TestPostgresStoreListByWorkspaceReturnsRecentRecordings(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	db := newPostgresExecutorSpy(
		postgresRow("rec_new", "wsp_default", "New", domain.RecordingStatusCompleted, domain.WorkflowTypeMeeting, "en", "", "", int64(0), createdAt.Add(time.Hour), createdAt.Add(time.Hour)),
		postgresRow("rec_old", "wsp_default", "Old", domain.RecordingStatusUploaded, domain.WorkflowTypeMemo, "zh", "", "", int64(0), createdAt, createdAt),
	)
	store := NewPostgresStore(db)

	recordings, err := store.ListByWorkspace(ListRecordingsInput{WorkspaceID: "wsp_default", Limit: 10})
	if err != nil {
		t.Fatalf("ListByWorkspace returned error: %v", err)
	}
	if got, want := len(recordings), 2; got != want {
		t.Fatalf("recordings = %d, want %d", got, want)
	}
	if recordings[0].ID != "rec_new" || recordings[1].ID != "rec_old" {
		t.Fatalf("recordings = %+v, want returned ordering", recordings)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "where workspace_id = $1") || !strings.Contains(query, "deleted_at is null") || !strings.Contains(query, "order by created_at desc") || !strings.Contains(query, "limit $2") {
		t.Fatalf("query = %q, want workspace list query", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "wsp_default"; got != want {
		t.Fatalf("workspace id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[1], 10; got != want {
		t.Fatalf("limit arg = %v, want %v", got, want)
	}
}

func TestPostgresStoreListByWorkspaceClampsDefaultAndMaxLimit(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		wantLimit int
	}{
		{name: "default", wantLimit: 50},
		{name: "max", limit: 500, wantLimit: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newPostgresExecutorSpy()
			store := NewPostgresStore(db)

			if _, err := store.ListByWorkspace(ListRecordingsInput{WorkspaceID: "wsp_default", Limit: tt.limit}); err != nil {
				t.Fatalf("ListByWorkspace returned error: %v", err)
			}
			if got, want := db.calls[0].args[1], tt.wantLimit; got != want {
				t.Fatalf("limit arg = %v, want %v", got, want)
			}
		})
	}
}

func TestPostgresStoreUpdateForWorkspaceRenamesRecording(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"wsp_default",
		"Customer interview",
		domain.RecordingStatusCompleted,
		domain.WorkflowTypeMeeting,
		"en",
		"recordings/rec_pg/original.wav",
		"audio/wav",
		int64(12345),
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	recording, ok, err := store.UpdateForWorkspace(UpdateRecordingInput{
		WorkspaceID: "wsp_default",
		ID:          "rec_pg",
		Title:       " Customer interview ",
	})
	if err != nil {
		t.Fatalf("UpdateForWorkspace returned error: %v", err)
	}
	if !ok {
		t.Fatal("UpdateForWorkspace ok = false, want true")
	}
	if recording.ID != "rec_pg" || recording.Title != "Customer interview" {
		t.Fatalf("recording = %+v, want renamed rec_pg", recording)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "update recordings") || !strings.Contains(query, "set title") || !strings.Contains(query, "workspace_id = $1") || !strings.Contains(query, "deleted_at is null") || !strings.Contains(query, "returning") {
		t.Fatalf("query = %q, want workspace-scoped title update returning", db.calls[0].query)
	}
	if got, want := len(db.calls[0].args), 4; got != want {
		t.Fatalf("update args = %d, want %d", got, want)
	}
	if got, want := db.calls[0].args[0], "wsp_default"; got != want {
		t.Fatalf("workspace id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[1], "rec_pg"; got != want {
		t.Fatalf("id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[2], "Customer interview"; got != want {
		t.Fatalf("title arg = %q, want %q", got, want)
	}
	if _, ok := db.calls[0].args[3].(time.Time); !ok {
		t.Fatalf("updated_at arg = %#v, want time.Time", db.calls[0].args[3])
	}
}

func TestPostgresStoreUpdateForWorkspaceReturnsFalseForMissingRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	_, ok, err := store.UpdateForWorkspace(UpdateRecordingInput{
		WorkspaceID: "wsp_default",
		ID:          "rec_missing",
		Title:       "Renamed",
	})
	if err != nil {
		t.Fatalf("UpdateForWorkspace returned error: %v", err)
	}
	if ok {
		t.Fatal("UpdateForWorkspace ok = true, want false")
	}
}

func TestPostgresStoreUpdateForWorkspaceValidatesInput(t *testing.T) {
	db := newPostgresExecutorSpy()
	store := NewPostgresStore(db)

	_, _, err := store.UpdateForWorkspace(UpdateRecordingInput{
		WorkspaceID: "wsp_default",
		ID:          "rec_pg",
		Title:       "  ",
	})
	if err == nil {
		t.Fatal("UpdateForWorkspace returned nil error, want validation error")
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
		"wsp_default",
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
		WorkspaceID: "wsp_default",
		ID:          "rec_pg",
		Status:      domain.RecordingStatusProcessing,
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
	if !strings.Contains(query, "update recordings") || !strings.Contains(query, "set status") || !strings.Contains(query, "deleted_at is null") || !strings.Contains(query, "returning") {
		t.Fatalf("query = %q, want update recordings set status returning", db.calls[0].query)
	}
	if got, want := len(db.calls[0].args), 5; got != want {
		t.Fatalf("update args = %d, want %d", got, want)
	}
	if got, want := db.calls[0].args[0], "wsp_default"; got != want {
		t.Fatalf("workspace id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[1], "rec_pg"; got != want {
		t.Fatalf("id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[2], domain.RecordingStatusProcessing; got != want {
		t.Fatalf("status arg = %q, want %q", got, want)
	}
	if _, ok := db.calls[0].args[3].(time.Time); !ok {
		t.Fatalf("updated_at arg = %#v, want time.Time", db.calls[0].args[3])
	}
	if got, want := db.calls[0].args[4], ""; got != want {
		t.Fatalf("failure reason arg = %q, want empty", got)
	}
}

func TestPostgresStoreUpdateStatusPersistsFailureMetadata(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	failedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"wsp_default",
		"Weekly sync",
		domain.RecordingStatusFailed,
		domain.WorkflowTypeMeeting,
		"en",
		"recordings/rec_pg/original.wav",
		"audio/wav",
		int64(12345),
		"transcribe audio: provider failed",
		nil,
		failedAt,
		createdAt,
		failedAt,
	))
	store := NewPostgresStore(db)

	recording, err := store.UpdateStatus(UpdateRecordingStatusInput{
		WorkspaceID:   "wsp_default",
		ID:            "rec_pg",
		Status:        domain.RecordingStatusFailed,
		FailureReason: "transcribe audio: provider failed",
	})
	if err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}

	if recording.Status != domain.RecordingStatusFailed || recording.FailureReason != "transcribe audio: provider failed" {
		t.Fatalf("recording = %+v, want failed with reason", recording)
	}
	if recording.FailedAt == nil || !recording.FailedAt.Equal(failedAt) {
		t.Fatalf("FailedAt = %v, want %s", recording.FailedAt, failedAt)
	}
	if recording.CompletedAt != nil {
		t.Fatalf("CompletedAt = %v, want nil", recording.CompletedAt)
	}
	if got, want := db.calls[0].args[4], "transcribe audio: provider failed"; got != want {
		t.Fatalf("failure reason arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreResetForRetryClearsFailureMetadata(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"wsp_default",
		"Weekly sync",
		domain.RecordingStatusUploaded,
		domain.WorkflowTypeMeeting,
		"en",
		"recordings/rec_pg/original.wav",
		"audio/wav",
		int64(12345),
		"",
		nil,
		nil,
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	recording, err := store.ResetForRetry(RetryRecordingInput{WorkspaceID: "wsp_default", ID: "rec_pg"})
	if err != nil {
		t.Fatalf("ResetForRetry returned error: %v", err)
	}

	if recording.Status != domain.RecordingStatusUploaded || recording.FailureReason != "" || recording.FailedAt != nil || recording.CompletedAt != nil {
		t.Fatalf("recording = %+v, want uploaded with cleared failure metadata", recording)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "status = 'failed'") || !strings.Contains(query, "failure_reason = ''") || !strings.Contains(query, "deleted_at is null") {
		t.Fatalf("query = %q, want failed-only reset that clears failure metadata", db.calls[0].query)
	}
	if got, want := len(db.calls[0].args), 3; got != want {
		t.Fatalf("reset args = %d, want %d", got, want)
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

func TestPostgresStoreSoftDeleteForWorkspaceMarksRecordingDeleted(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	deletedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"wsp_default",
		"Weekly sync",
		domain.RecordingStatusCompleted,
		domain.WorkflowTypeMeeting,
		"en",
		"recordings/rec_pg/original.wav",
		"audio/wav",
		int64(12345),
		"",
		nil,
		nil,
		deletedAt,
		"usr_dev",
		createdAt,
		deletedAt,
	))
	store := NewPostgresStore(db)

	recording, ok, err := store.SoftDeleteForWorkspace(SoftDeleteRecordingInput{
		WorkspaceID:     "wsp_default",
		ID:              "rec_pg",
		DeletedByUserID: "usr_dev",
	})
	if err != nil {
		t.Fatalf("SoftDeleteForWorkspace returned error: %v", err)
	}
	if !ok {
		t.Fatal("SoftDeleteForWorkspace ok = false, want true")
	}
	if recording.DeletedAt == nil || !recording.DeletedAt.Equal(deletedAt) {
		t.Fatalf("DeletedAt = %v, want %s", recording.DeletedAt, deletedAt)
	}
	if recording.DeletedByUserID != "usr_dev" {
		t.Fatalf("DeletedByUserID = %q, want usr_dev", recording.DeletedByUserID)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "update recordings") || !strings.Contains(query, "set deleted_at") || !strings.Contains(query, "deleted_by_user_id") || !strings.Contains(query, "deleted_at is null") {
		t.Fatalf("query = %q, want soft delete update", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "wsp_default"; got != want {
		t.Fatalf("workspace id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[1], "rec_pg"; got != want {
		t.Fatalf("recording id arg = %q, want %q", got, want)
	}
	if _, ok := db.calls[0].args[2].(time.Time); !ok {
		t.Fatalf("deleted_at arg = %#v, want time.Time", db.calls[0].args[2])
	}
	if got, want := db.calls[0].args[3], "usr_dev"; got != want {
		t.Fatalf("deleted_by arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreSoftDeleteForWorkspaceReturnsFalseForMissingRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	_, ok, err := store.SoftDeleteForWorkspace(SoftDeleteRecordingInput{
		WorkspaceID:     "wsp_default",
		ID:              "rec_missing",
		DeletedByUserID: "usr_dev",
	})
	if err != nil {
		t.Fatalf("SoftDeleteForWorkspace returned error: %v", err)
	}
	if ok {
		t.Fatal("SoftDeleteForWorkspace ok = true, want false")
	}
}

func TestPostgresStoreListDeletedByWorkspaceReturnsDeletedRecordings(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	deletedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(
		postgresRow(
			"rec_deleted",
			"wsp_default",
			"Deleted",
			domain.RecordingStatusCompleted,
			domain.WorkflowTypeMeeting,
			"en",
			"recordings/rec_deleted/original.wav",
			"audio/wav",
			int64(12345),
			"",
			nil,
			nil,
			deletedAt,
			"usr_dev",
			createdAt,
			deletedAt,
		),
	)
	store := NewPostgresStore(db)

	recordings, err := store.ListDeletedByWorkspace(ListDeletedRecordingsInput{WorkspaceID: "wsp_default", Limit: 10})
	if err != nil {
		t.Fatalf("ListDeletedByWorkspace returned error: %v", err)
	}
	if got, want := len(recordings), 1; got != want {
		t.Fatalf("recordings = %d, want %d", got, want)
	}
	if recordings[0].ID != "rec_deleted" {
		t.Fatalf("recording ID = %q, want rec_deleted", recordings[0].ID)
	}
	if recordings[0].DeletedAt == nil || !recordings[0].DeletedAt.Equal(deletedAt) {
		t.Fatalf("DeletedAt = %v, want %s", recordings[0].DeletedAt, deletedAt)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "where workspace_id = $1") || !strings.Contains(query, "deleted_at is not null") || !strings.Contains(query, "order by deleted_at desc") || !strings.Contains(query, "limit $2") {
		t.Fatalf("query = %q, want deleted workspace list query", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "wsp_default"; got != want {
		t.Fatalf("workspace id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[1], 10; got != want {
		t.Fatalf("limit arg = %v, want %v", got, want)
	}
}

func TestPostgresStoreRestoreForWorkspaceClearsDeletionMetadata(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	restoredAt := createdAt.Add(2 * time.Minute)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_pg",
		"wsp_default",
		"Weekly sync",
		domain.RecordingStatusCompleted,
		domain.WorkflowTypeMeeting,
		"en",
		"recordings/rec_pg/original.wav",
		"audio/wav",
		int64(12345),
		"",
		nil,
		nil,
		nil,
		nil,
		createdAt,
		restoredAt,
	))
	store := NewPostgresStore(db)

	recording, ok, err := store.RestoreForWorkspace(RestoreRecordingInput{
		WorkspaceID: "wsp_default",
		ID:          "rec_pg",
	})
	if err != nil {
		t.Fatalf("RestoreForWorkspace returned error: %v", err)
	}
	if !ok {
		t.Fatal("RestoreForWorkspace ok = false, want true")
	}
	if recording.DeletedAt != nil || recording.DeletedByUserID != "" {
		t.Fatalf("restored deletion metadata = %v/%q, want cleared", recording.DeletedAt, recording.DeletedByUserID)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "update recordings") || !strings.Contains(query, "set deleted_at = null") || !strings.Contains(query, "deleted_by_user_id = null") || !strings.Contains(query, "deleted_at is not null") {
		t.Fatalf("query = %q, want restore update", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "wsp_default"; got != want {
		t.Fatalf("workspace id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[1], "rec_pg"; got != want {
		t.Fatalf("recording id arg = %q, want %q", got, want)
	}
	if _, ok := db.calls[0].args[2].(time.Time); !ok {
		t.Fatalf("updated_at arg = %#v, want time.Time", db.calls[0].args[2])
	}
}

func TestPostgresStoreRestoreForWorkspaceReturnsFalseForMissingRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	_, ok, err := store.RestoreForWorkspace(RestoreRecordingInput{
		WorkspaceID: "wsp_default",
		ID:          "rec_missing",
	})
	if err != nil {
		t.Fatalf("RestoreForWorkspace returned error: %v", err)
	}
	if ok {
		t.Fatal("RestoreForWorkspace ok = true, want false")
	}
}

func TestPostgresStorePurgeForWorkspaceDeletesRowsAndReturnsArtifacts(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	now := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(
		postgresRow(
			"rec_pg",
			"wsp_default",
			"workspaces/wsp_default/recordings/rec_pg/original.wav",
			"workspaces/wsp_default/recordings/rec_pg/normalized.wav",
		),
		postgresRow(
			purgeArtifactID("rec_pg", "workspaces/wsp_default/recordings/rec_pg/original.wav"),
			"rec_pg",
			"wsp_default",
			"workspaces/wsp_default/recordings/rec_pg/original.wav",
			RecordingPurgeArtifactKindOriginalAudio,
			string(RecordingPurgeArtifactStatusPending),
			0,
			now,
			"",
			now,
			now,
			nil,
		),
		postgresRow(
			purgeArtifactID("rec_pg", "workspaces/wsp_default/recordings/rec_pg/normalized.wav"),
			"rec_pg",
			"wsp_default",
			"workspaces/wsp_default/recordings/rec_pg/normalized.wav",
			RecordingPurgeArtifactKindNormalizedAudio,
			string(RecordingPurgeArtifactStatusPending),
			0,
			now,
			"",
			now,
			now,
			nil,
		),
		postgresRow("rec_pg"),
	)
	store := NewPostgresStore(db)

	result, ok, err := store.PurgeForWorkspace(PurgeRecordingInput{WorkspaceID: "wsp_default", ID: "rec_pg"})
	if err != nil {
		t.Fatalf("PurgeForWorkspace returned error: %v", err)
	}
	if !ok {
		t.Fatal("PurgeForWorkspace ok = false, want true")
	}
	if got, want := len(result.Artifacts), 2; got != want {
		t.Fatalf("artifacts = %d, want %d", got, want)
	}
	if result.Artifacts[0].ObjectKey != "workspaces/wsp_default/recordings/rec_pg/original.wav" {
		t.Fatalf("first artifact = %+v, want original audio artifact", result.Artifacts[0])
	}
	if result.Artifacts[1].ObjectKey != "workspaces/wsp_default/recordings/rec_pg/normalized.wav" {
		t.Fatalf("second artifact = %+v, want normalized audio artifact", result.Artifacts[1])
	}

	if db.commits != 1 || db.rollbacks != 0 {
		t.Fatalf("commits/rollbacks = %d/%d, want 1/0", db.commits, db.rollbacks)
	}
	joinedQueries := strings.ToLower(strings.Join(queriesFromCalls(db.calls), "\n"))
	for _, want := range []string{
		"for update of r",
		"insert into recording_purge_artifacts",
		"delete from recording_mind_maps",
		"delete from recording_transcript_segments",
		"delete from recording_summaries",
		"delete from recording_transcripts",
		"delete from recording_audio_probes",
		"delete from recording_normalized_audios",
		"delete from recordings",
	} {
		if !strings.Contains(joinedQueries, want) {
			t.Fatalf("queries = %q, want %q", joinedQueries, want)
		}
	}
}

func TestPostgresStorePurgeForWorkspaceReturnsFalseForMissingOrActiveRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	_, ok, err := store.PurgeForWorkspace(PurgeRecordingInput{WorkspaceID: "wsp_default", ID: "rec_missing"})
	if err != nil {
		t.Fatalf("PurgeForWorkspace returned error: %v", err)
	}
	if ok {
		t.Fatal("PurgeForWorkspace ok = true, want false")
	}
	if db.commits != 0 || db.rollbacks != 1 {
		t.Fatalf("commits/rollbacks = %d/%d, want 0/1", db.commits, db.rollbacks)
	}
	query := strings.ToLower(db.calls[1].query)
	if !strings.Contains(query, "deleted_at is not null") {
		t.Fatalf("query = %q, want soft-deleted-only purge", db.calls[1].query)
	}
}

func TestPostgresStoreClaimPurgeArtifactsClaimsRetryableRows(t *testing.T) {
	now := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	db := newPostgresExecutorSpy(postgresRow(
		"rpa_1",
		"rec_pg",
		"wsp_default",
		"workspaces/wsp_default/recordings/rec_pg/original.wav",
		RecordingPurgeArtifactKindOriginalAudio,
		string(RecordingPurgeArtifactStatusDeleting),
		1,
		now,
		"previous failure",
		now.Add(-time.Hour),
		now,
		nil,
	))
	store := NewPostgresStore(db)

	artifacts, err := store.ClaimPurgeArtifacts(ClaimPurgeArtifactsInput{Limit: 10})
	if err != nil {
		t.Fatalf("ClaimPurgeArtifacts returned error: %v", err)
	}
	if got, want := len(artifacts), 1; got != want {
		t.Fatalf("artifacts = %d, want %d", got, want)
	}
	if artifacts[0].ID != "rpa_1" || artifacts[0].Status != RecordingPurgeArtifactStatusDeleting {
		t.Fatalf("artifact = %+v, want claimed artifact", artifacts[0])
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "for update skip locked") || !strings.Contains(query, "status in ('pending', 'failed')") || !strings.Contains(query, "status = 'deleting'") {
		t.Fatalf("query = %q, want retryable claim query", db.calls[0].query)
	}
	if got, want := db.calls[0].args[1], 10; got != want {
		t.Fatalf("limit arg = %v, want %v", got, want)
	}
}

func TestPostgresStoreMarkPurgeArtifactDeleted(t *testing.T) {
	db := newPostgresExecutorSpy(postgresRow("rpa_1"))
	store := NewPostgresStore(db)

	ok, err := store.MarkPurgeArtifactDeleted(MarkPurgeArtifactDeletedInput{ID: "rpa_1"})
	if err != nil {
		t.Fatalf("MarkPurgeArtifactDeleted returned error: %v", err)
	}
	if !ok {
		t.Fatal("MarkPurgeArtifactDeleted ok = false, want true")
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "status = 'deleted'") || !strings.Contains(query, "deleted_at = $2") {
		t.Fatalf("query = %q, want mark deleted query", db.calls[0].query)
	}
}

func TestPostgresStoreMarkPurgeArtifactFailed(t *testing.T) {
	nextAttemptAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	db := newPostgresExecutorSpy(postgresRow("rpa_1"))
	store := NewPostgresStore(db)

	ok, err := store.MarkPurgeArtifactFailed(MarkPurgeArtifactFailedInput{
		ID:            "rpa_1",
		LastError:     "delete object: permission denied",
		NextAttemptAt: nextAttemptAt,
	})
	if err != nil {
		t.Fatalf("MarkPurgeArtifactFailed returned error: %v", err)
	}
	if !ok {
		t.Fatal("MarkPurgeArtifactFailed ok = false, want true")
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "status = 'failed'") || !strings.Contains(query, "attempt_count = attempt_count + 1") || !strings.Contains(query, "next_attempt_at = $3") || !strings.Contains(query, "deleted_at is null") {
		t.Fatalf("query = %q, want mark failed query", db.calls[0].query)
	}
	if got, want := db.calls[0].args[2], nextAttemptAt; got != want {
		t.Fatalf("next attempt arg = %v, want %v", got, want)
	}
}

func TestPostgresStoreMarkPurgeArtifactFailedDoesNotOverwriteDeletedArtifact(t *testing.T) {
	nextAttemptAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	ok, err := store.MarkPurgeArtifactFailed(MarkPurgeArtifactFailedInput{
		ID:            "rpa_deleted",
		LastError:     "delete object canceled",
		NextAttemptAt: nextAttemptAt,
	})
	if err != nil {
		t.Fatalf("MarkPurgeArtifactFailed returned error: %v", err)
	}
	if ok {
		t.Fatal("MarkPurgeArtifactFailed ok = true, want false for already deleted artifact")
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "deleted_at is null") {
		t.Fatalf("query = %q, want deleted artifact guard", db.calls[0].query)
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

	probe, ok, err := store.GetAudioProbe("rec_probe")
	if err != nil {
		t.Fatalf("GetAudioProbe returned error: %v", err)
	}
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

	_, ok, err := store.GetAudioProbe("rec_missing")
	if err != nil {
		t.Fatalf("GetAudioProbe returned error: %v", err)
	}
	if ok {
		t.Fatal("GetAudioProbe(rec_missing) ok = true, want false")
	}
}

func TestPostgresStoreGetAudioProbeReturnsErrorForDatabaseFailure(t *testing.T) {
	dbErr := errors.New("database unavailable")
	db := newPostgresExecutorSpy(postgresErrorRow(dbErr))
	store := NewPostgresStore(db)

	_, ok, err := store.GetAudioProbe("rec_error")
	if err == nil {
		t.Fatal("GetAudioProbe returned nil error, want database error")
	}
	if ok {
		t.Fatal("GetAudioProbe(rec_error) ok = true, want false")
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

	transcript, ok, err := store.GetTranscript("rec_transcript")
	if err != nil {
		t.Fatalf("GetTranscript returned error: %v", err)
	}
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
	_, ok, err := store.GetTranscript("rec_missing")
	if err != nil {
		t.Fatalf("GetTranscript returned error: %v", err)
	}
	if ok {
		t.Fatal("GetTranscript(rec_missing) ok = true, want false")
	}
}

func TestPostgresStoreListTranscriptSegmentsReturnsExistingSegments(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 2, 3, 4, 0, time.UTC)
	db := newPostgresExecutorSpy(
		postgresRow("rec_transcript-seg-000000", "rec_transcript", 0, 0, 1200, "speaker_1", "hello", 0.95, createdAt),
		postgresRow("rec_transcript-seg-000002", "rec_transcript", 2, 2400, 3600, "speaker_2", "again", 0.97, createdAt),
	)
	store := NewPostgresStore(db)

	segments, err := store.ListTranscriptSegments("rec_transcript")
	if err != nil {
		t.Fatalf("ListTranscriptSegments returned error: %v", err)
	}
	if got, want := len(segments), 2; got != want {
		t.Fatalf("segments = %d, want %d", got, want)
	}
	if segments[0].ID != "rec_transcript-seg-000000" || segments[0].SegmentIndex != 0 || segments[0].Text != "hello" {
		t.Fatalf("first segment = %+v, want persisted first segment", segments[0])
	}
	if segments[1].ID != "rec_transcript-seg-000002" || segments[1].SegmentIndex != 2 || segments[1].Text != "again" {
		t.Fatalf("second segment = %+v, want non-contiguous persisted segment", segments[1])
	}
	if got, want := len(db.calls), 1; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "from recording_transcript_segments") || !strings.Contains(query, "where recording_id = $1") || !strings.Contains(query, "order by segment_index") {
		t.Fatalf("query = %q, want select transcript segments by recording_id ordered by segment_index", db.calls[0].query)
	}
	if strings.Contains(query, "segment_index = $2") {
		t.Fatalf("query = %q, want no contiguous segment_index lookup", db.calls[0].query)
	}
}

func TestPostgresStoreGetTranscriptReturnsErrorForDatabaseFailure(t *testing.T) {
	dbErr := errors.New("database unavailable")
	db := newPostgresExecutorSpy(postgresErrorRow(dbErr))
	store := NewPostgresStore(db)
	_, ok, err := store.GetTranscript("rec_error")
	if err == nil {
		t.Fatal("GetTranscript returned nil error, want database error")
	}
	if ok {
		t.Fatal("GetTranscript(rec_error) ok = true, want false")
	}
}

func TestPostgresStoreListTranscriptSegmentsReturnsErrorForDatabaseFailure(t *testing.T) {
	dbErr := errors.New("database unavailable")
	db := newPostgresExecutorSpy()
	db.queryErr = dbErr
	store := NewPostgresStore(db)
	segments, err := store.ListTranscriptSegments("rec_error")
	if err == nil {
		t.Fatal("ListTranscriptSegments returned nil error, want database error")
	}
	if segments != nil {
		t.Fatalf("segments = %#v, want nil", segments)
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

	summary, ok, err := store.GetSummary("rec_summary")
	if err != nil {
		t.Fatalf("GetSummary returned error: %v", err)
	}
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
	_, ok, err := store.GetSummary("rec_missing")
	if err != nil {
		t.Fatalf("GetSummary returned error: %v", err)
	}
	if ok {
		t.Fatal("GetSummary(rec_missing) ok = true, want false")
	}
}

func TestPostgresStoreGetSummaryReturnsErrorForDatabaseFailure(t *testing.T) {
	dbErr := errors.New("database unavailable")
	db := newPostgresExecutorSpy(postgresErrorRow(dbErr))
	store := NewPostgresStore(db)
	_, ok, err := store.GetSummary("rec_error")
	if err == nil {
		t.Fatal("GetSummary returned nil error, want database error")
	}
	if ok {
		t.Fatal("GetSummary(rec_error) ok = true, want false")
	}
}

func TestPostgresStoreUpsertMindMapInsertsOrUpdatesMindMap(t *testing.T) {
	generatedAt := time.Date(2026, 6, 6, 4, 5, 6, 0, time.UTC)
	createdAt := generatedAt
	updatedAt := generatedAt.Add(time.Second)
	root := []byte(`{"label":"Weekly sync","children":[{"label":"Launch"}]}`)
	raw := []byte(`{"title":"Weekly sync"}`)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_mind_map",
		"fake_llm",
		"fake-mind-map-v1",
		"Weekly sync",
		root,
		"- Weekly sync\n  - Launch",
		raw,
		generatedAt,
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	mindMap, err := store.UpsertMindMap(UpsertMindMapInput{
		RecordingID:     "rec_mind_map",
		Provider:        "fake_llm",
		Model:           "fake-mind-map-v1",
		Title:           "Weekly sync",
		RootJSON:        root,
		ContentMarkdown: "- Weekly sync\n  - Launch",
		RawResultJSON:   raw,
		GeneratedAt:     generatedAt,
	})
	if err != nil {
		t.Fatalf("UpsertMindMap returned error: %v", err)
	}
	if mindMap.RecordingID != "rec_mind_map" || mindMap.Provider != "fake_llm" || string(mindMap.RootJSON) != string(root) {
		t.Fatalf("mind map = %+v, want persisted mind map", mindMap)
	}
	if got, want := len(db.calls), 1; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "insert into recording_mind_maps") || !strings.Contains(query, "on conflict") || !strings.Contains(query, "returning") {
		t.Fatalf("query = %q, want insert into recording_mind_maps on conflict returning", db.calls[0].query)
	}
	if got, want := len(db.calls[0].args), 10; got != want {
		t.Fatalf("mind map upsert args = %d, want %d", got, want)
	}
}

func TestPostgresStoreGetMindMapReturnsExistingMindMap(t *testing.T) {
	generatedAt := time.Date(2026, 6, 6, 4, 5, 6, 0, time.UTC)
	createdAt := generatedAt
	updatedAt := generatedAt.Add(time.Second)
	root := []byte(`{"label":"Weekly sync","children":[{"label":"Launch"}]}`)
	raw := []byte(`{"title":"Weekly sync"}`)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_mind_map",
		"fake_llm",
		"fake-mind-map-v1",
		"Weekly sync",
		root,
		"- Weekly sync\n  - Launch",
		raw,
		generatedAt,
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	mindMap, ok, err := store.GetMindMap("rec_mind_map")
	if err != nil {
		t.Fatalf("GetMindMap returned error: %v", err)
	}
	if !ok {
		t.Fatal("GetMindMap(rec_mind_map) ok = false, want true")
	}
	if mindMap.RecordingID != "rec_mind_map" || string(mindMap.RootJSON) != string(root) {
		t.Fatalf("mind map = %+v, want persisted mind map", mindMap)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "select") || !strings.Contains(query, "from recording_mind_maps") {
		t.Fatalf("query = %q, want select from recording_mind_maps", db.calls[0].query)
	}
}

func TestPostgresStoreGetMindMapReturnsFalseForMissingRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)
	_, ok, err := store.GetMindMap("rec_missing")
	if err != nil {
		t.Fatalf("GetMindMap returned error: %v", err)
	}
	if ok {
		t.Fatal("GetMindMap(rec_missing) ok = true, want false")
	}
}

func TestPostgresStoreGetMindMapReturnsErrorForDatabaseFailure(t *testing.T) {
	dbErr := errors.New("database unavailable")
	db := newPostgresExecutorSpy(postgresErrorRow(dbErr))
	store := NewPostgresStore(db)
	_, ok, err := store.GetMindMap("rec_error")
	if err == nil {
		t.Fatal("GetMindMap returned nil error, want database error")
	}
	if ok {
		t.Fatal("GetMindMap(rec_error) ok = true, want false")
	}
}

func TestPostgresStoreUpsertNormalizedAudioInsertsOrUpdatesMetadata(t *testing.T) {
	normalizedAt := time.Date(2026, 6, 6, 4, 5, 6, 0, time.UTC)
	createdAt := normalizedAt
	updatedAt := normalizedAt.Add(time.Second)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_normalized",
		"recordings/20260606T150747.170276465Z/normalized.wav",
		"audio/wav",
		int64(32044),
		"wav",
		"pcm_s16le",
		16000,
		1,
		1.25,
		normalizedAt,
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	normalized, err := store.UpsertNormalizedAudio(UpsertNormalizedAudioInput{
		RecordingID:     "rec_normalized",
		ObjectKey:       "recordings/20260606T150747.170276465Z/normalized.wav",
		ContentType:     "audio/wav",
		SizeBytes:       32044,
		FormatName:      "wav",
		CodecName:       "pcm_s16le",
		SampleRate:      16000,
		Channels:        1,
		DurationSeconds: 1.25,
		NormalizedAt:    normalizedAt,
	})
	if err != nil {
		t.Fatalf("UpsertNormalizedAudio returned error: %v", err)
	}
	if normalized.RecordingID != "rec_normalized" || normalized.ObjectKey != "recordings/20260606T150747.170276465Z/normalized.wav" {
		t.Fatalf("normalized audio = %+v, want persisted object metadata", normalized)
	}
	if normalized.ContentType != "audio/wav" || normalized.SizeBytes != 32044 {
		t.Fatalf("normalized content metadata = %+v, want audio/wav size", normalized)
	}
	if normalized.FormatName != "wav" || normalized.CodecName != "pcm_s16le" || normalized.SampleRate != 16000 || normalized.Channels != 1 {
		t.Fatalf("normalized target metadata = %+v, want wav pcm_s16le 16k mono", normalized)
	}
	if normalized.DurationSeconds != 1.25 || !normalized.NormalizedAt.Equal(normalizedAt) {
		t.Fatalf("normalized timing metadata = %+v, want duration and normalized_at", normalized)
	}
	if !normalized.CreatedAt.Equal(createdAt) || !normalized.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("timestamps = %s/%s, want %s/%s", normalized.CreatedAt, normalized.UpdatedAt, createdAt, updatedAt)
	}
	if got, want := len(db.calls), 1; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "insert into recording_normalized_audios") || !strings.Contains(query, "on conflict") || !strings.Contains(query, "returning") {
		t.Fatalf("query = %q, want insert into recording_normalized_audios on conflict returning", db.calls[0].query)
	}
	if got, want := len(db.calls[0].args), 12; got != want {
		t.Fatalf("normalized audio upsert args = %d, want %d", got, want)
	}
	if got, want := db.calls[0].args[0], "rec_normalized"; got != want {
		t.Fatalf("recording_id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[1], "recordings/20260606T150747.170276465Z/normalized.wav"; got != want {
		t.Fatalf("object_key arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreUpsertNormalizedAudioRejectsInvalidInputBeforeQuery(t *testing.T) {
	tests := []struct {
		name  string
		input UpsertNormalizedAudioInput
	}{
		{name: "missing recording id", input: UpsertNormalizedAudioInput{ObjectKey: "recordings/rec/normalized.wav", ContentType: "audio/wav", SizeBytes: 1, FormatName: "wav", CodecName: "pcm_s16le", SampleRate: 16000, Channels: 1, NormalizedAt: time.Now()}},
		{name: "missing object key", input: UpsertNormalizedAudioInput{RecordingID: "rec", ContentType: "audio/wav", SizeBytes: 1, FormatName: "wav", CodecName: "pcm_s16le", SampleRate: 16000, Channels: 1, NormalizedAt: time.Now()}},
		{name: "missing content type", input: UpsertNormalizedAudioInput{RecordingID: "rec", ObjectKey: "recordings/rec/normalized.wav", SizeBytes: 1, FormatName: "wav", CodecName: "pcm_s16le", SampleRate: 16000, Channels: 1, NormalizedAt: time.Now()}},
		{name: "non-positive size", input: UpsertNormalizedAudioInput{RecordingID: "rec", ObjectKey: "recordings/rec/normalized.wav", ContentType: "audio/wav", FormatName: "wav", CodecName: "pcm_s16le", SampleRate: 16000, Channels: 1, NormalizedAt: time.Now()}},
		{name: "missing normalized timestamp", input: UpsertNormalizedAudioInput{RecordingID: "rec", ObjectKey: "recordings/rec/normalized.wav", ContentType: "audio/wav", SizeBytes: 1, FormatName: "wav", CodecName: "pcm_s16le", SampleRate: 16000, Channels: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newPostgresExecutorSpy()
			store := NewPostgresStore(db)
			_, err := store.UpsertNormalizedAudio(tt.input)
			if err == nil {
				t.Fatal("UpsertNormalizedAudio returned nil error, want validation error")
			}
			if got, want := len(db.calls), 0; got != want {
				t.Fatalf("query calls = %d, want %d", got, want)
			}
		})
	}
}

func TestPostgresStoreGetNormalizedAudioReturnsExistingMetadata(t *testing.T) {
	normalizedAt := time.Date(2026, 6, 6, 4, 5, 6, 0, time.UTC)
	createdAt := normalizedAt
	updatedAt := normalizedAt.Add(time.Second)
	db := newPostgresExecutorSpy(postgresRow(
		"rec_normalized",
		"recordings/20260606T150747.170276465Z/normalized.wav",
		"audio/wav",
		int64(32044),
		"wav",
		"pcm_s16le",
		16000,
		1,
		1.25,
		normalizedAt,
		createdAt,
		updatedAt,
	))
	store := NewPostgresStore(db)

	normalized, ok, err := store.GetNormalizedAudio("rec_normalized")
	if err != nil {
		t.Fatalf("GetNormalizedAudio returned error: %v", err)
	}
	if !ok {
		t.Fatal("GetNormalizedAudio(rec_normalized) ok = false, want true")
	}
	if normalized.RecordingID != "rec_normalized" || normalized.ObjectKey == "" || normalized.ContentType != "audio/wav" {
		t.Fatalf("normalized audio = %+v, want persisted normalized metadata", normalized)
	}
	if normalized.SampleRate != 16000 || normalized.Channels != 1 || normalized.CodecName != "pcm_s16le" {
		t.Fatalf("normalized target metadata = %+v, want wav pcm_s16le 16k mono", normalized)
	}
	if got, want := len(db.calls), 1; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "select") || !strings.Contains(query, "from recording_normalized_audios") {
		t.Fatalf("query = %q, want select from recording_normalized_audios", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "rec_normalized"; got != want {
		t.Fatalf("recording_id arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreGetNormalizedAudioReturnsFalseForMissingRecording(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)
	_, ok, err := store.GetNormalizedAudio("rec_missing")
	if err != nil {
		t.Fatalf("GetNormalizedAudio returned error: %v", err)
	}
	if ok {
		t.Fatal("GetNormalizedAudio(rec_missing) ok = true, want false")
	}
}

func TestPostgresStoreGetNormalizedAudioReturnsErrorForDatabaseFailure(t *testing.T) {
	dbErr := errors.New("database unavailable")
	db := newPostgresExecutorSpy(postgresErrorRow(dbErr))
	store := NewPostgresStore(db)
	_, ok, err := store.GetNormalizedAudio("rec_error")
	if err == nil {
		t.Fatal("GetNormalizedAudio returned nil error, want database error")
	}
	if ok {
		t.Fatal("GetNormalizedAudio(rec_error) ok = true, want false")
	}
}
