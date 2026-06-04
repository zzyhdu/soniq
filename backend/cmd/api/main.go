package main

import (
	"log"
	"net/http"

	"github.com/zzyhdu/soniq/backend/internal/api"
	"github.com/zzyhdu/soniq/backend/internal/config"
)

func main() {
	cfg := config.LoadFromEnv()
	if err := cfg.ValidateForStartup(); err != nil {
		log.Fatalf("invalid startup config: %v", err)
	}

	addr := cfg.APIAddress
	log.Printf("starting soniq-api on %s", addr)
	if err := http.ListenAndServe(addr, api.NewRouter()); err != nil {
		log.Fatalf("api server stopped: %v", err)
	}
}
