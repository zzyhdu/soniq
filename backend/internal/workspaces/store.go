package workspaces

import (
	"context"

	"github.com/zzyhdu/soniq/backend/internal/domain"
)

// Store is the persistence boundary for users and workspace memberships.
type Store interface {
	GetUser(ctx context.Context, userID string) (domain.User, bool, error)
	ListWorkspacesForUser(ctx context.Context, userID string) ([]domain.WorkspaceWithRole, error)
	GetWorkspaceForUser(ctx context.Context, userID string, workspaceID string) (domain.WorkspaceWithRole, bool, error)
}
