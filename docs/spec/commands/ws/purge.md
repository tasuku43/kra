---
title: "`gionx ws purge`"
status: implemented
---

# `gionx ws purge [--no-prompt --force] <id>`

## Purpose

Permanently delete a workspace and its archive from `GIONX_ROOT`, and remove the workspace snapshot
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

- For each repo under `GIONX_ROOT/workspaces/<id>/repos/<alias>`:
  - compute risk similar to `gion` (dirty / unpushed / diverged / unknown / clean)
- If any repo is not clean, require an additional confirmation before continuing.

2) Remove worktrees (if present)

- Remove each worktree under `workspaces/<id>/repos/<alias>`.
- Remove `workspaces/<id>/repos/` if it becomes empty.

3) Delete workspace and archive directories (if present)

- Delete `GIONX_ROOT/workspaces/<id>/` if it exists.
- Delete `GIONX_ROOT/archive/<id>/` if it exists.

4) Update metadata/index

- Append `workspace_events(event_type='purged', workspace_id='<id>', workspace_generation=<gen>, at=..., meta='{}')`.
- Remove the workspace snapshot row from `workspaces` for `<id>`.
  - This enables reusing the same workspace ID later as a new generation.

5) Commit the purge change (always)

- Commit message is fixed: `purge: <id>`
- Commit on the current branch.
- Stage only paths touched by this operation, at minimum:
  - removal of `workspaces/<id>/`
  - removal of `archive/<id>/`

If the Git working tree has unrelated changes, this command must not include them in the commit.

## Modes

- This command is explicit-id mode only.
- Interactive selection must use `gionx ws list --select --archived`.
