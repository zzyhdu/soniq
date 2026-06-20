package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/observability"
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
	UpdateForWorkspace(input recordings.UpdateRecordingInput) (domain.Recording, bool, error)
	GetTranscript(recordingID string) (recordings.RecordingTranscript, bool, error)
	ListTranscriptSegments(recordingID string) ([]recordings.RecordingTranscriptSegment, error)
	GetSummary(recordingID string) (recordings.RecordingSummary, bool, error)
	GetMindMap(recordingID string) (recordings.RecordingMindMap, bool, error)
	ResetForRetry(input recordings.RetryRecordingInput) (domain.Recording, error)
	UpdateStatus(input recordings.UpdateRecordingStatusInput) (domain.Recording, error)
	SoftDeleteForWorkspace(input recordings.SoftDeleteRecordingInput) (domain.Recording, bool, error)
	ListDeletedByWorkspace(input recordings.ListDeletedRecordingsInput) ([]domain.Recording, error)
	RestoreForWorkspace(input recordings.RestoreRecordingInput) (domain.Recording, bool, error)
	PurgeForWorkspace(input recordings.PurgeRecordingInput) (recordings.PurgeRecordingResult, bool, error)
	MarkPurgeArtifactDeleted(input recordings.MarkPurgeArtifactDeletedInput) (bool, error)
	MarkPurgeArtifactFailed(input recordings.MarkPurgeArtifactFailedInput) (bool, error)
}

// WorkspaceStore is the persistence seam required by identity and workspace-scoped handlers.
type WorkspaceStore interface {
	GetUser(ctx context.Context, userID string) (domain.User, bool, error)
	ListWorkspacesForUser(ctx context.Context, userID string) ([]domain.WorkspaceWithRole, error)
	GetWorkspaceForUser(ctx context.Context, userID string, workspaceID string) (domain.WorkspaceWithRole, bool, error)
}

// RecordingProcessor is the enqueue seam invoked after a recording is created.
type RecordingProcessor interface {
	Enqueue(ctx context.Context, recording domain.Recording) error
}

type noopRecordingProcessor struct{}

type unconfiguredRecordingStore struct{}
type unconfiguredWorkspaceStore struct{}
type defaultDevWorkspaceStore struct{}

func (noopRecordingProcessor) Enqueue(context.Context, domain.Recording) error { return nil }

func (unconfiguredRecordingStore) Create(recordings.CreateRecordingInput) (domain.Recording, error) {
	return domain.Recording{}, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) GetForWorkspace(recordings.GetRecordingInput) (domain.Recording, bool, error) {
	return domain.Recording{}, false, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) ListByWorkspace(recordings.ListRecordingsInput) ([]domain.Recording, error) {
	return nil, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) UpdateForWorkspace(recordings.UpdateRecordingInput) (domain.Recording, bool, error) {
	return domain.Recording{}, false, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) GetTranscript(string) (recordings.RecordingTranscript, bool, error) {
	return recordings.RecordingTranscript{}, false, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) ListTranscriptSegments(string) ([]recordings.RecordingTranscriptSegment, error) {
	return nil, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) GetSummary(string) (recordings.RecordingSummary, bool, error) {
	return recordings.RecordingSummary{}, false, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) GetMindMap(string) (recordings.RecordingMindMap, bool, error) {
	return recordings.RecordingMindMap{}, false, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) ResetForRetry(recordings.RetryRecordingInput) (domain.Recording, error) {
	return domain.Recording{}, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) UpdateStatus(recordings.UpdateRecordingStatusInput) (domain.Recording, error) {
	return domain.Recording{}, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) SoftDeleteForWorkspace(recordings.SoftDeleteRecordingInput) (domain.Recording, bool, error) {
	return domain.Recording{}, false, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) ListDeletedByWorkspace(recordings.ListDeletedRecordingsInput) ([]domain.Recording, error) {
	return nil, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) RestoreForWorkspace(recordings.RestoreRecordingInput) (domain.Recording, bool, error) {
	return domain.Recording{}, false, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) PurgeForWorkspace(recordings.PurgeRecordingInput) (recordings.PurgeRecordingResult, bool, error) {
	return recordings.PurgeRecordingResult{}, false, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) MarkPurgeArtifactDeleted(recordings.MarkPurgeArtifactDeletedInput) (bool, error) {
	return false, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) MarkPurgeArtifactFailed(recordings.MarkPurgeArtifactFailedInput) (bool, error) {
	return false, errRecordingStoreNotConfigured
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
	return newRouterWithDependencies(store, workspaceStore, authResolver, processor, nil, nil, nil, RouterOptions{})
}

// NewRouterWithStorage builds the HTTP handler with injected recording store, processor, and object storage dependencies.
func NewRouterWithStorage(store RecordingStore, processor RecordingProcessor, objectStore storage.ObjectStore) http.Handler {
	return NewRouterWithStorageAndIdentity(store, defaultDevWorkspaceStore{}, NewDevAuthResolver("usr_dev"), processor, objectStore)
}

// NewRouterWithStorageAndIdentity builds the HTTP handler with all API dependencies.
func NewRouterWithStorageAndIdentity(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor, objectStore storage.ObjectStore) http.Handler {
	return newRouterWithDependencies(store, workspaceStore, authResolver, processor, objectStore, nil, nil, RouterOptions{})
}

// NewRouterWithStorageIdentityAndPasswordAuth builds the HTTP handler with password auth endpoints enabled.
func NewRouterWithStorageIdentityAndPasswordAuth(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor, objectStore storage.ObjectStore, authConfig PasswordAuthConfig) http.Handler {
	return newRouterWithStorageIdentityPasswordAuthAndReadiness(store, workspaceStore, authResolver, processor, objectStore, authConfig, nil, RouterOptions{})
}

// NewRouterWithStorageIdentityPasswordAuthAndReadiness builds the HTTP handler with production API dependencies and readiness checks.
func NewRouterWithStorageIdentityPasswordAuthAndReadiness(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor, objectStore storage.ObjectStore, authConfig PasswordAuthConfig, readinessChecker ReadinessChecker) http.Handler {
	return newRouterWithStorageIdentityPasswordAuthAndReadiness(store, workspaceStore, authResolver, processor, objectStore, authConfig, readinessChecker, RouterOptions{})
}

// NewRouterWithStorageIdentityPasswordAuthReadinessAndOptions builds the production API handler with optional observability settings.
func NewRouterWithStorageIdentityPasswordAuthReadinessAndOptions(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor, objectStore storage.ObjectStore, authConfig PasswordAuthConfig, readinessChecker ReadinessChecker, options RouterOptions) http.Handler {
	return newRouterWithStorageIdentityPasswordAuthAndReadiness(store, workspaceStore, authResolver, processor, objectStore, authConfig, readinessChecker, options)
}

// NewRouterWithReadiness builds a local/test HTTP handler with an injected readiness checker.
func NewRouterWithReadiness(readinessChecker ReadinessChecker) http.Handler {
	return newRouterWithDependencies(unconfiguredRecordingStore{}, unconfiguredWorkspaceStore{}, NewDevAuthResolver("usr_dev"), noopRecordingProcessor{}, nil, nil, readinessChecker, RouterOptions{})
}

func newRouterWithStorageIdentityPasswordAuthAndReadiness(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor, objectStore storage.ObjectStore, authConfig PasswordAuthConfig, readinessChecker ReadinessChecker, options RouterOptions) http.Handler {
	return newRouterWithDependencies(store, workspaceStore, authResolver, processor, objectStore, &authConfig, readinessChecker, options)
}

func newRouterWithDependencies(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor, objectStore storage.ObjectStore, authConfig *PasswordAuthConfig, readinessChecker ReadinessChecker, options RouterOptions) http.Handler {
	if processor == nil {
		processor = noopRecordingProcessor{}
	}
	if workspaceStore == nil {
		workspaceStore = unconfiguredWorkspaceStore{}
	}
	if authResolver == nil {
		authResolver = NewDevAuthResolver("usr_dev")
	}

	metrics := observability.NewMetrics()
	router := chi.NewRouter()
	router.Use(requestLoggingMiddleware(nil, metrics))
	router.Use(requestTracingMiddleware(options.HTTPTracing))
	if authConfig != nil {
		router.Use(csrfProtectionMiddleware(*authConfig))
	}
	router.MethodFunc(http.MethodGet, "/healthz", healthzHandler)
	router.MethodFunc(http.MethodGet, "/readyz", readyzHandler(readinessChecker))
	router.Method(http.MethodGet, "/metrics", metrics.Handler())
	router.MethodFunc(http.MethodGet, "/openapi.yaml", openAPIHandler)
	router.MethodFunc(http.MethodGet, "/api-console", apiConsoleHandler)
	if authConfig != nil {
		router.MethodFunc(http.MethodPost, "/auth/signup", signUpHandler(*authConfig))
		router.MethodFunc(http.MethodPost, "/auth/signin", signInHandler(*authConfig))
		router.MethodFunc(http.MethodPost, "/auth/signout", signOutHandler(*authConfig))
	}

	router.Group(func(router chi.Router) {
		router.Use(requireAuth(authResolver))

		router.MethodFunc(http.MethodGet, "/me", meHandler(workspaceStore))
		router.MethodFunc(http.MethodGet, "/workspaces", workspacesHandler(workspaceStore))

		router.Route("/workspaces/{workspace_id}", func(router chi.Router) {
			router.Use(requireWorkspace(workspaceStore))

			router.MethodFunc(http.MethodGet, "/recordings", listRecordingsHandler(store))
			router.MethodFunc(http.MethodPost, "/recordings", createRecordingHandler(store))
			router.MethodFunc(http.MethodPost, "/recordings/upload", uploadRecordingHandler(store, processor, objectStore))
			router.MethodFunc(http.MethodGet, "/recordings/trash", listDeletedRecordingsHandler(store))
			router.MethodFunc(http.MethodGet, "/recordings/{recording_id}", getRecordingHandler(store))
			router.MethodFunc(http.MethodPatch, "/recordings/{recording_id}", updateRecordingHandler(store))
			router.MethodFunc(http.MethodDelete, "/recordings/{recording_id}", deleteRecordingHandler(store))
			router.MethodFunc(http.MethodGet, "/recordings/{recording_id}/status", getRecordingStatusHandler(store))
			router.MethodFunc(http.MethodGet, "/recordings/{recording_id}/details", getRecordingDetailsHandler(store))
			router.MethodFunc(http.MethodPost, "/recordings/{recording_id}/restore", restoreRecordingHandler(store))
			router.MethodFunc(http.MethodDelete, "/recordings/{recording_id}/purge", purgeRecordingHandler(store, objectStore))
			router.MethodFunc(http.MethodPost, "/recordings/{recording_id}/retry", retryRecordingHandler(store, processor))
		})
	})

	return router
}
