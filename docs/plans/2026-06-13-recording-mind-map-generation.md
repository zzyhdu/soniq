# Recording Mind Map Generation Plan

**Goal:** Generate a mind map artifact for each processed recording so users can scan the key structure of a transcript/summary, retrieve it through the recording details API, and view it in the Web UI.

## Scope

- Persist one latest mind map per recording in Soniq Postgres.
- Generate the mind map in the Temporal processing workflow after summarization and before completion.
- Keep deterministic fake-provider behavior for automated tests and smoke.
- Extend the OpenAI-compatible LLM adapter to generate structured mind map JSON for real-provider runs.
- Return the mind map in recording details.
- Render a simple tree-style mind map in the Web UI.

## Non-Scope

- No editable mind map canvas.
- No manual regenerate endpoint.
- No separate workflow status for mind map generation.
- No provider-specific UI controls.
- No graph-layout dependency until the artifact contract is proven.

## Data Contract

Store one row in `recording_mind_maps`:

- `recording_id`
- `provider`
- `model`
- `title`
- `root_json`
- `content_markdown`
- `raw_result_json`
- `generated_at`
- timestamps

API response shape:

```json
{
  "mind_map": {
    "recording_id": "rec_x",
    "provider": "fake_llm",
    "model": "fake-mind-map-v1",
    "title": "Weekly sync",
    "root": {
      "label": "Weekly sync",
      "children": [
        { "label": "Launch status", "children": [] }
      ]
    },
    "content_markdown": "- Weekly sync\n  - Launch status",
    "generated_at": "2026-06-13T00:00:00Z"
  }
}
```

## Implementation Tasks

1. Add migration and recording store methods for mind maps.
2. Add mind map provider contract, fake implementation, OpenAI-compatible implementation, and activity persistence.
3. Add workflow step after summary generation.
4. Extend details API, OpenAPI, and TypeScript API client DTOs.
5. Render the mind map in `RecordingResults`.
6. Update docs and smoke expectations if needed.

## Verification

```bash
make fmt
make lint
make test
pnpm test
pnpm typecheck
pnpm web:build
```
