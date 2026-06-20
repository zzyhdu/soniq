package processing

import (
	"context"
	"errors"
	"strings"

	"github.com/zzyhdu/soniq/backend/internal/domain"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
)

// WorkflowStarter is the minimal Temporal client behavior needed to start workflows.
type WorkflowStarter interface {
	ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error)
}

// TemporalRecordingProcessorConfig contains Temporal workflow start settings.
type TemporalRecordingProcessorConfig struct {
	TaskQueue                             string
	DeleteOriginalAudioAfterTranscription bool
}

// TemporalRecordingProcessor starts the recording processing workflow for new recordings.
type TemporalRecordingProcessor struct {
	starter WorkflowStarter
	config  TemporalRecordingProcessorConfig
}

// NewTemporalRecordingProcessor builds a Temporal-backed recording processor.
func NewTemporalRecordingProcessor(starter WorkflowStarter, config TemporalRecordingProcessorConfig) TemporalRecordingProcessor {
	return TemporalRecordingProcessor{
		starter: starter,
		config:  config,
	}
}

// Enqueue starts RecordingProcessingWorkflow asynchronously for recording.
func (p TemporalRecordingProcessor) Enqueue(ctx context.Context, recording domain.Recording) error {
	if strings.TrimSpace(p.config.TaskQueue) == "" {
		return errors.New("temporal task queue is required")
	}
	if p.starter == nil {
		return errors.New("temporal workflow starter is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	_, err := p.starter.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID:                    "recording-processing-" + recording.ID,
			TaskQueue:             p.config.TaskQueue,
			WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY,
		},
		workflows.RecordingProcessingWorkflow,
		workflows.RecordingProcessingInput{
			WorkspaceID:                           recording.WorkspaceID,
			RecordingID:                           recording.ID,
			WorkflowType:                          recording.WorkflowType,
			Language:                              recording.Language,
			DeleteOriginalAudioAfterTranscription: p.config.DeleteOriginalAudioAfterTranscription,
		},
	)
	return err
}
