package main

import (
	"context"
	"log"
	"net/http"

	"github.com/zzyhdu/soniq/backend/internal/api"
	"github.com/zzyhdu/soniq/backend/internal/config"
	"github.com/zzyhdu/soniq/backend/internal/processing"
	"github.com/zzyhdu/soniq/backend/internal/recordings"
	"go.temporal.io/sdk/client"
)

func main() {
	cfg := config.LoadFromEnv()
	if err := cfg.ValidateForStartup(); err != nil {
		log.Fatalf("invalid startup config: %v", err)
	}

	handler, cleanup, err := buildHandler(context.Background(), cfg, dialTemporalClient)
	if err != nil {
		log.Fatalf("build api handler: %v", err)
	}
	defer cleanup()

	addr := cfg.APIAddress
	log.Printf("starting soniq-api on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("api server stopped: %v", err)
	}
}

type temporalWorkflowClient interface {
	processing.WorkflowStarter
	Close()
}

type temporalClientFactory func(context.Context, config.Config) (temporalWorkflowClient, error)

func buildHandler(ctx context.Context, cfg config.Config, factory temporalClientFactory) (http.Handler, func(), error) {
	temporalClient, err := factory(ctx, cfg)
	if err != nil {
		return nil, func() {}, err
	}

	processor := processing.NewTemporalRecordingProcessor(temporalClient, processing.TemporalRecordingProcessorConfig{
		TaskQueue: cfg.TemporalTaskQueue,
	})
	handler := api.NewRouterWithProcessor(recordings.NewMemoryStore(), processor)
	cleanup := func() {
		temporalClient.Close()
	}

	return handler, cleanup, nil
}

func dialTemporalClient(ctx context.Context, cfg config.Config) (temporalWorkflowClient, error) {
	return client.DialContext(ctx, client.Options{
		HostPort:  cfg.TemporalAddress,
		Namespace: cfg.TemporalNamespace,
	})
}
