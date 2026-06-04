package domain

import "testing"

func TestIsValidWorkflowTypeAcceptsSupportedValues(t *testing.T) {
	validTypes := []string{"memo", "meeting", "lecture", "interview"}

	for _, workflowType := range validTypes {
		t.Run(workflowType, func(t *testing.T) {
			if !IsValidWorkflowType(workflowType) {
				t.Fatalf("IsValidWorkflowType(%q) = false, want true", workflowType)
			}
		})
	}
}

func TestIsValidWorkflowTypeRejectsUnsupportedValues(t *testing.T) {
	invalidTypes := []string{"", "podcast", "MEETING", " meeting ", "call"}

	for _, workflowType := range invalidTypes {
		t.Run(workflowType, func(t *testing.T) {
			if IsValidWorkflowType(workflowType) {
				t.Fatalf("IsValidWorkflowType(%q) = true, want false", workflowType)
			}
		})
	}
}

func TestRecordingStatusConstants(t *testing.T) {
	if RecordingStatusUploaded != RecordingStatus("uploaded") {
		t.Fatalf("RecordingStatusUploaded = %q, want uploaded", RecordingStatusUploaded)
	}
}
