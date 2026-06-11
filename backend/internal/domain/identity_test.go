package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestIsValidWorkspaceRoleAcceptsSupportedValues(t *testing.T) {
	validRoles := []string{"owner", "member"}

	for _, role := range validRoles {
		t.Run(role, func(t *testing.T) {
			if !IsValidWorkspaceRole(role) {
				t.Fatalf("IsValidWorkspaceRole(%q) = false, want true", role)
			}
		})
	}
}

func TestIsValidWorkspaceRoleRejectsUnsupportedValues(t *testing.T) {
	invalidRoles := []string{"", "admin", "OWNER", " owner ", "viewer"}

	for _, role := range invalidRoles {
		t.Run(role, func(t *testing.T) {
			if IsValidWorkspaceRole(role) {
				t.Fatalf("IsValidWorkspaceRole(%q) = true, want false", role)
			}
		})
	}
}

func TestIdentityJSONUsesSnakeCaseFields(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	payload := struct {
		User      User              `json:"user"`
		Workspace WorkspaceWithRole `json:"workspace"`
	}{
		User: User{
			ID:          "usr_dev",
			Email:       "dev@local.soniq",
			DisplayName: "Local Developer",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		Workspace: WorkspaceWithRole{
			ID:              "wsp_default",
			Name:            "Default Workspace",
			CreatedByUserID: "usr_dev",
			Role:            WorkspaceRoleOwner,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	body := string(encoded)
	for _, want := range []string{`"display_name"`, `"created_by_user_id"`, `"role":"owner"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("encoded identity payload = %s, want it to contain %s", body, want)
		}
	}
}
