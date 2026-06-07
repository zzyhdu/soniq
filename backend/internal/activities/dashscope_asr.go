package activities

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	dashScopeASRProviderName = "dashscope_asr"
	dashScopeASRDefaultURL   = "https://dashscope.aliyuncs.com/api/v1"
	dashScopeASRDefaultModel = "paraformer-v2"
	dashScopeASRDefaultLimit = int64(10 * 1024 * 1024)
)

// DashScopeASRProvider calls Aliyun DashScope native non-realtime ASR.
type DashScopeASRProvider struct {
	BaseURL        string
	APIKey         string
	Model          string
	Language       string
	LanguageHints  []string
	Diarization    bool
	MaxBase64Bytes int64
	HTTPClient     *http.Client
	PollInterval   time.Duration
	PollTimeout    time.Duration
	Now            func() time.Time
}

type dashScopeASRSubmitRequest struct {
	Model      string                    `json:"model"`
	Input      dashScopeASRInput         `json:"input"`
	Parameters dashScopeASRRequestParams `json:"parameters"`
}

type dashScopeASRInput struct {
	FileURLs []string `json:"file_urls"`
}

type dashScopeASRRequestParams struct {
	ChannelID                 []int    `json:"channel_id"`
	LanguageHints             []string `json:"language_hints,omitempty"`
	DiarizationEnabled        bool     `json:"diarization_enabled,omitempty"`
	TimestampAlignmentEnabled bool     `json:"timestamp_alignment_enabled,omitempty"`
}

type dashScopeASRSubmitResponse struct {
	Output struct {
		TaskID string `json:"task_id"`
	} `json:"output"`
}

type dashScopeASRTaskResponse struct {
	Output struct {
		TaskStatus string                   `json:"task_status"`
		Message    string                   `json:"message"`
		Results    []dashScopeASRTaskResult `json:"results"`
	} `json:"output"`
}

type dashScopeASRTaskResult struct {
	SubtaskStatus    string `json:"subtask_status"`
	Message          string `json:"message"`
	TranscriptionURL string `json:"transcription_url"`
}

type dashScopeASRResultJSON struct {
	Transcripts []dashScopeASRTranscript `json:"transcripts"`
}

type dashScopeASRTranscript struct {
	Text      string                     `json:"text"`
	Sentences []dashScopeASRSentenceJSON `json:"sentences"`
}

type dashScopeASRSentenceJSON struct {
	BeginTime int             `json:"begin_time"`
	EndTime   int             `json:"end_time"`
	Text      string          `json:"text"`
	SpeakerID json.RawMessage `json:"speaker_id"`
	Speaker   json.RawMessage `json:"speaker"`
	SPK       json.RawMessage `json:"spk"`
}

func (p DashScopeASRProvider) Transcribe(ctx context.Context, request TranscriptionRequest) (TranscriptionResult, error) {
	if strings.TrimSpace(request.AudioPath) == "" {
		return TranscriptionResult{}, errors.New("audio path is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if baseURL == "" {
		baseURL = dashScopeASRDefaultURL
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = dashScopeASRDefaultModel
	}
	language := strings.TrimSpace(p.Language)
	if language == "" {
		language = strings.TrimSpace(request.Language)
	}
	maxBase64Bytes := p.MaxBase64Bytes
	if maxBase64Bytes <= 0 {
		maxBase64Bytes = dashScopeASRDefaultLimit
	}

	dataURL, err := audioFileDataURL(request.AudioPath)
	if err != nil {
		return TranscriptionResult{}, err
	}
	encodedLen := len(dataURL[strings.Index(dataURL, ",")+1:])
	if int64(encodedLen) > maxBase64Bytes {
		return TranscriptionResult{}, fmt.Errorf("base64 audio payload size %d exceeds limit %d", encodedLen, maxBase64Bytes)
	}

	languageHints := p.LanguageHints
	if len(languageHints) == 0 && language != "" && language != "auto" {
		languageHints = []string{language}
	}
	if len(languageHints) == 0 {
		languageHints = []string{"zh", "en"}
	}
	body := dashScopeASRSubmitRequest{
		Model: model,
		Input: dashScopeASRInput{FileURLs: []string{dataURL}},
		Parameters: dashScopeASRRequestParams{
			ChannelID:                 []int{0},
			LanguageHints:             languageHints,
			DiarizationEnabled:        p.Diarization,
			TimestampAlignmentEnabled: strings.HasPrefix(model, "paraformer"),
		},
	}
	submitResponse, err := p.requestJSON(ctx, http.MethodPost, baseURL+"/services/audio/asr/transcription", body)
	if err != nil {
		return TranscriptionResult{}, err
	}
	var submitted dashScopeASRSubmitResponse
	if err := json.Unmarshal(submitResponse, &submitted); err != nil {
		return TranscriptionResult{}, fmt.Errorf("decode DashScope submit response: %w", err)
	}
	taskID := strings.TrimSpace(submitted.Output.TaskID)
	if taskID == "" {
		return TranscriptionResult{}, errors.New("DashScope submit response missing output.task_id")
	}

	task, err := p.waitForTask(ctx, baseURL, taskID)
	if err != nil {
		return TranscriptionResult{}, err
	}
	transcriptionURL, err := dashScopeTranscriptionURL(task)
	if err != nil {
		return TranscriptionResult{}, err
	}
	rawResult, err := p.downloadResult(ctx, transcriptionURL)
	if err != nil {
		return TranscriptionResult{}, err
	}
	var parsed dashScopeASRResultJSON
	if err := json.Unmarshal(rawResult, &parsed); err != nil {
		return TranscriptionResult{}, fmt.Errorf("decode DashScope transcription result: %w", err)
	}
	text, segments := mapDashScopeASRResult(parsed)
	if strings.TrimSpace(text) == "" {
		return TranscriptionResult{}, errors.New("DashScope transcription text is empty")
	}
	transcribedAt := time.Now().UTC()
	if p.Now != nil {
		transcribedAt = p.Now().UTC()
	}
	return TranscriptionResult{
		Provider:      dashScopeASRProviderName,
		Model:         model,
		Language:      language,
		Text:          text,
		RawResultJSON: append([]byte(nil), rawResult...),
		TranscribedAt: transcribedAt,
		Segments:      segments,
	}, nil
}

func (p DashScopeASRProvider) waitForTask(ctx context.Context, baseURL, taskID string) (dashScopeASRTaskResponse, error) {
	interval := p.PollInterval
	if interval <= 0 {
		interval = 3 * time.Second
	}
	timeout := p.PollTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	for {
		rawTask, err := p.requestJSON(ctx, http.MethodGet, baseURL+"/tasks/"+taskID, nil)
		if err != nil {
			return dashScopeASRTaskResponse{}, err
		}
		var task dashScopeASRTaskResponse
		if err := json.Unmarshal(rawTask, &task); err != nil {
			return dashScopeASRTaskResponse{}, fmt.Errorf("decode DashScope task response: %w", err)
		}
		switch strings.ToUpper(strings.TrimSpace(task.Output.TaskStatus)) {
		case "SUCCEEDED":
			return task, nil
		case "FAILED", "CANCELED":
			return dashScopeASRTaskResponse{}, fmt.Errorf("DashScope task %s: %s", task.Output.TaskStatus, task.Output.Message)
		}
		if time.Now().After(deadline) {
			return dashScopeASRTaskResponse{}, fmt.Errorf("timed out waiting for DashScope task %s", taskID)
		}
		select {
		case <-ctx.Done():
			return dashScopeASRTaskResponse{}, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (p DashScopeASRProvider) requestJSON(ctx context.Context, method, url string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		rawBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode DashScope request: %w", err)
		}
		reader = bytes.NewReader(rawBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("build DashScope request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable")
	if apiKey := strings.TrimSpace(p.APIKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call DashScope ASR provider: %w", err)
	}
	defer resp.Body.Close()
	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read DashScope response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("DashScope ASR provider returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawResponse)))
	}
	return rawResponse, nil
}

func (p DashScopeASRProvider) downloadResult(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build DashScope result request: %w", err)
	}
	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download DashScope transcription result: %w", err)
	}
	defer resp.Body.Close()
	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read DashScope transcription result: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("DashScope transcription result returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawResponse)))
	}
	return rawResponse, nil
}

func dashScopeTranscriptionURL(task dashScopeASRTaskResponse) (string, error) {
	if len(task.Output.Results) == 0 {
		return "", errors.New("DashScope task response missing output.results")
	}
	first := task.Output.Results[0]
	if strings.ToUpper(strings.TrimSpace(first.SubtaskStatus)) != "SUCCEEDED" {
		return "", fmt.Errorf("DashScope subtask %s: %s", first.SubtaskStatus, first.Message)
	}
	if strings.TrimSpace(first.TranscriptionURL) == "" {
		return "", errors.New("DashScope task response missing transcription_url")
	}
	return first.TranscriptionURL, nil
}

func mapDashScopeASRResult(result dashScopeASRResultJSON) (string, []TranscriptionSegmentResult) {
	var textParts []string
	var segments []TranscriptionSegmentResult
	for _, transcript := range result.Transcripts {
		if strings.TrimSpace(transcript.Text) != "" {
			textParts = append(textParts, strings.TrimSpace(transcript.Text))
		}
		for _, sentence := range transcript.Sentences {
			segments = append(segments, TranscriptionSegmentResult{
				SegmentIndex: len(segments),
				StartMS:      sentence.BeginTime,
				EndMS:        sentence.EndTime,
				SpeakerLabel: dashScopeSpeakerLabel(sentence),
				Text:         sentence.Text,
				Confidence:   0,
			})
		}
	}
	if len(textParts) == 0 && len(segments) > 0 {
		for _, segment := range segments {
			textParts = append(textParts, segment.Text)
		}
	}
	return strings.Join(textParts, ""), segments
}

func dashScopeSpeakerLabel(sentence dashScopeASRSentenceJSON) string {
	for _, raw := range []json.RawMessage{sentence.SpeakerID, sentence.Speaker, sentence.SPK} {
		if label := rawJSONLabel(raw); label != "" {
			return label
		}
	}
	return ""
}

func rawJSONLabel(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		if number == float64(int64(number)) {
			return strconv.FormatInt(int64(number), 10)
		}
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	return ""
}

func audioFileDataURL(path string) (string, error) {
	audio, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read audio file: %w", err)
	}
	mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(audio), nil
}
