package api

import (
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
)

const maxUploadRequestBytes = 100 << 20 // 100 MiB

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
		objectKey := recordingAudioObjectKey(workspaceID, header.Filename)
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

func recordingAudioObjectKey(workspaceID string, filename string) string {
	name := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	if name == "." || name == ".." || name == "/" || name == "" {
		name = "audio"
	}
	return "workspaces/" + workspaceID + "/recordings/" + time.Now().UTC().Format("20060102T150405.000000000Z") + "/" + name
}

func getRecordingHandler(store RecordingStore, workspaceID string, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(recording)
	}
}

func getRecordingStatusHandler(store RecordingStore, workspaceID string, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			ID            string                 `json:"id"`
			WorkspaceID   string                 `json:"workspace_id"`
			Status        domain.RecordingStatus `json:"status"`
			FailureReason string                 `json:"failure_reason,omitempty"`
			CompletedAt   *time.Time             `json:"completed_at,omitempty"`
			FailedAt      *time.Time             `json:"failed_at,omitempty"`
		}{
			ID:            recording.ID,
			WorkspaceID:   recording.WorkspaceID,
			Status:        recording.Status,
			FailureReason: recording.FailureReason,
			CompletedAt:   recording.CompletedAt,
			FailedAt:      recording.FailedAt,
		})
	}
}

func getRecordingDetailsHandler(store RecordingStore, workspaceID string, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	}
}

func retryRecordingHandler(store RecordingStore, processor RecordingProcessor, workspaceID string, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
		if recording.Status != domain.RecordingStatusFailed {
			http.Error(w, "recording is not failed", http.StatusConflict)
			return
		}
		if strings.TrimSpace(recording.AudioObjectKey) == "" {
			http.Error(w, "recording has no audio to retry", http.StatusConflict)
			return
		}

		retryStore, ok := store.(RecordingRetryStore)
		if !ok {
			http.Error(w, "recording retry is not configured", http.StatusInternalServerError)
			return
		}
		resetRecording, err := retryStore.ResetForRetry(recordings.RetryRecordingInput{WorkspaceID: workspaceID, ID: id})
		if err != nil {
			http.Error(w, "reset recording retry", http.StatusInternalServerError)
			return
		}
		processingEnqueued := true
		if err := processor.Enqueue(resetRecording); err != nil {
			processingEnqueued = false
			failedRecording, failErr := retryStore.UpdateStatus(recordings.UpdateRecordingStatusInput{
				WorkspaceID:   workspaceID,
				ID:            id,
				Status:        domain.RecordingStatusFailed,
				FailureReason: "retry enqueue failed: " + err.Error(),
			})
			if failErr != nil {
				http.Error(w, "mark retry enqueue failure", http.StatusInternalServerError)
				return
			}
			resetRecording = failedRecording
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(retryRecordingResponse{
			Recording:          toRecordingResponse(resetRecording),
			ProcessingEnqueued: processingEnqueued,
		})
	}
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
