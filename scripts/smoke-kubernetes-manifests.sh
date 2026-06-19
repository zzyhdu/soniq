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
    ("PodDisruptionBudget", "soniq-api", "soniq"),
    ("PodDisruptionBudget", "soniq-worker", "soniq"),
    ("HorizontalPodAutoscaler", "soniq-api", "soniq"),
    ("NetworkPolicy", "soniq-api", "soniq"),
    ("NetworkPolicy", "soniq-worker", "soniq"),
    ("NetworkPolicy", "soniq-migrate", "soniq"),
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
    "WORKER_METRICS_ADDRESS",
    "WORKER_MAX_CONCURRENT_WORKFLOW_TASKS",
    "WORKER_MAX_CONCURRENT_ACTIVITIES",
    "WORKER_MAX_CONCURRENT_LOCAL_ACTIVITIES",
    "WORKER_TASK_QUEUE_ACTIVITIES_PER_SECOND",
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
api_pdb = by_id[("PodDisruptionBudget", "soniq-api", "soniq")]
worker_pdb = by_id[("PodDisruptionBudget", "soniq-worker", "soniq")]
api_hpa = by_id[("HorizontalPodAutoscaler", "soniq-api", "soniq")]
api_network_policy = by_id[("NetworkPolicy", "soniq-api", "soniq")]
worker_network_policy = by_id[("NetworkPolicy", "soniq-worker", "soniq")]
migrate_network_policy = by_id[("NetworkPolicy", "soniq-migrate", "soniq")]
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
    if security_context.get("readOnlyRootFilesystem") is not True:
        raise SystemExit(f"{resource_id(resource)} must use a read-only root filesystem")
    dropped = ((security_context.get("capabilities") or {}).get("drop")) or []
    if "ALL" not in dropped:
        raise SystemExit(f"{resource_id(resource)} must drop all capabilities")
    volumes = pod_spec(resource).get("volumes") or []
    tmp_volumes = [volume for volume in volumes if volume.get("name") == "tmp"]
    if len(tmp_volumes) != 1 or "emptyDir" not in tmp_volumes[0]:
        raise SystemExit(f"{resource_id(resource)} must define a tmp emptyDir volume")
    volume_mounts = container.get("volumeMounts") or []
    has_tmp_mount = any(
        mount.get("name") == "tmp" and mount.get("mountPath") == "/tmp"
        for mount in volume_mounts
    )
    if not has_tmp_mount:
        raise SystemExit(f"{resource_id(resource)} must mount tmp at /tmp")

if first_container(api).get("livenessProbe", {}).get("httpGet", {}).get("path") != "/healthz":
    raise SystemExit("soniq-api livenessProbe must use /healthz")
if first_container(api).get("readinessProbe", {}).get("httpGet", {}).get("path") != "/readyz":
    raise SystemExit("soniq-api readinessProbe must use /readyz")
worker_ports = {
    (port.get("name"), port.get("containerPort"))
    for port in first_container(worker).get("ports") or []
}
if ("metrics", 9091) not in worker_ports:
    raise SystemExit("soniq-worker must expose metrics containerPort 9091")

service_selector = (service.get("spec") or {}).get("selector") or {}
api_pod_labels = (
    (((api.get("spec") or {}).get("template") or {}).get("metadata") or {}).get("labels")
    or {}
)
for key, value in service_selector.items():
    if api_pod_labels.get(key) != value:
        raise SystemExit(f"soniq-api Service selector {key}={value} does not match API pod labels")

for pdb, deployment in [(api_pdb, api), (worker_pdb, worker)]:
    pdb_spec = pdb.get("spec") or {}
    if pdb_spec.get("minAvailable") != 1:
        raise SystemExit(f"{resource_id(pdb)} minAvailable must be 1")
    selector = ((pdb_spec.get("selector") or {}).get("matchLabels")) or {}
    pod_labels = (
        (((deployment.get("spec") or {}).get("template") or {}).get("metadata") or {}).get("labels")
        or {}
    )
    for key, value in selector.items():
        if pod_labels.get(key) != value:
            raise SystemExit(f"{resource_id(pdb)} selector {key}={value} does not match pod labels")


def require_topology_spread(deployment, component):
    constraints = pod_spec(deployment).get("topologySpreadConstraints") or []
    if len(constraints) != 1:
        raise SystemExit(f"{resource_id(deployment)} must define exactly one topology spread constraint")
    constraint = constraints[0]
    if constraint.get("maxSkew") != 1:
        raise SystemExit(f"{resource_id(deployment)} topology spread maxSkew must be 1")
    if constraint.get("topologyKey") != "kubernetes.io/hostname":
        raise SystemExit(f"{resource_id(deployment)} topology spread must use kubernetes.io/hostname")
    if constraint.get("whenUnsatisfiable") != "ScheduleAnyway":
        raise SystemExit(f"{resource_id(deployment)} topology spread must use ScheduleAnyway")
    selector = ((constraint.get("labelSelector") or {}).get("matchLabels")) or {}
    expected_selector = {
        "app.kubernetes.io/name": resource_id(deployment)[1],
        "app.kubernetes.io/component": component,
    }
    for key, value in expected_selector.items():
        if selector.get(key) != value:
            raise SystemExit(f"{resource_id(deployment)} topology spread selector must include {key}={value}")
    pod_labels = (
        (((deployment.get("spec") or {}).get("template") or {}).get("metadata") or {}).get("labels")
        or {}
    )
    for key, value in selector.items():
        if pod_labels.get(key) != value:
            raise SystemExit(f"{resource_id(deployment)} topology spread selector {key}={value} does not match pod labels")


require_topology_spread(api, "api")
require_topology_spread(worker, "worker")

hpa_spec = api_hpa.get("spec") or {}
scale_target = hpa_spec.get("scaleTargetRef") or {}
if scale_target.get("apiVersion") != "apps/v1":
    raise SystemExit("soniq-api HPA must target apps/v1")
if scale_target.get("kind") != "Deployment":
    raise SystemExit("soniq-api HPA must target a Deployment")
if scale_target.get("name") != "soniq-api":
    raise SystemExit("soniq-api HPA must target soniq-api")
if hpa_spec.get("minReplicas") != 2:
    raise SystemExit("soniq-api HPA minReplicas must be 2")
if hpa_spec.get("maxReplicas") != 6:
    raise SystemExit("soniq-api HPA maxReplicas must be 6")
cpu_metrics = [
    metric
    for metric in hpa_spec.get("metrics") or []
    if metric.get("type") == "Resource"
    and ((metric.get("resource") or {}).get("name")) == "cpu"
]
if len(cpu_metrics) != 1:
    raise SystemExit("soniq-api HPA must define exactly one CPU resource metric")
cpu_target = (((cpu_metrics[0].get("resource") or {}).get("target")) or {})
if cpu_target.get("type") != "Utilization":
    raise SystemExit("soniq-api HPA CPU metric must use utilization")
if cpu_target.get("averageUtilization") != 70:
    raise SystemExit("soniq-api HPA CPU target averageUtilization must be 70")

for network_policy, deployment in [
    (api_network_policy, api),
    (worker_network_policy, worker),
    (migrate_network_policy, migrate),
]:
    policy_spec = network_policy.get("spec") or {}
    policy_types = set(policy_spec.get("policyTypes") or [])
    if policy_types != {"Ingress", "Egress"}:
        raise SystemExit(f"{resource_id(network_policy)} policyTypes must be Ingress and Egress")
    selector = ((policy_spec.get("podSelector") or {}).get("matchLabels")) or {}
    pod_labels = (
        (((deployment.get("spec") or {}).get("template") or {}).get("metadata") or {}).get("labels")
        or {}
    )
    for key, value in selector.items():
        if pod_labels.get(key) != value:
            raise SystemExit(f"{resource_id(network_policy)} selector {key}={value} does not match pod labels")
    egress_ports = {
        (port.get("protocol", "TCP"), port.get("port"))
        for rule in policy_spec.get("egress") or []
        for port in rule.get("ports") or []
    }
    for required_port in [
        ("UDP", 53),
        ("TCP", 53),
        ("TCP", 80),
        ("TCP", 443),
        ("TCP", 9000),
        ("TCP", 5432),
        ("TCP", 7233),
    ]:
        if required_port not in egress_ports:
            raise SystemExit(f"{resource_id(network_policy)} is missing egress port {required_port}")

api_ingress_ports = {
    (port.get("protocol", "TCP"), port.get("port"))
    for rule in (api_network_policy.get("spec") or {}).get("ingress") or []
    for port in rule.get("ports") or []
}
if ("TCP", 8080) not in api_ingress_ports:
    raise SystemExit("soniq-api NetworkPolicy must allow ingress TCP 8080")
for network_policy in [worker_network_policy, migrate_network_policy]:
    ingress = (network_policy.get("spec") or {}).get("ingress")
    if ingress != []:
        raise SystemExit(f"{resource_id(network_policy)} must deny ingress")

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
