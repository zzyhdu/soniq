package config

import "testing"

func TestLoadFromEnvUsesDevelopmentDefaults(t *testing.T) {
	cfg := LoadFromEnv()

	if cfg.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", cfg.AppEnv)
	}
	if cfg.PublicURL != "http://localhost:8080" {
		t.Fatalf("PublicURL = %q, want http://localhost:8080", cfg.PublicURL)
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
	if cfg.StorageProvider != "local" {
		t.Fatalf("StorageProvider = %q, want local", cfg.StorageProvider)
	}
	if cfg.LocalStoragePath != "var/uploads" {
		t.Fatalf("LocalStoragePath = %q, want var/uploads", cfg.LocalStoragePath)
	}
	if cfg.TranscriptionProvider != "faster_whisper" {
		t.Fatalf("TranscriptionProvider = %q, want faster_whisper", cfg.TranscriptionProvider)
	}
	if cfg.LLMProvider != "openai_compatible" {
		t.Fatalf("LLMProvider = %q, want openai_compatible", cfg.LLMProvider)
	}
	if cfg.LLMBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("LLMBaseURL = %q, want https://api.openai.com/v1", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "gpt-4o-mini" {
		t.Fatalf("LLMModel = %q, want gpt-4o-mini", cfg.LLMModel)
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
	t.Setenv("API_ADDRESS", ":9090")
	t.Setenv("POSTGRES_DSN", "postgres://custom_user:custom_password@db:5432/custom?sslmode=disable")
	t.Setenv("TEMPORAL_ADDRESS", "temporal:7233")
	t.Setenv("TEMPORAL_NAMESPACE", "soniq")
	t.Setenv("TEMPORAL_TASK_QUEUE", "custom-queue")
	t.Setenv("STORAGE_PROVIDER", "local_fs")
	t.Setenv("LOCAL_STORAGE_PATH", "/tmp/soniq-uploads")
	t.Setenv("TRANSCRIPTION_PROVIDER", "openai_compatible_transcription")
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
	if cfg.StorageProvider != "local_fs" {
		t.Fatalf("StorageProvider = %q, want local_fs", cfg.StorageProvider)
	}
	if cfg.LocalStoragePath != "/tmp/soniq-uploads" {
		t.Fatalf("LocalStoragePath = %q, want override", cfg.LocalStoragePath)
	}
	if cfg.TranscriptionProvider != "openai_compatible_transcription" {
		t.Fatalf("TranscriptionProvider = %q, want override", cfg.TranscriptionProvider)
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

func TestValidateForStartupAllowsMissingLLMAPIKeyInDevelopment(t *testing.T) {
	cfg := LoadFromEnv()
	cfg.AppEnv = "development"
	cfg.LLMAPIKey = ""

	if err := cfg.ValidateForStartup(); err != nil {
		t.Fatalf("ValidateForStartup() error = %v, want nil", err)
	}
	if !cfg.NeedsLLMAPIKeyForExternalProvider() {
		t.Fatal("NeedsLLMAPIKeyForExternalProvider() = false, want true")
	}
}
