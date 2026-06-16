# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.4
ARG DEBIAN_VERSION=bookworm

FROM golang:${GO_VERSION}-${DEBIAN_VERSION} AS backend-build

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend ./

ARG APP_VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

RUN set -eux; \
	ldflags="-s -w \
		-X github.com/zzyhdu/soniq/backend/internal/version.Version=${APP_VERSION} \
		-X github.com/zzyhdu/soniq/backend/internal/version.Commit=${VCS_REF} \
		-X github.com/zzyhdu/soniq/backend/internal/version.BuildDate=${BUILD_DATE}"; \
	CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="$ldflags" -o /out/soniq-api ./cmd/api; \
	CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="$ldflags" -o /out/soniq-worker ./cmd/worker; \
	CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="$ldflags" -o /out/soniq-migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot AS api

ARG APP_VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="Soniq API" \
	org.opencontainers.image.description="Soniq HTTP API server" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${VCS_REF}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.source="https://github.com/zzyhdu/soniq"

ENV APP_VERSION="${APP_VERSION}" \
	LOG_FORMAT=json \
	LOCAL_STORAGE_PATH=/tmp/soniq/uploads

WORKDIR /tmp
COPY --from=backend-build /out/soniq-api /soniq-api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/soniq-api"]

FROM debian:bookworm-slim AS worker

RUN set -eux; \
	apt-get update; \
	apt-get install -y --no-install-recommends ca-certificates ffmpeg; \
	rm -rf /var/lib/apt/lists/*; \
	groupadd --gid 10001 soniq; \
	useradd --uid 10001 --gid 10001 --create-home --home-dir /home/soniq --shell /usr/sbin/nologin soniq

ARG APP_VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="Soniq Worker" \
	org.opencontainers.image.description="Soniq Temporal worker" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${VCS_REF}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.source="https://github.com/zzyhdu/soniq"

ENV APP_VERSION="${APP_VERSION}" \
	LOG_FORMAT=json \
	LOCAL_STORAGE_PATH=/tmp/soniq/uploads

WORKDIR /home/soniq
COPY --from=backend-build /out/soniq-worker /usr/local/bin/soniq-worker
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/soniq-worker"]

FROM gcr.io/distroless/static-debian12:nonroot AS migrate

ARG APP_VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="Soniq Migrate" \
	org.opencontainers.image.description="Soniq application database migration command" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${VCS_REF}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.source="https://github.com/zzyhdu/soniq"

ENV APP_VERSION="${APP_VERSION}" \
	LOG_FORMAT=json

WORKDIR /tmp
COPY --from=backend-build /out/soniq-migrate /soniq-migrate
USER nonroot:nonroot
ENTRYPOINT ["/soniq-migrate"]
