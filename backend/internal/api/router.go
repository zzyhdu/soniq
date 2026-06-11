package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	apidocs "github.com/zzyhdu/soniq/backend/doc"
	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
)

const maxUploadRequestBytes = 100 << 20 // 100 MiB

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

type recordingDetailsResponse struct {
	Recording  recordingResponse            `json:"recording"`
	Transcript *recordingTranscriptResponse `json:"transcript"`
	Segments   []recordingSegmentResponse   `json:"segments"`
	Summary    *recordingSummaryResponse    `json:"summary"`
}

type uploadRecordingResponse struct {
	Recording          domain.Recording `json:"recording"`
	ProcessingEnqueued bool             `json:"processing_enqueued"`
}

type listRecordingsResponse struct {
	Recordings []recordingResponse `json:"recordings"`
}

type listWorkspacesResponse struct {
	Workspaces []workspaceResponse `json:"workspaces"`
}

type recordingResponse struct {
	ID               string                 `json:"id"`
	WorkspaceID      string                 `json:"workspace_id"`
	Title            string                 `json:"title"`
	Status           domain.RecordingStatus `json:"status"`
	WorkflowType     domain.WorkflowType    `json:"workflow_type"`
	Language         string                 `json:"language"`
	AudioObjectKey   string                 `json:"audio_object_key,omitempty"`
	AudioContentType string                 `json:"audio_content_type,omitempty"`
	AudioSizeBytes   int64                  `json:"audio_size_bytes,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type recordingTranscriptResponse struct {
	RecordingID   string    `json:"recording_id"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	Language      string    `json:"language"`
	Text          string    `json:"text"`
	TranscribedAt time.Time `json:"transcribed_at"`
}

type recordingSegmentResponse struct {
	ID           string  `json:"id"`
	RecordingID  string  `json:"recording_id"`
	SegmentIndex int     `json:"segment_index"`
	StartMS      int     `json:"start_ms"`
	EndMS        int     `json:"end_ms"`
	SpeakerLabel string  `json:"speaker_label"`
	Text         string  `json:"text"`
	Confidence   float64 `json:"confidence"`
}

type recordingSummaryResponse struct {
	RecordingID     string              `json:"recording_id"`
	Provider        string              `json:"provider"`
	Model           string              `json:"model"`
	Type            domain.WorkflowType `json:"type"`
	Title           string              `json:"title"`
	Overview        string              `json:"overview"`
	ContentMarkdown string              `json:"content_markdown"`
	SummarizedAt    time.Time           `json:"summarized_at"`
}

type workspaceResponse struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Role      domain.WorkspaceRole `json:"role"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
}

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

func openAPIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serveEmbeddedFile(w, apidocs.OpenAPI, "application/yaml; charset=utf-8")
}

func apiConsoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serveEmbeddedFile(w, apidocs.APIConsole, "text/html; charset=utf-8")
}

func serveEmbeddedFile(w http.ResponseWriter, body []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(body)
}

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

func workspaceByIDHandler(store RecordingStore, workspaceStore WorkspaceStore, authResolver AuthResolver, processor RecordingProcessor, objectStore storage.ObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, rest, ok := parseWorkspacePath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if !authorizeWorkspace(w, r, workspaceStore, authResolver, workspaceID) {
			return
		}
		switch {
		case rest == "/recordings":
			switch r.Method {
			case http.MethodGet:
				listRecordingsHandler(store, workspaceID)(w, r)
				return
			case http.MethodPost:
				createRecordingHandler(store, workspaceID)(w, r)
				return
			default:
				w.Header().Set("Allow", "GET, POST")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
		case rest == "/recordings/upload":
			uploadRecordingHandler(store, processor, objectStore, workspaceID)(w, r)
			return
		case strings.HasPrefix(rest, "/recordings/"):
			recordingByIDHandler(store, workspaceID, strings.TrimPrefix(rest, "/recordings/"))(w, r)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}
}

func createRecordingHandler(store RecordingStore, workspaceID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var request struct {
			Title        string `json:"title"`
			WorkflowType string `json:"workflow_type"`
			Language     string `json:"language"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		recording, err := store.Create(recordings.CreateRecordingInput{
			WorkspaceID:  workspaceID,
			Title:        request.Title,
			WorkflowType: domain.WorkflowType(request.WorkflowType),
			Language:     request.Language,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(recording)
	}
}

func listRecordingsHandler(store RecordingStore, workspaceID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		limit := 50
		if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed < 0 {
				http.Error(w, "invalid limit", http.StatusBadRequest)
				return
			}
			limit = parsed
		}
		recordingRows, err := store.ListByWorkspace(recordings.ListRecordingsInput{
			WorkspaceID: workspaceID,
			Limit:       limit,
		})
		if err != nil {
			http.Error(w, "list recordings", http.StatusInternalServerError)
			return
		}
		response := listRecordingsResponse{Recordings: make([]recordingResponse, 0, len(recordingRows))}
		for _, recording := range recordingRows {
			response.Recordings = append(response.Recordings, toRecordingResponse(recording))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

func uploadRecordingHandler(store RecordingStore, processor RecordingProcessor, objectStore storage.ObjectStore, workspaceID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if objectStore == nil {
			http.Error(w, "object storage is not configured", http.StatusInternalServerError)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxUploadRequestBytes)
		if err := r.ParseMultipartForm(maxUploadRequestBytes); err != nil {
			http.Error(w, "invalid multipart form", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("audio")
		if err != nil {
			http.Error(w, "audio file is required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		contentType := header.Header.Get("Content-Type")
		objectKey := recordingAudioObjectKey(header.Filename)
		putResult, err := objectStore.PutObject(r.Context(), storage.PutObjectInput{
			Key:         objectKey,
			Body:        file,
			ContentType: contentType,
		})
		if err != nil {
			http.Error(w, "store audio object", http.StatusInternalServerError)
			return
		}

		recording, err := store.Create(recordings.CreateRecordingInput{
			WorkspaceID:      workspaceID,
			Title:            r.FormValue("title"),
			WorkflowType:     domain.WorkflowType(r.FormValue("workflow_type")),
			Language:         r.FormValue("language"),
			AudioObjectKey:   putResult.Key,
			AudioContentType: contentType,
			AudioSizeBytes:   putResult.SizeBytes,
		})
		if err != nil {
			_ = objectStore.DeleteObject(r.Context(), putResult.Key)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		processingEnqueued := processor.Enqueue(recording) == nil

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(uploadRecordingResponse{
			Recording:          recording,
			ProcessingEnqueued: processingEnqueued,
		})
	}
}

func recordingAudioObjectKey(filename string) string {
	name := filepath.Base(filename)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "audio"
	}
	return "recordings/" + time.Now().UTC().Format("20060102T150405.000000000Z") + "/" + name
}

func recordingByIDHandler(store RecordingStore, workspaceID string, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var wantsDetails bool
		id, wantsStatus := strings.CutSuffix(path, "/status")
		if !wantsStatus {
			id, wantsDetails = strings.CutSuffix(path, "/details")
		}
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}

		recording, ok, err := store.GetForWorkspace(recordings.GetRecordingInput{WorkspaceID: workspaceID, ID: id})
		if err != nil {
			http.Error(w, "get recording", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}

		if wantsStatus {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(struct {
				ID          string                 `json:"id"`
				WorkspaceID string                 `json:"workspace_id"`
				Status      domain.RecordingStatus `json:"status"`
			}{
				ID:          recording.ID,
				WorkspaceID: recording.WorkspaceID,
				Status:      recording.Status,
			})
			return
		}
		if wantsDetails {
			detailsStore, ok := store.(RecordingDetailsStore)
			if !ok {
				http.Error(w, "recording details are not configured", http.StatusInternalServerError)
				return
			}
			details := recordingDetailsResponse{Recording: toRecordingResponse(recording), Segments: []recordingSegmentResponse{}}
			transcript, hasTranscript, err := detailsStore.GetTranscript(id)
			if err != nil {
				http.Error(w, "get recording transcript", http.StatusInternalServerError)
				return
			}
			if hasTranscript {
				details.Transcript = toRecordingTranscriptResponse(transcript)
				segments, err := detailsStore.ListTranscriptSegments(id)
				if err != nil {
					http.Error(w, "list recording transcript segments", http.StatusInternalServerError)
					return
				}
				details.Segments = toRecordingSegmentResponses(segments)
			}
			summary, hasSummary, err := detailsStore.GetSummary(id)
			if err != nil {
				http.Error(w, "get recording summary", http.StatusInternalServerError)
				return
			}
			if hasSummary {
				details.Summary = toRecordingSummaryResponse(summary)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(details)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(recording)
	}
}

func toRecordingResponse(recording domain.Recording) recordingResponse {
	return recordingResponse{
		ID:               recording.ID,
		WorkspaceID:      recording.WorkspaceID,
		Title:            recording.Title,
		Status:           recording.Status,
		WorkflowType:     recording.WorkflowType,
		Language:         recording.Language,
		AudioObjectKey:   recording.AudioObjectKey,
		AudioContentType: recording.AudioContentType,
		AudioSizeBytes:   recording.AudioSizeBytes,
		CreatedAt:        recording.CreatedAt,
		UpdatedAt:        recording.UpdatedAt,
	}
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

func authorizeWorkspace(w http.ResponseWriter, r *http.Request, workspaceStore WorkspaceStore, authResolver AuthResolver, workspaceID string) bool {
	currentUser, ok := resolveCurrentUser(w, r, authResolver)
	if !ok {
		return false
	}
	_, found, err := workspaceStore.GetWorkspaceForUser(r.Context(), currentUser.UserID, workspaceID)
	if err != nil {
		http.Error(w, "get workspace", http.StatusInternalServerError)
		return false
	}
	if !found {
		http.NotFound(w, r)
		return false
	}
	return true
}

func parseWorkspacePath(path string) (string, string, bool) {
	path = strings.TrimPrefix(path, "/workspaces/")
	workspaceID, rest, ok := strings.Cut(path, "/")
	if !ok || workspaceID == "" {
		return "", "", false
	}
	return workspaceID, "/" + rest, true
}

func toRecordingTranscriptResponse(transcript recordings.RecordingTranscript) *recordingTranscriptResponse {
	return &recordingTranscriptResponse{
		RecordingID:   transcript.RecordingID,
		Provider:      transcript.Provider,
		Model:         transcript.Model,
		Language:      transcript.Language,
		Text:          transcript.Text,
		TranscribedAt: transcript.TranscribedAt,
	}
}

func toRecordingSegmentResponses(segments []recordings.RecordingTranscriptSegment) []recordingSegmentResponse {
	responses := make([]recordingSegmentResponse, 0, len(segments))
	for _, segment := range segments {
		responses = append(responses, recordingSegmentResponse{
			ID:           segment.ID,
			RecordingID:  segment.RecordingID,
			SegmentIndex: segment.SegmentIndex,
			StartMS:      segment.StartMS,
			EndMS:        segment.EndMS,
			SpeakerLabel: segment.SpeakerLabel,
			Text:         segment.Text,
			Confidence:   segment.Confidence,
		})
	}
	return responses
}

func toRecordingSummaryResponse(summary recordings.RecordingSummary) *recordingSummaryResponse {
	return &recordingSummaryResponse{
		RecordingID:     summary.RecordingID,
		Provider:        summary.Provider,
		Model:           summary.Model,
		Type:            summary.Type,
		Title:           summary.Title,
		Overview:        summary.Overview,
		ContentMarkdown: summary.ContentMarkdown,
		SummarizedAt:    summary.SummarizedAt,
	}
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(struct {
		Status  string `json:"status"`
		Service string `json:"service"`
	}{
		Status:  "ok",
		Service: "soniq-api",
	})
}
