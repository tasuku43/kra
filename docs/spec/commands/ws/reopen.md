---
title: "`kra ws --act reopen`"
status: implemented
---

# `kra ws --act reopen [--commit] <id>`
# `kra ws --act reopen --dry-run --format json <id>`

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

4) Update workspace metadata/index

- Mark the workspace as `active`.
- Update `updated_at`.
- When `--commit` is enabled, record:
  - `reopened_commit_sha` (the commit created by this operation)

5) Commit the reopen change (`--commit` only)

- Commit message is fixed: `reopen: <id>`
- Commit on the current branch.
- Stage only paths touched by this operation, at minimum:
  - `workspaces/<id>/` (excluding `repos/**`, which is ignored)
  - removal of `archive/<id>/`

If `--commit` is enabled, unrelated changes must not be included in the commit.

6) Append an event

- Append `workspace_events(event_type='reopened', workspace_id='<id>', at=...)` (this is the source of truth
  for the reopen timestamp).

## Modes

- This command is explicit-id mode only.
- Interactive selection must use `kra ws select --archived`.
- JSON execution in phase 1 is `--dry-run` preflight only.

## FS metadata behavior

- `ws reopen` must read `workspaces/<id>/.kra.meta.json` (moved from archive) and recreate worktrees from
  `repos_restore`.
- Reopen flow must not require index-only rows to rebuild worktrees.
- On success, update `.kra.meta.json.workspace.status` to `active` atomically.
