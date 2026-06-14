# Recording Soft Delete Plan

## Goal

Add user-facing recording deletion without immediately destroying data. The
workspace delete action will move a recording out of the active library by
marking it as deleted. A later Trash milestone will expose deleted recordings,
restore, and permanent purge.

## Product Behavior

Phase 1, implemented now:

- The recording detail Delete action opens a confirmation dialog.
- Confirming delete calls `DELETE /workspaces/{workspace_id}/recordings/{recording_id}`.
- The API soft deletes the recording by setting `deleted_at` and
  `deleted_by_user_id`.
- Normal list, detail, status, retry, and result endpoints exclude deleted
  recordings.
- The Web UI removes the deleted item from the Recording Library and clears the
  detail pane.
- No audio or generated artifacts are physically deleted in Phase 1.

Future Trash phase:

- Add `GET /workspaces/{workspace_id}/recordings/trash`.
- Add restore for soft-deleted recordings.
- Add permanent purge from Trash.
- Purge deletes database children, storage artifacts, search/vector artifacts,
  and writes audit events. Temporal history still follows Temporal retention.

## Architecture

- Soft delete is a business operation, not a cascade operation.
- `recordings` gains nullable deletion metadata:
  - `deleted_at TIMESTAMPTZ`
  - `deleted_by_user_id TEXT REFERENCES users(id)`
- Existing child table `ON DELETE CASCADE` constraints remain as a hard-delete
  integrity backstop for the future purge flow.
- Active recording reads use `deleted_at IS NULL`.
- Workflow activities that look up active recordings will stop finding deleted
  recordings after deletion; a later workflow-cancel pass can reduce wasted
  processing for in-flight deletes.

## Files

- `backend/migrations/0005_add_recording_soft_delete.*.sql`
- `backend/internal/domain/recording.go`
- `backend/internal/recordings/store.go`
- `backend/internal/recordings/postgres_store.go`
- `backend/internal/api/router.go`
- `backend/internal/api/recording_handlers.go`
- `backend/internal/api/recording_responses.go`
- `backend/internal/api/recordings_test.go`
- `packages/api-client/src/recordings.ts`
- `packages/api-client/src/recordings.test.ts`
- `apps/web/src/api/queries.ts`
- `apps/web/src/App.tsx`
- `apps/web/src/App.test.tsx`
- `backend/doc/openapi.yaml`
- `docs/workflows.md`

## Test Strategy

- Backend API tests for successful soft delete, missing recording, deleted
  recording hidden from reads, and unauthorized workspace access.
- Store tests for `deleted_at IS NULL` filters and soft-delete update query.
- API client test for DELETE path encoding and 204 handling.
- Web test for confirmation dialog, successful removal from the list, and
  failed deletion error display.

## Acceptance Criteria

- Deleting a recording from the detail header removes it from the active library
  without physically purging DB child rows or artifacts.
- Refreshing the active library does not show deleted recordings.
- Direct active reads for deleted recordings return `404 not_found`.
- Automated backend and frontend tests pass.
