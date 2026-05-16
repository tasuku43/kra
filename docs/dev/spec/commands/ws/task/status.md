---
title: "`kra ws task status`"
status: implemented
---

# `kra ws task status [--id <workspace-id>] [--current] [--select] <task-id> <todo|doing|blocked|done> [--format human|json]`

## Purpose

Set one structured task to an explicit status.

## Behavior

- Resolve target workspace using the shared `ws` targeting contract.
- Command is valid only for active workspaces.
- Target task is resolved by stable `task-id`.
- Preserve task title and description.
- Allowed transitions:
  - `todo -> doing|blocked|done`
  - `doing -> todo|blocked|done`
  - `blocked -> todo|doing|done`
  - `done -> todo`
- Same-state updates succeed with `changed=false`.
- Missing task ID fails with `not_found`.
- After updating `tasks.md`, exit without invoking cmux sync.

## Human output

- Success output should confirm:
  - workspace ID
  - task ID
  - previous status
  - new status

## JSON envelope

- `ok`
- `action=ws.task.status`
- `workspace_id`
- `result`:
  - `task`:
    - `id`
    - `title`
    - `previous_status`
    - `status`
    - `changed`
- `error`
