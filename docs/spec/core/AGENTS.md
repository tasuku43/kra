---
title: "AGENTS.md"
status: planned
---

# AGENTS.md

## Goal

Provide a default, repo-local guide for coding agents (and humans) to understand:
- what `gionx` is optimizing for
- what each directory under `GIONX_ROOT` means
- where to put notes and artifacts
- how to safely close/archive a workspace

## Files

`gionx` generates two `AGENTS.md` files in the no-template MVP:

- `GIONX_ROOT/AGENTS.md`
  - global guidance for using `gionx`
  - expected directory structure
  - policies (what is tracked in Git, what is ignored)
- `GIONX_ROOT/workspaces/<id>/AGENTS.md`
  - task-specific context for the workspace
  - the meaning of `notes/`, `artifacts/`, and `repos/`

## Content (recommended skeleton)

### `GIONX_ROOT/AGENTS.md`

Include:

- Purpose: manage task workspaces with safe archiving
- Directory map:
  - `workspaces/<id>/notes/`: text-first logs (investigation notes, decisions, links)
  - `workspaces/<id>/artifacts/`: file-first evidence (screenshots, log dumps, curl outputs, PoCs)
  - `workspaces/<id>/repos/<alias>/`: git worktrees (NOT Git-tracked)
  - `archive/<id>/`: archived workspaces (Git-tracked)
- Workflow:
  - `gionx ws create` -> `gionx ws add-repo` -> work -> `gionx ws close`
- Git policy:
  - track: everything except `workspaces/**/repos/**`
  - ignore: `workspaces/**/repos/**`

### `GIONX_ROOT/workspaces/<id>/AGENTS.md`

Include:

- Workspace ID: `<id>`
- Description: (user-provided at creation time)
- Notes:
  - where to write investigation notes
  - where to store artifacts
  - keep code changes inside `repos/` worktrees only
- Closing:
  - run `gionx ws close <id>` to archive and remove worktrees

## Notes

- `gionx` does not define a formal priority/override mechanism between the two files.
  Agents are expected to load relevant files based on their working directory and file touches.
