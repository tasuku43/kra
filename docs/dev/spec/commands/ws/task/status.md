---
title: "`kra ws task status`"
status: implemented
---

# `kra ws task status [--id <workspace-id>] [--current] [--select] <task-id> <todo|doing|blocked|done> [--format human|json]`

## Purpose

Set one structured task to an explicit status, then sync the declaration into cmux sidebar state.

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
- After updating `tasks.md`, run the same reconcile behavior as `kra ws task sync`.
- If task mutation succeeds but follow-up sync fails, the task update remains committed and the
  command reports a warning instead of rolling back `tasks.md`.

## Human output

- Success output should confirm:
  - workspace ID
  - task ID
  - previous status
  - new status
  - sync summary:
    - `task sync: applied N (set X, cleared Y)`
    - or `task sync skipped: ...`
- If sync fails after mutation, render a warning line instead of the sync summary.

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
  - `sync`:
    - `state`
    - `targets`
    - `set`
    - `cleared`
    - `warning` (optional)
- `warnings` (optional when post-mutation sync fails)
- `error`
