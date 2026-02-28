---
title: "`kra ws reopen`"
status: implemented
---

# `kra ws reopen [--no-commit] [--commit] <id>`
# `kra ws reopen --dry-run --format json <id>`

## Purpose

Reopen a previously closed workspace:
- move `archive/<id>` back to `workspaces/<id>`
- recreate Git worktrees for the workspace from metadata

This supports the common case where a task was considered done, but more work is needed.

## Behavior (MVP)

### Preconditions

- `archive/<id>` must exist
- `workspaces/<id>` must not exist
- Workspace `<id>` metadata must exist and be `archived`

### Steps

1) Restore workspace directory

- Move `KRA_ROOT/archive/<id>/` to `KRA_ROOT/workspaces/<id>/` using an atomic rename.

2) Recreate repos directory

- Ensure `KRA_ROOT/workspaces/<id>/repos/` exists.

3) Recreate worktrees from metadata

For each recorded workspace repo entry:
- Ensure the bare repo exists in the repo pool and `fetch` (prefetch should start as soon as possible)
- Create a worktree at `KRA_ROOT/workspaces/<id>/repos/<alias>`
- Check out the recorded branch:
  - if the remote branch exists, check it out (track it)
  - otherwise, create it from the default branch
- If the branch is already checked out by another worktree, error (Git worktree constraint).

4) Commit pre-reopen snapshot (default; skipped by `--no-commit`)

- Commit message is fixed: `reopen-pre: <id>`
- Stage allowlist: `archive/<id>/`
- Preserve unrelated staged changes outside allowlist.
- `--commit` is accepted for backward compatibility and keeps default behavior.

5) Update workspace metadata/index

- Mark the workspace as `active`.
- Update `updated_at`.
- In default commit mode, record:
  - `reopened_commit_sha` (the commit created by this operation)

6) Commit the reopen change (default; skipped by `--no-commit`)

- Commit message is fixed: `reopen: <id>`
- Commit on the current branch.
- Stage only paths touched by this operation, at minimum:
  - `workspaces/<id>/` (excluding `repos/**`, which is ignored)
  - removal of `archive/<id>/`
  - `.kra/state/workspace-baselines/<id>.json`
  - `.kra/state/workspace-workstate.json`

If post-reopen commit fails, do not auto-rollback filesystem rename; keep reopened state and return error.
In default commit mode, unrelated changes must not be included in lifecycle commits.

7) Append an event

- Append `workspace_events(event_type='reopened', workspace_id='<id>', at=...)` (this is the source of truth
  for the reopen timestamp).

## Modes

- This command is explicit-id mode only.
- Interactive selection must use `kra ws reopen --select --archived`.
- JSON execution in phase 1 is `--dry-run` preflight only.

## FS metadata behavior

- `ws reopen` must read `workspaces/<id>/.kra.meta.json` (moved from archive) and recreate worktrees from
  `repos_restore`.
- Reopen flow must not require index-only rows to rebuild worktrees.
- On success, update `.kra.meta.json.workspace.status` to `active` atomically.
- On successful reopen, refresh runtime baseline/cache for `<id>`:
  - recreate `.kra/state/workspace-baselines/<id>.json` from reopened state
  - clear `.kra/state/workspace-workstate.json` entry for `<id>` (state restarts from `todo`)
