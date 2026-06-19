#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKER="${DOCKER:-docker}"
HELM_IMAGE="${HELM_IMAGE:-alpine/helm:4.2.2}"
HOST_KUBECONFIG="${KUBECONFIG:-}"

if command -v helm >/dev/null 2>&1; then
  exec helm "$@"
fi

docker_args=(
  run
  --rm
  --network
  host
  -v
  "$ROOT_DIR:/work"
  -w
  /work
)

if [[ -n "$HOST_KUBECONFIG" ]]; then
  IFS=':' read -r -a kubeconfig_entries <<<"$HOST_KUBECONFIG"
  container_kubeconfigs=()
  kubeconfig_index=0
  for entry in "${kubeconfig_entries[@]}"; do
    if [[ -z "$entry" ]]; then
      continue
    fi

    if [[ "$entry" = /* ]]; then
      host_path="$entry"
    else
      host_path="$(pwd -P)/$entry"
    fi
    if [[ ! -e "$host_path" ]]; then
      printf 'KUBECONFIG path does not exist: %s\n' "$entry" >&2
      exit 1
    fi

    if [[ -d "$host_path" ]]; then
      container_path="/kubeconfig/$kubeconfig_index"
      docker_args+=(-v "$host_path:$container_path:ro")
      container_kubeconfigs+=("$container_path")
    else
      host_dir="$(cd "$(dirname "$host_path")" && pwd -P)"
      host_file="$(basename "$host_path")"
      container_dir="/kubeconfig/$kubeconfig_index"
      docker_args+=(-v "$host_dir:$container_dir:ro")
      container_kubeconfigs+=("$container_dir/$host_file")
    fi
    kubeconfig_index=$((kubeconfig_index + 1))
  done

  if [[ "${#container_kubeconfigs[@]}" -eq 0 ]]; then
    printf 'KUBECONFIG is set but contains no usable paths\n' >&2
    exit 1
  fi

  container_kubeconfig="$(IFS=:; printf '%s' "${container_kubeconfigs[*]}")"
  docker_args+=(-e "KUBECONFIG=$container_kubeconfig")
else
  docker_args+=(-v "$HOME/.kube:/root/.kube:ro")
fi

exec "$DOCKER" "${docker_args[@]}" "$HELM_IMAGE" "$@"
