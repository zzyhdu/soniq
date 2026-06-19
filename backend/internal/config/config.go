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
	LogFormat                                    string
	LogLevel                                     string
	AuthSessionTTLHours                          int64
	AuthCookieSecure                             bool
	APIAddress                                   string
	PostgresDSN                                  string
	TemporalAddress                              string
	TemporalNamespace                            string
	TemporalTaskQueue                            string
	WorkerMaxConcurrentWorkflowTasks             int64
	WorkerMaxConcurrentActivities                int64
	WorkerMaxConcurrentLocalActivities           int64
	WorkerTaskQueueActivitiesPerSecond           float64
	PurgeArtifactCleanupIntervalSeconds          int64
	PurgeArtifactCleanupBatchSize                int64
	StorageProvider                              string
	S3Endpoint                                   string
	S3Region                                     string
	S3Bucket                                     string
	S3AccessKey                                  string
	S3SecretKey                                  string
	S3ForcePathStyle                             bool
	TranscriptionProvider                        string
	TranscriptionBaseURL                         string
	TranscriptionAPIKey                          string
	TranscriptionModel                           string
	TranscriptionAuthHeader                      string
	TranscriptionLanguage                        string
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
		AppEnv:                              envString("APP_ENV", "development"),
		PublicURL:                           envString("APP_PUBLIC_URL", "http://localhost:8080"),
		LogFormat:                           envString("LOG_FORMAT", "text"),
		LogLevel:                            envString("LOG_LEVEL", "info"),
		AuthSessionTTLHours:                 envInt64("AUTH_SESSION_TTL_HOURS", 24*30),
		AuthCookieSecure:                    envBool("AUTH_COOKIE_SECURE", false),
		APIAddress:                          envString("API_ADDRESS", ":8080"),
		PostgresDSN:                         envString("POSTGRES_DSN", "postgres://soniq_user:soniq_password@localhost:5432/soniq?sslmode=disable"),
		TemporalAddress:                     envString("TEMPORAL_ADDRESS", "localhost:7233"),
		TemporalNamespace:                   envString("TEMPORAL_NAMESPACE", "default"),
		TemporalTaskQueue:                   envString("TEMPORAL_TASK_QUEUE", "soniq-audio-pipeline"),
		WorkerMaxConcurrentWorkflowTasks:    envInt64ForValidation("WORKER_MAX_CONCURRENT_WORKFLOW_TASKS", 20),
		WorkerMaxConcurrentActivities:       envInt64ForValidation("WORKER_MAX_CONCURRENT_ACTIVITIES", 4),
		WorkerMaxConcurrentLocalActivities:  envInt64ForValidation("WORKER_MAX_CONCURRENT_LOCAL_ACTIVITIES", 4),
		WorkerTaskQueueActivitiesPerSecond:  envFloat64("WORKER_TASK_QUEUE_ACTIVITIES_PER_SECOND", 0),
		PurgeArtifactCleanupIntervalSeconds: envInt64("PURGE_ARTIFACT_CLEANUP_INTERVAL_SECONDS", 300),
		PurgeArtifactCleanupBatchSize:       envInt64("PURGE_ARTIFACT_CLEANUP_BATCH_SIZE", 25),
		StorageProvider:                     envString("STORAGE_PROVIDER", "s3_compatible"),
		S3Endpoint:                          envString("S3_ENDPOINT", "http://localhost:9000"),
		S3Region:                            envString("S3_REGION", "us-east-1"),
		S3Bucket:                            envString("S3_BUCKET", "soniq"),
		S3AccessKey:                         envString("S3_ACCESS_KEY", "soniq_minio_user"),
		S3SecretKey:                         envString("S3_SECRET_KEY", "soniq_minio_password"),
		S3ForcePathStyle:                    envBool("S3_FORCE_PATH_STYLE", true),
		TranscriptionProvider:               envString("TRANSCRIPTION_PROVIDER", "fake_transcription"),
		TranscriptionBaseURL:                envString("TRANSCRIPTION_BASE_URL", "https://api.xiaomimimo.com/v1"),
		TranscriptionAPIKey:                 envStringWithFallback("TRANSCRIPTION_API_KEY", "MIMO_API_KEY", ""),
		TranscriptionModel:                  envString("TRANSCRIPTION_MODEL", "mimo-v2.5-asr"),
		TranscriptionAuthHeader:             envString("TRANSCRIPTION_AUTH_HEADER", "api-key"),
		TranscriptionLanguage:               envString("TRANSCRIPTION_LANGUAGE", "auto"),
		DashScopeBaseURL:                    envString("DASHSCOPE_BASE_URL", "https://dashscope.aliyuncs.com/api/v1"),
		DashScopeAPIKey:                     envString("DASHSCOPE_API_KEY", ""),
		DashScopeASRModel:                   envString("DASHSCOPE_ASR_MODEL", "paraformer-v2"),
		LLMProvider:                         envString("LLM_PROVIDER", "fake_llm"),
		LLMBaseURL:                          envString("LLM_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
		LLMAPIKey:                           envStringWithFallback("LLM_API_KEY", "DASHSCOPE_API_KEY", ""),
		LLMModel:                            envString("LLM_MODEL", "qwen3.7-plus"),
		PrivacyAllowExternalModelProviders:  envBool("PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS", true),
		PrivacyDeleteOriginalAudioAfterTranscription: envBool("PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION", false),
	}
}

// ValidateForStartup checks the minimal invariants required before starting a process.
func (c Config) ValidateForStartup() error {
	if strings.TrimSpace(c.PostgresDSN) == "" {
		return fmt.Errorf("POSTGRES_DSN is required")
	}
	if !isSupportedLogFormat(c.LogFormat) {
		return fmt.Errorf("LOG_FORMAT must be one of: json, text")
	}
	if !isSupportedLogLevel(c.LogLevel) {
		return fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error")
	}
	if c.AuthSessionTTLHours <= 0 {
		return fmt.Errorf("AUTH_SESSION_TTL_HOURS must be positive")
	}
	if strings.TrimSpace(c.TemporalTaskQueue) == "" {
		return fmt.Errorf("TEMPORAL_TASK_QUEUE is required")
	}
	if c.WorkerMaxConcurrentWorkflowTasks <= 1 {
		return fmt.Errorf("WORKER_MAX_CONCURRENT_WORKFLOW_TASKS must be greater than 1")
	}
	if c.WorkerMaxConcurrentActivities <= 0 {
		return fmt.Errorf("WORKER_MAX_CONCURRENT_ACTIVITIES must be positive")
	}
	if c.WorkerMaxConcurrentLocalActivities <= 0 {
		return fmt.Errorf("WORKER_MAX_CONCURRENT_LOCAL_ACTIVITIES must be positive")
	}
	if c.WorkerTaskQueueActivitiesPerSecond < 0 {
		return fmt.Errorf("WORKER_TASK_QUEUE_ACTIVITIES_PER_SECOND must be non-negative")
	}
	if c.PurgeArtifactCleanupIntervalSeconds <= 0 {
		return fmt.Errorf("PURGE_ARTIFACT_CLEANUP_INTERVAL_SECONDS must be positive")
	}
	if c.PurgeArtifactCleanupBatchSize <= 0 {
		return fmt.Errorf("PURGE_ARTIFACT_CLEANUP_BATCH_SIZE must be positive")
	}
	if strings.TrimSpace(c.StorageProvider) == "" {
		return fmt.Errorf("STORAGE_PROVIDER is required")
	}
	switch strings.ToLower(strings.TrimSpace(c.StorageProvider)) {
	case "s3_compatible":
		if strings.TrimSpace(c.S3Endpoint) == "" {
			return fmt.Errorf("S3_ENDPOINT is required for s3_compatible storage")
		}
		if strings.TrimSpace(c.S3Region) == "" {
			return fmt.Errorf("S3_REGION is required for s3_compatible storage")
		}
		if strings.TrimSpace(c.S3Bucket) == "" {
			return fmt.Errorf("S3_BUCKET is required for s3_compatible storage")
		}
		if strings.TrimSpace(c.S3AccessKey) == "" {
			return fmt.Errorf("S3_ACCESS_KEY is required for s3_compatible storage")
		}
		if strings.TrimSpace(c.S3SecretKey) == "" {
			return fmt.Errorf("S3_SECRET_KEY is required for s3_compatible storage")
		}
	default:
		return fmt.Errorf("unsupported STORAGE_PROVIDER %q", c.StorageProvider)
	}
	if strings.TrimSpace(c.TranscriptionProvider) == "" {
		return fmt.Errorf("TRANSCRIPTION_PROVIDER is required")
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

func isSupportedLogFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json", "text":
		return true
	default:
		return false
	}
}

func isSupportedLogLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
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

func envInt64ForValidation(key string, fallback int64) int64 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	var parsed int64
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed); err != nil {
		return fallback
	}
	return parsed
}

func envFloat64(key string, fallback float64) float64 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	var parsed float64
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%f", &parsed); err != nil {
		return fallback
	}
	return parsed
}
