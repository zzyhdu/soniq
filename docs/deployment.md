# Deployment Contract

This document defines the runtime contract for deploying Soniq outside local
development. It describes Soniq-owned processes and external dependencies. It
does not prescribe a specific Kubernetes, Helm, Terraform, or cloud vendor
implementation.

## Managed Processes

Soniq currently deploys three backend process types:

- `soniq-api`: HTTP API server. It serves `/healthz`, `/readyz`, auth,
  workspace, recording, upload, retry, details, Trash, restore, and purge APIs.
- `soniq-worker`: Temporal worker. It polls `TEMPORAL_TASK_QUEUE`, runs audio
  probe/normalize/transcription/summary/mind-map activities, and retries purge
  artifact cleanup rows.
- `soniq-migrate`: one-shot migration command for the Soniq application
  Postgres database.

The API and worker are stateless. Uploaded and generated audio artifacts must
live in S3-compatible object storage, not pod-local or node-local disk.

## External Dependencies

Production deployments must provide these dependencies before starting API and
worker pods:

- Soniq application Postgres. This is separate from Temporal-owned Postgres.
- Temporal frontend reachable by API and worker.
- S3-compatible object storage reachable by API, worker, and any real external
  ASR provider that needs to read presigned audio URLs.
- Optional external ASR and LLM providers.

Soniq migrations only apply to the Soniq application Postgres database. They
must not be pointed at Temporal's database.

## Migration Strategy

Run `soniq-migrate` as a separate deployment step before API and worker rollout.
Only one migration runner should execute for a given release.

Recommended order:

1. Build and publish `soniq-migrate`, `soniq-api`, and `soniq-worker` images.
2. Apply runtime config and secrets.
3. Run the `soniq-migrate` job against Soniq application Postgres.
4. Start or roll out `soniq-api`.
5. Start or roll out `soniq-worker`.
6. Check `/readyz` before sending traffic to the API.

Do not run migrations from every API or worker pod startup.

## Configuration

Non-sensitive values belong in ConfigMap-style configuration:

| Key | Required | Notes |
| --- | --- | --- |
| `APP_ENV` | no | Use `production` for deployed environments. |
| `APP_PUBLIC_URL` | no | Public API/web origin used by future callbacks and links. |
| `LOG_FORMAT` | yes | Use `json` for log collection. |
| `LOG_LEVEL` | yes | `debug`, `info`, `warn`, or `error`. |
| `AUTH_SESSION_TTL_HOURS` | yes | Opaque session cookie lifetime. |
| `AUTH_COOKIE_SECURE` | yes | Use `true` behind HTTPS. |
| `API_ADDRESS` | yes for API | Usually `:8080` in containers. |
| `TEMPORAL_ADDRESS` | yes | Temporal frontend host and port. |
| `TEMPORAL_NAMESPACE` | yes | Temporal namespace used by API and worker. |
| `TEMPORAL_TASK_QUEUE` | yes | Must match between API and worker. |
| `PURGE_ARTIFACT_CLEANUP_INTERVAL_SECONDS` | yes for worker | Background purge cleanup interval. |
| `PURGE_ARTIFACT_CLEANUP_BATCH_SIZE` | yes for worker | Cleanup batch size. |
| `STORAGE_PROVIDER` | yes | Only `s3_compatible` is supported. |
| `S3_ENDPOINT` | yes | S3-compatible endpoint. Must be reachable by API and worker. |
| `S3_REGION` | yes | S3-compatible region value. |
| `S3_BUCKET` | yes | Bucket for uploaded and generated artifacts. |
| `S3_FORCE_PATH_STYLE` | yes | `true` for MinIO, usually `false` for cloud object storage. |
| `TRANSCRIPTION_PROVIDER` | yes | `fake_transcription`, `openai_compatible_asr`, or `dashscope_asr`. |
| `TRANSCRIPTION_BASE_URL` | provider-specific | OpenAI-compatible ASR base URL. |
| `TRANSCRIPTION_MODEL` | provider-specific | ASR model name. |
| `TRANSCRIPTION_AUTH_HEADER` | provider-specific | `api-key` or `bearer`. |
| `TRANSCRIPTION_LANGUAGE` | no | Language hint, such as `auto`, `zh`, or `en`. |
| `DASHSCOPE_BASE_URL` | provider-specific | DashScope native ASR endpoint. |
| `DASHSCOPE_ASR_MODEL` | provider-specific | DashScope ASR model. |
| `LLM_PROVIDER` | yes | `fake_llm`, `openai_compatible`, or supported local provider. |
| `LLM_BASE_URL` | provider-specific | OpenAI-compatible LLM base URL. |
| `LLM_MODEL` | provider-specific | LLM model name. |
| `PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS` | yes | Set `false` to reject external model providers. |
| `PRIVACY_DELETE_ORIGINAL_AUDIO_AFTER_TRANSCRIPTION` | yes | Deletes original audio after transcription succeeds. |

Sensitive values belong in Secret-style configuration:

| Key | Required | Notes |
| --- | --- | --- |
| `POSTGRES_DSN` | yes | Soniq application Postgres DSN. |
| `S3_ACCESS_KEY` | yes | S3-compatible access key. |
| `S3_SECRET_KEY` | yes | S3-compatible secret key. |
| `TRANSCRIPTION_API_KEY` | provider-specific | OpenAI-compatible ASR key. |
| `MIMO_API_KEY` | optional | Alias fallback for Xiaomi MiMo ASR. |
| `DASHSCOPE_API_KEY` | provider-specific | DashScope native ASR key. |
| `LLM_API_KEY` | provider-specific | External LLM key. |

Current auth uses opaque session tokens stored in Postgres and CSRF tokens
bound to those session tokens. There is no separate session signing secret yet.

## Storage Requirements

Object storage must support S3-compatible `PutObject`, `GetObject`,
`DeleteObject`, `HeadBucket`, and presigned GET URLs.

Real external ASR providers receive presigned normalized-audio URLs. For those
runs, `S3_ENDPOINT` must produce URLs reachable by that provider. A local
cluster-only MinIO endpoint is suitable for fake-provider verification, but not
for external ASR calls.

## Backup Boundaries

Back up these data stores:

- Soniq application Postgres, including users, workspaces, recordings,
  transcripts, summaries, mind maps, sessions, and purge cleanup rows.
- S3-compatible bucket objects, including original audio when retained and
  normalized audio artifacts.

Temporal history and visibility storage are owned by the Temporal deployment.
Back up Temporal according to the Temporal deployment strategy, not Soniq
application migrations.

## Readiness

`GET /healthz` means the API process is alive.

`GET /readyz` means the API can reach required dependencies:

- Soniq application Postgres.
- Current Soniq migration version.
- Temporal frontend.
- S3-compatible bucket.

Kubernetes or any load balancer should use `/readyz` for API readiness and
`/healthz` for API liveness.

## Shutdown Behavior

`soniq-api` handles `SIGINT` and `SIGTERM` gracefully. On shutdown it stops
accepting new HTTP connections, waits up to 25 seconds for in-flight requests
to complete, then closes runtime dependencies.

`soniq-worker` handles the same signals through a shared process context. On
shutdown it asks the Temporal worker to stop polling for new tasks, waits up to
25 seconds for the Temporal SDK worker to drain, and cancels the purge artifact
cleanup loop.

Kubernetes pods should allow at least 30 seconds of termination grace for API
and worker pods so the process-level 25 second drain can finish before the
container is force-killed.

## Disruption Budgets

API and worker deployments should run at least two replicas in production-like
environments. Soniq's Kubernetes manifests set API replicas to 2 and worker
replicas to 2 by default, with PodDisruptionBudgets requiring at least one API
pod and one worker pod to remain available during voluntary disruptions such as
node drains and cluster upgrades.

## Network Boundaries

Kubernetes NetworkPolicy should be enabled in clusters whose CNI plugin enforces
it. Soniq's baseline policies select API, worker, and migration pods:

- API allows inbound TCP 8080 for HTTP traffic.
- Worker and migration pods deny inbound traffic.
- API, worker, and migration pods allow outbound DNS plus TCP 80, 443, 9000,
  5432, and 7233 for object storage, Postgres, Temporal, and external model
  providers.

Production environments with dependencies on different ports or stricter IP
boundaries should override the Helm `networkPolicy` values with cluster-specific
egress rules.
