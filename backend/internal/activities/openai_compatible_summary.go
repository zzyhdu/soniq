package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatibleSummaryProvider summarizes transcripts through an OpenAI-compatible chat completions API.
type OpenAICompatibleSummaryProvider struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

type openAICompatibleSummaryRequest struct {
	Model       string                           `json:"model"`
	Messages    []openAICompatibleSummaryMessage `json:"messages"`
	Temperature float64                          `json:"temperature,omitempty"`
}

type openAICompatibleSummaryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAICompatibleSummaryResponse struct {
	ID      string                          `json:"id"`
	Model   string                          `json:"model"`
	Choices []openAICompatibleSummaryChoice `json:"choices"`
	Usage   map[string]any                  `json:"usage,omitempty"`
}

type openAICompatibleSummaryChoice struct {
	Message openAICompatibleSummaryMessage `json:"message"`
}

func (p OpenAICompatibleSummaryProvider) Summarize(ctx context.Context, request SummaryRequest) (SummaryResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if baseURL == "" {
		return SummaryResult{}, errors.New("summary base url is required")
	}
	apiKey := strings.TrimSpace(p.APIKey)
	if apiKey == "" {
		return SummaryResult{}, errors.New("summary api key is required")
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		return SummaryResult{}, errors.New("summary model is required")
	}
	body := openAICompatibleSummaryRequest{
		Model:       model,
		Temperature: 0.2,
		Messages: []openAICompatibleSummaryMessage{
			{Role: "system", Content: summarySystemPrompt()},
			{Role: "user", Content: summaryUserPrompt(request)},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return SummaryResult{}, fmt.Errorf("marshal summary request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return SummaryResult{}, fmt.Errorf("build summary request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return SummaryResult{}, fmt.Errorf("call summary provider: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return SummaryResult{}, fmt.Errorf("read summary response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SummaryResult{}, fmt.Errorf("summary provider returned HTTP %d: %s", resp.StatusCode, redactSecret(string(raw), apiKey))
	}
	var parsed openAICompatibleSummaryResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return SummaryResult{}, fmt.Errorf("parse summary response: %w", err)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return SummaryResult{}, errors.New("summary provider returned no content")
	}
	markdown := strings.TrimSpace(parsed.Choices[0].Message.Content)
	return SummaryResult{
		Provider:        "openai_compatible_llm",
		Model:           model,
		Title:           defaultSummaryTitle(request),
		Overview:        extractSummaryOverview(markdown),
		ContentMarkdown: markdown,
		RawResultJSON:   append([]byte(nil), raw...),
		SummarizedAt:    time.Now().UTC(),
	}, nil
}

func summarySystemPrompt() string {
	return "你是 Soniq 的会议纪要助手。请根据转写文本生成准确、简洁、可执行的中文 Markdown 摘要，不要编造未出现的信息。"
}

func summaryUserPrompt(request SummaryRequest) string {
	language := strings.TrimSpace(request.Language)
	if language == "" {
		language = "auto"
	}
	return fmt.Sprintf("会议标题：%s\n工作流类型：%s\n语言：%s\n\n请输出 Markdown，包含：## 概览、## 关键讨论、## 行动项。\n\n转写文本：\n%s", defaultSummaryTitle(request), request.WorkflowType, language, strings.TrimSpace(request.TranscriptText))
}

func defaultSummaryTitle(request SummaryRequest) string {
	title := strings.TrimSpace(request.Title)
	if title != "" {
		return title
	}
	if strings.TrimSpace(request.RecordingID) != "" {
		return request.RecordingID
	}
	return "Untitled recording"
}

func extractSummaryOverview(markdown string) string {
	text := strings.TrimSpace(markdown)
	if text == "" {
		return "No summary content available."
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "## 概览" {
			return firstNonHeadingParagraph(lines[i+1:])
		}
	}
	return firstNonHeadingParagraph(lines)
}

func firstNonHeadingParagraph(lines []string) string {
	var parts []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") && len(parts) == 0 {
			continue
		}
		if strings.HasPrefix(trimmed, "##") && len(parts) > 0 {
			break
		}
		parts = append(parts, strings.TrimPrefix(trimmed, "- "))
	}
	overview := strings.TrimSpace(strings.Join(parts, " "))
	if overview == "" {
		return "No summary content available."
	}
	return truncateRunes(overview, 240)
}

func redactSecret(value, secret string) string {
	if strings.TrimSpace(secret) == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[redacted]")
}
