# OpenAPI + Scalar API Console Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add a local API console so developers can inspect Soniq API docs, upload audio, poll processing status, and read transcript/summary results from a browser.

**Architecture:** Keep OpenAPI as the API contract source of truth in `docs/openapi.yaml`. Serve a static Scalar API Reference page from the Go API so `Try it` requests are same-origin and avoid CORS setup. Keep this as a developer console, not the final product web UI.

**Tech Stack:** OpenAPI 3.1 YAML, Scalar API Reference via CDN, Go `net/http`, existing Soniq API handlers/tests.

---

## Scope

### In scope

- Add `docs/openapi.yaml` for the currently implemented API.
- Add `web/api.html` that embeds Scalar and points to `/openapi.yaml`.
- Serve `/openapi.yaml` and `/api-console` from the existing Go API router.
- Add tests proving both static endpoints are reachable and load the expected content.
- Manually verify Scalar can exercise upload/status/details against the local API service.

### Out of scope

- No React, Vite, router, frontend state management, or app shell.
- No OpenAPI code generation yet.
- No new cloud service or object storage integration.
- No business API semantic changes.
- No authentication or multi-workspace UI.

---

## Current API facts

The current backend already supports:

- `GET /healthz`
- `POST /recordings`: creates a recording session without audio and does not start workflow processing.
- `POST /recordings/upload`: uploads audio, creates a recording, and starts Temporal processing.
- `GET /recordings/{id}`
- `GET /recordings/{id}/status`
- `GET /recordings/{id}/details`: returns recording, transcript, segments, and summary.

`workflow_type` values come from `backend/internal/domain` and existing docs: `meeting`, `lecture`, `interview`, `memo`.

---

## Task 1: Add OpenAPI contract

**Objective:** Create the API contract for the existing Soniq endpoints.

**Files:**

- Create: `docs/openapi.yaml`

**Implementation details:**

Create an OpenAPI 3.1 document with:

```yaml
openapi: 3.1.0
info:
  title: Soniq API
  version: 0.1.0
  description: Local-first API for uploading recordings, tracking processing, and reading transcript/summary results.
servers:
  - url: /
    description: Same-origin local API server
```

Add paths:

- `GET /healthz`
- `POST /recordings`
- `POST /recordings/upload`
- `GET /recordings/{id}`
- `GET /recordings/{id}/status`
- `GET /recordings/{id}/details`

Use these schema names under `components.schemas`:

- `WorkflowType`
- `RecordingStatus`
- `Recording`
- `CreateRecordingRequest`
- `RecordingStatusResponse`
- `RecordingTranscript`
- `RecordingTranscriptSegment`
- `RecordingSummary`
- `RecordingDetails`
- `HealthzResponse`
- `ErrorResponse`

For `/recordings/upload`, use multipart form data:

```yaml
requestBody:
  required: true
  content:
    multipart/form-data:
      schema:
        type: object
        required: [workflow_type, audio]
        properties:
          title:
            type: string
          workflow_type:
            $ref: '#/components/schemas/WorkflowType'
          language:
            type: string
            example: zh
          audio:
            type: string
            format: binary
```

**Verification:**

Run:

```bash
test -f docs/openapi.yaml
grep -q 'openapi: 3.1.0' docs/openapi.yaml
grep -q '/recordings/upload:' docs/openapi.yaml
grep -q 'format: binary' docs/openapi.yaml
grep -q '/recordings/{id}/details:' docs/openapi.yaml
```

Expected: all commands exit `0`.

---

## Task 2: Add Scalar API console page

**Objective:** Add a static HTML page that renders the OpenAPI contract through Scalar.

**Files:**

- Create: `web/api.html`

**Implementation details:**

Use CDN-based Scalar to avoid adding a frontend build step:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Soniq API Console</title>
  </head>
  <body>
    <script
      id="api-reference"
      data-url="/openapi.yaml"
      src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"
    ></script>
  </body>
</html>
```

Keep it intentionally minimal. This is a developer console, not the final Soniq web product.

**Verification:**

Run:

```bash
test -f web/api.html
grep -q '@scalar/api-reference' web/api.html
grep -q '/openapi.yaml' web/api.html
```

Expected: all commands exit `0`.

---

## Task 3: Serve OpenAPI and Scalar from the Go API

**Objective:** Make `/openapi.yaml` and `/api-console` available from the existing API process.

**Files:**

- Modify: `backend/internal/api/router.go`
- Test: `backend/internal/api/router_test.go` or `backend/internal/api/recordings_test.go`

**Implementation approach:**

Add routes in both router builders:

```go
mux.HandleFunc("/openapi.yaml", openAPIHandler)
mux.HandleFunc("/api-console", apiConsoleHandler)
```

Implement simple handlers first:

- `openAPIHandler`: reads `docs/openapi.yaml`, returns YAML.
- `apiConsoleHandler`: reads `web/api.html`, returns HTML.

Use simple filesystem reads for the first version. Do not introduce `go:embed` yet unless tests reveal path problems. The current local development flow runs `make api` from the repository root, so relative paths are acceptable for this milestone.

Suggested response content types:

- `/openapi.yaml`: `application/yaml; charset=utf-8`
- `/api-console`: `text/html; charset=utf-8`

**Tests to write first:**

Add tests that fail before implementation:

```go
func TestOpenAPIEndpointServesContract(t *testing.T) {
    router := NewRouterWithStore(newFakeRecordingStore())
    request := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
    response := httptest.NewRecorder()

    router.ServeHTTP(response, request)

    if response.Code != http.StatusOK {
        t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
    }
    if !strings.Contains(response.Body.String(), "openapi: 3.1.0") {
        t.Fatalf("body missing OpenAPI version: %s", response.Body.String())
    }
    if !strings.Contains(response.Body.String(), "/recordings/upload:") {
        t.Fatalf("body missing upload path")
    }
}
```

```go
func TestAPIConsoleEndpointServesScalarPage(t *testing.T) {
    router := NewRouterWithStore(newFakeRecordingStore())
    request := httptest.NewRequest(http.MethodGet, "/api-console", nil)
    response := httptest.NewRecorder()

    router.ServeHTTP(response, request)

    if response.Code != http.StatusOK {
        t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
    }
    if !strings.Contains(response.Body.String(), "@scalar/api-reference") {
        t.Fatalf("body missing Scalar script")
    }
    if !strings.Contains(response.Body.String(), "/openapi.yaml") {
        t.Fatalf("body missing OpenAPI URL")
    }
}
```

**Verification:**

Run targeted tests:

```bash
cd backend
go test ./internal/api
```

Expected: `ok github.com/zzyhdu/soniq/backend/internal/api`.

---

## Task 4: Verify API contract manually through Scalar

**Objective:** Prove the page loads and can drive the current API.

**Files:**

- No source changes expected.

**Steps:**

Start the API:

```bash
cd /home/yangsan/playground/soniq
make api
```

Open:

```text
http://localhost:8080/api-console
```

Manual workflow:

1. Open `GET /healthz`, click Try it, confirm `200`.
2. Open `POST /recordings/upload`.
3. Fill:
   - `title`: `API console smoke`
   - `workflow_type`: `meeting`
   - `language`: `zh`
   - `audio`: choose `testdata/asr/mimo-tts/mp3/zh-four-speaker-standup.mp3`
4. Execute request and capture `id`.
5. Call `GET /recordings/{id}/status` until status is `completed` or `failed`.
6. Call `GET /recordings/{id}/details` and confirm transcript/summary fields are visible.

**Expected:**

- Scalar loads without CORS errors because spec and API requests are same-origin.
- Upload form shows a file picker for `audio`.
- Status endpoint returns JSON.
- Details endpoint returns `recording`, and after processing completes includes `transcript`, `segments`, and `summary`.

---

## Task 5: Run quality gates

**Objective:** Ensure the API console work does not regress backend behavior.

**Commands:**

```bash
cd /home/yangsan/playground/soniq/backend
go test ./...
cd /home/yangsan/playground/soniq
git diff --check
```

Optional OpenAPI lint if Node is available:

```bash
npx @redocly/cli lint docs/openapi.yaml
```

**Expected:**

- `go test ./...` passes.
- `git diff --check` exits `0`.
- Redocly lint either passes or reports only non-blocking style warnings that can be triaged.

---

## Commit boundary

After implementation and verification, ask for commit confirmation before committing.

Suggested commit message:

```bash
git commit -m "Add OpenAPI Scalar API console"
```

Files expected in the commit:

- `docs/openapi.yaml`
- `web/api.html`
- `backend/internal/api/router.go`
- `backend/internal/api/router_test.go` or `backend/internal/api/recordings_test.go`

---

## Acceptance criteria

- `/openapi.yaml` returns the OpenAPI 3.1 contract.
- `/api-console` returns the Scalar HTML page.
- Scalar renders all current core API endpoints.
- `POST /recordings/upload` appears with a file chooser.
- Same-origin Try it requests work against the local API service.
- `GET /recordings/{id}/details` is documented and usable from the console.
- `go test ./...` passes.
- No frontend build tool or new cloud service is introduced.
