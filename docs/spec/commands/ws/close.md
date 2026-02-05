# `gionx ws close <id>`

## Purpose

Close a workspace:
- keep investigation notes and artifacts as an archive
- remove Git worktrees to keep the working area clean

This is the primary "task completed" flow in `gionx`.

## Behavior (MVP)

### Preconditions

- `GIONX_ROOT` must be a Git working tree (or `gionx init` must have been run).
- Workspace `<id>` must exist in the global state store.

### Steps

1) Inspect repo risk (live)

- For each repo under `GIONX_ROOT/workspaces/<id>/repos/<alias>`:
  - compute risk similar to `gion` (dirty / unpushed / diverged / unknown / clean)
- If any repo is not clean, prompt for confirmation before continuing.

2) Remove worktrees

- Remove each worktree under `workspaces/<id>/repos/<alias>`.
- Remove `workspaces/<id>/repos/` if it becomes empty.

3) Archive the workspace contents

- Move `GIONX_ROOT/workspaces/<id>/` to `GIONX_ROOT/archive/<id>/` using an atomic rename.
- After this step, `GIONX_ROOT/workspaces/<id>/` should not exist.

4) Update state store

- Mark the workspace as `closed`.
- Update `updated_at`.
- Record:
  - `archived_at`
  - `archived_commit_sha` (the commit created by this operation)

5) Commit the archive change (always)

- Commit message is fixed: `archive: <id>`
- Commit on the current branch.
- Stage only paths touched by this operation, at minimum:
  - `archive/<id>/`
  - removal of `workspaces/<id>/` (and any emptied parent folders as needed)
 - After committing, store the commit SHA in the state store as `archived_commit_sha`.

If the Git working tree has unrelated changes, this command must not include them in the commit.

## Notes

- `workspaces/**/repos/**` is ignored in `.gitignore`, but `ws close` must still delete worktrees
  (archives should not contain repos).
