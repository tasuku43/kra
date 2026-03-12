---
title: "Lifecycle Journal"
status: implemented
---

# Lifecycle Journal

## Purpose

Define a root-local recovery journal for workspace lifecycle operations that have
irreversible filesystem phases.

The first journalized flow is `ws close`.

The journal exists to make interrupted operations:

- detectable by `doctor`
- resumable by `doctor --fix` when safe
- auditable without relying on SQL or shell history

## File location

- `KRA_ROOT/.kra/state/operations/ws-close/<id>.json`

One file exists per in-flight close operation.

## Schema (v1)

```json
{
  "version": 1,
  "operation": "ws-close",
  "workspace_id": "SREP-1234",
  "started_at": 1772800000,
  "updated_at": 1772800042,
  "phase": "workspace_renamed",
  "commit_enabled": true,
  "close_pre_commit_sha": "abc123",
  "archive_commit_sha": "",
  "workspace_path_present": false,
  "archive_path_present": true
}
```

## Phases

`ws close` advances monotonically through:

1. `risk_checked`
2. `close_pre_committed`
3. `worktrees_removed`
4. `metadata_archived`
5. `workspace_renamed`
6. `archive_committed`
7. `completed`

The journal file is removed after successful completion.

## Semantics

- The journal is runtime operational state, not canonical workspace state.
- Canonical workspace truth remains:
  - directory layout under `workspaces/` and `archive/`
  - workspace metadata in `.kra.meta.json`
- Missing journal file means "no known in-flight lifecycle operation".
- Journal presence with `phase != completed` means lifecycle recovery must be considered before a new close starts.

## Persistence rules

- Save operations must ensure parent directories exist.
- Writes must use temp-file + rename replacement semantics.
- Phase updates must be monotonic.
- Journal content should be sufficient for deterministic post-rename recovery without recomputing prior phases.
- `commit_enabled` records whether post-rename archive commit should be resumed or skipped.

## Recovery contract

`doctor` should classify an unfinished journal as one of:

- `ws_close_resume_ready`
- `ws_close_reset_ready`
- `ws_close_manual_required`

`doctor --fix` may resume only when the journal and filesystem state agree on a safe post-rename recovery path.
- phase `workspace_renamed`:
  - resume archive commit when `commit_enabled=true`
  - skip directly to finalization when `commit_enabled=false`
- phase `archive_committed`:
  - resume finalization only
- phase `risk_checked`:
  - clear the journal only when filesystem state still matches a pre-rename active workspace
  - this is for retries after pre-irreversible failures (for example pre-close snapshot/allowlist failures)

## Non-goals

- general workflow history
- per-command analytics
- replacing canonical workspace metadata
