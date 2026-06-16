ENV_FILE ?= .env
COMPOSE_FILE ?= compose.temporal.yml
POSTGRES_USER ?= soniq_user
POSTGRES_DB ?= soniq
POSTGRES_DSN ?= postgres://soniq_user:soniq_password@localhost:5432/soniq?sslmode=disable
DOCKER ?= docker
APP_VERSION ?= dev
DEFAULT_VCS_REF := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DEFAULT_BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VCS_REF ?= $(DEFAULT_VCS_REF)
BUILD_DATE ?= $(DEFAULT_BUILD_DATE)
SONIQ_API_IMAGE ?= soniq-api:$(APP_VERSION)
SONIQ_WORKER_IMAGE ?= soniq-worker:$(APP_VERSION)
SONIQ_MIGRATE_IMAGE ?= soniq-migrate:$(APP_VERSION)
CONFIG_ENV_KEYS := \
	APP_ENV \
	APP_PUBLIC_URL \
	COMPOSE_FILE \
	LOG_FORMAT \
	LOG_LEVEL \
	AUTH_SESSION_TTL_HOURS \
	AUTH_COOKIE_SECURE \
	API_ADDRESS \
	POSTGRES_DSN \
	POSTGRES_USER \
	POSTGRES_DB \
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

.PHONY: fmt lint test api worker env-check migrate debug-purge-artifacts temporal-up temporal-down temporal-logs temporal-ps smoke-postgres-temporal docker-build docker-build-api docker-build-worker docker-build-migrate

$(foreach key,$(CONFIG_ENV_KEYS),$(eval api worker env-check migrate debug-purge-artifacts smoke-postgres-temporal: export $(key) := $($(key))))

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
	@echo "compose_file=$(COMPOSE_FILE)"
	@echo "log_format=$(LOG_FORMAT)"
	@echo "log_level=$(LOG_LEVEL)"
	@echo "auth_session_ttl_hours=$(AUTH_SESSION_TTL_HOURS)"
	@echo "auth_cookie_secure=$(AUTH_COOKIE_SECURE)"
	@echo "postgres_user=$(POSTGRES_USER)"
	@echo "postgres_db=$(POSTGRES_DB)"
	@echo "transcription_provider=$(TRANSCRIPTION_PROVIDER)"
	@echo "dashscope_asr_model=$(DASHSCOPE_ASR_MODEL)"
	@echo "llm_provider=$(LLM_PROVIDER)"
	@echo "privacy_allow_external_model_providers=$(PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS)"

migrate:
	cd backend && go run ./cmd/migrate

docker-build: docker-build-api docker-build-worker docker-build-migrate

docker-build-api:
	$(DOCKER) build --target api \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg VCS_REF=$(VCS_REF) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(SONIQ_API_IMAGE) .

docker-build-worker:
	$(DOCKER) build --target worker \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg VCS_REF=$(VCS_REF) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(SONIQ_WORKER_IMAGE) .

docker-build-migrate:
	$(DOCKER) build --target migrate \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg VCS_REF=$(VCS_REF) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(SONIQ_MIGRATE_IMAGE) .

debug-purge-artifacts: export ENV_FILE := $(ENV_FILE)
debug-purge-artifacts: export LIMIT := $(LIMIT)
debug-purge-artifacts: export STUCK_AFTER_MINUTES := $(STUCK_AFTER_MINUTES)
debug-purge-artifacts:
	./scripts/debug-purge-artifacts.sh

temporal-up:
	docker compose -f $(COMPOSE_FILE) up -d

temporal-down:
	docker compose -f $(COMPOSE_FILE) down

temporal-logs:
	docker compose -f $(COMPOSE_FILE) logs -f temporal temporal-ui

temporal-ps:
	docker compose -f $(COMPOSE_FILE) ps

smoke-postgres-temporal:
	./scripts/smoke-postgres-temporal.sh
