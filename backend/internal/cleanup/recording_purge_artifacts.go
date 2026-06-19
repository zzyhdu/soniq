package cleanup

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"github.com/zzyhdu/soniq/backend/internal/storage"
)

// RecordingPurgeArtifactStore persists purge artifact cleanup state.
type RecordingPurgeArtifactStore interface {
	ClaimPurgeArtifacts(input recordings.ClaimPurgeArtifactsInput) ([]recordings.RecordingPurgeArtifact, error)
	MarkPurgeArtifactDeleted(input recordings.MarkPurgeArtifactDeletedInput) (bool, error)
	MarkPurgeArtifactFailed(input recordings.MarkPurgeArtifactFailedInput) (bool, error)
}

// RecordingPurgeArtifactMetrics records low-cardinality cleanup metrics.
type RecordingPurgeArtifactMetrics interface {
	ObservePurgeArtifactsClaimed(count int)
	ObservePurgeArtifactDeleted()
	ObservePurgeArtifactFailed()
	ObservePurgeCleanupRun(result string, duration time.Duration)
}

// RecordingPurgeArtifactCleanerOptions configures the cleanup loop.
type RecordingPurgeArtifactCleanerOptions struct {
	Interval  time.Duration
	BatchSize int
	Logger    *slog.Logger
	Metrics   RecordingPurgeArtifactMetrics
}

// RecordingPurgeArtifactCleaner deletes object-storage artifacts left by permanent recording purge.
type RecordingPurgeArtifactCleaner struct {
	store       RecordingPurgeArtifactStore
	objectStore storage.ObjectStore
	interval    time.Duration
	batchSize   int
	logger      *slog.Logger
	metrics     RecordingPurgeArtifactMetrics
}

// NewRecordingPurgeArtifactCleaner creates a purge artifact cleanup runner.
func NewRecordingPurgeArtifactCleaner(store RecordingPurgeArtifactStore, objectStore storage.ObjectStore, options RecordingPurgeArtifactCleanerOptions) *RecordingPurgeArtifactCleaner {
	interval := options.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = 25
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &RecordingPurgeArtifactCleaner{
		store:       store,
		objectStore: objectStore,
		interval:    interval,
		batchSize:   batchSize,
		logger:      logger,
		metrics:     options.Metrics,
	}
}

// Run starts the periodic cleanup loop until the context is canceled.
func (c *RecordingPurgeArtifactCleaner) Run(ctx context.Context) {
	if err := c.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		c.logger.ErrorContext(ctx, "recording purge artifact cleanup failed",
			slog.String("event", "purge_artifact_cleanup_run_failed"),
			slog.Any("error", err),
		)
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				c.logger.ErrorContext(ctx, "recording purge artifact cleanup failed",
					slog.String("event", "purge_artifact_cleanup_run_failed"),
					slog.Any("error", err),
				)
			}
		}
	}
}

// RunOnce claims one batch and attempts cleanup.
func (c *RecordingPurgeArtifactCleaner) RunOnce(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() {
		result := "success"
		if err != nil {
			result = "error"
		}
		c.observePurgeCleanupRun(result, time.Since(startedAt))
	}()
	if c == nil || c.store == nil {
		return errors.New("purge artifact store is required")
	}
	if c.objectStore == nil {
		return errors.New("object store is required")
	}
	artifacts, err := c.store.ClaimPurgeArtifacts(recordings.ClaimPurgeArtifactsInput{Limit: c.batchSize})
	if err != nil {
		return err
	}
	if len(artifacts) > 0 {
		c.observePurgeArtifactsClaimed(len(artifacts))
		c.logger.InfoContext(ctx, "claimed recording purge artifacts",
			slog.String("event", "purge_artifact_cleanup_claimed"),
			slog.Int("artifact_count", len(artifacts)),
			slog.Int("batch_size", c.batchSize),
		)
	}
	var runErr error
	for _, artifact := range artifacts {
		if err := c.objectStore.DeleteObject(ctx, artifact.ObjectKey); err != nil {
			c.observePurgeArtifactFailed()
			nextAttemptAt := time.Now().UTC().Add(purgeArtifactRetryDelay(artifact.AttemptCount + 1))
			_, markErr := c.store.MarkPurgeArtifactFailed(recordings.MarkPurgeArtifactFailedInput{
				ID:            artifact.ID,
				LastError:     err.Error(),
				NextAttemptAt: nextAttemptAt,
			})
			c.logger.WarnContext(ctx, "recording purge artifact cleanup failed",
				append(purgeArtifactLogAttrs("purge_artifact_cleanup_failed", artifact),
					slog.Int("attempt_count", artifact.AttemptCount+1),
					slog.Time("next_attempt_at", nextAttemptAt),
					slog.String("error", err.Error()),
					slog.Any("mark_error", markErr),
				)...,
			)
			runErr = errors.Join(runErr, err, markErr)
			continue
		}
		_, err := c.store.MarkPurgeArtifactDeleted(recordings.MarkPurgeArtifactDeletedInput{ID: artifact.ID})
		if err != nil {
			c.logger.WarnContext(ctx, "recording purge artifact mark deleted failed",
				append(purgeArtifactLogAttrs("purge_artifact_cleanup_failed", artifact),
					slog.Int("attempt_count", artifact.AttemptCount),
					slog.String("error", err.Error()),
				)...,
			)
			runErr = errors.Join(runErr, err)
			continue
		}
		c.observePurgeArtifactDeleted()
		c.logger.InfoContext(ctx, "recording purge artifact deleted",
			append(purgeArtifactLogAttrs("purge_artifact_cleanup_deleted", artifact),
				slog.Int("attempt_count", artifact.AttemptCount),
			)...,
		)
	}
	return runErr
}

func (c *RecordingPurgeArtifactCleaner) observePurgeArtifactsClaimed(count int) {
	if c != nil && c.metrics != nil {
		c.metrics.ObservePurgeArtifactsClaimed(count)
	}
}

func (c *RecordingPurgeArtifactCleaner) observePurgeArtifactDeleted() {
	if c != nil && c.metrics != nil {
		c.metrics.ObservePurgeArtifactDeleted()
	}
}

func (c *RecordingPurgeArtifactCleaner) observePurgeArtifactFailed() {
	if c != nil && c.metrics != nil {
		c.metrics.ObservePurgeArtifactFailed()
	}
}

func (c *RecordingPurgeArtifactCleaner) observePurgeCleanupRun(result string, duration time.Duration) {
	if c != nil && c.metrics != nil {
		c.metrics.ObservePurgeCleanupRun(result, duration)
	}
}

func purgeArtifactLogAttrs(event string, artifact recordings.RecordingPurgeArtifact) []any {
	return []any{
		slog.String("event", event),
		slog.String("artifact_id", artifact.ID),
		slog.String("recording_id", artifact.RecordingID),
		slog.String("workspace_id", artifact.WorkspaceID),
		slog.String("artifact_kind", artifact.ArtifactKind),
	}
}

func purgeArtifactRetryDelay(attemptCount int) time.Duration {
	if attemptCount <= 1 {
		return time.Minute
	}
	if attemptCount == 2 {
		return 5 * time.Minute
	}
	if attemptCount == 3 {
		return 15 * time.Minute
	}
	return time.Hour
}
