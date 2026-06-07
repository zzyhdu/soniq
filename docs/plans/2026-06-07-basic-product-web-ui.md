# Basic Product Web UI Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add the first Soniq product web UI for uploading an audio recording, watching processing status, and reading transcript/summary results.

**Architecture:** Introduce a pnpm workspace with `apps/` and `packages/` so the current Web app can grow alongside future mobile and miniapp clients. Keep platform-independent API DTOs/client code in `packages/api-client`; keep React Query hooks, UI state, and browser-specific behavior in `apps/web`. Treat this milestone as local/developer product UI with no auth, workspace, or provider configuration UI yet.

**Tech Stack:** pnpm workspace, React + Vite + TypeScript, Tailwind CSS, shadcn/ui, `@tanstack/react-query`, browser `fetch`/`FormData`, current Go API endpoints documented in `backend/doc/openapi.yaml`.

---

## Scope

### In scope

- Create a pnpm workspace rooted at the repository root.
- Create `apps/web` as a React + Vite + TypeScript app.
- Create `packages/api-client` with DTOs and typed API functions for current recording endpoints.
- Add React Query provider setup and hooks for upload, status polling, and details loading.
- Build a simple single-page UI:
  - workflow type selection;
  - optional title and language fields;
  - audio file picker;
  - upload action;
  - processing status display;
  - transcript segments display;
  - summary display.
- Add Tailwind CSS and shadcn/ui from the start so the first product UI has a reusable visual foundation.
- Add a local dev workflow where Vite proxies API calls to the Go API.
- Add focused tests for API client behavior and core Web UI state.
- Document how to run the Go API/worker and Web dev server together.

### Out of scope

- No auth, sessions, multi-user, workspace, RBAC, or provider settings UI.
- No mobile app or WeChat miniapp implementation yet.
- No generated OpenAPI client yet; hand-written DTOs are acceptable for the first UI slice.
- No backend semantic API changes unless implementation discovers a real UI-blocking API mismatch.
- No production static serving from Go in the first task unless explicitly approved after the dev UI works.
- No full design system, dark-mode polish, or broad component library beyond the shadcn/ui primitives needed for the upload/status/results page.

---

## Current API facts

The Web UI should target the existing same-origin or proxied API surface:

- `POST /recordings/upload` accepts `multipart/form-data` with `workflow_type`, `audio`, optional `title`, and optional `language`.
- Upload success returns an envelope with `recording` and `processing_enqueued`.
- `GET /recordings/{id}/status` returns the current recording status.
- `GET /recordings/{id}/details` returns `recording`, `segments`, optional `transcript`, and optional `summary`.
- Valid `workflow_type` values are `memo`, `meeting`, `lecture`, and `interview`.
- Current status values are `uploaded`, `processing`, `transcribing`, `summarizing`, `completed`, `failed`, and `cancelled`.

---

## Repository shape after this milestone

```txt
pnpm-workspace.yaml
package.json
pnpm-lock.yaml
apps/
  web/
    components.json
    index.html
    package.json
    src/
      App.tsx
      main.tsx
      api/queries.ts
      components/
        ui/
      lib/utils.ts
      styles.css
    tsconfig.json
    vite.config.ts
packages/
  api-client/
    package.json
    src/index.ts
    src/recordings.ts
    tsconfig.json
```

Future client directories remain reserved but unimplemented:

```txt
apps/mobile/
apps/miniapp/
packages/shared/
packages/ui/
packages/config/
```

---

## Version check before implementation

Before installing dependencies, verify current stable versions and record them in the task report:

```bash
pnpm --version
pnpm view react version
pnpm view react-dom version
pnpm view vite version
pnpm view typescript version
pnpm view @vitejs/plugin-react version
pnpm view @tanstack/react-query version
pnpm view tailwindcss version
pnpm view @tailwindcss/vite version
pnpm view shadcn version
pnpm view lucide-react version
pnpm view class-variance-authority version
pnpm view clsx version
pnpm view tailwind-merge version
pnpm view tw-animate-css version
pnpm view vitest version
pnpm view @testing-library/react version
pnpm view @testing-library/user-event version
pnpm view jsdom version
```

If `pnpm` is missing, stop and ask before installing it globally. Do not switch to npm or yarn.

Recently observed registry versions on 2026-06-07 were React `19.2.7`, Vite `8.0.16`, TypeScript `6.0.3`, `@tanstack/react-query` `5.101.0`, Tailwind CSS `4.3.0`, `@tailwindcss/vite` `4.3.0`, and shadcn CLI package `4.10.0`; re-check before implementation because the registry is the source of truth.

---

## Task 1: Create pnpm workspace skeleton

**Objective:** Add the minimal JS/TS workspace structure without building UI behavior yet.

**Files:**

- Create: `pnpm-workspace.yaml`
- Create: `package.json`
- Create: `apps/web/package.json`
- Create: `packages/api-client/package.json`
- Create: `apps/.gitkeep` only if needed before creating `apps/web`
- Create: `packages/.gitkeep` only if needed before creating `packages/api-client`

**Implementation details:**

Root `pnpm-workspace.yaml` should include:

```yaml
packages:
  - apps/*
  - packages/*
```

Root `package.json` should be private and expose workspace scripts:

```json
{
  "private": true,
  "scripts": {
    "web:dev": "pnpm --filter @soniq/web dev",
    "web:build": "pnpm --filter @soniq/web build",
    "web:test": "pnpm --filter @soniq/web test",
    "api-client:test": "pnpm --filter @soniq/api-client test",
    "typecheck": "pnpm -r typecheck",
    "test": "pnpm -r test"
  }
}
```

Use scoped package names:

- `@soniq/web`
- `@soniq/api-client`

**Verification:**

Run:

```bash
pnpm install
pnpm -r list --depth 0
```

Expected: pnpm creates `pnpm-lock.yaml` and lists both workspace packages.

**Commit boundary:** Ask the user before committing. Suggested message: `chore: add pnpm workspace skeleton`.

---

## Task 2: Add typed API client package

**Objective:** Create a platform-independent client for the current recording API.

**Files:**

- Create: `packages/api-client/tsconfig.json`
- Create: `packages/api-client/src/index.ts`
- Create: `packages/api-client/src/recordings.ts`
- Create: `packages/api-client/src/recordings.test.ts`
- Modify: `packages/api-client/package.json`

**DTOs to define:**

```ts
export type WorkflowType = 'memo' | 'meeting' | 'lecture' | 'interview';

export type RecordingStatus =
  | 'uploaded'
  | 'processing'
  | 'transcribing'
  | 'summarizing'
  | 'completed'
  | 'failed'
  | 'cancelled';
```

Define DTOs matching `backend/doc/openapi.yaml`, using snake_case field names for wire compatibility:

- `Recording`
- `UploadRecordingResponse`
- `RecordingStatusResponse`
- `RecordingTranscript`
- `RecordingTranscriptSegment`
- `RecordingSummary`
- `RecordingDetails`
- `SoniqApiError`

**Client functions:**

```ts
uploadRecording(input: UploadRecordingInput): Promise<UploadRecordingResponse>
getRecordingStatus(recordingId: string): Promise<RecordingStatusResponse>
getRecordingDetails(recordingId: string): Promise<RecordingDetails>
```

`uploadRecording` should build `FormData` and append optional fields only when present.

**Tests to write first:**

Use Vitest with a fake `fetch` to verify:

- `uploadRecording` sends `POST /recordings/upload` with `FormData`.
- optional `title` and `language` are included only when provided.
- JSON error responses produce a typed error with status and message.
- status/details paths URL-encode `recordingId`.

**Verification:**

Run:

```bash
pnpm --filter @soniq/api-client test
pnpm --filter @soniq/api-client typecheck
```

Expected: all client tests and TypeScript checks pass.

**Commit boundary:** Ask the user before committing. Suggested message: `feat: add typed recording API client`.

---

## Task 3: Scaffold React Vite app with Tailwind and shadcn/ui

**Objective:** Create a runnable Web app shell with React Query, Tailwind CSS, and shadcn/ui wired in.

**Files:**

- Create: `apps/web/components.json`
- Create: `apps/web/index.html`
- Create: `apps/web/tsconfig.json`
- Create: `apps/web/vite.config.ts`
- Create: `apps/web/src/main.tsx`
- Create: `apps/web/src/App.tsx`
- Create: `apps/web/src/lib/utils.ts`
- Create: `apps/web/src/components/ui/button.tsx`
- Create: `apps/web/src/components/ui/card.tsx`
- Create: `apps/web/src/components/ui/input.tsx`
- Create: `apps/web/src/components/ui/label.tsx`
- Create: `apps/web/src/components/ui/select.tsx`
- Create: `apps/web/src/components/ui/badge.tsx`
- Create: `apps/web/src/styles.css`
- Modify: `apps/web/package.json`

**Implementation details:**

- Configure Vite React plugin.
- Configure Tailwind CSS v4 through `@tailwindcss/vite` and `@import "tailwindcss";` in `styles.css`.
- Configure path alias `@/*` to `apps/web/src/*` for shadcn/ui imports.
- Add dependency on `@soniq/api-client` through `workspace:*`.
- Wrap the app in `QueryClientProvider`.
- Configure Vite dev proxy so browser calls to `/healthz` and `/recordings/*` go to `http://localhost:8080`.
- Initialize shadcn/ui for a Vite React app with neutral/base styling and CSS variables.
- Add only the primitives needed by the first UI: `button`, `card`, `input`, `label`, `select`, and `badge`.
- Keep the first app shell simple: title, short description, and a shadcn `Card` placeholder panel.

**Suggested shadcn command shape:**

Run from `apps/web` after dependencies are installed and alias config exists:

```bash
pnpm dlx shadcn@latest init
pnpm dlx shadcn@latest add button card input label select badge
```

If the CLI output differs for the current shadcn version, follow the CLI prompts for Vite/React/Tailwind v4 and record the chosen options in the task report.

**Verification:**

Run:

```bash
pnpm --filter @soniq/web typecheck
pnpm --filter @soniq/web build
```

Expected: TypeScript and Vite production build pass.

**Commit boundary:** Ask the user before committing. Suggested message: `feat: scaffold React web app`.

---

## Task 4: Add upload form and mutation hook

**Objective:** Let a user select an audio file and start processing from the Web app.

**Files:**

- Create: `apps/web/src/api/queries.ts`
- Create: `apps/web/src/components/RecordingUploadForm.tsx`
- Create: `apps/web/src/components/RecordingUploadForm.test.tsx`
- Modify: `apps/web/src/App.tsx`
- Modify: `apps/web/src/styles.css`

**Implementation details:**

- Add `useUploadRecording` with React Query `useMutation`.
- Build the form with shadcn `Card`, `Input`, `Label`, `Select`, and `Button` primitives.
- Default workflow type to `meeting`.
- Default language to `zh` but allow editing.
- Disable submit until a file is selected.
- After successful upload, lift `recording.id` into `App` state for status/details loading.
- Show whether `processing_enqueued` is true.

**Tests to write first:**

Use Testing Library to verify:

- submit button is disabled without a file;
- selecting a file enables upload;
- workflow type/title/language are passed to the mutation;
- upload errors are shown to the user.

**Verification:**

Run:

```bash
pnpm --filter @soniq/web test
pnpm --filter @soniq/web typecheck
pnpm --filter @soniq/web build
```

Expected: tests, TypeScript, and build pass.

**Commit boundary:** Ask the user before committing. Suggested message: `feat: add recording upload form`.

---

## Task 5: Add status polling

**Objective:** Display current processing state and poll until terminal status.

**Files:**

- Modify: `apps/web/src/api/queries.ts`
- Create: `apps/web/src/components/RecordingStatusPanel.tsx`
- Create: `apps/web/src/components/RecordingStatusPanel.test.tsx`
- Modify: `apps/web/src/App.tsx`

**Implementation details:**

- Add `useRecordingStatus(recordingId)` using React Query `useQuery`.
- Poll every 1-2 seconds while status is not `completed`, `failed`, or `cancelled`.
- Stop polling when no recording id exists.
- Render status labels for `uploaded`, `processing`, `transcribing`, `summarizing`, `completed`, `failed`, and `cancelled`.
- Use shadcn `Badge` for status and `Card` for the status panel.
- Surface API errors without clearing the selected recording id.

**Tests to write first:**

Verify:

- no request is made without a recording id;
- polling is enabled for non-terminal status;
- polling stops for terminal status;
- failed/cancelled statuses are visually distinct from completed.

**Verification:**

Run:

```bash
pnpm --filter @soniq/web test
pnpm --filter @soniq/web typecheck
pnpm --filter @soniq/web build
```

Expected: tests, TypeScript, and build pass.

**Commit boundary:** Ask the user before committing. Suggested message: `feat: poll recording processing status`.

---

## Task 6: Display transcript and summary results

**Objective:** Load and render final transcript segments and summary when processing completes.

**Files:**

- Modify: `apps/web/src/api/queries.ts`
- Create: `apps/web/src/components/RecordingResults.tsx`
- Create: `apps/web/src/components/RecordingResults.test.tsx`
- Modify: `apps/web/src/App.tsx`
- Modify: `apps/web/src/styles.css`

**Implementation details:**

- Add `useRecordingDetails(recordingId, enabled)`.
- Enable details loading once status is `completed`.
- Render summary text when present.
- Render transcript and summary sections with shadcn `Card` primitives and Tailwind spacing/typography utilities.
- Render transcript segments in order, with speaker label and time range if available.
- Handle missing summary/transcript gracefully because `RecordingDetails` may include `segments` before optional objects exist.

**Tests to write first:**

Verify:

- details are not requested before completion;
- summary renders when present;
- segments render in order;
- empty transcript/summary states are readable.

**Verification:**

Run:

```bash
pnpm --filter @soniq/web test
pnpm --filter @soniq/web typecheck
pnpm --filter @soniq/web build
```

Expected: tests, TypeScript, and build pass.

**Commit boundary:** Ask the user before committing. Suggested message: `feat: show recording transcript and summary`.

---

## Task 7: Add local Web UI documentation

**Objective:** Document how to run and verify the Web UI with the existing backend pipeline.

**Files:**

- Modify: `docs/development.md`
- Modify: `docs/roadmap.md`
- Modify: `docs/quality.md` if the frontend path boundary still says top-level `web`

**Documentation details:**

Update docs to explain:

- Web app lives at `apps/web`, not top-level `web`.
- API client lives at `packages/api-client`.
- Use `pnpm`, not npm or yarn.
- Tailwind CSS and shadcn/ui are part of the first Web UI milestone, with only the primitives needed for upload/status/results.
- Start backend dependencies with `make temporal-up`.
- Start Go API and worker in separate terminals.
- Start Web UI with `pnpm web:dev`.
- Upload a sample audio file such as `testdata/asr/mimo-tts/mp3/zh-four-speaker-standup.mp3`.
- Expected local flow: upload succeeds, status progresses, results display after completion.

**Verification:**

Run:

```bash
git diff --check
pnpm test
pnpm typecheck
pnpm web:build
```

Expected: docs have no whitespace errors and frontend checks pass.

**Commit boundary:** Ask the user before committing. Suggested message: `docs: document basic web UI workflow`.

---

## Task 8: Run end-to-end manual verification

**Objective:** Prove the Web UI works against the real local backend pipeline.

**Files:**

- No expected source changes unless verification reveals a bug.

**Prerequisites:**

- Local Postgres and Temporal are running.
- Go API can connect to Soniq Postgres with local ignored env values.
- Temporal worker is running with fake transcription and fake summarization providers.
- Web dev server is running.

**Verification flow:**

Run backend stack:

```bash
make temporal-up
make api
make worker
```

Run Web app:

```bash
pnpm web:dev
```

In the browser:

1. Open the Vite local URL.
2. Select `testdata/asr/mimo-tts/mp3/zh-four-speaker-standup.mp3`.
3. Choose workflow type `meeting`.
4. Set language `zh`.
5. Upload.
6. Confirm status reaches `completed`.
7. Confirm transcript segments and summary render.

Also verify through CLI if useful:

```bash
pnpm test
pnpm typecheck
pnpm web:build
go test ./... -count=1
git diff --check
```

Expected: Web UI shows completed processing results, frontend checks pass, backend tests still pass, and whitespace checks pass.

**Commit boundary:** If verification required source changes, ask before committing. Suggested final milestone message: `feat: add basic product web UI`.

---

## Acceptance criteria

- `pnpm install` creates a pnpm lockfile and recognizes all workspace packages.
- `packages/api-client` exposes typed functions for upload, status, and details.
- `apps/web` starts through `pnpm web:dev` and proxies API calls to the Go API in local development.
- A user can upload audio from the browser and see the created recording id/status.
- The UI polls until processing reaches a terminal state.
- Completed recordings show transcript segments and summary.
- `pnpm test`, `pnpm typecheck`, `pnpm web:build`, `go test ./... -count=1`, and `git diff --check` pass before the milestone is considered complete.
- No auth/workspace/provider settings have been introduced prematurely.

---

## Open questions before implementation

- Should the Go API eventually serve `apps/web/dist` in production, or should this milestone remain dev-server-only until deployment requirements are clearer?
- Should `packages/api-client` remain hand-written for now, or should a later task generate types from `backend/doc/openapi.yaml` once the API shape stabilizes further?
- Which shadcn/ui base color should be selected during CLI initialization? Default to `neutral` unless the user prefers another palette.
