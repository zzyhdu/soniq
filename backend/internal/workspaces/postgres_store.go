package workspaces

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/zzyhdu/soniq/backend/internal/domain"
)

// PostgresRows is the subset of database rows behavior used by PostgresStore.
type PostgresRows interface {
	Close()
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// PostgresExecutor is the subset of database behavior used by PostgresStore.
type PostgresExecutor interface {
	QueryRow(ctx context.Context, query string, args ...any) interface{ Scan(dest ...any) error }
	Query(ctx context.Context, query string, args ...any) (PostgresRows, error)
}

// PostgresStore persists users and workspace memberships in Postgres.
type PostgresStore struct {
	db PostgresExecutor
}

// NewPostgresStore creates a Postgres-backed workspace store.
func NewPostgresStore(db PostgresExecutor) *PostgresStore {
	return &PostgresStore{db: db}
}

// GetUser returns a user by id.
func (s *PostgresStore) GetUser(ctx context.Context, userID string) (domain.User, bool, error) {
	if strings.TrimSpace(userID) == "" {
		return domain.User{}, false, fmt.Errorf("user id is required")
	}
	if s == nil || s.db == nil {
		return domain.User{}, false, fmt.Errorf("postgres workspace store requires database executor")
	}

	var user domain.User
	row := s.db.QueryRow(
		ctx,
		`SELECT id, email, display_name, created_at, updated_at
FROM users
WHERE id = $1`,
		userID,
	)
	if err := row.Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt, &user.UpdatedAt); err != nil {
		if isNoRows(err) {
			return domain.User{}, false, nil
		}
		return domain.User{}, false, fmt.Errorf("get user: %w", err)
	}
	return user, true, nil
}

// ListWorkspacesForUser returns all workspaces a user can access.
func (s *PostgresStore) ListWorkspacesForUser(ctx context.Context, userID string) ([]domain.WorkspaceWithRole, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user id is required")
	}
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("postgres workspace store requires database executor")
	}

	rows, err := s.db.Query(
		ctx,
		`SELECT w.id, w.name, w.created_by_user_id, wm.role, w.created_at, w.updated_at
FROM workspaces w
JOIN workspace_members wm ON wm.workspace_id = w.id
WHERE wm.user_id = $1
ORDER BY w.created_at ASC, w.id ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list workspaces for user: %w", err)
	}
	defer rows.Close()

	workspaces := []domain.WorkspaceWithRole{}
	for rows.Next() {
		var workspace domain.WorkspaceWithRole
		if err := scanWorkspaceWithRole(rows, &workspace); err != nil {
			return nil, fmt.Errorf("scan workspace membership: %w", err)
		}
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workspaces for user rows: %w", err)
	}
	return workspaces, nil
}

// GetWorkspaceForUser returns a workspace only when the user is a member.
func (s *PostgresStore) GetWorkspaceForUser(ctx context.Context, userID string, workspaceID string) (domain.WorkspaceWithRole, bool, error) {
	if strings.TrimSpace(userID) == "" {
		return domain.WorkspaceWithRole{}, false, fmt.Errorf("user id is required")
	}
	if strings.TrimSpace(workspaceID) == "" {
		return domain.WorkspaceWithRole{}, false, fmt.Errorf("workspace id is required")
	}
	if s == nil || s.db == nil {
		return domain.WorkspaceWithRole{}, false, fmt.Errorf("postgres workspace store requires database executor")
	}

	var workspace domain.WorkspaceWithRole
	row := s.db.QueryRow(
		ctx,
		`SELECT w.id, w.name, w.created_by_user_id, wm.role, w.created_at, w.updated_at
FROM workspaces w
JOIN workspace_members wm ON wm.workspace_id = w.id
WHERE wm.user_id = $1
  AND w.id = $2`,
		userID,
		workspaceID,
	)
	if err := scanWorkspaceWithRole(row, &workspace); err != nil {
		if isNoRows(err) {
			return domain.WorkspaceWithRole{}, false, nil
		}
		return domain.WorkspaceWithRole{}, false, fmt.Errorf("get workspace for user: %w", err)
	}
	return workspace, true, nil
}

func scanWorkspaceWithRole(row interface{ Scan(dest ...any) error }, workspace *domain.WorkspaceWithRole) error {
	return row.Scan(
		&workspace.ID,
		&workspace.Name,
		&workspace.CreatedByUserID,
		&workspace.Role,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
	)
}

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows)
}
