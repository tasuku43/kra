---
title: "`kra ws task list`"
status: planned
---

# `kra ws task list [--id <workspace-id>] [--current] [--select] [--format human|json]`

Alias:
- `kra ws task ls`

## Purpose

List structured tasks for one workspace from `tasks.md`.

## Behavior

- Resolve target workspace using the shared `ws` targeting contract.
- Accept both active and archived workspaces.
- Read structured tasks from `<workspace>/tasks.md` using the workspace task contract.
- When `tasks.md` is absent, return success with zero tasks.
- When `tasks.md` exists but `## Tasks` is absent, return success with zero tasks.
- Non-task Markdown outside the structured task contract is ignored.
- Duplicate structured task IDs are a contract conflict and fail the command.
- A task-like block that starts with `### TASK-...` but violates the task contract is a contract
  conflict and fails the command.

## Human output

- Print one summary header:
  - `Tasks: X total (doing Y, blocked Z, todo A, done B)`
- Render sections in fixed order:
  - `Doing`
  - `Blocked`
  - `Todo`
  - `Done`
- Omit empty sections.
- Within each section, preserve file order from `tasks.md`.
- Default row shape is summary-first:
  - `• TASK-001: short title`
- The command may optionally render a muted description preview under each row, but title-first
  summary remains the stable default contract.

## JSON envelope

- `ok`
- `action=ws.task.list`
- `workspace_id`
- `result`:
  - `task_state`
  - `counts`:
    - `total`
    - `doing`
    - `blocked`
    - `todo`
    - `done`
  - `items[]`:
    - `id`
    - `title`
    - `status`
    - `description`
- `error`
