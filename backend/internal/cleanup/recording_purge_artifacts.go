package cleanup

import (
	"context"
	"errors"
	"log"
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

// RecordingPurgeArtifactCleanerOptions configures the cleanup loop.
type RecordingPurgeArtifactCleanerOptions struct {
	Interval  time.Duration
	BatchSize int
}

// RecordingPurgeArtifactCleaner deletes object-storage artifacts left by permanent recording purge.
type RecordingPurgeArtifactCleaner struct {
	store       RecordingPurgeArtifactStore
	objectStore storage.ObjectStore
	interval    time.Duration
	batchSize   int
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
	return &RecordingPurgeArtifactCleaner{
		store:       store,
		objectStore: objectStore,
		interval:    interval,
		batchSize:   batchSize,
	}
}

// Run starts the periodic cleanup loop until the context is canceled.
func (c *RecordingPurgeArtifactCleaner) Run(ctx context.Context) {
	if err := c.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("recording purge artifact cleanup failed: %v", err)
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("recording purge artifact cleanup failed: %v", err)
			}
		}
	}
}

// RunOnce claims one batch and attempts cleanup.
func (c *RecordingPurgeArtifactCleaner) RunOnce(ctx context.Context) error {
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
	var runErr error
	for _, artifact := range artifacts {
		if err := c.objectStore.DeleteObject(ctx, artifact.ObjectKey); err != nil {
			_, markErr := c.store.MarkPurgeArtifactFailed(recordings.MarkPurgeArtifactFailedInput{
				ID:            artifact.ID,
				LastError:     err.Error(),
				NextAttemptAt: time.Now().UTC().Add(purgeArtifactRetryDelay(artifact.AttemptCount + 1)),
			})
			runErr = errors.Join(runErr, err, markErr)
			continue
		}
		_, err := c.store.MarkPurgeArtifactDeleted(recordings.MarkPurgeArtifactDeletedInput{ID: artifact.ID})
		runErr = errors.Join(runErr, err)
	}
	return runErr
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
