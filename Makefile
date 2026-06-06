.PHONY: fmt lint test api worker temporal-up temporal-down temporal-logs temporal-ps smoke-postgres-temporal

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
