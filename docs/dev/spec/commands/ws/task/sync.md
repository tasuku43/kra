---
title: "`kra ws task sync`"
status: implemented
---

# `kra ws task sync [--id <workspace-id>] [--current] [--select | --all] [--format human|json]`

## Purpose

Project the current `tasks.md` declaration into cmux sidebar task pills for one or more active workspaces.

## Behavior

- Resolve target workspace using the shared `ws` targeting contract.
- `--all` resolves all active workspaces non-interactively and syncs them in parallel.
- Command is valid only for active workspaces.
- Read structured tasks from `tasks.md` using the workspace task contract.
- Build desired cmux task pills from the current declaration:
  - `todo` uses `○ ...`
  - `doing` uses `● ...`
  - `blocked` uses `▲ ...`
  - `done` uses `✔ ...`
- Reconcile only the cmux `task:` namespace.
- Reconcile is full, declarative, and order-preserving:
  - all existing `task:` entries are cleared first
  - current tasks are then inserted again in `tasks.md` order
  - non-`task:` cmux status entries are untouched
- This includes the empty structured-task case:
  - if `## Tasks` exists but contains zero valid structured tasks, all existing `task:*` entries are
    cleared
- Invalid task-like blocks fail closed:
  - if `## Tasks` contains a `### TASK-...` block that violates the task contract, sync returns an
    error and does not clear existing `task:*` entries
- Projected entries use:
  - key: `task:TASK-001`
  - icon: `checklist`
  - value: `<prefix> TASK-001 <title>`
- `todo` should use a quiet default-text/white color so backlog stays visible without overpowering
  `doing` and `blocked`.
- If no mapped cmux workspace exists, return success with `state=skipped`.
- In `--all` mode, each workspace is handled independently:
  - one workspace failure must not prevent other targets from syncing
  - skipped workspaces still count as successful results with `state=skipped`
  - overall command returns partial failure when any workspace fails

## Human output

- Render one result summary line only:
  - `task sync: applied N (set X, cleared Y)`
  - or `task sync skipped: ...`
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
