package activities

import (
	"context"
	"testing"

	"github.com/zzyhdu/soniq/backend/internal/domain"
)

func TestValidateRecordingActivityAcceptsValidInput(t *testing.T) {
	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	}

	if err := ValidateRecordingActivity(context.Background(), input); err != nil {
		t.Fatalf("ValidateRecordingActivity() error = %v, want nil", err)
	}
}

func TestValidateRecordingActivityRejectsMissingRecordingID(t *testing.T) {
	input := RecordingProcessingInput{
		WorkflowType: domain.WorkflowTypeMeeting,
		Language:     "en",
	}

	if err := ValidateRecordingActivity(context.Background(), input); err == nil {
		t.Fatalf("ValidateRecordingActivity() error = nil, want error")
	}
}

func TestValidateRecordingActivityRejectsInvalidWorkflowType(t *testing.T) {
	input := RecordingProcessingInput{
		RecordingID:  "rec_test",
		WorkflowType: "podcast",
		Language:     "en",
	}

	if err := ValidateRecordingActivity(context.Background(), input); err == nil {
		t.Fatalf("ValidateRecordingActivity() error = nil, want error")
	}
}

func TestMarkRecordingProcessingActivityAcceptsRecordingID(t *testing.T) {
	if err := MarkRecordingProcessingActivity(context.Background(), "rec_test"); err != nil {
		t.Fatalf("MarkRecordingProcessingActivity() error = %v, want nil", err)
	}
}

func TestMarkRecordingProcessingActivityRejectsMissingRecordingID(t *testing.T) {
	if err := MarkRecordingProcessingActivity(context.Background(), ""); err == nil {
		t.Fatalf("MarkRecordingProcessingActivity() error = nil, want error")
	}
}

func TestCompleteRecordingProcessingActivityReturnsCompletedResult(t *testing.T) {
	result, err := CompleteRecordingProcessingActivity(context.Background(), "rec_test")
	if err != nil {
		t.Fatalf("CompleteRecordingProcessingActivity() error = %v, want nil", err)
	}

	want := RecordingProcessingResult{
		RecordingID: "rec_test",
		Status:      domain.RecordingStatusCompleted,
	}
	if result != want {
		t.Fatalf("CompleteRecordingProcessingActivity() = %#v, want %#v", result, want)
	}
}

func TestCompleteRecordingProcessingActivityRejectsMissingRecordingID(t *testing.T) {
	if _, err := CompleteRecordingProcessingActivity(context.Background(), ""); err == nil {
		t.Fatalf("CompleteRecordingProcessingActivity() error = nil, want error")
	}
}
