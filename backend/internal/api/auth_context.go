package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/zzyhdu/soniq/backend/internal/domain"
)

type currentUserContextKey struct{}
type workspaceContextKey struct{}

func requireAuth(authResolver AuthResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			currentUser, ok := resolveCurrentUser(w, r, authResolver)
			if !ok {
				return
			}
			setRequestLogUserID(r.Context(), currentUser.UserID)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), currentUserContextKey{}, currentUser)))
		})
	}
}

func requireWorkspace(workspaceStore WorkspaceStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			currentUser, ok := currentUserFromRequest(w, r)
			if !ok {
				return
			}
			workspaceID := chi.URLParam(r, "workspace_id")
			if workspaceID == "" {
				writeAPIError(w, http.StatusNotFound, errorCodeNotFound, "not found")
				return
			}
			workspace, found, err := workspaceStore.GetWorkspaceForUser(r.Context(), currentUser.UserID, workspaceID)
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "get workspace")
				return
			}
			if !found {
				writeAPIError(w, http.StatusNotFound, errorCodeNotFound, "not found")
				return
			}
			setRequestLogWorkspaceID(r.Context(), workspace.ID)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), workspaceContextKey{}, workspace)))
		})
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

func currentUserFromRequest(w http.ResponseWriter, r *http.Request) (CurrentUser, bool) {
	currentUser, ok := r.Context().Value(currentUserContextKey{}).(CurrentUser)
	if !ok || strings.TrimSpace(currentUser.UserID) == "" {
		writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "current user context is required")
		return CurrentUser{}, false
	}
	return currentUser, true
}

func workspaceFromRequest(w http.ResponseWriter, r *http.Request) (domain.WorkspaceWithRole, bool) {
	workspace, ok := r.Context().Value(workspaceContextKey{}).(domain.WorkspaceWithRole)
	if !ok || strings.TrimSpace(workspace.ID) == "" {
		writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "workspace context is required")
		return domain.WorkspaceWithRole{}, false
	}
	return workspace, true
}

func recordingIDFromRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	recordingID := chi.URLParam(r, "recording_id")
	if recordingID == "" {
		writeAPIError(w, http.StatusNotFound, errorCodeNotFound, "not found")
		return "", false
	}
	return recordingID, true
}
