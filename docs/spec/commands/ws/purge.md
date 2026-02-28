---
title: "`kra ws purge`"
status: implemented
---

# `kra ws purge [--no-prompt --force] [--no-commit] [--commit] <id>`
# `kra ws purge --dry-run --format json <id>`

## Purpose

Permanently delete a workspace and its archive from `KRA_ROOT`, and remove the workspace snapshot
from runtime index data.

This is a destructive operation. It is separate from `ws close`, which keeps an archive.

## Behavior (MVP)

### Preconditions

- Workspace `<id>` should exist as metadata/index entry (either `active` or `archived`).

### Confirmation

- Always prompt for confirmation.
- In a no-prompt mode, this command must refuse unless an explicit force flag is provided.

### Steps

1) If the workspace is `active`, inspect repo risk (live)

- For each repo under `KRA_ROOT/workspaces/<id>/repos/<alias>`:
  - compute risk similar to `gion` (dirty / unpushed / diverged / unknown / clean)
- If any repo is not clean, require an additional confirmation before continuing.

2) Commit pre-purge snapshot (default; skipped by `--no-commit`)

- Commit message is fixed: `purge-pre: <id>`
- Stage allowlist: `archive/<id>/`
- Preserve unrelated staged changes outside allowlist.
- `--commit` is accepted for backward compatibility and keeps default behavior.

3) Remove worktrees (if present)

- Remove each worktree under `workspaces/<id>/repos/<alias>`.
- Remove `workspaces/<id>/repos/` if it becomes empty.

4) Delete workspace and archive directories (if present)

- Delete `KRA_ROOT/workspaces/<id>/` if it exists.
- Delete `KRA_ROOT/archive/<id>/` if it exists.

5) Update metadata/index

- Append `workspace_events(event_type='purged', workspace_id='<id>', workspace_generation=<gen>, at=..., meta='{}')`.
- Remove the workspace snapshot row from `workspaces` for `<id>`.
  - This enables reusing the same workspace ID later as a new generation.

6) Commit the purge change (default; skipped by `--no-commit`)

- Commit message is fixed: `purge: <id>`
- Commit on the current branch.
- Stage only paths touched by this operation, at minimum:
  - removal of `workspaces/<id>/`
  - removal of `archive/<id>/`
  - `.kra/state/workspace-baselines/<id>.json`
  - `.kra/state/workspace-workstate.json`

In default commit mode, unrelated changes must not be included in lifecycle commits.

## Modes

- This command is explicit-id mode only.
- Interactive selection must use `kra ws purge --select --archived`.
- JSON execution in phase 1 is `--dry-run` preflight only.

## Purge guard policy

- Workspace metadata contains purge guard state (`protection.purge_guard.enabled` in `.kra.meta.json`).
- `ws create` initializes purge guard as enabled.
- `ws purge` must fail with conflict guidance while purge guard is enabled:
  - `hint: run 'kra ws unlock <id>' before purge`
- `ws close`/`ws reopen` preserve purge guard value.
- Purge execution is archived-only in current policy.
- On successful purge, remove runtime baseline/cache entries for `<id>`:
  - `.kra/state/workspace-baselines/<id>.json`
  - `.kra/state/workspace-workstate.json` entry for `<id>`
