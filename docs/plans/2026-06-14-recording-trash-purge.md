# Recording Trash Purge Plan

## Goal

Add permanent deletion for recordings that are already in Trash. Active
recording delete remains soft delete; hard delete is only available from Trash.

## Product Behavior

First pass:

- Trash rows get a `Delete forever` action.
- The action opens a destructive confirmation dialog.
- Confirming calls `DELETE /workspaces/{workspace_id}/recordings/{recording_id}/purge`.
- Purge is allowed only when the recording belongs to the workspace and
  `deleted_at IS NOT NULL`.
- Purged recordings disappear from Trash and cannot be restored.
- Active recordings cannot be purged directly.
- Original and normalized audio cleanup is tracked through an outbox table and
  retried by the worker when immediate cleanup fails.

Out of scope for the first pass:

- Empty Trash / bulk purge.
- Automatic retention windows.
- Undo after permanent delete.
- Full audit event UI.
- Temporal workflow cancellation for already-running workflows.

## API

Add:

```txt
DELETE /workspaces/{workspace_id}/recordings/{recording_id}/purge
```

Semantics:

- Requires auth, workspace access, and CSRF.
- Returns `204 No Content` when the recording row is purged.
- Returns `404 not_found` when the recording is missing, belongs to another
  workspace, or is not currently soft-deleted.
- Does not delete active recordings; callers must soft-delete first.

## Data And Storage

Data owned by a recording today:

- `recordings`
- `recording_audio_probes`
- `recording_normalized_audios`
- `recording_transcripts`
- `recording_transcript_segments`
- `recording_summaries`
- `recording_mind_maps`
- Original audio object from `recordings.audio_object_key`
- Normalized audio object from `recording_normalized_audios.object_key`

Recommended first implementation:

1. In the store layer, add `PurgeForWorkspace`.
2. In one Postgres transaction:
   - Select the soft-deleted recording by `workspace_id + recording_id`.
   - Lock it with `FOR UPDATE`.
   - Read the original and normalized object keys.
   - Explicitly delete child rows in business-owned order.
   - Delete the parent `recordings` row last.
3. Return the collected object keys to the API handler.
4. After the DB transaction commits, delete the collected object keys from the
   configured object store.
5. Mark artifact cleanup rows `deleted` on success or `failed` with
   `next_attempt_at` on failure.

Keep existing `ON DELETE CASCADE` constraints as an integrity backstop, but do
not rely on them as the primary business flow.

## Consistency Decision

Use Option B: Purge Artifact Outbox.

Reasoning:

- Permanent delete should have production-shaped failure behavior.
- DB deletion and artifact cleanup intent should be committed atomically.
- Storage deletion can fail independently of Postgres and must be retryable.
- We should not lose the object keys when deleting the recording row.

Implementation shape:

- Add a small `recording_purge_artifacts` table before deleting DB rows.
- Store every object key that must be deleted.
- Delete DB rows and persist artifact cleanup records in the same transaction.
- Attempt object deletion synchronously in the API handler after the transaction
  commits.
- Mark artifact cleanup rows as deleted when storage deletion succeeds.
- Keep failed/pending cleanup rows retryable by the worker cleanup loop.
- The worker claims retryable rows with `FOR UPDATE SKIP LOCKED`, calls
  `ObjectStore.DeleteObject`, and writes either `deleted_at` or the next retry
  time with backoff.

## Backend Files

- `backend/internal/recordings/store.go`
- `backend/internal/recordings/postgres_store.go`
- `backend/internal/recordings/postgres_store_test.go`
- `backend/internal/cleanup/recording_purge_artifacts.go`
- `backend/internal/cleanup/recording_purge_artifacts_test.go`
- `backend/internal/config/config.go`
- `backend/internal/config/config_test.go`
- `backend/cmd/api/main.go`
- `backend/cmd/worker/main.go`
- `backend/internal/api/router.go`
- `backend/internal/api/recording_handlers.go`
- `backend/internal/api/recordings_test.go`
- `backend/doc/openapi.yaml`
- `backend/migrations/0006_add_recording_purge_artifacts.*.sql`
- `backend/cmd/migrate`

## Frontend Files

- `packages/api-client/src/recordings.ts`
- `packages/api-client/src/recordings.test.ts`
- `apps/web/src/api/queries.ts`
- `apps/web/src/App.tsx`
- `apps/web/src/App.test.tsx`

## Test Strategy

Backend:

- Purge soft-deleted recording returns `204`.
- Purge missing, cross-workspace, and active recordings returns `404`.
- Active list, Trash list, details, status, retry, and restore cannot find a
  purged recording.
- Store test verifies the SQL requires `deleted_at IS NOT NULL`.
- Store test verifies child-table deletes happen before the parent recording
  delete.
- API test verifies storage object keys are deleted.
- API test verifies missing object store behavior.
- Cleanup runner test verifies successful object deletion marks artifacts
  deleted.
- Cleanup runner test verifies failed object deletion marks artifacts retryable.

Frontend:

- Trash row shows `Delete forever`.
- Confirmation dialog prevents accidental purge.
- Successful purge removes the item from Trash.
- Failed purge keeps the item visible and shows an error.

## Acceptance Criteria

- Only Trash recordings can be permanently deleted.
- Permanent delete removes the recording and all current DB child rows.
- Permanent delete attempts cleanup of original and normalized audio objects.
- Failed object cleanup remains retryable from `recording_purge_artifacts`.
- Active soft-delete, Trash list, and restore behavior remain unchanged.
- Automated backend and frontend checks pass.
