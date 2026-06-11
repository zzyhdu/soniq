package api

import (
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
)

type listWorkspacesResponse struct {
	Workspaces []workspaceResponse `json:"workspaces"`
}

type workspaceResponse struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Role      domain.WorkspaceRole `json:"role"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
}

func toWorkspaceResponse(workspace domain.WorkspaceWithRole) workspaceResponse {
	return workspaceResponse{
		ID:        workspace.ID,
		Name:      workspace.Name,
		Role:      workspace.Role,
		CreatedAt: workspace.CreatedAt,
		UpdatedAt: workspace.UpdatedAt,
	}
}
