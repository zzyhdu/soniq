# Soniq Quality Standard

This document defines the project management and engineering quality rules for Soniq. It is intentionally lightweight: every rule should help the project move faster with less rework.

## Goals

Soniq should be developed as a durable, self-hostable audio intelligence platform. Quality means:

- small, verifiable increments;
- clear architecture boundaries;
- tests for behavior, not just implementation;
- privacy and provider-control designed in from the beginning;
- every milestone produces a runnable demo or a concrete verified artifact.

## Operating Model

Use this flow for non-trivial work:

```txt
Roadmap
  ↓
Milestone
  ↓
Implementation Plan
  ↓
Work Item
  ↓
TDD / CI Gate
  ↓
Review
  ↓
Release Note
```

### Roadmap

`docs/roadmap.md` defines product phases and should change slowly.

### Implementation Plans

Before starting a multi-step feature, create a plan under `docs/plans/`.

A good plan must include:

- goal;
- architecture approach;
- exact files to create or modify;
- bite-sized implementation tasks;
- test strategy;
- commands for verification;
- acceptance criteria.

### Work Items

Implementation plans are split into concrete work items. A work item can live in any tracker: GitHub Issues, Linear, a plain checklist, or another project-management board.

For local agent-assisted workflows, use the active external tracker or dashboard as the execution queue. That tracker is not part of the Soniq product architecture; it is project-management tooling for building Soniq.

Work items should be small enough to finish in less than one day, preferably a few focused hours.

Every work item should include:

- goal;
- scope and non-scope;
- expected file paths;
- acceptance criteria;
- verification commands.

## Definition of Done

A task is Done only when all relevant checks are satisfied:

- [ ] The implementation matches the stated scope.
- [ ] No unrelated features or opportunistic refactors were added.
- [ ] New behavior has tests, unless explicitly documented as non-behavioral setup.
- [ ] Tests pass locally.
- [ ] Format and lint checks pass locally.
- [ ] Errors include useful context and are not swallowed.
- [ ] Configuration changes are reflected in `.env.example` when applicable.
- [ ] Documentation is updated when behavior, setup, or architecture changes.
- [ ] A manual verification command or demo path is documented.
- [ ] No real secrets, tokens, private audio, transcripts, or personal data are committed.

## Testing Policy

Use test-driven development for behavior-bearing code.

Recommended cycle:

```txt
RED: write a failing test
GREEN: implement the minimum code to pass
REFACTOR: clean up while keeping tests green
```

Test priorities:

1. domain validation and provider-selection rules;
2. config loading and privacy-mode enforcement;
3. storage provider behavior;
4. API handlers;
5. Temporal workflow/activity behavior;
6. repository/database behavior;
7. frontend state and user-visible error states.

Do not require heavy integration tests for every small change early in the project, but every milestone should have at least one runnable end-to-end or smoke verification path.

## Required Local Commands

The project should converge on these commands:

```bash
make fmt
make lint
make test
make dev-up
make dev-down
```

Until these exist, each implementation plan must state the temporary equivalent commands.

## Architecture Boundaries

Keep boundaries explicit:

```txt
backend/cmd/*              process entrypoints only
backend/internal/api       HTTP handlers, middleware, routing
backend/internal/config    configuration loading and validation
backend/internal/domain    core types and domain validation
backend/internal/db        persistence and migrations
backend/internal/storage   object storage abstractions and implementations
backend/internal/providers external ASR/LLM/notification adapters
backend/internal/workflows Temporal workflow orchestration
backend/internal/activities Temporal activities with side effects
web                        frontend app
```

Rules:

- API handlers should not run long audio or AI jobs directly.
- Temporal workflows must stay deterministic.
- External I/O belongs in activities or providers, not workflow code.
- Provider choice must be configuration-driven.
- Provider-specific code must not leak across the codebase.

## Privacy and Security Rules

Audio, transcripts, summaries, and generated notes are sensitive data.

Required project behavior:

- External AI provider usage must be explicit.
- Private/offline mode must reject unapproved external model providers.
- Secrets must come from environment variables or a secret provider, not committed files.
- `.env` and `.env.*` must stay ignored except `.env.example`.
- Retention/deletion behavior must be documented when implemented.
- Audit events should cover upload, workflow start/completion/failure, transcript generation, summary generation, deletion, retention-policy changes, and provider-configuration changes.

## Temporal and Idempotency Rules

Temporal activities may be retried. Side effects must therefore be idempotent.

Use deterministic artifact keys such as:

```txt
workspaces/{workspace_id}/recordings/{recording_id}/audio/original
workspaces/{workspace_id}/recordings/{recording_id}/audio/normalized.wav
workspaces/{workspace_id}/recordings/{recording_id}/transcripts/raw.v1.json
workspaces/{workspace_id}/recordings/{recording_id}/summaries/summary.v1.json
```

Avoid unbounded duplicate artifacts on retries.

## Observability Rules

From the first runnable backend, logs should include:

- recording id;
- workspace id when available;
- workflow id and run id when available;
- current step;
- provider name;
- elapsed time for external calls;
- structured error context.

## Review Checklist

Before merging or committing a meaningful change, review:

- [ ] Is the scope still small and aligned with the task?
- [ ] Are the boundaries clean?
- [ ] Are tests meaningful and behavior-oriented?
- [ ] Does error handling preserve context?
- [ ] Can another developer reproduce the verification locally?
- [ ] Are security/privacy implications addressed?

## Git and Commit Policy

- Use small commits with clear messages: `feat:`, `fix:`, `docs:`, `chore:`, `refactor:`.
- Do not commit generated local data, secrets, dependency directories, or private artifacts.
- Commit only after the user confirms the exact commit scope.
