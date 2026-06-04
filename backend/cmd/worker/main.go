package main

import (
	"context"
	"log"

	"github.com/zzyhdu/soniq/backend/internal/activities"
	"github.com/zzyhdu/soniq/backend/internal/config"
	"github.com/zzyhdu/soniq/backend/internal/workflows"
	"go.temporal.io/sdk/client"
	temporalworker "go.temporal.io/sdk/worker"
)

func main() {
	cfg := config.LoadFromEnv()
	if err := cfg.ValidateForStartup(); err != nil {
		log.Fatalf("invalid startup config: %v", err)
	}

	log.Printf("starting temporal worker")
	log.Printf("temporal_address=%s", cfg.TemporalAddress)
	log.Printf("temporal_namespace=%s", cfg.TemporalNamespace)
	log.Printf("temporal_task_queue=%s", cfg.TemporalTaskQueue)

	if err := run(context.Background(), cfg); err != nil {
		log.Fatalf("worker stopped: %v", err)
	}
}

func run(ctx context.Context, cfg config.Config) error {
	temporalClient, err := client.DialContext(ctx, client.Options{
		HostPort:  cfg.TemporalAddress,
		Namespace: cfg.TemporalNamespace,
	})
	if err != nil {
		return err
	}
	defer temporalClient.Close()

	worker := temporalworker.New(temporalClient, cfg.TemporalTaskQueue, temporalworker.Options{})
	registerRecordingProcessing(worker)

	return worker.Run(temporalworker.InterruptCh())
}

type recordingProcessingRegistry interface {
	RegisterWorkflow(interface{})
	RegisterActivity(interface{})
}

func registerRecordingProcessing(registry recordingProcessingRegistry) {
	registry.RegisterWorkflow(workflows.RecordingProcessingWorkflow)
	registry.RegisterActivity(activities.ValidateRecordingActivity)
	registry.RegisterActivity(activities.MarkRecordingProcessingActivity)
	registry.RegisterActivity(activities.CompleteRecordingProcessingActivity)
}
