package api

import (
	"encoding/json"
	"errors"
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

func createRecordingHandler(store RecordingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		workspace, ok := workspaceFromRequest(w, r)
		if !ok {
			return
		}

		var request struct {
			Title        string `json:"title"`
			WorkflowType string `json:"workflow_type"`
			Language     string `json:"language"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeAPIError(w, http.StatusBadRequest, errorCodeValidationFailed, "invalid json")
			return
		}

		recording, err := store.Create(recordings.CreateRecordingInput{
			WorkspaceID:  workspace.ID,
			Title:        request.Title,
			WorkflowType: domain.WorkflowType(request.WorkflowType),
			Language:     request.Language,
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, errorCodeValidationFailed, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(recording)
	}
}

func listRecordingsHandler(store RecordingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		workspace, ok := workspaceFromRequest(w, r)
		if !ok {
			return
		}
		limit := 50
		if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed < 0 {
				writeAPIError(w, http.StatusBadRequest, errorCodeValidationFailed, "invalid limit")
				return
			}
			limit = parsed
		}
		recordingRows, err := store.ListByWorkspace(recordings.ListRecordingsInput{
			WorkspaceID: workspace.ID,
			Limit:       limit,
		})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "list recordings")
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

func uploadRecordingHandler(store RecordingStore, processor RecordingProcessor, objectStore storage.ObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		workspace, ok := workspaceFromRequest(w, r)
		if !ok {
			return
		}
		if objectStore == nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "object storage is not configured")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxUploadRequestBytes)
		if err := r.ParseMultipartForm(maxUploadRequestBytes); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeAPIError(w, http.StatusRequestEntityTooLarge, errorCodeRequestTooLarge, "request too large")
				return
			}
			writeAPIError(w, http.StatusBadRequest, errorCodeValidationFailed, "invalid multipart form")
			return
		}

		file, header, err := r.FormFile("audio")
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, errorCodeValidationFailed, "audio file is required")
			return
		}
		defer file.Close()

		contentType := header.Header.Get("Content-Type")
		objectKey := recordingAudioObjectKey(workspace.ID, header.Filename)
		putResult, err := objectStore.PutObject(r.Context(), storage.PutObjectInput{
			Key:         objectKey,
			Body:        file,
			ContentType: contentType,
		})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "store audio object")
			return
		}

		recording, err := store.Create(recordings.CreateRecordingInput{
			WorkspaceID:      workspace.ID,
			Title:            r.FormValue("title"),
			WorkflowType:     domain.WorkflowType(r.FormValue("workflow_type")),
			Language:         r.FormValue("language"),
			AudioObjectKey:   putResult.Key,
			AudioContentType: contentType,
			AudioSizeBytes:   putResult.SizeBytes,
		})
		if err != nil {
			_ = objectStore.DeleteObject(r.Context(), putResult.Key)
			writeAPIError(w, http.StatusBadRequest, errorCodeValidationFailed, err.Error())
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

func getRecordingHandler(store RecordingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		workspace, ok := workspaceFromRequest(w, r)
		if !ok {
			return
		}
		id, ok := recordingIDFromRequest(w, r)
		if !ok {
			return
		}

		recording, ok, err := store.GetForWorkspace(recordings.GetRecordingInput{WorkspaceID: workspace.ID, ID: id})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "get recording")
			return
		}
		if !ok {
			writeAPIError(w, http.StatusNotFound, errorCodeNotFound, "not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(recording)
	}
}

func deleteRecordingHandler(store RecordingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeMethodNotAllowed(w, http.MethodDelete)
			return
		}
		workspace, ok := workspaceFromRequest(w, r)
		if !ok {
			return
		}
		currentUser, ok := currentUserFromRequest(w, r)
		if !ok {
			return
		}
		id, ok := recordingIDFromRequest(w, r)
		if !ok {
			return
		}

		deleteStore, ok := store.(RecordingSoftDeleteStore)
		if !ok {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "recording delete is not configured")
			return
		}
		_, found, err := deleteStore.SoftDeleteForWorkspace(recordings.SoftDeleteRecordingInput{
			WorkspaceID:     workspace.ID,
			ID:              id,
			DeletedByUserID: currentUser.UserID,
		})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "delete recording")
			return
		}
		if !found {
			writeAPIError(w, http.StatusNotFound, errorCodeNotFound, "not found")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func getRecordingStatusHandler(store RecordingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		workspace, ok := workspaceFromRequest(w, r)
		if !ok {
			return
		}
		id, ok := recordingIDFromRequest(w, r)
		if !ok {
			return
		}

		recording, ok, err := store.GetForWorkspace(recordings.GetRecordingInput{WorkspaceID: workspace.ID, ID: id})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "get recording")
			return
		}
		if !ok {
			writeAPIError(w, http.StatusNotFound, errorCodeNotFound, "not found")
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

func getRecordingDetailsHandler(store RecordingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		workspace, ok := workspaceFromRequest(w, r)
		if !ok {
			return
		}
		id, ok := recordingIDFromRequest(w, r)
		if !ok {
			return
		}

		recording, ok, err := store.GetForWorkspace(recordings.GetRecordingInput{WorkspaceID: workspace.ID, ID: id})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "get recording")
			return
		}
		if !ok {
			writeAPIError(w, http.StatusNotFound, errorCodeNotFound, "not found")
			return
		}

		detailsStore, ok := store.(RecordingDetailsStore)
		if !ok {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "recording details are not configured")
			return
		}
		details := recordingDetailsResponse{Recording: toRecordingResponse(recording), Segments: []recordingSegmentResponse{}}
		transcript, hasTranscript, err := detailsStore.GetTranscript(id)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "get recording transcript")
			return
		}
		if hasTranscript {
			details.Transcript = toRecordingTranscriptResponse(transcript)
			segments, err := detailsStore.ListTranscriptSegments(id)
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "list recording transcript segments")
				return
			}
			details.Segments = toRecordingSegmentResponses(segments)
		}
		summary, hasSummary, err := detailsStore.GetSummary(id)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "get recording summary")
			return
		}
		if hasSummary {
			details.Summary = toRecordingSummaryResponse(summary)
		}
		mindMap, hasMindMap, err := detailsStore.GetMindMap(id)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "get recording mind map")
			return
		}
		if hasMindMap {
			details.MindMap = toRecordingMindMapResponse(mindMap)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(details)
	}
}

func retryRecordingHandler(store RecordingStore, processor RecordingProcessor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		workspace, ok := workspaceFromRequest(w, r)
		if !ok {
			return
		}
		id, ok := recordingIDFromRequest(w, r)
		if !ok {
			return
		}

		recording, ok, err := store.GetForWorkspace(recordings.GetRecordingInput{WorkspaceID: workspace.ID, ID: id})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "get recording")
			return
		}
		if !ok {
			writeAPIError(w, http.StatusNotFound, errorCodeNotFound, "not found")
			return
		}
		if recording.Status != domain.RecordingStatusFailed {
			writeAPIError(w, http.StatusConflict, errorCodeConflict, "recording is not failed")
			return
		}
		if strings.TrimSpace(recording.AudioObjectKey) == "" {
			writeAPIError(w, http.StatusConflict, errorCodeConflict, "recording has no audio to retry")
			return
		}

		retryStore, ok := store.(RecordingRetryStore)
		if !ok {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "recording retry is not configured")
			return
		}
		resetRecording, err := retryStore.ResetForRetry(recordings.RetryRecordingInput{WorkspaceID: workspace.ID, ID: id})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "reset recording retry")
			return
		}
		processingEnqueued := true
		if err := processor.Enqueue(resetRecording); err != nil {
			processingEnqueued = false
			failedRecording, failErr := retryStore.UpdateStatus(recordings.UpdateRecordingStatusInput{
				WorkspaceID:   workspace.ID,
				ID:            id,
				Status:        domain.RecordingStatusFailed,
				FailureReason: "retry enqueue failed: " + err.Error(),
			})
			if failErr != nil {
				writeAPIError(w, http.StatusInternalServerError, errorCodeInternalError, "mark retry enqueue failure")
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
