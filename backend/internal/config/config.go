package config

import (
	"fmt"
	"os"
	"strings"
)

// Config contains runtime configuration for the Soniq backend.
type Config struct {
	AppEnv                                       string
	PublicURL                                    string
	APIAddress                                   string
	PostgresDSN                                  string
	TemporalAddress                              string
	TemporalNamespace                            string
	TemporalTaskQueue                            string
	StorageProvider                              string
	LocalStoragePath                             string
	TranscriptionProvider                        string
	LLMProvider                                  string
	LLMBaseURL                                   string
	LLMAPIKey                                    string
	LLMModel                                     string
	PrivacyAllowExternalModelProviders           bool
	PrivacyDeleteOriginalAudioAfterTranscription bool
}

// LoadFromEnv loads configuration from environment variables with local-development defaults.
func LoadFromEnv() Config {
	return Config{
		AppEnv:                             envString("APP_ENV", "development"),
		PublicURL:                          envString("APP_PUBLIC_URL", "http://localhost:8080"),
		APIAddress:                         envString("API_ADDRESS", ":8080"),
		PostgresDSN:                        envString("POSTGRES_DSN", "postgres://soniq_user:soniq_password@localhost:5432/soniq?sslmode=disable"),
		TemporalAddress:                    envString("TEMPORAL_ADDRESS", "localhost:7233"),
		TemporalNamespace:                  envString("TEMPORAL_NAMESPACE", "default"),
		TemporalTaskQueue:                  envString("TEMPORAL_TASK_QUEUE", "soniq-audio-pipeline"),
		StorageProvider:                    envString("STORAGE_PROVIDER", "local"),
		LocalStoragePath:                   envString("LOCAL_STORAGE_PATH", "var/uploads"),
		TranscriptionProvider:              envString("TRANSCRIPTION_PROVIDER", "faster_whisper"),
		LLMProvider:                        envString("LLM_PROVIDER", "openai_compatible"),
		LLMBaseURL:                         envString("LLM_BASE_URL", "https://api.openai.com/v1"),
		LLMAPIKey:                          envString("LLM_API_KEY", ""),
		LLMModel:                           envString("LLM_MODEL", "gpt-4o-mini"),
		PrivacyAllowExternalModelProviders: envBool("PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS", true),
		PrivacyDeleteOriginalAudioAfterTranscription: envBool("PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION", false),
	}
}

// ValidateForStartup checks the minimal invariants required before starting a process.
func (c Config) ValidateForStartup() error {
	if strings.TrimSpace(c.PostgresDSN) == "" {
		return fmt.Errorf("POSTGRES_DSN is required")
	}
	if strings.TrimSpace(c.TemporalTaskQueue) == "" {
		return fmt.Errorf("TEMPORAL_TASK_QUEUE is required")
	}
	if strings.TrimSpace(c.StorageProvider) == "" {
		return fmt.Errorf("STORAGE_PROVIDER is required")
	}
	if strings.TrimSpace(c.LocalStoragePath) == "" {
		return fmt.Errorf("LOCAL_STORAGE_PATH is required")
	}
	if strings.TrimSpace(c.TranscriptionProvider) == "" {
		return fmt.Errorf("TRANSCRIPTION_PROVIDER is required")
	}
	if strings.TrimSpace(c.LLMProvider) == "" {
		return fmt.Errorf("LLM_PROVIDER is required")
	}
	return nil
}

// NeedsLLMAPIKeyForExternalProvider reports whether the selected LLM provider usually needs an API key.
func (c Config) NeedsLLMAPIKeyForExternalProvider() bool {
	if strings.TrimSpace(c.LLMAPIKey) != "" {
		return false
	}
	provider := strings.TrimSpace(c.LLMProvider)
	if provider == "" || provider == "ollama" {
		return false
	}
	return c.PrivacyAllowExternalModelProviders
}

func envString(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
