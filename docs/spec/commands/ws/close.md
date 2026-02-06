---
title: "`gionx ws close`"
status: implemented
pending:
  - UX-WS-002-shared-selector
  - UX-WS-003-bulk-close-risk-gate
---

# `gionx ws close [<id>]`

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

- Mark the workspace as `archived`.
- Update `updated_at`.
- Record:
  - `archived_commit_sha` (the commit created by this operation)

5) Commit the archive change (always)

- Commit message is fixed: `archive: <id>`
- Commit on the current branch.
- Stage only paths touched by this operation, at minimum:
  - `archive/<id>/`
  - removal of `workspaces/<id>/` (and any emptied parent folders as needed)
 - After committing, store the commit SHA in the state store as `archived_commit_sha`.

6) Append an event

- Append `workspace_events(event_type='archived', workspace_id='<id>', at=...)` (this is the source of truth
  for the archive timestamp).

If the Git working tree has unrelated changes, this command must not include them in the commit.

## Notes

- `workspaces/**/repos/**` is ignored in `.gitignore`, but `ws close` must still delete worktrees
  (archives should not contain repos).

## Next UX iteration (planned)

### Selector mode and direct mode

- If `<id>` is provided, run existing direct mode.
- If `<id>` is omitted, launch shared selector UI (`commands/ws/selector.md`) in `active` scope.
- Selector mode allows multiple selection.
- Non-TTY invocation without `<id>` must error (no fallback mode).

### Bulk close safety gate

- After selector confirmation, evaluate risk for all selected workspaces.
- `risky` is defined as `dirty` / `unpushed` / `diverged` (plus `unknown` as non-safe).
- If any selected workspace is non-clean (`risky` or `unknown`), abort the whole operation (no partial close).
- Risk label semantics and severity follow `commands/ws/selector.md`.

### Commit strictness (non-repo files)

- Policy: non-`repos/` contents must be captured in the archive commit.
- Stage by allowlist only:
  - `workspaces/<id>/`
  - `archive/<id>/`
- Verify staged paths are a strict subset of the allowlist; otherwise abort.
- If `gitignore` causes any non-`repos/` files under selected workspace to be unstageable, abort.
