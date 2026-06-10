# AGENTS.md

## Project Context

Soniq is a self-hostable audio intelligence platform for recording upload,
transcription, summarization, and structured notes. The backend is a Go modular
monolith plus Temporal workers. The frontend lives in `apps/web` and shared
TypeScript API code lives in `packages/api-client`.

Current milestone: basic product Web UI. The latest active plan should be found
under `docs/plans/`; at the time this file was added, the active plan was
`docs/plans/2026-06-07-basic-product-web-ui.md`.

## Read First

For non-trivial work, read only the relevant sections of these files before
editing:

- `docs/quality.md` for development process, Definition of Done, testing, and
  commit policy.
- `docs/development.md` for local commands and runtime setup.
- `docs/roadmap.md` for current milestone status.
- The latest applicable file in `docs/plans/` for task scope and acceptance
  criteria.
- `docs/architecture.md`, `docs/workflows.md`, and `docs/providers.md` when
  touching API, Temporal, activities, storage, ASR, or LLM provider boundaries.

## Working Rules

- Keep changes small, scoped, and aligned with the active plan.
- For behavior-bearing code, use TDD where practical: write or update tests
  first, then implement the smallest passing change.
- Do not add unrelated refactors, new architecture, or premature product
  features.
- Update documentation when behavior, setup, configuration, or architecture
  changes.
- Do not commit generated local data, secrets, private audio, transcripts,
  summaries, or personal data.
- Do not create a git commit unless the user confirms the exact commit scope.

## Architecture Rules

- API handlers are synchronous user-facing boundaries; they must not run long
  audio or AI jobs directly.
- Temporal workflows must stay deterministic. External I/O belongs in
  activities or providers.
- Temporal activities can be retried, so side effects must be idempotent and
  should use deterministic artifact keys.
- Provider choice must be configuration-driven.
- Provider-specific code must not leak across unrelated packages.
- Soniq application Postgres is separate from Temporal-owned Postgres. Do not
  apply Soniq migrations to Temporal's database.
- Default automated smoke tests should use deterministic fake model providers.
  Real external ASR/LLM provider calls are manual and opt-in only.

## Verification Commands

Backend checks:

```bash
make fmt
make lint
make test
```

Frontend/workspace checks:

```bash
pnpm test
pnpm typecheck
pnpm web:build
```

Full local pipeline smoke, when the task warrants end-to-end verification:

```bash
make smoke-postgres-temporal
```

Web UI local run:

```bash
make temporal-up
make api
make worker
pnpm web:dev
```

## Current Web UI Milestone

The basic Web UI plan is complete through upload, status polling,
transcript/summary result display, local Web UI documentation, and end-to-end
manual verification against the real local backend pipeline.

Keep the UI local/developer focused for now: no auth, workspace settings,
provider settings, or production static serving unless explicitly requested.
