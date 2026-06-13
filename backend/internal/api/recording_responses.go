package api

import (
	"encoding/json"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
)

type recordingDetailsResponse struct {
	Recording  recordingResponse            `json:"recording"`
	Transcript *recordingTranscriptResponse `json:"transcript"`
	Segments   []recordingSegmentResponse   `json:"segments"`
	Summary    *recordingSummaryResponse    `json:"summary"`
	MindMap    *recordingMindMapResponse    `json:"mind_map"`
}

type uploadRecordingResponse struct {
	Recording          domain.Recording `json:"recording"`
	ProcessingEnqueued bool             `json:"processing_enqueued"`
}

type retryRecordingResponse struct {
	Recording          recordingResponse `json:"recording"`
	ProcessingEnqueued bool              `json:"processing_enqueued"`
}

type listRecordingsResponse struct {
	Recordings []recordingResponse `json:"recordings"`
}

type recordingResponse struct {
	ID               string                 `json:"id"`
	WorkspaceID      string                 `json:"workspace_id"`
	Title            string                 `json:"title"`
	Status           domain.RecordingStatus `json:"status"`
	WorkflowType     domain.WorkflowType    `json:"workflow_type"`
	Language         string                 `json:"language"`
	AudioObjectKey   string                 `json:"audio_object_key,omitempty"`
	AudioContentType string                 `json:"audio_content_type,omitempty"`
	AudioSizeBytes   int64                  `json:"audio_size_bytes,omitempty"`
	FailureReason    string                 `json:"failure_reason,omitempty"`
	CompletedAt      *time.Time             `json:"completed_at,omitempty"`
	FailedAt         *time.Time             `json:"failed_at,omitempty"`
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

type recordingMindMapResponse struct {
	RecordingID     string          `json:"recording_id"`
	Provider        string          `json:"provider"`
	Model           string          `json:"model"`
	Title           string          `json:"title"`
	Root            json.RawMessage `json:"root"`
	ContentMarkdown string          `json:"content_markdown"`
	GeneratedAt     time.Time       `json:"generated_at"`
}

func toRecordingResponse(recording domain.Recording) recordingResponse {
	return recordingResponse{
		ID:               recording.ID,
		WorkspaceID:      recording.WorkspaceID,
		Title:            recording.Title,
		Status:           recording.Status,
		WorkflowType:     recording.WorkflowType,
		Language:         recording.Language,
		AudioObjectKey:   recording.AudioObjectKey,
		AudioContentType: recording.AudioContentType,
		AudioSizeBytes:   recording.AudioSizeBytes,
		FailureReason:    recording.FailureReason,
		CompletedAt:      recording.CompletedAt,
		FailedAt:         recording.FailedAt,
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

func toRecordingMindMapResponse(mindMap recordings.RecordingMindMap) *recordingMindMapResponse {
	return &recordingMindMapResponse{
		RecordingID:     mindMap.RecordingID,
		Provider:        mindMap.Provider,
		Model:           mindMap.Model,
		Title:           mindMap.Title,
		Root:            json.RawMessage(append([]byte(nil), mindMap.RootJSON...)),
		ContentMarkdown: mindMap.ContentMarkdown,
		GeneratedAt:     mindMap.GeneratedAt,
	}
}
