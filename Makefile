ENV_FILE ?= .env
CONFIG_ENV_KEYS := \
	APP_ENV \
	APP_PUBLIC_URL \
	API_ADDRESS \
	POSTGRES_DSN \
	TEMPORAL_ADDRESS \
	TEMPORAL_NAMESPACE \
	TEMPORAL_TASK_QUEUE \
	STORAGE_PROVIDER \
	LOCAL_STORAGE_PATH \
	TRANSCRIPTION_PROVIDER \
	TRANSCRIPTION_BASE_URL \
	TRANSCRIPTION_API_KEY \
	MIMO_API_KEY \
	TRANSCRIPTION_MODEL \
	TRANSCRIPTION_AUTH_HEADER \
	TRANSCRIPTION_LANGUAGE \
	TRANSCRIPTION_MAX_BASE64_BYTES \
	DASHSCOPE_BASE_URL \
	DASHSCOPE_API_KEY \
	DASHSCOPE_ASR_MODEL \
	LLM_PROVIDER \
	LLM_BASE_URL \
	LLM_API_KEY \
	LLM_MODEL \
	PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION \
	PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS

$(foreach key,$(CONFIG_ENV_KEYS),$(eval __ENV_ORIGIN_$(key) := $(origin $(key)))$(eval __ENV_VALUE_$(key) := $(value $(key))))

ifneq (,$(wildcard $(ENV_FILE)))
include $(ENV_FILE)
endif

$(foreach key,$(CONFIG_ENV_KEYS),$(if $(filter environment command line,$(__ENV_ORIGIN_$(key))),$(eval override $(key) := $(__ENV_VALUE_$(key)))))

.PHONY: fmt lint test api worker env-check temporal-up temporal-down temporal-logs temporal-ps smoke-postgres-temporal

$(foreach key,$(CONFIG_ENV_KEYS),$(eval api worker env-check smoke-postgres-temporal: export $(key) := $($(key))))

fmt:
	cd backend && go fmt ./...

lint:
	cd backend && go vet ./...

test:
	cd backend && go test ./...

api:
	cd backend && go run ./cmd/api

worker:
	cd backend && go run ./cmd/worker

env-check:
	@echo "env_file=$(if $(wildcard $(ENV_FILE)),$(ENV_FILE),not found)"
	@echo "api_address=$(API_ADDRESS)"
	@echo "transcription_provider=$(TRANSCRIPTION_PROVIDER)"
	@echo "dashscope_asr_model=$(DASHSCOPE_ASR_MODEL)"
	@echo "llm_provider=$(LLM_PROVIDER)"
	@echo "privacy_allow_external_model_providers=$(PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS)"

temporal-up:
	docker compose -f compose.temporal.yml up -d

temporal-down:
	docker compose -f compose.temporal.yml down

temporal-logs:
	docker compose -f compose.temporal.yml logs -f temporal temporal-ui

temporal-ps:
	docker compose -f compose.temporal.yml ps

smoke-postgres-temporal:
	./scripts/smoke-postgres-temporal.sh
