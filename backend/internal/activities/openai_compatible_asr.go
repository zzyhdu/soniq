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

const (
	openAICompatibleASRProviderName = "openai_compatible_asr"
	openAICompatibleASRDefaultModel = "mimo-v2.5-asr"
	openAICompatibleASRDefaultLang  = "auto"
)

// OpenAICompatibleASRProvider calls an OpenAI-compatible chat-completions ASR endpoint.
type OpenAICompatibleASRProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	AuthHeader string
	Language   string
	HTTPClient *http.Client
	Now        func() time.Time
}

type openAICompatibleASRRequest struct {
	Model      string                         `json:"model"`
	Messages   []openAICompatibleASRMessage   `json:"messages"`
	ASROptions openAICompatibleASRRequestOpts `json:"asr_options"`
}

type openAICompatibleASRMessage struct {
	Role    string                           `json:"role"`
	Content []openAICompatibleASRContentPart `json:"content"`
}

type openAICompatibleASRContentPart struct {
	Type       string                   `json:"type"`
	InputAudio openAICompatibleASRAudio `json:"input_audio"`
}

type openAICompatibleASRAudio struct {
	Data string `json:"data"`
}

type openAICompatibleASRRequestOpts struct {
	Language string `json:"language"`
}

type openAICompatibleASRResponse struct {
	Model   string                      `json:"model"`
	Choices []openAICompatibleASRChoice `json:"choices"`
}

type openAICompatibleASRChoice struct {
	Message openAICompatibleASRChoiceMessage `json:"message"`
}

type openAICompatibleASRChoiceMessage struct {
	Content string `json:"content"`
}

func (p OpenAICompatibleASRProvider) Transcribe(ctx context.Context, request TranscriptionRequest) (TranscriptionResult, error) {
	audioURL := strings.TrimSpace(request.AudioURL)
	if audioURL == "" {
		return TranscriptionResult{}, errors.New("audio URL is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if baseURL == "" {
		return TranscriptionResult{}, errors.New("transcription base URL is required")
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = openAICompatibleASRDefaultModel
	}
	language := strings.TrimSpace(p.Language)
	if language == "" {
		language = request.Language
	}
	if language == "" {
		language = openAICompatibleASRDefaultLang
	}
	body := openAICompatibleASRRequest{
		Model: model,
		Messages: []openAICompatibleASRMessage{{
			Role: "user",
			Content: []openAICompatibleASRContentPart{{
				Type:       "input_audio",
				InputAudio: openAICompatibleASRAudio{Data: audioURL},
			}},
		}},
		ASROptions: openAICompatibleASRRequestOpts{Language: language},
	}
	rawRequest, err := json.Marshal(body)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("encode transcription request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(rawRequest))
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("build transcription request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	p.applyAuthHeader(httpReq)

	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("call transcription provider: %w", err)
	}
	defer resp.Body.Close()
	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("read transcription response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TranscriptionResult{}, fmt.Errorf("transcription provider returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawResponse)))
	}
	var parsed openAICompatibleASRResponse
	if err := json.Unmarshal(rawResponse, &parsed); err != nil {
		return TranscriptionResult{}, fmt.Errorf("decode transcription response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return TranscriptionResult{}, errors.New("transcription response choices are empty")
	}
	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if text == "" {
		return TranscriptionResult{}, errors.New("transcription response content is empty")
	}
	modelOut := strings.TrimSpace(parsed.Model)
	if modelOut == "" {
		modelOut = model
	}
	transcribedAt := time.Now().UTC()
	if p.Now != nil {
		transcribedAt = p.Now().UTC()
	}
	return TranscriptionResult{
		Provider:      openAICompatibleASRProviderName,
		Model:         modelOut,
		Language:      language,
		Text:          text,
		RawResultJSON: append([]byte(nil), rawResponse...),
		TranscribedAt: transcribedAt,
		Segments: []TranscriptionSegmentResult{{
			SegmentIndex: 0,
			StartMS:      0,
			EndMS:        0,
			Text:         text,
			Confidence:   0,
		}},
	}, nil
}

func (p OpenAICompatibleASRProvider) applyAuthHeader(req *http.Request) {
	apiKey := strings.TrimSpace(p.APIKey)
	if apiKey == "" {
		return
	}
	switch strings.ToLower(strings.TrimSpace(p.AuthHeader)) {
	case "bearer", "authorization", "authorization-bearer":
		req.Header.Set("Authorization", "Bearer "+apiKey)
	default:
		req.Header.Set("api-key", apiKey)
	}
}
