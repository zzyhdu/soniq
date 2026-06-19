# Kubernetes Deployment

This document describes the first Kubernetes deployment foundation for Soniq.
It includes raw manifests under `deploy/kubernetes/base/` for direct inspection
and a first Helm chart under `deploy/helm/soniq/` for parameterized deployment.

## Scope

The raw manifests and Helm chart deploy only Soniq-owned resources:

- `soniq-migrate` Job.
- `soniq-api` Deployment and Service.
- `soniq-worker` Deployment.
- `soniq-api` and `soniq-worker` PodDisruptionBudgets.
- `soniq-api` HorizontalPodAutoscaler in raw manifests.
- `soniq-api`, `soniq-worker`, and `soniq-migrate` NetworkPolicies.
- `soniq-config` ConfigMap.
- `soniq-secret` reference, with an example Secret in the raw manifests and
  optional Secret rendering in Helm.
- `soniq` Namespace in raw manifests, with optional Namespace rendering in
  Helm.

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

Helm chart work uses the same local-or-Docker tooling pattern. Check the Helm
CLI with:

```bash
make helm-version
```

When local `helm` is unavailable, the Makefile falls back to a Dockerized Helm
image.

Validate the Helm chart:

```bash
make helm-lint
make helm-template
```

The default chart renders ServiceAccount, ConfigMap, API Service, API
Deployment, worker Deployment, API/worker PodDisruptionBudgets, API/worker/
migration NetworkPolicies, and a pre-install/pre-upgrade migration Job hook. It
references `soniq-secret` but does not render a Secret by default, so real
secret material stays outside committed values. It also follows the Helm
release namespace by default instead of creating a Namespace resource.

For local rendering of the optional Namespace and placeholder Secret path:

```bash
HELM_TEMPLATE_ARGS='--set namespace.create=true --set secret.create=true --set migrate.hook.enabled=false --set-string secret.stringData.POSTGRES_DSN=postgres://soniq_user:change-me@postgres.example.com:5432/soniq?sslmode=require --set-string secret.stringData.S3_ACCESS_KEY=change-me --set-string secret.stringData.S3_SECRET_KEY=change-me' \
  make helm-template
```

For a real Helm install, create the namespace and Secret first, then install
into that namespace:

```bash
kubectl create namespace soniq --dry-run=client -o yaml | kubectl apply -f -
kubectl -n soniq create secret generic soniq-secret \
  --from-literal=POSTGRES_DSN='postgres://soniq_user:change-me@postgres.example.com:5432/soniq?sslmode=require' \
  --from-literal=S3_ACCESS_KEY='change-me' \
  --from-literal=S3_SECRET_KEY='change-me' \
  --dry-run=client -o yaml | kubectl apply -f -
```

Then apply the chart with any private config overrides:

```bash
helm upgrade --install soniq deploy/helm/soniq \
  --namespace soniq \
  --wait \
  --timeout 5m \
  -f values.production.yaml
```

The migration Job runs as a Helm `pre-install,pre-upgrade` hook by default, so
application resources are not applied until migrations complete successfully.
Because pre-install hooks run before normal chart resources are created, the
Secret referenced by `soniq-secret` must exist before install. The hook receives
non-secret config directly from chart values and sensitive values from the
pre-existing Secret. Set `migrate.hook.enabled=false` only when you
intentionally want to manage the migration Job separately. The chart rejects
`secret.create=true` with the default migration hook enabled because that
combination cannot work on a first install.

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

Run the same kind smoke through the Helm chart:

```bash
make k8s-smoke-kind-helm
```

This uses the same Compose-managed Postgres, Temporal, and MinIO dependencies,
but deploys Soniq API, worker, and the migration hook with
`helm upgrade --install`. The smoke script creates the external `soniq-secret`
before invoking Helm because the migration hook needs Secret values before
normal chart resources are applied. When local `helm` is unavailable, this
target uses `scripts/helm-cluster.sh` to run the Dockerized Helm image with the
host network and mounted `~/.kube` directory so it can reach the local kind API
server.

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
kubectl apply -f deploy/kubernetes/base/api-pdb.yaml
kubectl apply -f deploy/kubernetes/base/worker-pdb.yaml
kubectl apply -f deploy/kubernetes/base/api-hpa.yaml
kubectl apply -f deploy/kubernetes/base/api-networkpolicy.yaml
kubectl apply -f deploy/kubernetes/base/worker-networkpolicy.yaml
kubectl apply -f deploy/kubernetes/base/migrate-networkpolicy.yaml
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

## Shutdown

The API and worker Deployment specs set `terminationGracePeriodSeconds: 30`.
`soniq-api` uses `http.Server.Shutdown` with a 25 second timeout, so Kubernetes
has time to remove the pod from service endpoints and let in-flight requests
finish. `soniq-worker` passes the same signal context to the Temporal worker and
purge artifact cleanup loop; the Temporal worker uses a 25 second
`WorkerStopTimeout` before the pod-level grace period expires.

## Disruption Budgets

The raw manifests and Helm chart create PodDisruptionBudgets for API and worker.
Both default to `minAvailable: 1`. With the default 2 API replicas and 2 worker
replicas, Kubernetes can voluntarily evict one pod during node maintenance or
cluster upgrades while keeping at least one API pod and one worker pod running.

## Pod Placement

The raw manifests and Helm chart add topology spread constraints to API and
worker pods. The default constraint spreads replicas by
`kubernetes.io/hostname`, uses `maxSkew: 1`, and sets
`whenUnsatisfiable: ScheduleAnyway`.

`ScheduleAnyway` means Kubernetes tries to distribute pods across nodes, but it
can still schedule pods when a cluster has only one usable node. Production
values can switch this to `DoNotSchedule` when strict spreading is required.

## Security Hardening

API, worker, and migration containers run as non-root users, drop all Linux
capabilities, disable privilege escalation, and set
`readOnlyRootFilesystem: true`.

The manifests mount an `emptyDir` volume at `/tmp` for each container. This
gives the application a narrow writable runtime path for multipart buffering,
worker audio staging, and ffmpeg/ffprobe temporary files without making the
image filesystem writable.

## Autoscaling

The raw manifests create an API HorizontalPodAutoscaler. It targets the
`soniq-api` Deployment, keeps 2 to 6 API replicas, and scales on 70% average CPU
utilization.

The Helm chart exposes `api.autoscaling` and `worker.autoscaling`, but keeps
both disabled by default. Enable them in production values only when the cluster
has resource metrics available, for example metrics-server. When autoscaling is
enabled for a component, Helm does not render that Deployment's static
`replicas` value, so the HorizontalPodAutoscaler controls replica count.

Worker CPU autoscaling is only a baseline option. A later production setup
should prefer Temporal task queue backlog or custom worker metrics for worker
scaling decisions.

## Network Policies

The raw manifests and Helm chart create NetworkPolicies for API, worker, and
migration pods. These policies require a Kubernetes CNI plugin that enforces
NetworkPolicy.

The baseline policy allows API ingress on TCP 8080, denies worker and migration
ingress, and allows egress for DNS plus TCP 80, 443, 9000, 5432, and 7233.
Those ports cover the default HTTP/HTTPS, MinIO/S3-compatible, Postgres, and
Temporal paths. If production dependencies use different ports or require
destination-specific allowlists, override Helm `networkPolicy` values before
installing.

## Update Migrations

Kubernetes Jobs are not updated in place after their pod template changes. For
a new migration run in this raw-manifest phase, delete and recreate the Job:

```bash
kubectl -n soniq delete job soniq-migrate
kubectl apply -f deploy/kubernetes/base/migrate-job.yaml
```

The Helm chart already runs migrations through pre-install/pre-upgrade hooks.
This raw-manifest path keeps the behavior explicit.

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

- No Ingress or TLS manifest.
- Worker autoscaling is CPU-based when enabled; backlog-based worker scaling is
  not implemented yet.
- No remote-cluster release smoke yet; current cluster smoke coverage is local
  kind for both raw manifests and Helm.
- Worker exposes no HTTP health endpoint, so worker health is currently based
  on process status and logs.
