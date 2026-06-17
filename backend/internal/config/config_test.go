package config

import (
	"os"
	"testing"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"APP_ENV",
		"APP_PUBLIC_URL",
		"LOG_FORMAT",
		"LOG_LEVEL",
		"AUTH_SESSION_TTL_HOURS",
		"AUTH_COOKIE_SECURE",
		"API_ADDRESS",
		"POSTGRES_DSN",
		"TEMPORAL_ADDRESS",
		"TEMPORAL_NAMESPACE",
		"TEMPORAL_TASK_QUEUE",
		"PURGE_ARTIFACT_CLEANUP_INTERVAL_SECONDS",
		"PURGE_ARTIFACT_CLEANUP_BATCH_SIZE",
		"STORAGE_PROVIDER",
		"LOCAL_STORAGE_PATH",
		"S3_ENDPOINT",
		"S3_REGION",
		"S3_BUCKET",
		"S3_ACCESS_KEY",
		"S3_SECRET_KEY",
		"S3_FORCE_PATH_STYLE",
		"TRANSCRIPTION_PROVIDER",
		"TRANSCRIPTION_BASE_URL",
		"TRANSCRIPTION_API_KEY",
		"MIMO_API_KEY",
		"TRANSCRIPTION_MODEL",
		"TRANSCRIPTION_AUTH_HEADER",
		"TRANSCRIPTION_LANGUAGE",
		"TRANSCRIPTION_MAX_BASE64_BYTES",
		"DASHSCOPE_BASE_URL",
		"DASHSCOPE_API_KEY",
		"DASHSCOPE_ASR_MODEL",
		"LLM_PROVIDER",
		"LLM_BASE_URL",
		"LLM_API_KEY",
		"LLM_MODEL",
		"PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS",
		"PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION",
	} {
		key := key
		oldValue, hadValue := os.LookupEnv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if hadValue {
				os.Setenv(key, oldValue)
			} else {
				os.Unsetenv(key)
			}
		})
	}
}

func TestLoadFromEnvUsesDevelopmentDefaults(t *testing.T) {
	clearConfigEnv(t)
	cfg := LoadFromEnv()

	if cfg.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", cfg.AppEnv)
	}
	if cfg.PublicURL != "http://localhost:8080" {
		t.Fatalf("PublicURL = %q, want http://localhost:8080", cfg.PublicURL)
	}
	if cfg.LogFormat != "text" {
		t.Fatalf("LogFormat = %q, want text", cfg.LogFormat)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.AuthSessionTTLHours != 720 {
		t.Fatalf("AuthSessionTTLHours = %d, want 720", cfg.AuthSessionTTLHours)
	}
	if cfg.AuthCookieSecure {
		t.Fatal("AuthCookieSecure = true, want false")
	}
	if cfg.APIAddress != ":8080" {
		t.Fatalf("APIAddress = %q, want :8080", cfg.APIAddress)
	}
	if cfg.PostgresDSN != "postgres://soniq_user:soniq_password@localhost:5432/soniq?sslmode=disable" {
		t.Fatalf("PostgresDSN = %q, want local development DSN", cfg.PostgresDSN)
	}
	if cfg.TemporalAddress != "localhost:7233" {
		t.Fatalf("TemporalAddress = %q, want localhost:7233", cfg.TemporalAddress)
	}
	if cfg.TemporalNamespace != "default" {
		t.Fatalf("TemporalNamespace = %q, want default", cfg.TemporalNamespace)
	}
	if cfg.TemporalTaskQueue != "soniq-audio-pipeline" {
		t.Fatalf("TemporalTaskQueue = %q, want soniq-audio-pipeline", cfg.TemporalTaskQueue)
	}
	if cfg.PurgeArtifactCleanupIntervalSeconds != 300 {
		t.Fatalf("PurgeArtifactCleanupIntervalSeconds = %d, want 300", cfg.PurgeArtifactCleanupIntervalSeconds)
	}
	if cfg.PurgeArtifactCleanupBatchSize != 25 {
		t.Fatalf("PurgeArtifactCleanupBatchSize = %d, want 25", cfg.PurgeArtifactCleanupBatchSize)
	}
	if cfg.StorageProvider != "local" {
		t.Fatalf("StorageProvider = %q, want local", cfg.StorageProvider)
	}
	if cfg.LocalStoragePath != "var/uploads" {
		t.Fatalf("LocalStoragePath = %q, want var/uploads", cfg.LocalStoragePath)
	}
	if cfg.S3Endpoint != "http://localhost:9000" {
		t.Fatalf("S3Endpoint = %q, want local MinIO endpoint", cfg.S3Endpoint)
	}
	if cfg.S3Region != "us-east-1" {
		t.Fatalf("S3Region = %q, want us-east-1", cfg.S3Region)
	}
	if cfg.S3Bucket != "soniq" {
		t.Fatalf("S3Bucket = %q, want soniq", cfg.S3Bucket)
	}
	if cfg.S3AccessKey != "soniq_minio_user" {
		t.Fatalf("S3AccessKey = %q, want local MinIO user", cfg.S3AccessKey)
	}
	if cfg.S3SecretKey != "soniq_minio_password" {
		t.Fatal("S3SecretKey default was not loaded")
	}
	if !cfg.S3ForcePathStyle {
		t.Fatal("S3ForcePathStyle = false, want true for local MinIO")
	}
	if cfg.TranscriptionProvider != "fake_transcription" {
		t.Fatalf("TranscriptionProvider = %q, want fake_transcription", cfg.TranscriptionProvider)
	}
	if cfg.TranscriptionBaseURL != "https://api.xiaomimimo.com/v1" {
		t.Fatalf("TranscriptionBaseURL = %q, want Xiaomi MiMo default", cfg.TranscriptionBaseURL)
	}
	if cfg.TranscriptionAPIKey != "" {
		t.Fatal("TranscriptionAPIKey default should be empty")
	}
	if cfg.TranscriptionModel != "mimo-v2.5-asr" {
		t.Fatalf("TranscriptionModel = %q, want mimo-v2.5-asr", cfg.TranscriptionModel)
	}
	if cfg.TranscriptionAuthHeader != "api-key" {
		t.Fatalf("TranscriptionAuthHeader = %q, want api-key", cfg.TranscriptionAuthHeader)
	}
	if cfg.TranscriptionLanguage != "auto" {
		t.Fatalf("TranscriptionLanguage = %q, want auto", cfg.TranscriptionLanguage)
	}
	if cfg.TranscriptionMaxBase64Bytes != 10*1024*1024 {
		t.Fatalf("TranscriptionMaxBase64Bytes = %d, want 10MiB", cfg.TranscriptionMaxBase64Bytes)
	}
	if cfg.DashScopeBaseURL != "https://dashscope.aliyuncs.com/api/v1" {
		t.Fatalf("DashScopeBaseURL = %q, want DashScope default", cfg.DashScopeBaseURL)
	}
	if cfg.DashScopeAPIKey != "" {
		t.Fatal("DashScopeAPIKey default should be empty")
	}
	if cfg.DashScopeASRModel != "paraformer-v2" {
		t.Fatalf("DashScopeASRModel = %q, want paraformer-v2", cfg.DashScopeASRModel)
	}
	if cfg.LLMProvider != "fake_llm" {
		t.Fatalf("LLMProvider = %q, want fake_llm", cfg.LLMProvider)
	}
	if cfg.LLMBaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("LLMBaseURL = %q, want DashScope OpenAI-compatible default", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "qwen3.7-plus" {
		t.Fatalf("LLMModel = %q, want qwen3.7-plus", cfg.LLMModel)
	}
	if !cfg.PrivacyAllowExternalModelProviders {
		t.Fatal("PrivacyAllowExternalModelProviders = false, want true")
	}
	if cfg.PrivacyDeleteOriginalAudioAfterTranscription {
		t.Fatal("PrivacyDeleteOriginalAudioAfterTranscription = true, want false")
	}
}

func TestLoadFromEnvAppliesEnvironmentOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_PUBLIC_URL", "http://127.0.0.1:9090")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("AUTH_SESSION_TTL_HOURS", "168")
	t.Setenv("AUTH_COOKIE_SECURE", "true")
	t.Setenv("API_ADDRESS", ":9090")
	t.Setenv("POSTGRES_DSN", "postgres://custom_user:custom_password@db:5432/custom?sslmode=disable")
	t.Setenv("TEMPORAL_ADDRESS", "temporal:7233")
	t.Setenv("TEMPORAL_NAMESPACE", "soniq")
	t.Setenv("TEMPORAL_TASK_QUEUE", "custom-queue")
	t.Setenv("PURGE_ARTIFACT_CLEANUP_INTERVAL_SECONDS", "15")
	t.Setenv("PURGE_ARTIFACT_CLEANUP_BATCH_SIZE", "7")
	t.Setenv("STORAGE_PROVIDER", "local_fs")
	t.Setenv("LOCAL_STORAGE_PATH", "/tmp/soniq-uploads")
	t.Setenv("S3_ENDPOINT", "https://s3.example.test")
	t.Setenv("S3_REGION", "ap-southeast-1")
	t.Setenv("S3_BUCKET", "custom-bucket")
	t.Setenv("S3_ACCESS_KEY", "custom-access")
	t.Setenv("S3_SECRET_KEY", "custom-secret")
	t.Setenv("S3_FORCE_PATH_STYLE", "false")
	t.Setenv("TRANSCRIPTION_PROVIDER", "openai_compatible_asr")
	t.Setenv("TRANSCRIPTION_BASE_URL", "http://asr.example.test/v1")
	t.Setenv("TRANSCRIPTION_API_KEY", "transcription-secret")
	t.Setenv("TRANSCRIPTION_MODEL", "mimo-v2.5-asr")
	t.Setenv("TRANSCRIPTION_AUTH_HEADER", "bearer")
	t.Setenv("TRANSCRIPTION_LANGUAGE", "zh")
	t.Setenv("TRANSCRIPTION_MAX_BASE64_BYTES", "12345")
	t.Setenv("DASHSCOPE_BASE_URL", "http://dashscope.example.test/api/v1")
	t.Setenv("DASHSCOPE_API_KEY", "dashscope-secret")
	t.Setenv("DASHSCOPE_ASR_MODEL", "fun-asr")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("LLM_BASE_URL", "http://localhost:11434/v1")
	t.Setenv("LLM_API_KEY", "secret-key")
	t.Setenv("LLM_MODEL", "llama3")
	t.Setenv("PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS", "false")
	t.Setenv("PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION", "true")

	cfg := LoadFromEnv()

	if cfg.AppEnv != "test" {
		t.Fatalf("AppEnv = %q, want test", cfg.AppEnv)
	}
	if cfg.PublicURL != "http://127.0.0.1:9090" {
		t.Fatalf("PublicURL = %q, want override", cfg.PublicURL)
	}
	if cfg.LogFormat != "json" {
		t.Fatalf("LogFormat = %q, want json", cfg.LogFormat)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.AuthSessionTTLHours != 168 {
		t.Fatalf("AuthSessionTTLHours = %d, want 168", cfg.AuthSessionTTLHours)
	}
	if !cfg.AuthCookieSecure {
		t.Fatal("AuthCookieSecure = false, want true")
	}
	if cfg.APIAddress != ":9090" {
		t.Fatalf("APIAddress = %q, want :9090", cfg.APIAddress)
	}
	if cfg.PostgresDSN != "postgres://custom_user:custom_password@db:5432/custom?sslmode=disable" {
		t.Fatalf("PostgresDSN = %q, want override", cfg.PostgresDSN)
	}
	if cfg.TemporalAddress != "temporal:7233" {
		t.Fatalf("TemporalAddress = %q, want override", cfg.TemporalAddress)
	}
	if cfg.TemporalNamespace != "soniq" {
		t.Fatalf("TemporalNamespace = %q, want override", cfg.TemporalNamespace)
	}
	if cfg.TemporalTaskQueue != "custom-queue" {
		t.Fatalf("TemporalTaskQueue = %q, want override", cfg.TemporalTaskQueue)
	}
	if cfg.PurgeArtifactCleanupIntervalSeconds != 15 {
		t.Fatalf("PurgeArtifactCleanupIntervalSeconds = %d, want override", cfg.PurgeArtifactCleanupIntervalSeconds)
	}
	if cfg.PurgeArtifactCleanupBatchSize != 7 {
		t.Fatalf("PurgeArtifactCleanupBatchSize = %d, want override", cfg.PurgeArtifactCleanupBatchSize)
	}
	if cfg.StorageProvider != "local_fs" {
		t.Fatalf("StorageProvider = %q, want local_fs", cfg.StorageProvider)
	}
	if cfg.LocalStoragePath != "/tmp/soniq-uploads" {
		t.Fatalf("LocalStoragePath = %q, want override", cfg.LocalStoragePath)
	}
	if cfg.S3Endpoint != "https://s3.example.test" {
		t.Fatalf("S3Endpoint = %q, want override", cfg.S3Endpoint)
	}
	if cfg.S3Region != "ap-southeast-1" {
		t.Fatalf("S3Region = %q, want override", cfg.S3Region)
	}
	if cfg.S3Bucket != "custom-bucket" {
		t.Fatalf("S3Bucket = %q, want override", cfg.S3Bucket)
	}
	if cfg.S3AccessKey != "custom-access" {
		t.Fatalf("S3AccessKey = %q, want override", cfg.S3AccessKey)
	}
	if cfg.S3SecretKey != "custom-secret" {
		t.Fatal("S3SecretKey override was not loaded")
	}
	if cfg.S3ForcePathStyle {
		t.Fatal("S3ForcePathStyle = true, want false override")
	}
	if cfg.TranscriptionProvider != "openai_compatible_asr" {
		t.Fatalf("TranscriptionProvider = %q, want override", cfg.TranscriptionProvider)
	}
	if cfg.TranscriptionBaseURL != "http://asr.example.test/v1" {
		t.Fatalf("TranscriptionBaseURL = %q, want override", cfg.TranscriptionBaseURL)
	}
	if cfg.TranscriptionAPIKey != "transcription-secret" {
		t.Fatal("TranscriptionAPIKey override was not loaded")
	}
	if cfg.TranscriptionModel != "mimo-v2.5-asr" {
		t.Fatalf("TranscriptionModel = %q, want mimo-v2.5-asr", cfg.TranscriptionModel)
	}
	if cfg.TranscriptionAuthHeader != "bearer" {
		t.Fatalf("TranscriptionAuthHeader = %q, want bearer", cfg.TranscriptionAuthHeader)
	}
	if cfg.TranscriptionLanguage != "zh" {
		t.Fatalf("TranscriptionLanguage = %q, want zh", cfg.TranscriptionLanguage)
	}
	if cfg.TranscriptionMaxBase64Bytes != 12345 {
		t.Fatalf("TranscriptionMaxBase64Bytes = %d, want 12345", cfg.TranscriptionMaxBase64Bytes)
	}
	if cfg.DashScopeBaseURL != "http://dashscope.example.test/api/v1" {
		t.Fatalf("DashScopeBaseURL = %q, want override", cfg.DashScopeBaseURL)
	}
	if cfg.DashScopeAPIKey != "dashscope-secret" {
		t.Fatal("DashScopeAPIKey override was not loaded")
	}
	if cfg.DashScopeASRModel != "fun-asr" {
		t.Fatalf("DashScopeASRModel = %q, want fun-asr", cfg.DashScopeASRModel)
	}
	if cfg.LLMProvider != "ollama" {
		t.Fatalf("LLMProvider = %q, want ollama", cfg.LLMProvider)
	}
	if cfg.LLMBaseURL != "http://localhost:11434/v1" {
		t.Fatalf("LLMBaseURL = %q, want override", cfg.LLMBaseURL)
	}
	if cfg.LLMAPIKey != "secret-key" {
		t.Fatal("LLMAPIKey override was not loaded")
	}
	if cfg.LLMModel != "llama3" {
		t.Fatalf("LLMModel = %q, want llama3", cfg.LLMModel)
	}
	if cfg.PrivacyAllowExternalModelProviders {
		t.Fatal("PrivacyAllowExternalModelProviders = true, want false")
	}
	if !cfg.PrivacyDeleteOriginalAudioAfterTranscription {
		t.Fatal("PrivacyDeleteOriginalAudioAfterTranscription = false, want true")
	}
}

func TestLoadFromEnvUsesDashScopeAPIKeyAsLLMFallback(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("DASHSCOPE_API_KEY", "dashscope-secret")

	cfg := LoadFromEnv()

	if cfg.LLMAPIKey != "dashscope-secret" {
		t.Fatal("LLMAPIKey did not fall back to DASHSCOPE_API_KEY")
	}
}

func TestValidateForStartupRejectsRequiredEmptyValues(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.TemporalTaskQueue = ""

	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want error for empty TemporalTaskQueue")
	}
}

func TestValidateForStartupRejectsEmptyPostgresDSN(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.PostgresDSN = ""

	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want error for empty PostgresDSN")
	}
}

func TestValidateForStartupRejectsEmptyLocalStoragePath(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.LocalStoragePath = ""

	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want error for empty LocalStoragePath")
	}
}

func TestValidateForStartupAllowsS3CompatibleStorageWithoutLocalStoragePath(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.StorageProvider = "s3_compatible"
	cfg.LocalStoragePath = ""
	cfg.S3Endpoint = "http://localhost:9000"
	cfg.S3Region = "us-east-1"
	cfg.S3Bucket = "soniq"
	cfg.S3AccessKey = "soniq_minio_user"
	cfg.S3SecretKey = "soniq_minio_password"

	if err := cfg.ValidateForStartup(); err != nil {
		t.Fatalf("ValidateForStartup() error = %v, want nil", err)
	}
}

func TestValidateForStartupRejectsIncompleteS3CompatibleStorage(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.StorageProvider = "s3_compatible"
	cfg.S3Bucket = ""

	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want missing S3_BUCKET error")
	}
}

func TestValidateForStartupRejectsUnsupportedStorageProvider(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.StorageProvider = "local_fs"

	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want unsupported storage provider error")
	}
}

func TestValidateForStartupRejectsInvalidPurgeArtifactCleanupConfig(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.PurgeArtifactCleanupIntervalSeconds = 0
	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want invalid cleanup interval error")
	}

	cfg = LoadFromEnv()
	cfg.PurgeArtifactCleanupBatchSize = 0
	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want invalid cleanup batch size error")
	}
}

func TestValidateForStartupRejectsInvalidLogConfig(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.LogFormat = "yaml"
	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want invalid log format error")
	}

	cfg = LoadFromEnv()
	cfg.LogLevel = "verbose"
	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want invalid log level error")
	}
}

func TestValidateForStartupRejectsInvalidAuthConfig(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.AuthSessionTTLHours = 0
	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want missing session ttl error")
	}
}

func TestValidateForStartupAllowsMissingLLMAPIKeyForFakeProvider(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.LLMProvider = "fake_llm"
	cfg.LLMAPIKey = ""

	if err := cfg.ValidateForStartup(); err != nil {
		t.Fatalf("ValidateForStartup() error = %v, want nil", err)
	}
	if cfg.NeedsLLMAPIKeyForExternalProvider() {
		t.Fatal("NeedsLLMAPIKeyForExternalProvider() = true for fake provider, want false")
	}
}

func TestLoadFromEnvFallsBackToMIMOAPIKeyForTranscription(t *testing.T) {
	t.Setenv("TRANSCRIPTION_API_KEY", "")
	t.Setenv("MIMO_API_KEY", "mimo-secret")

	cfg := LoadFromEnv()

	if cfg.TranscriptionAPIKey != "mimo-secret" {
		t.Fatal("TranscriptionAPIKey did not fall back to MIMO_API_KEY")
	}
}

func TestNeedsTranscriptionAPIKeyForExternalProvider(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.TranscriptionProvider = "openai_compatible_asr"
	cfg.TranscriptionAPIKey = ""
	cfg.PrivacyAllowExternalModelProviders = true

	if !cfg.NeedsTranscriptionAPIKeyForExternalProvider() {
		t.Fatal("NeedsTranscriptionAPIKeyForExternalProvider() = false, want true")
	}

	cfg.TranscriptionAPIKey = "secret"
	if cfg.NeedsTranscriptionAPIKeyForExternalProvider() {
		t.Fatal("NeedsTranscriptionAPIKeyForExternalProvider() = true with key set, want false")
	}

	cfg.TranscriptionAPIKey = ""
	cfg.TranscriptionProvider = "fake_transcription"
	if cfg.NeedsTranscriptionAPIKeyForExternalProvider() {
		t.Fatal("NeedsTranscriptionAPIKeyForExternalProvider() = true for fake provider, want false")
	}

	cfg.TranscriptionProvider = "openai_compatible_asr"
	cfg.PrivacyAllowExternalModelProviders = false
	if cfg.NeedsTranscriptionAPIKeyForExternalProvider() {
		t.Fatal("NeedsTranscriptionAPIKeyForExternalProvider() = true in private mode, want false")
	}
}

func TestValidateForStartupRejectsMissingTranscriptionExternalConfig(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.TranscriptionProvider = "openai_compatible_asr"
	cfg.TranscriptionBaseURL = ""
	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want missing transcription base URL error")
	}

	cfg = LoadFromEnv()
	cfg.TranscriptionProvider = "openai_compatible_asr"
	cfg.TranscriptionAPIKey = ""
	cfg.PrivacyAllowExternalModelProviders = true
	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want missing transcription API key error")
	}

	cfg = LoadFromEnv()
	cfg.TranscriptionProvider = "openai_compatible_asr"
	cfg.TranscriptionAPIKey = "secret"
	cfg.TranscriptionModel = ""
	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want missing transcription model error")
	}
}

func TestValidateForStartupRejectsMissingDashScopeConfig(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.TranscriptionProvider = "dashscope_asr"
	cfg.DashScopeAPIKey = ""
	cfg.PrivacyAllowExternalModelProviders = true
	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want missing DashScope API key error")
	}

	cfg = LoadFromEnv()
	cfg.TranscriptionProvider = "dashscope_asr"
	cfg.DashScopeAPIKey = "secret"
	cfg.DashScopeBaseURL = ""
	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want missing DashScope base URL error")
	}

	cfg = LoadFromEnv()
	cfg.TranscriptionProvider = "dashscope_asr"
	cfg.DashScopeAPIKey = "secret"
	cfg.DashScopeASRModel = ""
	if err := cfg.ValidateForStartup(); err == nil {
		t.Fatal("ValidateForStartup() error = nil, want missing DashScope model error")
	}
}
