package workspaces

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
	calls    []postgresQueryCall
	rows     []*postgresRowStub
	queryErr error
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
		case *domain.WorkspaceRole:
			*target = value.(domain.WorkspaceRole)
		case *time.Time:
			*target = value.(time.Time)
		default:
			return sql.ErrNoRows
		}
	}
	return nil
}

type postgresRowsStub struct {
	rows  []*postgresRowStub
	index int
	err   error
}

func (r *postgresRowsStub) Close() {}

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
	return r.err
}

func TestPostgresStoreGetUserReturnsExistingUser(t *testing.T) {
	createdAt := time.Date(2026, 6, 11, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow("usr_dev", "dev@local.soniq", "Local Developer", createdAt, updatedAt))
	store := NewPostgresStore(db)

	user, ok, err := store.GetUser(context.Background(), "usr_dev")
	if err != nil {
		t.Fatalf("GetUser returned error: %v", err)
	}
	if !ok {
		t.Fatal("GetUser ok = false, want true")
	}
	if user.ID != "usr_dev" || user.Email != "dev@local.soniq" || user.DisplayName != "Local Developer" {
		t.Fatalf("user = %+v, want persisted user", user)
	}
	if user.CreatedAt != createdAt || user.UpdatedAt != updatedAt {
		t.Fatalf("timestamps = %s/%s, want %s/%s", user.CreatedAt, user.UpdatedAt, createdAt, updatedAt)
	}
	if got, want := len(db.calls), 1; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "from users") || !strings.Contains(query, "where id = $1") {
		t.Fatalf("query = %q, want scoped user lookup", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "usr_dev"; got != want {
		t.Fatalf("user id arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreGetUserReturnsFalseForMissingUser(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	_, ok, err := store.GetUser(context.Background(), "usr_missing")
	if err != nil {
		t.Fatalf("GetUser returned error: %v", err)
	}
	if ok {
		t.Fatal("GetUser ok = true, want false")
	}
}

func TestPostgresStoreGetUserRejectsMissingUserID(t *testing.T) {
	db := newPostgresExecutorSpy()
	store := NewPostgresStore(db)

	_, _, err := store.GetUser(context.Background(), "")
	if err == nil {
		t.Fatal("GetUser returned nil error, want missing user id error")
	}
	if got, want := len(db.calls), 0; got != want {
		t.Fatalf("query calls = %d, want %d", got, want)
	}
}

func TestPostgresStoreListWorkspacesForUserReturnsMemberships(t *testing.T) {
	createdAt := time.Date(2026, 6, 11, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(
		postgresRow("wsp_default", "Default Workspace", "usr_dev", domain.WorkspaceRoleOwner, createdAt, updatedAt),
		postgresRow("wsp_team", "Team Workspace", "usr_owner", domain.WorkspaceRoleMember, createdAt.Add(time.Hour), updatedAt.Add(time.Hour)),
	)
	store := NewPostgresStore(db)

	workspaces, err := store.ListWorkspacesForUser(context.Background(), "usr_dev")
	if err != nil {
		t.Fatalf("ListWorkspacesForUser returned error: %v", err)
	}
	if got, want := len(workspaces), 2; got != want {
		t.Fatalf("workspaces = %d, want %d", got, want)
	}
	if workspaces[0].ID != "wsp_default" || workspaces[0].Role != domain.WorkspaceRoleOwner {
		t.Fatalf("first workspace = %+v, want default owner workspace", workspaces[0])
	}
	if workspaces[1].ID != "wsp_team" || workspaces[1].Role != domain.WorkspaceRoleMember {
		t.Fatalf("second workspace = %+v, want team member workspace", workspaces[1])
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "from workspaces") || !strings.Contains(query, "join workspace_members") || !strings.Contains(query, "wm.user_id = $1") {
		t.Fatalf("query = %q, want workspace membership list query", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "usr_dev"; got != want {
		t.Fatalf("user id arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreListWorkspacesForUserReturnsDatabaseErrors(t *testing.T) {
	dbErr := errors.New("database unavailable")
	db := newPostgresExecutorSpy()
	db.queryErr = dbErr
	store := NewPostgresStore(db)

	_, err := store.ListWorkspacesForUser(context.Background(), "usr_dev")
	if !errors.Is(err, dbErr) {
		t.Fatalf("ListWorkspacesForUser error = %v, want wrapped database error", err)
	}
}

func TestPostgresStoreGetWorkspaceForUserValidatesMembership(t *testing.T) {
	createdAt := time.Date(2026, 6, 11, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow("wsp_default", "Default Workspace", "usr_dev", domain.WorkspaceRoleOwner, createdAt, updatedAt))
	store := NewPostgresStore(db)

	workspace, ok, err := store.GetWorkspaceForUser(context.Background(), "usr_dev", "wsp_default")
	if err != nil {
		t.Fatalf("GetWorkspaceForUser returned error: %v", err)
	}
	if !ok {
		t.Fatal("GetWorkspaceForUser ok = false, want true")
	}
	if workspace.ID != "wsp_default" || workspace.Role != domain.WorkspaceRoleOwner {
		t.Fatalf("workspace = %+v, want default owner workspace", workspace)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "from workspaces") || !strings.Contains(query, "join workspace_members") || !strings.Contains(query, "wm.user_id = $1") || !strings.Contains(query, "w.id = $2") {
		t.Fatalf("query = %q, want user/workspace membership lookup", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "usr_dev"; got != want {
		t.Fatalf("user id arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[1], "wsp_default"; got != want {
		t.Fatalf("workspace id arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreGetWorkspaceForUserReturnsFalseForNonMember(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	_, ok, err := store.GetWorkspaceForUser(context.Background(), "usr_dev", "wsp_other")
	if err != nil {
		t.Fatalf("GetWorkspaceForUser returned error: %v", err)
	}
	if ok {
		t.Fatal("GetWorkspaceForUser ok = true, want false")
	}
}

func TestPostgresStoreGetWorkspaceForUserRejectsMissingIDs(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		workspaceID string
	}{
		{name: "missing user id", workspaceID: "wsp_default"},
		{name: "missing workspace id", userID: "usr_dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newPostgresExecutorSpy()
			store := NewPostgresStore(db)

			_, _, err := store.GetWorkspaceForUser(context.Background(), tt.userID, tt.workspaceID)
			if err == nil {
				t.Fatal("GetWorkspaceForUser returned nil error, want validation error")
			}
			if got, want := len(db.calls), 0; got != want {
				t.Fatalf("query calls = %d, want %d", got, want)
			}
		})
	}
}
