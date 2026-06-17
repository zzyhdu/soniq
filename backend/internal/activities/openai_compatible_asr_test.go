package activities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompatibleASRProviderSendsChatCompletionsAudioURLRequest(t *testing.T) {
	audioURL := "https://objects.example.test/audio/normalized.wav?signature=redacted"
	var captured struct {
		Path        string
		Method      string
		APIKey      string
		ContentType string
		Body        map[string]any
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Path = r.URL.Path
		captured.Method = r.Method
		captured.APIKey = r.Header.Get("api-key")
		captured.ContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&captured.Body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","model":"mimo-v2.5-asr","choices":[{"message":{"role":"assistant","content":"你好，Soniq"}}]}`))
	}))
	defer server.Close()

	provider := OpenAICompatibleASRProvider{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-api-key",
		Model:      "mimo-v2.5-asr",
		AuthHeader: "api-key",
		Language:   "zh",
		HTTPClient: server.Client(),
		Now:        func() time.Time { return time.Date(2026, 6, 7, 1, 2, 3, 0, time.UTC) },
	}

	result, err := provider.Transcribe(context.Background(), TranscriptionRequest{RecordingID: "rec_asr", AudioURL: audioURL})
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}

	if captured.Path != "/v1/chat/completions" || captured.Method != http.MethodPost {
		t.Fatalf("request = %s %s, want POST /v1/chat/completions", captured.Method, captured.Path)
	}
	if captured.APIKey != "test-api-key" {
		t.Fatalf("api-key header = %q, want configured key", captured.APIKey)
	}
	if !strings.HasPrefix(captured.ContentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", captured.ContentType)
	}
	if captured.Body["model"] != "mimo-v2.5-asr" {
		t.Fatalf("model = %v, want mimo-v2.5-asr", captured.Body["model"])
	}
	assertASRLanguage(t, captured.Body, "zh")
	assertASRAudioURL(t, captured.Body, audioURL)

	if result.Provider != "openai_compatible_asr" || result.Model != "mimo-v2.5-asr" || result.Language != "zh" || result.Text != "你好，Soniq" {
		t.Fatalf("result = %+v, want mapped provider/model/language/text", result)
	}
	if result.TranscribedAt.IsZero() || !result.TranscribedAt.Equal(time.Date(2026, 6, 7, 1, 2, 3, 0, time.UTC)) {
		t.Fatalf("TranscribedAt = %v, want injected time", result.TranscribedAt)
	}
	if len(result.RawResultJSON) == 0 || !json.Valid(result.RawResultJSON) {
		t.Fatalf("RawResultJSON = %q, want valid raw response", string(result.RawResultJSON))
	}
	if len(result.Segments) != 1 || result.Segments[0].Text != "你好，Soniq" {
		t.Fatalf("segments = %+v, want one whole-text segment", result.Segments)
	}
}

func TestOpenAICompatibleASRProviderSupportsBearerAuth(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"bearer transcript"}}]}`))
	}))
	defer server.Close()

	provider := OpenAICompatibleASRProvider{BaseURL: server.URL, APIKey: "bearer-key", Model: "mimo-v2.5-asr", AuthHeader: "bearer", HTTPClient: server.Client()}
	if _, err := provider.Transcribe(context.Background(), TranscriptionRequest{RecordingID: "rec", AudioURL: "https://objects.example.test/audio.wav"}); err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}
	if authorization != "Bearer bearer-key" {
		t.Fatalf("Authorization = %q, want Bearer bearer-key", authorization)
	}
}

func TestOpenAICompatibleASRProviderReturnsUsefulErrorsWithoutLeakingKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream exploded", http.StatusBadGateway)
	}))
	defer server.Close()

	provider := OpenAICompatibleASRProvider{BaseURL: server.URL, APIKey: "super-secret-asr-key", Model: "mimo-v2.5-asr", HTTPClient: server.Client()}
	_, err := provider.Transcribe(context.Background(), TranscriptionRequest{RecordingID: "rec", AudioURL: "https://objects.example.test/audio.wav"})
	if err == nil {
		t.Fatal("Transcribe returned nil error, want upstream error")
	}
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "upstream exploded") {
		t.Fatalf("error = %v, want status/body context", err)
	}
	if strings.Contains(err.Error(), "super-secret-asr-key") {
		t.Fatalf("error leaked API key: %v", err)
	}
}

func TestOpenAICompatibleASRProviderRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "invalid json", body: `{`, want: "decode"},
		{name: "empty choices", body: `{"choices":[]}`, want: "choices"},
		{name: "empty content", body: `{"choices":[{"message":{"content":""}}]}`, want: "content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tt.body)) }))
			defer server.Close()
			provider := OpenAICompatibleASRProvider{BaseURL: server.URL, APIKey: "test-api-key", Model: "mimo-v2.5-asr", HTTPClient: server.Client()}
			_, err := provider.Transcribe(context.Background(), TranscriptionRequest{RecordingID: "rec", AudioURL: "https://objects.example.test/audio.wav"})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestOpenAICompatibleASRProviderRejectsMissingAudioURLBeforeRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()

	provider := OpenAICompatibleASRProvider{BaseURL: server.URL, APIKey: "test-api-key", Model: "mimo-v2.5-asr", HTTPClient: server.Client()}
	_, err := provider.Transcribe(context.Background(), TranscriptionRequest{RecordingID: "rec"})
	if err == nil || !strings.Contains(err.Error(), "audio URL") {
		t.Fatalf("error = %v, want missing audio URL error", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
}

func assertASRLanguage(t *testing.T, body map[string]any, want string) {
	t.Helper()
	options, ok := body["asr_options"].(map[string]any)
	if !ok {
		t.Fatalf("asr_options = %#v, want object", body["asr_options"])
	}
	if options["language"] != want {
		t.Fatalf("language = %v, want %s", options["language"], want)
	}
}

func assertASRAudioURL(t *testing.T, body map[string]any, want string) {
	t.Helper()
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v, want one message", body["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok || message["role"] != "user" {
		t.Fatalf("message = %#v, want user message", messages[0])
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content = %#v, want one content part", message["content"])
	}
	part, ok := content[0].(map[string]any)
	if !ok || part["type"] != "input_audio" {
		t.Fatalf("content part = %#v, want input_audio", content[0])
	}
	inputAudio, ok := part["input_audio"].(map[string]any)
	if !ok {
		t.Fatalf("input_audio = %#v, want object", part["input_audio"])
	}
	if inputAudio["data"] != want {
		t.Fatalf("input_audio.data = %#v, want audio URL", inputAudio["data"])
	}
}
