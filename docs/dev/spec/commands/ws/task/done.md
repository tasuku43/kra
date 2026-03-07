---
title: "`kra ws task done`"
status: planned
---

# `kra ws task done [--id <workspace-id>] [--current] [--select] <task-id> [--format human|json]`

## Purpose

Transition one structured task to `done`.

## Behavior

- Resolve target workspace using the shared `ws` targeting contract.
- Command is valid only for active workspaces.
- Target task is resolved by stable `task-id`.
- Preserve task title and description.
- Allowed transitions:
  - `todo -> done`
  - `doing -> done`
  - `blocked -> done`
- Idempotent transition:
  - `done -> done` succeeds with `changed=false`
- Missing task ID fails with `not_found`.

## Human output

- Success output should confirm:
  - workspace ID
  - task ID
  - previous status
  - new status (`done`)

## JSON envelope

- `ok`
- `action=ws.task.done`
- `workspace_id`
- `result`:
  - `task`:
    - `id`
    - `title`
    - `previous_status`
    - `status` (`done`)
    - `changed`
- `error`
