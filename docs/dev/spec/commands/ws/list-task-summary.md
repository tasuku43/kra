---
title: "`kra ws list` task summary extension"
status: implemented
---

# `kra ws list` task summary extension

## Purpose

Add structured task summaries to workspace list output while preserving the current
activity-oriented row ordering and primary status semantics.

## Relation to existing `ws list`

- This spec extends `docs/dev/spec/commands/ws/list.md`.
- Primary row ordering and work-state semantics remain unchanged.
- Structured task data follows:
  - `docs/dev/spec/concepts/workspace-tasks.md`
  - `docs/dev/spec/concepts/workspace-task-overview.md`

## Human output

- Keep the existing primary workspace row shape:
  - `• WS-101: login flow`
- Add one muted supplemental line directly under each workspace row before any `--tree` repo lines:
  - `tasks: empty`
  - `tasks: invalid`
  - `tasks: X total (doing Y, blocked Z, todo A, done B)`
- The supplemental task line is shown for both active and archived scope.
- Task summary does not affect active workspace ordering:
  - `in-progress` rows still sort before `todo` rows
  - task counts do not introduce a new sort key

## JSON extension

When `--format json` is used, each `items[]` entry is extended with:

- `tasks`:
  - `summary` (`empty` / `invalid` / `counts`)
  - `task_state` (optional when summary is `invalid`)
  - `counts` (optional when summary is `counts`)
    - `total`
    - `doing`
    - `blocked`
    - `todo`
    - `done`
  - `warning` (optional)

## TSV extension

When `--format tsv` is used, append stable columns:

- `task_summary`
- `task_total`
- `task_doing`
- `task_blocked`
- `task_todo`
- `task_done`

For `task_summary=empty` or `invalid`, numeric task columns are empty.
