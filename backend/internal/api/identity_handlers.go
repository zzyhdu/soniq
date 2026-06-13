package api

import (
	"encoding/json"
	"net/http"
)

func meHandler(workspaceStore WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		currentUser, ok := currentUserFromRequest(w, r)
		if !ok {
			return
		}
		user, found, err := workspaceStore.GetUser(r.Context(), currentUser.UserID)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "get current user")
			return
		}
		if !found {
			writeAPIError(w, http.StatusNotFound, errorCodeNotFound, "not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(user)
	}
}

func workspacesHandler(workspaceStore WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		currentUser, ok := currentUserFromRequest(w, r)
		if !ok {
			return
		}
		workspaces, err := workspaceStore.ListWorkspacesForUser(r.Context(), currentUser.UserID)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "list workspaces")
			return
		}
		response := listWorkspacesResponse{Workspaces: make([]workspaceResponse, 0, len(workspaces))}
		for _, workspace := range workspaces {
			response.Workspaces = append(response.Workspaces, toWorkspaceResponse(workspace))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}
