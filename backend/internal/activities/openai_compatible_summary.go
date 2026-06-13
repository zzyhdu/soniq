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

func (p OpenAICompatibleSummaryProvider) GenerateMindMap(ctx context.Context, request MindMapRequest) (MindMapResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if baseURL == "" {
		return MindMapResult{}, errors.New("mind map base url is required")
	}
	apiKey := strings.TrimSpace(p.APIKey)
	if apiKey == "" {
		return MindMapResult{}, errors.New("mind map api key is required")
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		return MindMapResult{}, errors.New("mind map model is required")
	}
	body := openAICompatibleSummaryRequest{
		Model:       model,
		Temperature: 0.1,
		Messages: []openAICompatibleSummaryMessage{
			{Role: "system", Content: mindMapSystemPrompt()},
			{Role: "user", Content: mindMapUserPrompt(request)},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return MindMapResult{}, fmt.Errorf("marshal mind map request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return MindMapResult{}, fmt.Errorf("build mind map request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return MindMapResult{}, fmt.Errorf("call mind map provider: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return MindMapResult{}, fmt.Errorf("read mind map response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return MindMapResult{}, fmt.Errorf("mind map provider returned HTTP %d: %s", resp.StatusCode, redactSecret(string(raw), apiKey))
	}
	var parsed openAICompatibleSummaryResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return MindMapResult{}, fmt.Errorf("parse mind map response: %w", err)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return MindMapResult{}, errors.New("mind map provider returned no content")
	}
	title, root, err := parseMindMapJSON(parsed.Choices[0].Message.Content)
	if err != nil {
		return MindMapResult{}, err
	}
	if title == "" {
		title = defaultMindMapTitle(request)
	}
	if strings.TrimSpace(root.Label) == "" {
		root.Label = title
	}
	return MindMapResult{
		Provider:        "openai_compatible_llm",
		Model:           model,
		Title:           title,
		Root:            root,
		ContentMarkdown: mindMapMarkdown(root),
		RawResultJSON:   append([]byte(nil), raw...),
		GeneratedAt:     time.Now().UTC(),
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

func mindMapSystemPrompt() string {
	return "你是 Soniq 的思维导图助手。请把转写和摘要整理成层次清晰的思维导图 JSON，不要编造未出现的信息。只输出 JSON，不要输出 Markdown 或解释。"
}

func mindMapUserPrompt(request MindMapRequest) string {
	language := strings.TrimSpace(request.Language)
	if language == "" {
		language = "auto"
	}
	return fmt.Sprintf(`标题：%s
工作流类型：%s
语言：%s

请输出如下 JSON：
{
  "title": "思维导图标题",
  "root": {
    "label": "根节点",
    "children": [
      {"label": "一级主题", "children": [{"label": "二级要点"}]}
    ]
  }
}

约束：
- root.children 控制在 3 到 7 个一级主题。
- 每个节点 label 简洁，适合作为思维导图节点。
- 不要包含没有依据的信息。

摘要：
%s

转写文本：
%s`, defaultMindMapTitle(request), request.WorkflowType, language, strings.TrimSpace(request.SummaryMarkdown), strings.TrimSpace(request.TranscriptText))
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

func defaultMindMapTitle(request MindMapRequest) string {
	title := strings.TrimSpace(request.Title)
	if title != "" {
		return title
	}
	if strings.TrimSpace(request.RecordingID) != "" {
		return request.RecordingID
	}
	return "Untitled recording"
}

func parseMindMapJSON(content string) (string, MindMapNode, error) {
	var parsed struct {
		Title string      `json:"title"`
		Root  MindMapNode `json:"root"`
	}
	payload := []byte(stripJSONFence(content))
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", MindMapNode{}, fmt.Errorf("parse mind map json: %w", err)
	}
	if strings.TrimSpace(parsed.Root.Label) == "" && len(parsed.Root.Children) == 0 {
		return "", MindMapNode{}, errors.New("mind map root is required")
	}
	return strings.TrimSpace(parsed.Title), trimMindMapNode(parsed.Root), nil
}

func stripJSONFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```JSON")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}

func trimMindMapNode(node MindMapNode) MindMapNode {
	node.Label = strings.TrimSpace(node.Label)
	children := make([]MindMapNode, 0, len(node.Children))
	for _, child := range node.Children {
		trimmed := trimMindMapNode(child)
		if trimmed.Label == "" && len(trimmed.Children) == 0 {
			continue
		}
		children = append(children, trimmed)
	}
	node.Children = children
	return node
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
