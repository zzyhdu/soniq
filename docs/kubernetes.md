# Kubernetes Deployment

This document describes the first Kubernetes deployment foundation for Soniq.
It intentionally uses raw manifests under `deploy/kubernetes/base/` instead of
Helm so the deployment contract stays easy to inspect.

## Scope

The base manifests deploy only Soniq-owned resources:

- `soniq-migrate` Job.
- `soniq-api` Deployment and Service.
- `soniq-worker` Deployment.
- `soniq-config` ConfigMap.
- `soniq-secret` example Secret.
- `soniq` Namespace.

They do not deploy Postgres, Temporal, MinIO, Ingress, TLS certificates,
Prometheus, Grafana, or external secret controllers.

## Prerequisites

Prepare these before applying the manifests to a real cluster:

- A Soniq application Postgres database.
- A Temporal frontend address and namespace.
- An S3-compatible bucket and credentials.
- Built and pushed images for:
  - `soniq-api`
  - `soniq-worker`
  - `soniq-migrate`

Build local image tags from the repository root:

```bash
make docker-build
```

For a remote cluster, push the images to a registry and update the `image`
fields in:

- `deploy/kubernetes/base/api-deployment.yaml`
- `deploy/kubernetes/base/worker-deployment.yaml`
- `deploy/kubernetes/base/migrate-job.yaml`

## Configure

Edit `deploy/kubernetes/base/configmap.yaml` for non-secret settings:

- `APP_PUBLIC_URL`
- `TEMPORAL_ADDRESS`
- `TEMPORAL_NAMESPACE`
- `TEMPORAL_TASK_QUEUE`
- `S3_ENDPOINT`
- `S3_REGION`
- `S3_BUCKET`
- `S3_FORCE_PATH_STYLE`
- provider selection and model names

Create a real Secret from `deploy/kubernetes/base/secret.example.yaml` and
replace every `change-me` value before real deployment. The example file is
safe to commit because it contains placeholders only.

Required Secret keys:

- `POSTGRES_DSN`
- `S3_ACCESS_KEY`
- `S3_SECRET_KEY`

Provider-specific Secret keys:

- `TRANSCRIPTION_API_KEY`
- `MIMO_API_KEY`
- `DASHSCOPE_API_KEY`
- `LLM_API_KEY`

## Validate Manifests

Render the base manifests locally:

```bash
make k8s-render
```

This target runs `kubectl kustomize deploy/kubernetes/base`. When local
`kubectl` is unavailable, the Makefile falls back to a Dockerized kubectl
image.

Run the local manifest smoke:

```bash
make k8s-smoke
```

This renders the base manifests and checks the expected resource contract. This
smoke requires `python3` with PyYAML and does not require a Kubernetes API
server.

Run server-side dry-run when a cluster is available:

```bash
make k8s-dry-run
```

Server-side dry-run asks the Kubernetes API server to validate resource kinds
and schemas without persisting the application objects. If the `soniq`
namespace does not exist, `make k8s-smoke` with `K8S_SMOKE_SERVER_DRY_RUN=1`
creates it temporarily for validation and deletes it afterward.

Run the local kind deployment smoke:

```bash
make k8s-smoke-kind
```

This starts the Compose-managed Postgres, Temporal, and MinIO dependencies,
creates or reuses the `soniq` kind cluster, connects the kind control-plane
container to the Compose Docker network, and applies a temporary smoke manifest.
The smoke manifest keeps Soniq API, worker, and migrate resources in Kubernetes,
but exposes the Compose dependencies through Kubernetes `Service` and
`EndpointSlice` objects:

- `soniq-postgresql:5432`
- `temporal:7233`
- `minio:9000`

This avoids using `localhost` from inside pods. In a pod, `localhost` means the
pod itself, not the developer machine or the Compose containers.

By default the kind smoke also signs up or signs in a smoke user, uploads a
generated WAV file through the port-forwarded API, waits for the Temporal
workflow to complete, verifies the persisted transcript, summary, and mind map
rows, then verifies the recording lifecycle path: soft delete, Trash restore,
soft delete again, permanent purge, purge artifact cleanup rows, and S3 object
deletion. Set `KIND_SMOKE_WORKFLOW=0` to run only the deployment readiness
portion.

## Apply

Apply config and secret first:

```bash
kubectl apply -f deploy/kubernetes/base/namespace.yaml
kubectl apply -f deploy/kubernetes/base/configmap.yaml
kubectl apply -f deploy/kubernetes/base/secret.example.yaml
```

Run migrations:

```bash
kubectl apply -f deploy/kubernetes/base/migrate-job.yaml
kubectl -n soniq wait --for=condition=complete job/soniq-migrate --timeout=120s
kubectl -n soniq logs job/soniq-migrate
```

Then start API and worker:

```bash
kubectl apply -f deploy/kubernetes/base/api-deployment.yaml
kubectl apply -f deploy/kubernetes/base/api-service.yaml
kubectl apply -f deploy/kubernetes/base/worker-deployment.yaml
```

Wait for API readiness:

```bash
kubectl -n soniq rollout status deployment/soniq-api
kubectl -n soniq get pods
```

For local inspection without Ingress:

```bash
kubectl -n soniq port-forward service/soniq-api 8080:80
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

If `AUTH_COOKIE_SECURE=true`, browser login requires HTTPS. For local
port-forward-only testing over HTTP, temporarily set `AUTH_COOKIE_SECURE=false`
in `soniq-config`.

## Update Migrations

Kubernetes Jobs are not updated in place after their pod template changes. For
a new migration run in this raw-manifest phase, delete and recreate the Job:

```bash
kubectl -n soniq delete job soniq-migrate
kubectl apply -f deploy/kubernetes/base/migrate-job.yaml
```

Helm pre-install/pre-upgrade hooks can handle this later, but this base layer
keeps the behavior explicit.

## Operational Checks

API readiness:

```bash
kubectl -n soniq exec deploy/soniq-api -- /soniq-api --version
kubectl -n soniq logs deploy/soniq-api
```

Worker logs:

```bash
kubectl -n soniq logs deploy/soniq-worker
```

Migration logs:

```bash
kubectl -n soniq logs job/soniq-migrate
```

## Current Limitations

- No Helm chart yet.
- No Ingress or TLS manifest.
- No HPA, PDB, NetworkPolicy, or topology spread constraints.
- No Helm-based cluster smoke script yet; the current kind smoke validates the
  raw-manifest deployment path.
- Worker exposes no HTTP health endpoint, so worker health is currently based
  on process status and logs.
