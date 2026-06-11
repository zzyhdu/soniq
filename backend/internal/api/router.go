package api

import (
	"context"
	"errors"
	"net/http"

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
	if processor == nil {
		processor = noopRecordingProcessor{}
	}
	if workspaceStore == nil {
		workspaceStore = unconfiguredWorkspaceStore{}
	}
	if authResolver == nil {
		authResolver = NewDevAuthResolver("usr_dev")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/openapi.yaml", openAPIHandler)
	mux.HandleFunc("/api-console", apiConsoleHandler)
	mux.HandleFunc("/me", meHandler(workspaceStore, authResolver))
	mux.HandleFunc("/workspaces", workspacesHandler(workspaceStore, authResolver))
	mux.HandleFunc("/workspaces/", workspaceByIDHandler(store, workspaceStore, authResolver, processor, nil))
	return mux
}

// NewRouterWithStorage builds the HTTP handler with injected recording store, processor, and object storage dependencies.
func NewRouterWithStorage(store RecordingStore, processor RecordingProcessor, objectStore storage.ObjectStore) http.Handler {
	return NewRouterWithStorageAndIdentity(store, defaultDevWorkspaceStore{}, NewDevAuthResolver("usr_dev"), processor, objectStore)
}

// NewRouterWithStorageAndIdentity builds the HTTP handler with all API dependencies.
func NewRouterWithStorageAndIdentity(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor, objectStore storage.ObjectStore) http.Handler {
	if processor == nil {
		processor = noopRecordingProcessor{}
	}
	if workspaceStore == nil {
		workspaceStore = unconfiguredWorkspaceStore{}
	}
	if authResolver == nil {
		authResolver = NewDevAuthResolver("usr_dev")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/openapi.yaml", openAPIHandler)
	mux.HandleFunc("/api-console", apiConsoleHandler)
	mux.HandleFunc("/me", meHandler(workspaceStore, authResolver))
	mux.HandleFunc("/workspaces", workspacesHandler(workspaceStore, authResolver))
	mux.HandleFunc("/workspaces/", workspaceByIDHandler(store, workspaceStore, authResolver, processor, objectStore))
	return mux
}
