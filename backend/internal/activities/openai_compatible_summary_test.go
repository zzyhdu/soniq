package activities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zzyhdu/soniq/backend/internal/domain"
)

func TestOpenAICompatibleSummaryProviderPostsChatCompletionAndParsesMarkdown(t *testing.T) {
	var capturedPath string
	var capturedAuth string
	var capturedBody openAICompatibleSummaryRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"model":"qwen-plus",
			"choices":[{"message":{"role":"assistant","content":"# 例会摘要\n\n## 概览\n讨论了语音转录测试计划。\n\n## 行动项\n- 检查转写质量"}}],
			"usage":{"prompt_tokens":10,"completion_tokens":20}
		}`))
	}))
	defer server.Close()

	provider := OpenAICompatibleSummaryProvider{
		BaseURL: server.URL + "/compatible-mode/v1",
		APIKey:  "summary-secret",
		Model:   "qwen-plus",
	}
	result, err := provider.Summarize(context.Background(), SummaryRequest{
		RecordingID:    "rec_123",
		Title:          "四人例会",
		WorkflowType:   domain.WorkflowTypeMeeting,
		Language:       "zh",
		TranscriptText: "大家早上好，我们先同步语音转录测试计划。",
	})
	if err != nil {
		t.Fatalf("Summarize returned error: %v", err)
	}
	if capturedPath != "/compatible-mode/v1/chat/completions" {
		t.Fatalf("path = %q, want chat completions path", capturedPath)
	}
	if capturedAuth != "Bearer summary-secret" {
		t.Fatalf("Authorization header = %q, want bearer key", capturedAuth)
	}
	if capturedBody.Model != "qwen-plus" {
		t.Fatalf("model = %q, want qwen-plus", capturedBody.Model)
	}
	if !strings.Contains(capturedBody.Messages[len(capturedBody.Messages)-1].Content, "四人例会") || !strings.Contains(capturedBody.Messages[len(capturedBody.Messages)-1].Content, "大家早上好") {
		t.Fatalf("user prompt did not include title and transcript: %#v", capturedBody.Messages)
	}
	if result.Provider != "openai_compatible_llm" || result.Model != "qwen-plus" {
		t.Fatalf("provider/model = %s/%s, want openai_compatible_llm/qwen-plus", result.Provider, result.Model)
	}
	if result.Title != "四人例会" {
		t.Fatalf("title = %q, want request title", result.Title)
	}
	if !strings.Contains(result.Overview, "讨论了语音转录测试计划") {
		t.Fatalf("overview = %q, want parsed overview", result.Overview)
	}
	if !strings.Contains(result.ContentMarkdown, "## 行动项") {
		t.Fatalf("markdown = %q, want full assistant markdown", result.ContentMarkdown)
	}
	if len(result.RawResultJSON) == 0 {
		t.Fatal("RawResultJSON is empty")
	}
}

func TestOpenAICompatibleSummaryProviderTruncatesChineseOverviewWithoutBreakingUTF8(t *testing.T) {
	longOverview := strings.Repeat("界", 241)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := openAICompatibleSummaryResponse{
			ID:    "chatcmpl-test",
			Model: "qwen-plus",
			Choices: []openAICompatibleSummaryChoice{{
				Message: openAICompatibleSummaryMessage{Role: "assistant", Content: "# 摘要\n\n" + longOverview},
			}},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := OpenAICompatibleSummaryProvider{
		BaseURL: server.URL + "/v1",
		APIKey:  "summary-secret",
		Model:   "qwen-plus",
	}
	result, err := provider.Summarize(context.Background(), SummaryRequest{RecordingID: "rec_zh", TranscriptText: "中文转写"})
	if err != nil {
		t.Fatalf("Summarize returned error: %v", err)
	}
	if !utf8.ValidString(result.Overview) {
		t.Fatalf("overview contains invalid UTF-8: %q", result.Overview)
	}
	if got, want := len([]rune(result.Overview)), 240; got != want {
		t.Fatalf("overview runes = %d, want %d", got, want)
	}
}

func TestOpenAICompatibleSummaryProviderDoesNotLeakAPIKeyOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad key summary-secret"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := OpenAICompatibleSummaryProvider{
		BaseURL: server.URL + "/v1",
		APIKey:  "summary-secret",
		Model:   "qwen-plus",
	}
	_, err := provider.Summarize(context.Background(), SummaryRequest{RecordingID: "rec_123", TranscriptText: "hello"})
	if err == nil {
		t.Fatal("Summarize error = nil, want HTTP error")
	}
	if strings.Contains(err.Error(), "summary-secret") {
		t.Fatalf("error leaked API key: %v", err)
	}
}
