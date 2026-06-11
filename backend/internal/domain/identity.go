package domain

import "time"

// User is the product identity that owns workspace memberships.
type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Workspace is the tenant boundary for recordings and future provider settings.
type Workspace struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	CreatedByUserID string    `json:"created_by_user_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// WorkspaceRole describes a user's role inside a workspace.
type WorkspaceRole string

const (
	WorkspaceRoleOwner  WorkspaceRole = "owner"
	WorkspaceRoleMember WorkspaceRole = "member"
)

// WorkspaceMembership links a user to a workspace.
type WorkspaceMembership struct {
	WorkspaceID string        `json:"workspace_id"`
	UserID      string        `json:"user_id"`
	Role        WorkspaceRole `json:"role"`
	CreatedAt   time.Time     `json:"created_at"`
}

// WorkspaceWithRole is the workspace list shape exposed to callers.
type WorkspaceWithRole struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	CreatedByUserID string        `json:"created_by_user_id"`
	Role            WorkspaceRole `json:"role"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// IsValidWorkspaceRole reports whether value is one of the supported workspace roles.
func IsValidWorkspaceRole(value string) bool {
	switch WorkspaceRole(value) {
	case WorkspaceRoleOwner, WorkspaceRoleMember:
		return true
	default:
		return false
	}
}
