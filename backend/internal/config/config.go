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
	TranscriptionBaseURL                         string
	TranscriptionAPIKey                          string
	TranscriptionModel                           string
	TranscriptionAuthHeader                      string
	TranscriptionLanguage                        string
	TranscriptionMaxBase64Bytes                  int64
	DashScopeBaseURL                             string
	DashScopeAPIKey                              string
	DashScopeASRModel                            string
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
		TranscriptionProvider:              envString("TRANSCRIPTION_PROVIDER", "fake_transcription"),
		TranscriptionBaseURL:               envString("TRANSCRIPTION_BASE_URL", "https://api.xiaomimimo.com/v1"),
		TranscriptionAPIKey:                envStringWithFallback("TRANSCRIPTION_API_KEY", "MIMO_API_KEY", ""),
		TranscriptionModel:                 envString("TRANSCRIPTION_MODEL", "mimo-v2.5-asr"),
		TranscriptionAuthHeader:            envString("TRANSCRIPTION_AUTH_HEADER", "api-key"),
		TranscriptionLanguage:              envString("TRANSCRIPTION_LANGUAGE", "auto"),
		TranscriptionMaxBase64Bytes:        envInt64("TRANSCRIPTION_MAX_BASE64_BYTES", 10*1024*1024),
		DashScopeBaseURL:                   envString("DASHSCOPE_BASE_URL", "https://dashscope.aliyuncs.com/api/v1"),
		DashScopeAPIKey:                    envString("DASHSCOPE_API_KEY", ""),
		DashScopeASRModel:                  envString("DASHSCOPE_ASR_MODEL", "paraformer-v2"),
		LLMProvider:                        envString("LLM_PROVIDER", "fake_llm"),
		LLMBaseURL:                         envString("LLM_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
		LLMAPIKey:                          envStringWithFallback("LLM_API_KEY", "DASHSCOPE_API_KEY", ""),
		LLMModel:                           envString("LLM_MODEL", "qwen3.7-plus"),
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
	if c.TranscriptionMaxBase64Bytes <= 0 {
		return fmt.Errorf("TRANSCRIPTION_MAX_BASE64_BYTES must be positive")
	}
	if strings.TrimSpace(c.TranscriptionProvider) == "dashscope_asr" {
		if strings.TrimSpace(c.DashScopeBaseURL) == "" {
			return fmt.Errorf("DASHSCOPE_BASE_URL is required for external transcription provider")
		}
		if strings.TrimSpace(c.DashScopeASRModel) == "" {
			return fmt.Errorf("DASHSCOPE_ASR_MODEL is required for external transcription provider")
		}
		if c.NeedsTranscriptionAPIKeyForExternalProvider() {
			return fmt.Errorf("DASHSCOPE_API_KEY is required for external transcription provider")
		}
	} else if c.isExternalTranscriptionProvider() {
		if strings.TrimSpace(c.TranscriptionBaseURL) == "" {
			return fmt.Errorf("TRANSCRIPTION_BASE_URL is required for external transcription provider")
		}
		if strings.TrimSpace(c.TranscriptionModel) == "" {
			return fmt.Errorf("TRANSCRIPTION_MODEL is required for external transcription provider")
		}
		if c.NeedsTranscriptionAPIKeyForExternalProvider() {
			return fmt.Errorf("TRANSCRIPTION_API_KEY is required for external transcription provider")
		}
	}
	if strings.TrimSpace(c.LLMProvider) == "" {
		return fmt.Errorf("LLM_PROVIDER is required")
	}
	if c.isExternalLLMProvider() {
		if strings.TrimSpace(c.LLMBaseURL) == "" {
			return fmt.Errorf("LLM_BASE_URL is required for external LLM provider")
		}
		if strings.TrimSpace(c.LLMModel) == "" {
			return fmt.Errorf("LLM_MODEL is required for external LLM provider")
		}
		if c.NeedsLLMAPIKeyForExternalProvider() {
			return fmt.Errorf("LLM_API_KEY is required for external LLM provider")
		}
	}
	return nil
}

// NeedsTranscriptionAPIKeyForExternalProvider reports whether the selected transcription provider needs an API key.
func (c Config) NeedsTranscriptionAPIKeyForExternalProvider() bool {
	if strings.TrimSpace(c.TranscriptionProvider) == "dashscope_asr" {
		if strings.TrimSpace(c.DashScopeAPIKey) != "" {
			return false
		}
		return c.PrivacyAllowExternalModelProviders
	}
	if strings.TrimSpace(c.TranscriptionAPIKey) != "" {
		return false
	}
	return c.isExternalTranscriptionProvider() && c.PrivacyAllowExternalModelProviders
}

func (c Config) isExternalTranscriptionProvider() bool {
	provider := strings.TrimSpace(c.TranscriptionProvider)
	return provider != "" && provider != "fake_transcription"
}

// NeedsLLMAPIKeyForExternalProvider reports whether the selected LLM provider usually needs an API key.
func (c Config) NeedsLLMAPIKeyForExternalProvider() bool {
	if strings.TrimSpace(c.LLMAPIKey) != "" {
		return false
	}
	return c.isExternalLLMProvider() && c.PrivacyAllowExternalModelProviders
}

func (c Config) isExternalLLMProvider() bool {
	provider := strings.TrimSpace(c.LLMProvider)
	return provider != "" && provider != "fake_llm" && provider != "ollama"
}

func envString(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return value
}

func envStringWithFallback(primaryKey, fallbackKey, fallback string) string {
	value, ok := os.LookupEnv(primaryKey)
	if ok && strings.TrimSpace(value) != "" {
		return value
	}
	value, ok = os.LookupEnv(fallbackKey)
	if ok {
		return value
	}
	return fallback
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

func envInt64(key string, fallback int64) int64 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	var parsed int64
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
