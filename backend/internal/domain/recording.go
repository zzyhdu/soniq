package domain

import "time"

// RecordingStatus describes the processing state of a recording.
type RecordingStatus string

const (
	RecordingStatusUploaded     RecordingStatus = "uploaded"
	RecordingStatusProcessing   RecordingStatus = "processing"
	RecordingStatusTranscribing RecordingStatus = "transcribing"
	RecordingStatusSummarizing  RecordingStatus = "summarizing"
	RecordingStatusCompleted    RecordingStatus = "completed"
	RecordingStatusFailed       RecordingStatus = "failed"
	RecordingStatusCancelled    RecordingStatus = "cancelled"
)

// WorkflowType describes the high-level recording processing template.
type WorkflowType string

const (
	WorkflowTypeMemo      WorkflowType = "memo"
	WorkflowTypeMeeting   WorkflowType = "meeting"
	WorkflowTypeLecture   WorkflowType = "lecture"
	WorkflowTypeInterview WorkflowType = "interview"
)

// Recording is the minimal domain representation used by the recording API skeleton.
type Recording struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Status       RecordingStatus `json:"status"`
	WorkflowType WorkflowType    `json:"workflow_type"`
	Language     string          `json:"language"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// IsValidWorkflowType reports whether value is one of the supported workflow types.
func IsValidWorkflowType(value string) bool {
	switch WorkflowType(value) {
	case WorkflowTypeMemo, WorkflowTypeMeeting, WorkflowTypeLecture, WorkflowTypeInterview:
		return true
	default:
		return false
	}
}
