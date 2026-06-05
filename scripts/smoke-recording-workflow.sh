#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"

response="$(curl -fsS -X POST "${API_URL}/recordings" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Weekly sync","workflow_type":"meeting","language":"en"}')"

printf '%s\n' "$response"

recording_id="$(printf '%s\n' "$response" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
printf 'Expected Temporal workflow ID: recording-processing-%s\n' "$recording_id"
