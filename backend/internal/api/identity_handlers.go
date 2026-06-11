package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

func meHandler(workspaceStore WorkspaceStore, authResolver AuthResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		currentUser, ok := resolveCurrentUser(w, r, authResolver)
		if !ok {
			return
		}
		user, found, err := workspaceStore.GetUser(r.Context(), currentUser.UserID)
		if err != nil {
			http.Error(w, "get current user", http.StatusInternalServerError)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(user)
	}
}

func workspacesHandler(workspaceStore WorkspaceStore, authResolver AuthResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		currentUser, ok := resolveCurrentUser(w, r, authResolver)
		if !ok {
			return
		}
		workspaces, err := workspaceStore.ListWorkspacesForUser(r.Context(), currentUser.UserID)
		if err != nil {
			http.Error(w, "list workspaces", http.StatusInternalServerError)
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
		http.Error(w, "resolve current user", http.StatusUnauthorized)
		return CurrentUser{}, false
	}
	if strings.TrimSpace(currentUser.UserID) == "" {
		http.Error(w, "current user is required", http.StatusUnauthorized)
		return CurrentUser{}, false
	}
	return currentUser, true
}
