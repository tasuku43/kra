---
title: "`kra ws task sync`"
status: implemented
---

# `kra ws task sync [--id <workspace-id>] [--current] [--select] [--format human|json]`

## Purpose

Project the current `tasks.md` declaration into cmux sidebar task pills for one active workspace.

## Behavior

- Resolve target workspace using the shared `ws` targeting contract.
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

## Human output

- Render one result summary line only:
  - `task sync: applied N (set X, cleared Y)`
  - or `task sync skipped: ...`
- Also include the target workspace ID in the result section.

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
- `error`
