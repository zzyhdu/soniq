package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	storedb "github.com/zzyhdu/soniq/backend/internal/db"
	"github.com/zzyhdu/soniq/backend/internal/domain"
)

var ErrUserAlreadyExists = errors.New("user already exists")

// SignUpInput contains credentials and defaults for creating a new user account.
type SignUpInput struct {
	Email         string
	DisplayName   string
	PasswordHash  string
	WorkspaceName string
}

// PostgresStore persists password auth and login sessions in Soniq Postgres.
type PostgresStore struct {
	db storedb.PostgresExecutor
}

// NewPostgresStore creates a Postgres-backed auth store.
func NewPostgresStore(db storedb.PostgresExecutor) *PostgresStore {
	return &PostgresStore{db: db}
}

// GetUserByEmail returns a login user and password hash by normalized email.
func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (domain.User, string, bool, error) {
	normalizedEmail := NormalizeEmail(email)
	if normalizedEmail == "" {
		return domain.User{}, "", false, ErrEmailRequired
	}
	if err := s.validate(); err != nil {
		return domain.User{}, "", false, err
	}

	var user domain.User
	var passwordHash string
	row := s.db.QueryRow(
		ctx,
		`SELECT id, email, display_name, password_hash, created_at, updated_at
FROM users
WHERE lower(email) = $1`,
		normalizedEmail,
	)
	if err := row.Scan(&user.ID, &user.Email, &user.DisplayName, &passwordHash, &user.CreatedAt, &user.UpdatedAt); err != nil {
		if isNoRows(err) {
			return domain.User{}, "", false, nil
		}
		return domain.User{}, "", false, fmt.Errorf("get user by email: %w", err)
	}
	return user, passwordHash, true, nil
}

// SignUp creates a user, default workspace, and owner membership.
func (s *PostgresStore) SignUp(ctx context.Context, input SignUpInput) (domain.User, error) {
	email := NormalizeEmail(input.Email)
	displayName := strings.TrimSpace(input.DisplayName)
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if err := ValidateEmail(email); err != nil {
		return domain.User{}, err
	}
	if displayName == "" {
		return domain.User{}, fmt.Errorf("display name is required")
	}
	if strings.TrimSpace(input.PasswordHash) == "" {
		return domain.User{}, ErrPasswordHashEmpty
	}
	if workspaceName == "" {
		workspaceName = "Personal Workspace"
	}
	if err := s.validate(); err != nil {
		return domain.User{}, err
	}

	userID, err := NewUserID()
	if err != nil {
		return domain.User{}, err
	}
	workspaceID, err := NewWorkspaceID()
	if err != nil {
		return domain.User{}, err
	}

	var user domain.User
	row := s.db.QueryRow(
		ctx,
		`WITH new_user AS (
  INSERT INTO users (id, email, display_name, password_hash, created_at, updated_at)
  VALUES ($1, $2, $3, $4, NOW(), NOW())
  RETURNING id, email, display_name, created_at, updated_at
), new_workspace AS (
  INSERT INTO workspaces (id, name, created_by_user_id, created_at, updated_at)
  SELECT $5, $6, id, NOW(), NOW()
  FROM new_user
  RETURNING id
), new_membership AS (
  INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
  SELECT new_workspace.id, new_user.id, 'owner', NOW()
  FROM new_user
  CROSS JOIN new_workspace
)
SELECT id, email, display_name, created_at, updated_at
FROM new_user`,
		userID,
		email,
		displayName,
		input.PasswordHash,
		workspaceID,
		workspaceName,
	)
	if err := row.Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt, &user.UpdatedAt); err != nil {
		if isUniqueViolation(err) {
			return domain.User{}, ErrUserAlreadyExists
		}
		return domain.User{}, fmt.Errorf("sign up user: %w", err)
	}
	return user, nil
}

// CreateSession persists a login session.
func (s *PostgresStore) CreateSession(ctx context.Context, input CreateSessionInput) (Session, error) {
	userID := strings.TrimSpace(input.UserID)
	tokenHash := strings.TrimSpace(input.TokenHash)
	if userID == "" {
		return Session{}, fmt.Errorf("user id is required")
	}
	if tokenHash == "" {
		return Session{}, fmt.Errorf("token hash is required")
	}
	if input.ExpiresAt.IsZero() {
		return Session{}, fmt.Errorf("expires at is required")
	}
	if err := s.validate(); err != nil {
		return Session{}, err
	}

	sessionID, err := NewSessionID()
	if err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	row := s.db.QueryRow(
		ctx,
		`INSERT INTO user_sessions (id, user_id, token_hash, created_at, last_seen_at, expires_at)
VALUES ($1, $2, $3, $4, $4, $5)
RETURNING id, user_id, token_hash, created_at, last_seen_at, expires_at, revoked_at`,
		sessionID,
		userID,
		tokenHash,
		now,
		input.ExpiresAt.UTC(),
	)
	session, err := scanSession(row)
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

// GetActiveSessionByTokenHash returns and touches a non-expired, non-revoked session.
func (s *PostgresStore) GetActiveSessionByTokenHash(ctx context.Context, tokenHash string, now time.Time) (Session, bool, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return Session{}, false, fmt.Errorf("token hash is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := s.validate(); err != nil {
		return Session{}, false, err
	}

	row := s.db.QueryRow(
		ctx,
		`UPDATE user_sessions
SET last_seen_at = $2
WHERE token_hash = $1
  AND revoked_at IS NULL
  AND expires_at > $2
RETURNING id, user_id, token_hash, created_at, last_seen_at, expires_at, revoked_at`,
		tokenHash,
		now.UTC(),
	)
	session, err := scanSession(row)
	if err != nil {
		if isNoRows(err) {
			return Session{}, false, nil
		}
		return Session{}, false, fmt.Errorf("get active session: %w", err)
	}
	return session, true, nil
}

// RevokeSession marks a session revoked if it exists.
func (s *PostgresStore) RevokeSession(ctx context.Context, tokenHash string, now time.Time) error {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := s.validate(); err != nil {
		return err
	}

	var id string
	row := s.db.QueryRow(
		ctx,
		`UPDATE user_sessions
SET revoked_at = $2
WHERE token_hash = $1
  AND revoked_at IS NULL
RETURNING id`,
		tokenHash,
		now.UTC(),
	)
	if err := row.Scan(&id); err != nil {
		if isNoRows(err) {
			return nil
		}
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *PostgresStore) validate() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres auth store requires database executor")
	}
	return nil
}

func scanSession(row storedb.PostgresRow) (Session, error) {
	var session Session
	var revokedAt sql.NullTime
	if err := row.Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&session.CreatedAt,
		&session.LastSeenAt,
		&session.ExpiresAt,
		&revokedAt,
	); err != nil {
		return Session{}, err
	}
	if revokedAt.Valid {
		session.RevokedAt = &revokedAt.Time
	}
	return session, nil
}

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
