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
