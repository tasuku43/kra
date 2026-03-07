---
title: "`kra ws task start`"
status: planned
---

# `kra ws task start [--id <workspace-id>] [--current] [--select] <task-id> [--format human|json]`

## Purpose

Transition one structured task to `doing`.

## Behavior

- Resolve target workspace using the shared `ws` targeting contract.
- Command is valid only for active workspaces.
- Target task is resolved by stable `task-id`.
- Preserve task title and description.
- Allowed transitions:
  - `todo -> doing`
  - `blocked -> doing`
- Idempotent transition:
  - `doing -> doing` succeeds with `changed=false`
- Invalid transition:
  - `done -> doing` fails with conflict
- Missing task ID fails with `not_found`.

## Human output

- Success output should confirm:
  - workspace ID
  - task ID
  - previous status
  - new status (`doing`)

## JSON envelope

- `ok`
- `action=ws.task.start`
- `workspace_id`
- `result`:
  - `task`:
    - `id`
    - `title`
    - `previous_status`
    - `status` (`doing`)
    - `changed`
- `error`
