package main

import (
	"log"

	"github.com/zzyhdu/soniq/backend/internal/config"
)

func main() {
	cfg := config.LoadFromEnv()
	if err := cfg.ValidateForStartup(); err != nil {
		log.Fatalf("invalid startup config: %v", err)
	}

	log.Printf("worker skeleton ready")
	log.Printf("temporal_address=%s", cfg.TemporalAddress)
	log.Printf("temporal_namespace=%s", cfg.TemporalNamespace)
	log.Printf("temporal_task_queue=%s", cfg.TemporalTaskQueue)
}
