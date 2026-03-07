---
title: "`kra ws dashboard` task summary extension"
status: planned
---

# `kra ws dashboard` task summary extension

## Purpose

Expose structured task summaries in dashboard surfaces while keeping dashboard primarily focused on
workspace lifecycle, risk, and context.

## Relation to existing `ws dashboard`

- This spec extends `docs/dev/spec/commands/ws/dashboard.md`.
- Structured task data follows:
  - `docs/dev/spec/concepts/workspace-tasks.md`
  - `docs/dev/spec/concepts/workspace-task-overview.md`
- Dashboard root summary cards remain unchanged in phase 1.

## Workspace rows

- Each workspace row gains one supplemental task field:
  - `tasks: empty`
  - `tasks: invalid`
  - `tasks: X total (doing Y, blocked Z, todo A, done B)`
- Task summary is supplemental and must not replace the existing risk/status-oriented row content.

## Detailed workspace panel

When `--workspace <id>` is used, the detailed panel adds one `Tasks:` section:

- summary line:
  - `tasks: empty`
  - `tasks: invalid`
  - `tasks: X total (doing Y, blocked Z, todo A, done B)`
- if valid task data exists, render focused subsections only for active work:
  - `Doing:` with `TASK-<nnn>: <title>` rows
  - `Blocked:` with `TASK-<nnn>: <title>` rows
- omit empty `Doing` / `Blocked` subsections
- `Todo` and `Done` task rows are intentionally omitted from the dashboard detail panel in phase 1
  to keep the view compact; full task inspection belongs to `kra ws task list`

## JSON extension

When `--format json` is used:

- each `workspaces[]` entry is extended with:
  - `tasks`:
    - `summary` (`empty` / `invalid` / `counts`)
    - `task_state` (optional when summary is `invalid`)
    - `counts` (optional when summary is `counts`)
    - `warning` (optional)
- for `--workspace <id>`, the selected workspace detail includes:
  - `tasks.active_items.doing[]`
  - `tasks.active_items.blocked[]`

## Invalid contract handling

- Invalid task contracts must not fail the dashboard command.
- Dashboard should surface the task summary as `invalid`.
- JSON output should include a warning message for the affected workspace when available.
