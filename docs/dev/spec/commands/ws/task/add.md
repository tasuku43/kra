---
title: "`kra ws task add`"
status: implemented
---

# `kra ws task add [--id <workspace-id>] [--current] [--select] --title "<text>" [--description "<text>"] [--format human|json]`

## Purpose

Add one structured task to the target workspace task document.

## Behavior

- Resolve target workspace using the shared `ws` targeting contract.
- Command is valid only for active workspaces.
- `--title` is required after trimming.
- `--description` is optional and may contain multi-line Markdown text.
- When `workspace.md` is absent, create it lazily.
- When `workspace.md` exists but `## Tasks` is absent, append one `## Tasks` section at EOF before
  writing the new task.
- Generate the next task ID as `TASK-<nnn>` using the maximum existing numeric suffix + 1.
- New tasks always start with `status: todo`.
- Preserve freeform content outside `## Tasks`.
- Preserve existing structured tasks and append the new task block at the end of `## Tasks`.
- Duplicate existing task IDs are a conflict and fail the command.

## Canonical write shape

New tasks are written in canonical form:

- heading: `### TASK-<nnn> <title>`
- required field line: `status: todo`
- optional description body after one blank line

## Human output

- Success output should confirm:
  - workspace ID
  - new task ID
  - task title
  - task document path

## JSON envelope

- `ok`
- `action=ws.task.add`
- `workspace_id`
- `result`:
  - `path`
  - `task`:
    - `id`
    - `title`
    - `status` (`todo`)
- `error`
