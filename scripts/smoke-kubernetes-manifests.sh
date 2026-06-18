#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KUSTOMIZE_DIR="${KUSTOMIZE_DIR:-deploy/kubernetes/base}"
KUBECTL_IMAGE="${KUBECTL_IMAGE:-bitnami/kubectl:latest}"
K8S_SMOKE_SERVER_DRY_RUN="${K8S_SMOKE_SERVER_DRY_RUN:-0}"
K8S_SMOKE_NAMESPACE="${K8S_SMOKE_NAMESPACE:-soniq}"
SERVER_DRY_RUN_NAMESPACE_CREATED=0
RENDERED_MANIFEST="$(mktemp "${TMPDIR:-/tmp}/soniq-k8s-render.XXXXXX.yaml")"

cleanup() {
  if [[ "${SERVER_DRY_RUN_NAMESPACE_CREATED:-0}" == "1" ]]; then
    run_kubectl delete namespace "$K8S_SMOKE_NAMESPACE" --ignore-not-found >/dev/null 2>&1 || true
  fi
  rm -f "$RENDERED_MANIFEST"
}
trap cleanup EXIT

log() {
  printf '[k8s-smoke] %s\n' "$*"
}

kubectl_cmd=()
if command -v kubectl >/dev/null 2>&1; then
  kubectl_cmd=(kubectl)
elif command -v docker >/dev/null 2>&1; then
  kubectl_cmd=(docker run --rm -v "$ROOT_DIR:/work" -w /work "$KUBECTL_IMAGE")
else
  printf 'kubectl or docker is required for Kubernetes manifest smoke\n' >&2
  exit 127
fi

run_kubectl() {
  "${kubectl_cmd[@]}" "$@"
}

cd "$ROOT_DIR"

log "rendering $KUSTOMIZE_DIR"
run_kubectl kustomize "$KUSTOMIZE_DIR" >"$RENDERED_MANIFEST"

log "validating rendered resource contract"
python3 - "$RENDERED_MANIFEST" <<'PY'
import sys

try:
    import yaml
except Exception as exc:
    raise SystemExit(f"python3 with PyYAML is required for manifest assertions: {exc}")

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as handle:
    resources = [resource for resource in yaml.safe_load_all(handle) if resource]

expected_resources = {
    ("Namespace", "soniq", None),
    ("ConfigMap", "soniq-config", "soniq"),
    ("Secret", "soniq-secret", "soniq"),
    ("Job", "soniq-migrate", "soniq"),
    ("Deployment", "soniq-api", "soniq"),
    ("Service", "soniq-api", "soniq"),
    ("Deployment", "soniq-worker", "soniq"),
}


def resource_id(resource):
    metadata = resource.get("metadata") or {}
    return (
        resource.get("kind"),
        metadata.get("name"),
        metadata.get("namespace"),
    )


by_id = {resource_id(resource): resource for resource in resources}
missing = sorted(expected_resources.difference(by_id))
unexpected = sorted(set(by_id).difference(expected_resources))
if missing or unexpected:
    raise SystemExit(
        "unexpected Kubernetes resources: "
        f"missing={missing or 'none'} unexpected={unexpected or 'none'}"
    )

config = by_id[("ConfigMap", "soniq-config", "soniq")]
config_data = config.get("data") or {}
for key in [
    "APP_ENV",
    "API_ADDRESS",
    "TEMPORAL_ADDRESS",
    "TEMPORAL_NAMESPACE",
    "TEMPORAL_TASK_QUEUE",
    "STORAGE_PROVIDER",
    "S3_ENDPOINT",
    "S3_REGION",
    "S3_BUCKET",
]:
    if not config_data.get(key):
        raise SystemExit(f"soniq-config is missing required key {key}")
if config_data.get("STORAGE_PROVIDER") != "s3_compatible":
    raise SystemExit("soniq-config STORAGE_PROVIDER must be s3_compatible")

secret = by_id[("Secret", "soniq-secret", "soniq")]
secret_data = secret.get("stringData") or {}
for key in ["POSTGRES_DSN", "S3_ACCESS_KEY", "S3_SECRET_KEY"]:
    if not secret_data.get(key):
        raise SystemExit(f"soniq-secret is missing required key {key}")

api = by_id[("Deployment", "soniq-api", "soniq")]
worker = by_id[("Deployment", "soniq-worker", "soniq")]
migrate = by_id[("Job", "soniq-migrate", "soniq")]
service = by_id[("Service", "soniq-api", "soniq")]
expected_pod_users = {
    ("Deployment", "soniq-api", "soniq"): (65532, 65532),
    ("Deployment", "soniq-worker", "soniq"): (10001, 10001),
    ("Job", "soniq-migrate", "soniq"): (65532, 65532),
}


def pod_spec(resource):
    kind = resource.get("kind")
    spec = resource.get("spec") or {}
    if kind == "Deployment":
        return (((spec.get("template") or {}).get("spec")) or {})
    if kind == "Job":
        return ((((spec.get("template") or {}).get("spec")) or {}))
    raise ValueError(f"unsupported resource kind {kind}")


def first_container(resource):
    containers = pod_spec(resource).get("containers") or []
    if len(containers) != 1:
        raise SystemExit(f"{resource_id(resource)} must have exactly one container")
    return containers[0]


def require_env_from(resource):
    env_from = first_container(resource).get("envFrom") or []
    names = {
        item.get("configMapRef", {}).get("name")
        for item in env_from
        if "configMapRef" in item
    }
    secret_names = {
        item.get("secretRef", {}).get("name")
        for item in env_from
        if "secretRef" in item
    }
    if "soniq-config" not in names:
        raise SystemExit(f"{resource_id(resource)} must read soniq-config")
    if "soniq-secret" not in secret_names:
        raise SystemExit(f"{resource_id(resource)} must read soniq-secret")


for resource in [api, worker, migrate]:
    container = first_container(resource)
    if not container.get("image"):
        raise SystemExit(f"{resource_id(resource)} is missing container image")
    require_env_from(resource)
    pod_security_context = pod_spec(resource).get("securityContext") or {}
    expected_uid, expected_gid = expected_pod_users[resource_id(resource)]
    if pod_security_context.get("runAsNonRoot") is not True:
        raise SystemExit(f"{resource_id(resource)} must require runAsNonRoot")
    if pod_security_context.get("runAsUser") != expected_uid:
        raise SystemExit(f"{resource_id(resource)} must run as UID {expected_uid}")
    if pod_security_context.get("runAsGroup") != expected_gid:
        raise SystemExit(f"{resource_id(resource)} must run as GID {expected_gid}")
    if (pod_security_context.get("seccompProfile") or {}).get("type") != "RuntimeDefault":
        raise SystemExit(f"{resource_id(resource)} must use RuntimeDefault seccomp")
    security_context = container.get("securityContext") or {}
    if security_context.get("allowPrivilegeEscalation") is not False:
        raise SystemExit(f"{resource_id(resource)} must disable privilege escalation")
    dropped = ((security_context.get("capabilities") or {}).get("drop")) or []
    if "ALL" not in dropped:
        raise SystemExit(f"{resource_id(resource)} must drop all capabilities")

if first_container(api).get("livenessProbe", {}).get("httpGet", {}).get("path") != "/healthz":
    raise SystemExit("soniq-api livenessProbe must use /healthz")
if first_container(api).get("readinessProbe", {}).get("httpGet", {}).get("path") != "/readyz":
    raise SystemExit("soniq-api readinessProbe must use /readyz")

service_selector = (service.get("spec") or {}).get("selector") or {}
api_pod_labels = (
    (((api.get("spec") or {}).get("template") or {}).get("metadata") or {}).get("labels")
    or {}
)
for key, value in service_selector.items():
    if api_pod_labels.get(key) != value:
        raise SystemExit(f"soniq-api Service selector {key}={value} does not match API pod labels")

if (migrate.get("spec") or {}).get("backoffLimit") != 3:
    raise SystemExit("soniq-migrate backoffLimit must be 3")
if pod_spec(migrate).get("restartPolicy") != "OnFailure":
    raise SystemExit("soniq-migrate restartPolicy must be OnFailure")

print("validated resources:")
for kind, name, namespace in sorted(expected_resources):
    qualified = f"{namespace}/{name}" if namespace else name
    print(f"  - {kind} {qualified}")
PY

if [[ "$K8S_SMOKE_SERVER_DRY_RUN" == "1" ]]; then
  log "running server-side dry-run"
  if ! run_kubectl get namespace "$K8S_SMOKE_NAMESPACE" >/dev/null 2>&1; then
    log "creating temporary namespace $K8S_SMOKE_NAMESPACE for server-side dry-run"
    run_kubectl create namespace "$K8S_SMOKE_NAMESPACE" >/dev/null
    SERVER_DRY_RUN_NAMESPACE_CREATED=1
  fi
  run_kubectl apply --dry-run=server -k "$KUSTOMIZE_DIR"
else
  log "skipping server-side dry-run; set K8S_SMOKE_SERVER_DRY_RUN=1 when a cluster is available"
fi

log "passed"
