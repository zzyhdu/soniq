# Recording Trash Restore Plan

## Goal

Expose soft-deleted recordings in a workspace Trash view and allow users to
restore them to the active recording library.

## Product Behavior

- `GET /workspaces/{workspace_id}/recordings/trash` lists soft-deleted
  recordings for the current workspace, newest deleted first.
- `POST /workspaces/{workspace_id}/recordings/{recording_id}/restore` clears
  deletion metadata and makes the recording visible in the active library again.
- The Web UI adds a Trash navigation entry, shows deleted recordings, and offers
  Restore per row.
- Restore does not re-run processing and does not modify existing transcript,
  summary, mind map, audio probe, normalized audio metadata, or storage
  artifacts.
- Permanent purge remains a later milestone.

## Architecture

- Trash and restore are business operations in the API/store layer.
- Active recording reads continue to use `deleted_at IS NULL`.
- Trash reads use `deleted_at IS NOT NULL`.
- Restore requires a workspace-scoped recording whose `deleted_at` is not null,
  then clears `deleted_at` and `deleted_by_user_id`.
- No new migration is needed because soft-delete metadata already exists.

## Files

- `backend/internal/recordings/store.go`
- `backend/internal/recordings/postgres_store.go`
- `backend/internal/api/router.go`
- `backend/internal/api/recording_handlers.go`
- `backend/internal/api/recordings_test.go`
- `backend/internal/recordings/postgres_store_test.go`
- `packages/api-client/src/recordings.ts`
- `packages/api-client/src/recordings.test.ts`
- `apps/web/src/api/queries.ts`
- `apps/web/src/App.tsx`
- `apps/web/src/App.test.tsx`
- `backend/doc/openapi.yaml`
- `docs/workflows.md`

## Test Strategy

- Backend API tests for listing Trash, restoring a deleted recording, active
  list visibility after restore, and restore not found for missing/cross
  workspace recordings.
- Store tests for `deleted_at IS NOT NULL` Trash listing and restore query
  clearing deletion metadata.
- API client tests for Trash list URL and restore POST path encoding.
- Web test for deleting a recording, viewing it in Trash, restoring it, and
  seeing it return to active recordings.

## Acceptance Criteria

- Deleted recordings are hidden from the active library but visible in Trash.
- Restoring a Trash item removes it from Trash and returns it to active
  recordings.
- Restore only affects recordings in the selected workspace.
- Automated backend and frontend checks pass.
