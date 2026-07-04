---
title: "`kra task`"
status: implemented
---

# `kra task`

## Purpose

Manage root-level inbox and cross-workspace tasks stored in `<KRA_ROOT>/workspace.md`.

## Commands

- `kra task list [--format human|json]`
- `kra task ls [--format human|json]`
- `kra task add --title "<text>" [--description "<text>"] [--format human|json]`
- `kra task [--todo-only] [--include-done] [--no-color]`
- `kra task status [--todo-only] [--include-done] [--no-color]`
- `kra task tui [--todo-only] [--include-done] [--no-color]`
- `kra task status <task-id> <todo|doing|blocked|done> [--format human|json]`

## Behavior

- Resolve the current `KRA_ROOT` using the normal root resolution policy.
- Read and write root `workspace.md` with the shared workspace task parser/model.
- Missing `workspace.md` is treated as an empty task document for `list`.
- `add` lazily creates root `workspace.md` and appends a canonical `## Tasks` section when needed.
- The default `kra task` command opens the same interactive status view pattern as `kra ws status`, scoped to
  root `workspace.md`.
- `kra task status` is the primary explicit root status view command, matching `kra ws status`.
- `kra task tui` is a compatibility alias for the same root status view.
- `kra task status <task-id> <status>` updates only the target task in root `workspace.md`.
- Task IDs use the same `TASK-<nnn>` sequence as workspace-local tasks.
- Root task commands do not mutate workspace-local task documents.
- Root task commands do not invoke cmux task sync.

## Output

- Human output follows the same task list/status/add shape as `kra ws task`, without a workspace ID line.
- JSON output follows the standard CLI envelope.
- JSON action names:
  - `task.list`
  - `task.add`
  - `task.status`
- JSON result payloads include:
  - `path`
  - task overview fields for `list`
  - task object for `add` and `status`

## Errors

- invalid arguments: `exitUsage` with JSON code `invalid_argument`
- root resolution failure: `exitError` with JSON code `not_found`
- invalid task contract: fail closed with JSON code `conflict`
