#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-compose.temporal.yml}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-soniq}"
K8S_NAMESPACE="${K8S_NAMESPACE:-soniq}"
KUSTOMIZE_DIR="${KUSTOMIZE_DIR:-deploy/kubernetes/base}"
POSTGRES_USER="${POSTGRES_USER:-soniq_user}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-soniq_password}"
POSTGRES_DB="${POSTGRES_DB:-soniq}"
S3_BUCKET="${S3_BUCKET:-soniq}"
S3_ACCESS_KEY="${S3_ACCESS_KEY:-soniq_minio_user}"
S3_SECRET_KEY="${S3_SECRET_KEY:-soniq_minio_password}"
KIND_SMOKE_BUILD_IMAGES="${KIND_SMOKE_BUILD_IMAGES:-1}"
KIND_SMOKE_CLEAN_NAMESPACE="${KIND_SMOKE_CLEAN_NAMESPACE:-1}"
KIND_SMOKE_API_PORT="${KIND_SMOKE_API_PORT:-18080}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/soniq-kind-smoke.XXXXXX")"
BASE_MANIFEST="$TMP_DIR/base.yaml"
SMOKE_MANIFEST="$TMP_DIR/smoke.yaml"
PORT_FORWARD_LOG="$TMP_DIR/port-forward.log"
PORT_FORWARD_PID=""

cleanup() {
  if [[ -n "$PORT_FORWARD_PID" ]] && kill -0 "$PORT_FORWARD_PID" 2>/dev/null; then
    kill "$PORT_FORWARD_PID" 2>/dev/null || true
    wait "$PORT_FORWARD_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

log() {
  printf '[kind-smoke] %s\n' "$*"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf '%s is required for kind Kubernetes smoke\n' "$1" >&2
    exit 127
  fi
}

wait_for_command() {
  local description="$1"
  local attempts="$2"
  shift 2
  local i
  for ((i = 1; i <= attempts; i++)); do
    if "$@" >/dev/null 2>&1; then
      log "$description is ready"
      return 0
    fi
    sleep 1
  done
  log "timed out waiting for $description"
  return 1
}

compose_container_id() {
  local service="$1"
  local container_id
  container_id="$(docker compose -f "$COMPOSE_FILE" ps -q "$service")"
  if [[ -z "$container_id" ]]; then
    printf 'compose service %s is not running\n' "$service" >&2
    exit 1
  fi
  printf '%s\n' "$container_id"
}

container_network_name() {
  local container_id="$1"
  docker inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' "$container_id" | head -n 1
}

container_ip_on_network() {
  local container_id="$1"
  local network="$2"
  docker inspect -f "{{with index .NetworkSettings.Networks \"$network\"}}{{.IPAddress}}{{end}}" "$container_id"
}

ensure_kind_cluster() {
  if kind get clusters | grep -qx "$KIND_CLUSTER_NAME"; then
    log "kind cluster $KIND_CLUSTER_NAME already exists"
  else
    log "creating kind cluster $KIND_CLUSTER_NAME"
    kind create cluster --name "$KIND_CLUSTER_NAME"
  fi
  kubectl config use-context "kind-$KIND_CLUSTER_NAME" >/dev/null
  kubectl wait --for=condition=Ready "node/$KIND_CLUSTER_NAME-control-plane" --timeout=120s
  kubectl -n kube-system wait --for=condition=Ready pod --all --timeout=120s
}

ensure_kind_node_on_compose_network() {
  local network="$1"
  local node_container="$KIND_CLUSTER_NAME-control-plane"
  if docker inspect -f "{{with index .NetworkSettings.Networks \"$network\"}}connected{{end}}" "$node_container" | grep -qx connected; then
    log "kind node $node_container is already connected to Docker network $network"
    return 0
  fi
  log "connecting kind node $node_container to Docker network $network"
  docker network connect "$network" "$node_container"
}

run_minio_mc() {
  docker compose -f "$COMPOSE_FILE" exec -T \
    -e S3_BUCKET="$S3_BUCKET" \
    -e S3_ACCESS_KEY="$S3_ACCESS_KEY" \
    -e S3_SECRET_KEY="$S3_SECRET_KEY" \
    minio sh -c '
      mc alias set smoke http://127.0.0.1:9000 "$S3_ACCESS_KEY" "$S3_SECRET_KEY" >/dev/null
      mc "$@"
    ' sh "$@"
}

assert_s3_bucket_ready() {
  run_minio_mc ls "smoke/$S3_BUCKET"
}

render_smoke_manifest() {
  local postgres_ip="$1"
  local temporal_ip="$2"
  local minio_ip="$3"

  log "rendering $KUSTOMIZE_DIR"
  kubectl kustomize "$KUSTOMIZE_DIR" >"$BASE_MANIFEST"

  log "patching rendered manifest for kind smoke"
  python3 - "$BASE_MANIFEST" "$SMOKE_MANIFEST" "$postgres_ip" "$temporal_ip" "$minio_ip" \
    "$POSTGRES_USER" "$POSTGRES_PASSWORD" "$POSTGRES_DB" "$S3_BUCKET" "$S3_ACCESS_KEY" "$S3_SECRET_KEY" <<'PY'
import sys

import yaml

(
    source_path,
    output_path,
    postgres_ip,
    temporal_ip,
    minio_ip,
    postgres_user,
    postgres_password,
    postgres_db,
    s3_bucket,
    s3_access_key,
    s3_secret_key,
) = sys.argv[1:]

with open(source_path, "r", encoding="utf-8") as handle:
    resources = [resource for resource in yaml.safe_load_all(handle) if resource]

for resource in resources:
    kind = resource.get("kind")
    metadata = resource.get("metadata") or {}
    name = metadata.get("name")
    if kind == "ConfigMap" and name == "soniq-config":
        data = resource.setdefault("data", {})
        data.update(
            {
                "APP_PUBLIC_URL": "http://localhost:8080",
                "AUTH_COOKIE_SECURE": "false",
                "TEMPORAL_ADDRESS": "temporal:7233",
                "TEMPORAL_NAMESPACE": "default",
                "TEMPORAL_TASK_QUEUE": "soniq-audio-pipeline",
                "STORAGE_PROVIDER": "s3_compatible",
                "S3_ENDPOINT": "http://minio:9000",
                "S3_REGION": "us-east-1",
                "S3_BUCKET": s3_bucket,
                "S3_FORCE_PATH_STYLE": "true",
                "TRANSCRIPTION_PROVIDER": "fake_transcription",
                "LLM_PROVIDER": "fake_llm",
                "PRIVACY_ALLOW_EXTERNAL_MODEL_PROVIDERS": "false",
            }
        )
    if kind == "Secret" and name == "soniq-secret":
        data = resource.setdefault("stringData", {})
        data.update(
            {
                "POSTGRES_DSN": f"postgres://{postgres_user}:{postgres_password}@soniq-postgresql:5432/{postgres_db}?sslmode=disable",
                "S3_ACCESS_KEY": s3_access_key,
                "S3_SECRET_KEY": s3_secret_key,
            }
        )


def external_service(name, port_name, port):
    return {
        "apiVersion": "v1",
        "kind": "Service",
        "metadata": {
            "name": name,
            "namespace": "soniq",
            "labels": {
                "app.kubernetes.io/name": name,
                "app.kubernetes.io/part-of": "soniq",
                "soniq.dev/smoke-dependency": "true",
            },
        },
        "spec": {
            "ports": [
                {
                    "name": port_name,
                    "port": port,
                    "targetPort": port,
                }
            ]
        },
    }


def external_endpoint_slice(name, port_name, port, ip):
    return {
        "apiVersion": "discovery.k8s.io/v1",
        "kind": "EndpointSlice",
        "metadata": {
            "name": f"{name}-smoke",
            "namespace": "soniq",
            "labels": {
                "app.kubernetes.io/name": name,
                "app.kubernetes.io/part-of": "soniq",
                "soniq.dev/smoke-dependency": "true",
                "kubernetes.io/service-name": name,
                "endpointslice.kubernetes.io/managed-by": "soniq-kind-smoke",
            },
        },
        "addressType": "IPv4",
        "ports": [
            {
                "name": port_name,
                "protocol": "TCP",
                "port": port,
            }
        ],
        "endpoints": [{"addresses": [ip]}],
    }


resources.extend(
    [
        external_service("soniq-postgresql", "postgres", 5432),
        external_endpoint_slice("soniq-postgresql", "postgres", 5432, postgres_ip),
        external_service("temporal", "grpc", 7233),
        external_endpoint_slice("temporal", "grpc", 7233, temporal_ip),
        external_service("minio", "s3", 9000),
        external_endpoint_slice("minio", "s3", 9000, minio_ip),
    ]
)

with open(output_path, "w", encoding="utf-8") as handle:
    yaml.safe_dump_all(resources, handle, sort_keys=False)
PY
}

show_debug_context() {
  log "recent pod status"
  kubectl -n "$K8S_NAMESPACE" get pods -o wide || true
  log "migrate logs"
  kubectl -n "$K8S_NAMESPACE" logs job/soniq-migrate || true
  log "api logs"
  kubectl -n "$K8S_NAMESPACE" logs deploy/soniq-api --tail=100 || true
  log "worker logs"
  kubectl -n "$K8S_NAMESPACE" logs deploy/soniq-worker --tail=100 || true
}

main() {
  cd "$ROOT_DIR"
  require_command docker
  require_command kind
  require_command kubectl
  require_command python3
  python3 -c 'import yaml' >/dev/null 2>&1 || {
    printf 'python3 with PyYAML is required for kind Kubernetes smoke\n' >&2
    exit 127
  }

  log "starting Compose dependencies"
  docker compose -f "$COMPOSE_FILE" up -d

  wait_for_command "Soniq Postgres" 60 \
    docker compose -f "$COMPOSE_FILE" exec -T soniq-postgresql pg_isready -U "$POSTGRES_USER"
  wait_for_command "Temporal frontend" 60 \
    docker compose -f "$COMPOSE_FILE" exec -T temporal temporal --address temporal:7233 operator namespace list
  wait_for_command "MinIO bucket $S3_BUCKET" 60 assert_s3_bucket_ready

  ensure_kind_cluster

  local postgres_container temporal_container minio_container compose_network
  postgres_container="$(compose_container_id soniq-postgresql)"
  temporal_container="$(compose_container_id temporal)"
  minio_container="$(compose_container_id minio)"
  compose_network="$(container_network_name "$postgres_container")"
  if [[ -z "$compose_network" ]]; then
    log "could not determine Compose network from soniq-postgresql"
    return 1
  fi
  ensure_kind_node_on_compose_network "$compose_network"

  local postgres_ip temporal_ip minio_ip
  postgres_ip="$(container_ip_on_network "$postgres_container" "$compose_network")"
  temporal_ip="$(container_ip_on_network "$temporal_container" "$compose_network")"
  minio_ip="$(container_ip_on_network "$minio_container" "$compose_network")"
  log "Compose dependency IPs on $compose_network: postgres=$postgres_ip temporal=$temporal_ip minio=$minio_ip"

  if [[ "$KIND_SMOKE_BUILD_IMAGES" == "1" ]]; then
    log "building Soniq runtime images"
    make docker-build
  fi
  log "loading Soniq images into kind cluster $KIND_CLUSTER_NAME"
  kind load docker-image soniq-api:dev soniq-worker:dev soniq-migrate:dev --name "$KIND_CLUSTER_NAME"

  render_smoke_manifest "$postgres_ip" "$temporal_ip" "$minio_ip"

  if [[ "$KIND_SMOKE_CLEAN_NAMESPACE" == "1" ]]; then
    log "cleaning namespace $K8S_NAMESPACE"
    kubectl delete namespace "$K8S_NAMESPACE" --ignore-not-found
    while kubectl get namespace "$K8S_NAMESPACE" >/dev/null 2>&1; do
      sleep 1
    done
  fi

  log "applying smoke manifest"
  kubectl apply -f "$SMOKE_MANIFEST"

  log "waiting for migration job"
  if ! kubectl -n "$K8S_NAMESPACE" wait --for=condition=complete job/soniq-migrate --timeout=180s; then
    show_debug_context
    return 1
  fi
  kubectl -n "$K8S_NAMESPACE" logs job/soniq-migrate

  log "waiting for API and worker deployments"
  if ! kubectl -n "$K8S_NAMESPACE" rollout status deployment/soniq-api --timeout=180s; then
    show_debug_context
    return 1
  fi
  if ! kubectl -n "$K8S_NAMESPACE" rollout status deployment/soniq-worker --timeout=180s; then
    show_debug_context
    return 1
  fi

  log "port-forwarding soniq-api on localhost:$KIND_SMOKE_API_PORT"
  kubectl -n "$K8S_NAMESPACE" port-forward service/soniq-api "$KIND_SMOKE_API_PORT:80" >"$PORT_FORWARD_LOG" 2>&1 &
  PORT_FORWARD_PID=$!
  wait_for_command "API /healthz" 30 curl -fsS "http://localhost:$KIND_SMOKE_API_PORT/healthz"
  wait_for_command "API /readyz" 30 curl -fsS "http://localhost:$KIND_SMOKE_API_PORT/readyz"

  log "passed"
}

main "$@"
