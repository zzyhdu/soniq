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

// RecordingProcessor is the enqueue seam invoked after a recording is created.
type RecordingProcessor interface {
	Enqueue(recording domain.Recording) error
}

type noopRecordingProcessor struct{}

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
	mux.HandleFunc("/recordings", createRecordingHandler(store, processor))
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
	mux.HandleFunc("/recordings", createRecordingHandler(store, processor))
	mux.HandleFunc("/recordings/upload", uploadRecordingHandler(store, processor, objectStore))
	mux.HandleFunc("/recordings/", recordingByIDHandler(store))
	return mux
}

func createRecordingHandler(store RecordingStore, processor RecordingProcessor) http.HandlerFunc {
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
		if err := processor.Enqueue(recording); err != nil {
			http.Error(w, "enqueue recording processor", http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := processor.Enqueue(recording); err != nil {
			http.Error(w, "enqueue recording processor", http.StatusInternalServerError)
			return
		}

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
		id, wantsStatus := strings.CutSuffix(path, "/status")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}

		recording, ok := store.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if wantsStatus {
			_ = json.NewEncoder(w).Encode(struct {
				ID     string                 `json:"id"`
				Status domain.RecordingStatus `json:"status"`
			}{
				ID:     recording.ID,
				Status: recording.Status,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(recording)
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
