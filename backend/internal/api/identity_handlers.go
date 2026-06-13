package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

func meHandler(workspaceStore WorkspaceStore, authResolver AuthResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		currentUser, ok := resolveCurrentUser(w, r, authResolver)
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

func workspacesHandler(workspaceStore WorkspaceStore, authResolver AuthResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		currentUser, ok := resolveCurrentUser(w, r, authResolver)
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

func resolveCurrentUser(w http.ResponseWriter, r *http.Request, authResolver AuthResolver) (CurrentUser, bool) {
	currentUser, err := authResolver.ResolveCurrentUser(r)
	if err != nil {
		writeAPIError(w, http.StatusUnauthorized, errorCodeUnauthenticated, "resolve current user")
		return CurrentUser{}, false
	}
	if strings.TrimSpace(currentUser.UserID) == "" {
		writeAPIError(w, http.StatusUnauthorized, errorCodeUnauthenticated, "current user is required")
		return CurrentUser{}, false
	}
	return currentUser, true
}
