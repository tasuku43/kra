---
title: "`kra ws task sync`"
status: implemented
---

# `kra ws task sync [--id <workspace-id>] [--current] [--select | --all] [--format human|json]`

## Purpose

Deprecated compatibility command. Task display is handled by `kra ws task tui` and cmux Dock, not
by cmux sidebar task pills.

## Behavior

- Resolve target workspace using the shared `ws` targeting contract.
- `--all` resolves all active workspaces non-interactively.
- Command is valid only for active workspaces.
- The command does not read or mutate `tasks.md`.
- The command does not list, set, or clear cmux status entries.
- Return success with `state=skipped` and a deprecation warning.
- In `--all` mode, return one skipped result per active workspace.

## Human output

- Render one result summary line:
  - `task sync skipped: task sync is deprecated; use kra ws task tui or cmux Dock instead`
- Also include the target workspace ID in the result section.
- In `--all` mode, render an aggregate summary plus per-workspace sync lines.

## JSON envelope

- `ok`
- `action=ws.task.sync`
- `workspace_id`
- `result`:
  - `sync`:
    - `state`
    - `targets`
    - `set`
    - `cleared`
    - `warning` (optional)
- In `--all` mode, `result` is batch-shaped:
  - `count`
  - `succeeded`
  - `failed`
  - `skipped`
  - `targets`
  - `set`
  - `cleared`
  - `workspaces[]`
  - `failures[]`
- `error`
