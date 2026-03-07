---
title: "`kra ws task block`"
status: planned
---

# `kra ws task block [--id <workspace-id>] [--current] [--select] <task-id> [--format human|json]`

## Purpose

Transition one structured task to `blocked`.

## Behavior

- Resolve target workspace using the shared `ws` targeting contract.
- Command is valid only for active workspaces.
- Target task is resolved by stable `task-id`.
- Preserve task title and description.
- Allowed transitions:
  - `todo -> blocked`
  - `doing -> blocked`
- Idempotent transition:
  - `blocked -> blocked` succeeds with `changed=false`
- Invalid transition:
  - `done -> blocked` fails with conflict
- Missing task ID fails with `not_found`.

## Human output

- Success output should confirm:
  - workspace ID
  - task ID
  - previous status
  - new status (`blocked`)

## JSON envelope

- `ok`
- `action=ws.task.block`
- `workspace_id`
- `result`:
  - `task`:
    - `id`
    - `title`
    - `previous_status`
    - `status` (`blocked`)
    - `changed`
- `error`
