#!/usr/bin/env bash
set -euo pipefail

COMPOSE_FILE=${OBSERVABILITY_COMPOSE_FILE:-compose.observability.yml}
COMPOSE_PROJECT=${OBSERVABILITY_COMPOSE_PROJECT:-soniq-observability}

echo "[observability-smoke] validating Docker Compose file"
docker compose -p "$COMPOSE_PROJECT" -f "$COMPOSE_FILE" config --quiet

echo "[observability-smoke] validating Grafana dashboard JSON"
python3 - <<'PY'
import json
from pathlib import Path

dashboard = Path("deploy/observability/grafana/dashboards/soniq-api-overview.json")
with dashboard.open(encoding="utf-8") as handle:
    data = json.load(handle)

required = {"title", "uid", "panels"}
missing = sorted(required - data.keys())
if missing:
    raise SystemExit(f"dashboard missing required keys: {', '.join(missing)}")
if not data["panels"]:
    raise SystemExit("dashboard must define at least one panel")
PY

echo "[observability-smoke] validating Temporal SDK dashboard metric contract"
(cd backend && go test ./internal/observability -run TestSoniqOverviewDashboardUsesTemporalSDKMetricNames -count=1)

echo "[observability-smoke] validating tracing instrumentation tests"
(cd backend && go test ./internal/observability ./internal/api ./internal/cleanup ./cmd/worker -run 'Test.*Tracing|Test.*Trace|Test.*Traces' -count=1)

echo "[observability-smoke] validating OpenTelemetry collector trace pipeline"
python3 - <<'PY'
from pathlib import Path

config = Path("deploy/observability/otel-collector/config.yaml").read_text(encoding="utf-8")
required = [
    "receivers:",
    "otlp:",
    "exporters:",
    "otlp/jaeger:",
    "pipelines:",
    "traces:",
    "jaeger:4317",
]
missing = [item for item in required if item not in config]
if missing:
    raise SystemExit(f"otel collector config missing expected entries: {', '.join(missing)}")
PY

echo "[observability-smoke] validating Prometheus config shape"
python3 - <<'PY'
from pathlib import Path

config = Path("deploy/observability/prometheus/prometheus.yml").read_text(encoding="utf-8")
required = [
    "job_name: soniq-api",
    "host.docker.internal:8080",
    "job_name: soniq-worker",
    "host.docker.internal:9091",
    "job_name: otel-collector",
    "otel-collector:8888",
    "otel-collector:8889",
]
missing = [item for item in required if item not in config]
if missing:
    raise SystemExit(f"prometheus config missing expected entries: {', '.join(missing)}")
PY

echo "[observability-smoke] passed"
