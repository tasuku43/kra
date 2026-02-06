---
title: "`gionx ws reopen`"
status: implemented
---

# `gionx ws reopen <id>`

## Purpose

Reopen a previously closed workspace:
- move `archive/<id>` back to `workspaces/<id>`
- recreate Git worktrees for the workspace from the state store

This supports the common case where a task was considered done, but more work is needed.

## Behavior (MVP)

### Preconditions

- `archive/<id>` must exist
- `workspaces/<id>` must not exist
- Workspace `<id>` must exist in the global state store and be `archived`

### Steps

1) Restore workspace directory

- Move `GIONX_ROOT/archive/<id>/` to `GIONX_ROOT/workspaces/<id>/` using an atomic rename.

2) Recreate repos directory

- Ensure `GIONX_ROOT/workspaces/<id>/repos/` exists.

3) Recreate worktrees from state store

For each recorded workspace repo entry:
- Ensure the bare repo exists in the repo pool and `fetch` (prefetch should start as soon as possible)
- Create a worktree at `GIONX_ROOT/workspaces/<id>/repos/<alias>`
- Check out the recorded branch:
  - if the remote branch exists, check it out (track it)
  - otherwise, create it from the default branch
- If the branch is already checked out by another worktree, error (Git worktree constraint).

4) Update state store

- Mark the workspace as `active`.
- Update `updated_at`.
- Record:
  - `reopened_commit_sha` (the commit created by this operation)

5) Commit the reopen change (always)

- Commit message is fixed: `reopen: <id>`
- Commit on the current branch.
- Stage only paths touched by this operation, at minimum:
  - `workspaces/<id>/` (excluding `repos/**`, which is ignored)
  - removal of `archive/<id>/`

If the Git working tree has unrelated changes, this command must not include them in the commit.

6) Append an event

- Append `workspace_events(event_type='reopened', workspace_id='<id>', at=...)` (this is the source of truth
  for the reopen timestamp).
