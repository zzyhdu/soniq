package activities

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDashScopeASRProviderSubmitsPollsDownloadsAndMapsSegments(t *testing.T) {
	audioPath := writeDashScopeASRTestAudio(t, []byte("normalized mp3 bytes"))
	var capturedSubmit struct {
		Path          string
		Method        string
		Authorization string
		AsyncHeader   string
		ContentType   string
		Body          map[string]any
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/services/audio/asr/transcription":
			capturedSubmit.Path = r.URL.Path
			capturedSubmit.Method = r.Method
			capturedSubmit.Authorization = r.Header.Get("Authorization")
			capturedSubmit.AsyncHeader = r.Header.Get("X-DashScope-Async")
			capturedSubmit.ContentType = r.Header.Get("Content-Type")
			if err := json.NewDecoder(r.Body).Decode(&capturedSubmit.Body); err != nil {
				t.Fatalf("decode submit body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"output":{"task_id":"task_123"}}`))
		case "/api/v1/tasks/task_123":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"output":{"task_status":"SUCCEEDED","results":[{"subtask_status":"SUCCEEDED","transcription_url":"` + serverResultURL(t, r, "/result.json") + `"}]}}`))
		case "/result.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"file_url":"data:audio/mpeg;base64,redacted",
				"transcripts":[{"channel_id":0,"text":"你好，Soniq。收到。","sentences":[
					{"sentence_id":1,"begin_time":100,"end_time":1200,"speaker_id":0,"text":"你好，Soniq。"},
					{"sentence_id":2,"begin_time":1300,"end_time":1800,"speaker_id":1,"text":"收到。"}
				]}]
			}`))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := DashScopeASRProvider{
		BaseURL:        server.URL + "/api/v1",
		APIKey:         "dashscope-test-key",
		Model:          "paraformer-v2",
		LanguageHints:  []string{"zh", "en"},
		Diarization:    true,
		MaxBase64Bytes: 1024,
		HTTPClient:     server.Client(),
		PollInterval:   time.Millisecond,
		Now:            func() time.Time { return time.Date(2026, 6, 7, 2, 3, 4, 0, time.UTC) },
	}

	result, err := provider.Transcribe(context.Background(), TranscriptionRequest{RecordingID: "rec_dashscope", AudioPath: audioPath, Language: "zh"})
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}

	if capturedSubmit.Path != "/api/v1/services/audio/asr/transcription" || capturedSubmit.Method != http.MethodPost {
		t.Fatalf("submit request = %s %s, want POST transcription endpoint", capturedSubmit.Method, capturedSubmit.Path)
	}
	if capturedSubmit.Authorization != "Bearer dashscope-test-key" {
		t.Fatalf("Authorization = %q, want Bearer key", capturedSubmit.Authorization)
	}
	if capturedSubmit.AsyncHeader != "enable" {
		t.Fatalf("X-DashScope-Async = %q, want enable", capturedSubmit.AsyncHeader)
	}
	if !strings.HasPrefix(capturedSubmit.ContentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", capturedSubmit.ContentType)
	}
	if capturedSubmit.Body["model"] != "paraformer-v2" {
		t.Fatalf("model = %v, want paraformer-v2", capturedSubmit.Body["model"])
	}
	input := capturedSubmit.Body["input"].(map[string]any)
	fileURLs := input["file_urls"].([]any)
	dataURL := fileURLs[0].(string)
	if !strings.HasPrefix(dataURL, "data:audio/mpeg;base64,") {
		t.Fatalf("file_urls[0] = %.32q, want audio/mpeg data URL", dataURL)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, "data:audio/mpeg;base64,"))
	if err != nil || string(decoded) != "normalized mp3 bytes" {
		t.Fatalf("decoded audio = %q err=%v, want original bytes", string(decoded), err)
	}
	parameters := capturedSubmit.Body["parameters"].(map[string]any)
	if parameters["diarization_enabled"] != true {
		t.Fatalf("diarization_enabled = %v, want true", parameters["diarization_enabled"])
	}
	if parameters["timestamp_alignment_enabled"] != true {
		t.Fatalf("timestamp_alignment_enabled = %v, want true for paraformer", parameters["timestamp_alignment_enabled"])
	}

	if result.Provider != "dashscope_asr" || result.Model != "paraformer-v2" || result.Language != "zh" || result.Text != "你好，Soniq。收到。" {
		t.Fatalf("result = %+v, want mapped provider/model/language/text", result)
	}
	if !result.TranscribedAt.Equal(time.Date(2026, 6, 7, 2, 3, 4, 0, time.UTC)) {
		t.Fatalf("TranscribedAt = %v, want injected time", result.TranscribedAt)
	}
	if len(result.RawResultJSON) == 0 || !json.Valid(result.RawResultJSON) {
		t.Fatalf("RawResultJSON = %q, want valid raw result", string(result.RawResultJSON))
	}
	if len(result.Segments) != 2 {
		t.Fatalf("segments = %+v, want 2", result.Segments)
	}
	if result.Segments[0].StartMS != 100 || result.Segments[0].EndMS != 1200 || result.Segments[0].SpeakerLabel != "0" || result.Segments[0].Text != "你好，Soniq。" {
		t.Fatalf("first segment = %+v, want mapped sentence", result.Segments[0])
	}
}

func TestDashScopeASRProviderRejectsOversizedPayloadBeforeRequest(t *testing.T) {
	audioPath := writeDashScopeASRTestAudio(t, []byte("1234567890"))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()

	provider := DashScopeASRProvider{BaseURL: server.URL, APIKey: "test-key", Model: "paraformer-v2", MaxBase64Bytes: 4, HTTPClient: server.Client()}
	_, err := provider.Transcribe(context.Background(), TranscriptionRequest{RecordingID: "rec", AudioPath: audioPath})
	if err == nil || !strings.Contains(err.Error(), "base64") {
		t.Fatalf("error = %v, want base64 size error", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
}

func TestDashScopeASRProviderReturnsUsefulErrorsWithoutLeakingKey(t *testing.T) {
	audioPath := writeDashScopeASRTestAudio(t, []byte("wav"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "dashscope upstream exploded", http.StatusBadGateway)
	}))
	defer server.Close()

	provider := DashScopeASRProvider{BaseURL: server.URL, APIKey: "super-secret-dashscope-key", Model: "paraformer-v2", MaxBase64Bytes: 1024, HTTPClient: server.Client()}
	_, err := provider.Transcribe(context.Background(), TranscriptionRequest{RecordingID: "rec", AudioPath: audioPath})
	if err == nil {
		t.Fatal("Transcribe returned nil error, want upstream error")
	}
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "dashscope upstream exploded") {
		t.Fatalf("error = %v, want status/body context", err)
	}
	if strings.Contains(err.Error(), "super-secret-dashscope-key") {
		t.Fatalf("error leaked API key: %v", err)
	}
}

func TestDashScopeASRProviderRejectsInvalidTaskResponses(t *testing.T) {
	audioPath := writeDashScopeASRTestAudio(t, []byte("wav"))
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "missing task id", body: "{\"output\":{}}", want: "task_id"},
		{name: "failed task", body: "{\"output\":{\"task_id\":\"task_1\"}}", want: "FAILED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/services/audio/asr/transcription" {
					_, _ = w.Write([]byte(tt.body))
					return
				}
				_, _ = w.Write([]byte(`{"output":{"task_status":"FAILED","message":"provider failed","results":[{"subtask_status":"FAILED","message":"bad audio"}]}}`))
			}))
			defer server.Close()
			provider := DashScopeASRProvider{BaseURL: server.URL + "/api/v1", APIKey: "test-key", Model: "paraformer-v2", MaxBase64Bytes: 1024, HTTPClient: server.Client(), PollInterval: time.Millisecond}
			_, err := provider.Transcribe(context.Background(), TranscriptionRequest{RecordingID: "rec", AudioPath: audioPath})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func writeDashScopeASRTestAudio(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "normalized.mp3")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test audio: %v", err)
	}
	return path
}

func serverResultURL(t *testing.T, r *http.Request, path string) string {
	t.Helper()
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + path
}
