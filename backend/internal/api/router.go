package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
)

var errRecordingStoreNotConfigured = errors.New("recording store is not configured")
var errWorkspaceStoreNotConfigured = errors.New("workspace store is not configured")

// RecordingStore is the persistence seam required by the recording HTTP handlers.
type RecordingStore interface {
	Create(recordings.CreateRecordingInput) (domain.Recording, error)
	GetForWorkspace(input recordings.GetRecordingInput) (domain.Recording, bool, error)
	ListByWorkspace(input recordings.ListRecordingsInput) ([]domain.Recording, error)
}

// RecordingDetailsStore is the optional persistence seam for transcript and summary detail reads.
type RecordingDetailsStore interface {
	RecordingStore
	GetTranscript(recordingID string) (recordings.RecordingTranscript, bool, error)
	ListTranscriptSegments(recordingID string) ([]recordings.RecordingTranscriptSegment, error)
	GetSummary(recordingID string) (recordings.RecordingSummary, bool, error)
}

// RecordingRetryStore is the optional persistence seam for resetting failed recordings before retry.
type RecordingRetryStore interface {
	RecordingStore
	ResetForRetry(input recordings.RetryRecordingInput) (domain.Recording, error)
	UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error)
}

// WorkspaceStore is the persistence seam required by identity and workspace-scoped handlers.
type WorkspaceStore interface {
	GetUser(ctx context.Context, userID string) (domain.User, bool, error)
	ListWorkspacesForUser(ctx context.Context, userID string) ([]domain.WorkspaceWithRole, error)
	GetWorkspaceForUser(ctx context.Context, userID string, workspaceID string) (domain.WorkspaceWithRole, bool, error)
}

// RecordingProcessor is the enqueue seam invoked after a recording is created.
type RecordingProcessor interface {
	Enqueue(recording domain.Recording) error
}

type noopRecordingProcessor struct{}

type unconfiguredRecordingStore struct{}
type unconfiguredWorkspaceStore struct{}
type defaultDevWorkspaceStore struct{}

func (noopRecordingProcessor) Enqueue(domain.Recording) error { return nil }

func (unconfiguredRecordingStore) Create(recordings.CreateRecordingInput) (domain.Recording, error) {
	return domain.Recording{}, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) GetForWorkspace(recordings.GetRecordingInput) (domain.Recording, bool, error) {
	return domain.Recording{}, false, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) ListByWorkspace(recordings.ListRecordingsInput) ([]domain.Recording, error) {
	return nil, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) ResetForRetry(recordings.RetryRecordingInput) (domain.Recording, error) {
	return domain.Recording{}, errRecordingStoreNotConfigured
}

func (unconfiguredWorkspaceStore) GetUser(context.Context, string) (domain.User, bool, error) {
	return domain.User{}, false, errWorkspaceStoreNotConfigured
}

func (unconfiguredWorkspaceStore) ListWorkspacesForUser(context.Context, string) ([]domain.WorkspaceWithRole, error) {
	return nil, errWorkspaceStoreNotConfigured
}

func (unconfiguredWorkspaceStore) GetWorkspaceForUser(context.Context, string, string) (domain.WorkspaceWithRole, bool, error) {
	return domain.WorkspaceWithRole{}, false, errWorkspaceStoreNotConfigured
}

func (defaultDevWorkspaceStore) GetUser(_ context.Context, userID string) (domain.User, bool, error) {
	if userID != "usr_dev" {
		return domain.User{}, false, nil
	}
	return domain.User{ID: "usr_dev", Email: "dev@local.soniq", DisplayName: "Local Developer"}, true, nil
}

func (defaultDevWorkspaceStore) ListWorkspacesForUser(_ context.Context, userID string) ([]domain.WorkspaceWithRole, error) {
	if userID != "usr_dev" {
		return []domain.WorkspaceWithRole{}, nil
	}
	return []domain.WorkspaceWithRole{defaultWorkspaceWithRole()}, nil
}

func (defaultDevWorkspaceStore) GetWorkspaceForUser(_ context.Context, userID string, workspaceID string) (domain.WorkspaceWithRole, bool, error) {
	if userID != "usr_dev" || workspaceID != "wsp_default" {
		return domain.WorkspaceWithRole{}, false, nil
	}
	return defaultWorkspaceWithRole(), true, nil
}

func defaultWorkspaceWithRole() domain.WorkspaceWithRole {
	return domain.WorkspaceWithRole{
		ID:              "wsp_default",
		Name:            "Default Workspace",
		CreatedByUserID: "usr_dev",
		Role:            domain.WorkspaceRoleOwner,
	}
}

// NewRouter builds the HTTP handler for the Soniq API.
func NewRouter() http.Handler {
	return NewRouterWithIdentity(unconfiguredRecordingStore{}, unconfiguredWorkspaceStore{}, NewDevAuthResolver("usr_dev"), noopRecordingProcessor{})
}

// NewRouterWithStore builds the HTTP handler with an injected recording store.
func NewRouterWithStore(store RecordingStore) http.Handler {
	return NewRouterWithIdentity(store, defaultDevWorkspaceStore{}, NewDevAuthResolver("usr_dev"), noopRecordingProcessor{})
}

// NewRouterWithProcessor builds the HTTP handler with injected recording store and processor dependencies.
func NewRouterWithProcessor(store RecordingStore, processor RecordingProcessor) http.Handler {
	return NewRouterWithIdentity(store, defaultDevWorkspaceStore{}, NewDevAuthResolver("usr_dev"), processor)
}

// NewRouterWithIdentity builds the HTTP handler with recording, workspace, auth, and processor dependencies.
func NewRouterWithIdentity(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor) http.Handler {
	return newRouterWithDependencies(store, workspaceStore, authResolver, processor, nil, nil)
}

// NewRouterWithStorage builds the HTTP handler with injected recording store, processor, and object storage dependencies.
func NewRouterWithStorage(store RecordingStore, processor RecordingProcessor, objectStore storage.ObjectStore) http.Handler {
	return NewRouterWithStorageAndIdentity(store, defaultDevWorkspaceStore{}, NewDevAuthResolver("usr_dev"), processor, objectStore)
}

// NewRouterWithStorageAndIdentity builds the HTTP handler with all API dependencies.
func NewRouterWithStorageAndIdentity(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor, objectStore storage.ObjectStore) http.Handler {
	return newRouterWithDependencies(store, workspaceStore, authResolver, processor, objectStore, nil)
}

// NewRouterWithStorageIdentityAndPasswordAuth builds the HTTP handler with password auth endpoints enabled.
func NewRouterWithStorageIdentityAndPasswordAuth(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor, objectStore storage.ObjectStore, authConfig PasswordAuthConfig) http.Handler {
	return newRouterWithDependencies(store, workspaceStore, authResolver, processor, objectStore, &authConfig)
}

func newRouterWithDependencies(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor, objectStore storage.ObjectStore, authConfig *PasswordAuthConfig) http.Handler {
	if processor == nil {
		processor = noopRecordingProcessor{}
	}
	if workspaceStore == nil {
		workspaceStore = unconfiguredWorkspaceStore{}
	}
	if authResolver == nil {
		authResolver = NewDevAuthResolver("usr_dev")
	}

	router := chi.NewRouter()
	router.MethodFunc(http.MethodGet, "/healthz", healthzHandler)
	router.MethodFunc(http.MethodGet, "/openapi.yaml", openAPIHandler)
	router.MethodFunc(http.MethodGet, "/api-console", apiConsoleHandler)
	if authConfig != nil {
		router.MethodFunc(http.MethodPost, "/auth/signup", signUpHandler(*authConfig))
		router.MethodFunc(http.MethodPost, "/auth/signin", signInHandler(*authConfig))
		router.MethodFunc(http.MethodPost, "/auth/signout", signOutHandler(*authConfig))
	}
	router.MethodFunc(http.MethodGet, "/me", meHandler(workspaceStore, authResolver))
	router.MethodFunc(http.MethodGet, "/workspaces", workspacesHandler(workspaceStore, authResolver))

	router.Route("/workspaces/{workspace_id}", func(router chi.Router) {
		router.MethodFunc(http.MethodGet, "/recordings", withAuthorizedWorkspace(workspaceStore, authResolver, func(w http.ResponseWriter, r *http.Request, workspaceID string) {
			listRecordingsHandler(store, workspaceID)(w, r)
		}))
		router.MethodFunc(http.MethodPost, "/recordings", withAuthorizedWorkspace(workspaceStore, authResolver, func(w http.ResponseWriter, r *http.Request, workspaceID string) {
			createRecordingHandler(store, workspaceID)(w, r)
		}))
		router.MethodFunc(http.MethodPost, "/recordings/upload", withAuthorizedWorkspace(workspaceStore, authResolver, func(w http.ResponseWriter, r *http.Request, workspaceID string) {
			uploadRecordingHandler(store, processor, objectStore, workspaceID)(w, r)
		}))
		router.MethodFunc(http.MethodGet, "/recordings/{recording_id}", withAuthorizedRecording(workspaceStore, authResolver, func(w http.ResponseWriter, r *http.Request, workspaceID string, recordingID string) {
			getRecordingHandler(store, workspaceID, recordingID)(w, r)
		}))
		router.MethodFunc(http.MethodGet, "/recordings/{recording_id}/status", withAuthorizedRecording(workspaceStore, authResolver, func(w http.ResponseWriter, r *http.Request, workspaceID string, recordingID string) {
			getRecordingStatusHandler(store, workspaceID, recordingID)(w, r)
		}))
		router.MethodFunc(http.MethodGet, "/recordings/{recording_id}/details", withAuthorizedRecording(workspaceStore, authResolver, func(w http.ResponseWriter, r *http.Request, workspaceID string, recordingID string) {
			getRecordingDetailsHandler(store, workspaceID, recordingID)(w, r)
		}))
		router.MethodFunc(http.MethodPost, "/recordings/{recording_id}/retry", withAuthorizedRecording(workspaceStore, authResolver, func(w http.ResponseWriter, r *http.Request, workspaceID string, recordingID string) {
			retryRecordingHandler(store, processor, workspaceID, recordingID)(w, r)
		}))
	})

	return router
}

type workspaceRouteHandler func(http.ResponseWriter, *http.Request, string)

type recordingRouteHandler func(http.ResponseWriter, *http.Request, string, string)

func withAuthorizedWorkspace(workspaceStore WorkspaceStore, authResolver AuthResolver, next workspaceRouteHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID := chi.URLParam(r, "workspace_id")
		if workspaceID == "" {
			http.NotFound(w, r)
			return
		}
		if !authorizeWorkspace(w, r, workspaceStore, authResolver, workspaceID) {
			return
		}
		next(w, r, workspaceID)
	}
}

func withAuthorizedRecording(workspaceStore WorkspaceStore, authResolver AuthResolver, next recordingRouteHandler) http.HandlerFunc {
	return withAuthorizedWorkspace(workspaceStore, authResolver, func(w http.ResponseWriter, r *http.Request, workspaceID string) {
		recordingID := chi.URLParam(r, "recording_id")
		if recordingID == "" {
			http.NotFound(w, r)
			return
		}
		next(w, r, workspaceID, recordingID)
	})
}
