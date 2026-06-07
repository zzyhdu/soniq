package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
)

const maxUploadRequestBytes = 100 << 20 // 100 MiB

var errRecordingStoreNotConfigured = errors.New("recording store is not configured")

// RecordingStore is the persistence seam required by the recording HTTP handlers.
type RecordingStore interface {
	Create(recordings.CreateRecordingInput) (domain.Recording, error)
	Get(id string) (domain.Recording, bool)
}

// RecordingDetailsStore is the optional persistence seam for transcript and summary detail reads.
type RecordingDetailsStore interface {
	RecordingStore
	GetTranscript(recordingID string) (recordings.RecordingTranscript, bool)
	ListTranscriptSegments(recordingID string) []recordings.RecordingTranscriptSegment
	GetSummary(recordingID string) (recordings.RecordingSummary, bool)
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

type recordingResponse struct {
	ID               string                 `json:"id"`
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

type unconfiguredRecordingStore struct{}

func (noopRecordingProcessor) Enqueue(domain.Recording) error { return nil }

func (unconfiguredRecordingStore) Create(recordings.CreateRecordingInput) (domain.Recording, error) {
	return domain.Recording{}, errRecordingStoreNotConfigured
}

func (unconfiguredRecordingStore) Get(string) (domain.Recording, bool) {
	return domain.Recording{}, false
}

// NewRouter builds the HTTP handler for the Soniq API.
func NewRouter() http.Handler {
	return NewRouterWithProcessor(unconfiguredRecordingStore{}, noopRecordingProcessor{})
}

// NewRouterWithStore builds the HTTP handler with an injected recording store.
func NewRouterWithStore(store RecordingStore) http.Handler {
	return NewRouterWithProcessor(store, noopRecordingProcessor{})
}

// NewRouterWithProcessor builds the HTTP handler with injected recording store and processor dependencies.
func NewRouterWithProcessor(store RecordingStore, processor RecordingProcessor) http.Handler {
	if processor == nil {
		processor = noopRecordingProcessor{}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/recordings", createRecordingHandler(store))
	mux.HandleFunc("/recordings/", recordingByIDHandler(store))
	return mux
}

// NewRouterWithStorage builds the HTTP handler with injected recording store, processor, and object storage dependencies.
func NewRouterWithStorage(store RecordingStore, processor RecordingProcessor, objectStore storage.ObjectStore) http.Handler {
	if processor == nil {
		processor = noopRecordingProcessor{}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/recordings", createRecordingHandler(store))
	mux.HandleFunc("/recordings/upload", uploadRecordingHandler(store, processor, objectStore))
	mux.HandleFunc("/recordings/", recordingByIDHandler(store))
	return mux
}

func createRecordingHandler(store RecordingStore) http.HandlerFunc {
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

func uploadRecordingHandler(store RecordingStore, processor RecordingProcessor, objectStore storage.ObjectStore) http.HandlerFunc {
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
		_ = processor.Enqueue(recording)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(recording)
	}
}

func recordingAudioObjectKey(filename string) string {
	name := filepath.Base(filename)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "audio"
	}
	return "recordings/" + time.Now().UTC().Format("20060102T150405.000000000Z") + "/" + name
}

func recordingByIDHandler(store RecordingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/recordings/")
		var wantsDetails bool
		id, wantsStatus := strings.CutSuffix(path, "/status")
		if !wantsStatus {
			id, wantsDetails = strings.CutSuffix(path, "/details")
		}
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}

		recording, ok := store.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}

		if wantsStatus {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(struct {
				ID     string                 `json:"id"`
				Status domain.RecordingStatus `json:"status"`
			}{
				ID:     recording.ID,
				Status: recording.Status,
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
			if transcript, ok := detailsStore.GetTranscript(id); ok {
				details.Transcript = toRecordingTranscriptResponse(transcript)
				details.Segments = toRecordingSegmentResponses(detailsStore.ListTranscriptSegments(id))
			}
			if summary, ok := detailsStore.GetSummary(id); ok {
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
