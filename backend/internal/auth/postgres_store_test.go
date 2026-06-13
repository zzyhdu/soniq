package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	storedb "github.com/zzyhdu/soniq/backend/internal/db"
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

func (s *postgresExecutorSpy) QueryRow(_ context.Context, query string, args ...any) storedb.PostgresRow {
	s.calls = append(s.calls, postgresQueryCall{query: query, args: append([]any(nil), args...)})
	if len(s.rows) == 0 {
		return &postgresRowStub{err: sql.ErrNoRows}
	}
	row := s.rows[0]
	s.rows = s.rows[1:]
	return row
}

func (s *postgresExecutorSpy) Query(_ context.Context, query string, args ...any) (storedb.PostgresRows, error) {
	s.calls = append(s.calls, postgresQueryCall{query: query, args: append([]any(nil), args...)})
	return nil, s.queryErr
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
		case *bool:
			*target = value.(bool)
		case *time.Time:
			*target = value.(time.Time)
		case *sql.NullTime:
			if value == nil {
				*target = sql.NullTime{}
				continue
			}
			*target = sql.NullTime{Time: value.(time.Time), Valid: true}
		default:
			return sql.ErrNoRows
		}
	}
	return nil
}

func TestPostgresStoreGetUserByEmailReturnsUserAndPasswordHash(t *testing.T) {
	createdAt := time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow("usr_dev", "dev@local.soniq", "Local Developer", "hash", createdAt, updatedAt))
	store := NewPostgresStore(db)

	user, passwordHash, ok, err := store.GetUserByEmail(context.Background(), " Dev@LOCAL.Soniq ")
	if err != nil {
		t.Fatalf("GetUserByEmail returned error: %v", err)
	}
	if !ok {
		t.Fatal("GetUserByEmail ok = false, want true")
	}
	if user.ID != "usr_dev" || user.Email != "dev@local.soniq" || user.DisplayName != "Local Developer" {
		t.Fatalf("user = %+v, want persisted user", user)
	}
	if passwordHash != "hash" {
		t.Fatalf("password hash = %q, want hash", passwordHash)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "from users") || !strings.Contains(query, "lower(email) = $1") {
		t.Fatalf("query = %q, want normalized email lookup", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "dev@local.soniq"; got != want {
		t.Fatalf("email arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreSignUpCreatesUserWorkspaceAndMembership(t *testing.T) {
	createdAt := time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	db := newPostgresExecutorSpy(postgresRow("usr_new", "owner@local.soniq", "Owner", createdAt, updatedAt))
	store := NewPostgresStore(db)

	user, err := store.SignUp(context.Background(), SignUpInput{
		Email:         " Owner@LOCAL.Soniq ",
		DisplayName:   "Owner",
		PasswordHash:  "password-hash",
		WorkspaceName: "Owner Workspace",
	})
	if err != nil {
		t.Fatalf("SignUp returned error: %v", err)
	}
	if user.ID != "usr_new" || user.Email != "owner@local.soniq" || user.DisplayName != "Owner" {
		t.Fatalf("user = %+v, want created user", user)
	}
	query := strings.ToLower(db.calls[0].query)
	for _, want := range []string{"insert into users", "insert into workspaces", "insert into workspace_members"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query = %q, want it to contain %q", db.calls[0].query, want)
		}
	}
	if userID, ok := db.calls[0].args[0].(string); !ok || !strings.HasPrefix(userID, "usr_") {
		t.Fatalf("user id arg = %#v, want generated usr_ id", db.calls[0].args[0])
	}
	if got, want := db.calls[0].args[1], "owner@local.soniq"; got != want {
		t.Fatalf("email arg = %q, want %q", got, want)
	}
	if got, want := db.calls[0].args[2], "Owner"; got != want {
		t.Fatalf("display name arg = %q, want %q", got, want)
	}
	if workspaceID, ok := db.calls[0].args[4].(string); !ok || !strings.HasPrefix(workspaceID, "wsp_") {
		t.Fatalf("workspace id arg = %#v, want generated wsp_ id", db.calls[0].args[4])
	}
	if got, want := db.calls[0].args[5], "Owner Workspace"; got != want {
		t.Fatalf("workspace name arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreSignUpMapsUniqueEmailToUserAlreadyExists(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(&pgconn.PgError{Code: "23505"}))
	store := NewPostgresStore(db)

	_, err := store.SignUp(context.Background(), SignUpInput{
		Email:        "owner@local.soniq",
		DisplayName:  "Owner",
		PasswordHash: "hash",
	})
	if !errors.Is(err, ErrUserAlreadyExists) {
		t.Fatalf("SignUp error = %v, want ErrUserAlreadyExists", err)
	}
}

func TestPostgresStoreCreateSessionInsertsSession(t *testing.T) {
	expiresAt := time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC)
	createdAt := expiresAt.Add(-time.Hour)
	db := newPostgresExecutorSpy(postgresRow("ses_test", "usr_dev", "token-hash", createdAt, createdAt, expiresAt, nil))
	store := NewPostgresStore(db)

	session, err := store.CreateSession(context.Background(), CreateSessionInput{
		UserID:    "usr_dev",
		TokenHash: "token-hash",
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if session.ID != "ses_test" || session.UserID != "usr_dev" || session.TokenHash != "token-hash" {
		t.Fatalf("session = %+v, want persisted session", session)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "insert into user_sessions") || !strings.Contains(query, "returning id, user_id, token_hash") {
		t.Fatalf("query = %q, want user_sessions insert", db.calls[0].query)
	}
	if sessionID, ok := db.calls[0].args[0].(string); !ok || !strings.HasPrefix(sessionID, "ses_") {
		t.Fatalf("session id arg = %#v, want ses_ id", db.calls[0].args[0])
	}
}

func TestPostgresStoreGetActiveSessionByTokenHashTouchesSession(t *testing.T) {
	now := time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	db := newPostgresExecutorSpy(postgresRow("ses_test", "usr_dev", "token-hash", now.Add(-time.Minute), now, expiresAt, nil))
	store := NewPostgresStore(db)

	session, ok, err := store.GetActiveSessionByTokenHash(context.Background(), "token-hash", now)
	if err != nil {
		t.Fatalf("GetActiveSessionByTokenHash returned error: %v", err)
	}
	if !ok {
		t.Fatal("GetActiveSessionByTokenHash ok = false, want true")
	}
	if session.UserID != "usr_dev" {
		t.Fatalf("session user id = %q, want usr_dev", session.UserID)
	}
	query := strings.ToLower(db.calls[0].query)
	if !strings.Contains(query, "update user_sessions") || !strings.Contains(query, "revoked_at is null") || !strings.Contains(query, "expires_at > $2") {
		t.Fatalf("query = %q, want active session touch query", db.calls[0].query)
	}
	if got, want := db.calls[0].args[0], "token-hash"; got != want {
		t.Fatalf("token hash arg = %q, want %q", got, want)
	}
}

func TestPostgresStoreRevokeSessionIgnoresMissingSession(t *testing.T) {
	db := newPostgresExecutorSpy(postgresErrorRow(sql.ErrNoRows))
	store := NewPostgresStore(db)

	if err := store.RevokeSession(context.Background(), "token-hash", time.Now()); err != nil {
		t.Fatalf("RevokeSession returned error: %v", err)
	}
}
