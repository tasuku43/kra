---
title: "`kra ws open`"
status: implemented
---

# `kra ws open [--id <id> | --current | --select] [--multi] [--concurrency <n>] [--format human|json]`

## Purpose

Open cmux workspace(s) from workspace action entrypoint.

## Inputs

- target mode:
  - `--id <id>`
  - `--current`
  - `--select`
- batch options:
  - `--multi`
  - `--concurrency <n>`

## Behavior

- Uses cmux integration flow from workspace command entrypoint.
- `--id` targets one active workspace.
- `--current` resolves workspace from current path under `workspaces/<id>/...`.
- `--select` opens workspace selector and resolves target workspace(s) interactively.
- `--multi` enables multi-target open flow.
- `--concurrency` is valid only with `--multi`.
- JSON mode remains non-interactive.
- 1:1 policy (`kra workspace` : `cmux workspace`):
  - when no mapping exists, create and select a new cmux workspace
  - newly created cmux workspace is labeled with `kra` / `managed by kra` (`icon=tag`, `color=#4F46E5`)
  - when mapping already exists and runtime workspace is reachable, create is skipped and operation falls back to switch

## Notes

- Parent shell cwd mutation still follows action-file protocol.
- Workspace-level command shape is `kra ws open ...` (not `kra cmux open ...`).
