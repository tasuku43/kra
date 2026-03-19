---
title: "`kra ws open`"
status: implemented
---

# `kra ws open [--id <id> | --current | --select | --multi-select] [--concurrency <n>] [--format human|json]`

## Purpose

Open cmux workspace(s) from workspace action entrypoint.

## Inputs

- target mode:
  - `--id <id>`
  - `--current`
  - `--select`
  - `--multi-select`
- batch options:
  - `--multi-select`
  - `--concurrency <n>`

## Behavior

- Uses cmux integration flow from workspace command entrypoint.
- `--id` targets one active workspace.
- `--current` resolves workspace from current path under `workspaces/<id>/...`.
- `--select` opens workspace selector and resolves target workspace(s) interactively.
- `--multi-select` enables multi-target open flow.
- `--concurrency` is valid only with `--multi-select`.
- `--multi-select` かつ `--concurrency` 未指定時は、自動並列度（`min(targets, max(2, GOMAXPROCS))`）で goroutine 実行する。
- JSON mode remains non-interactive.
- On successful open, advance each succeeded target's `.kra.meta.json.workspace.work_state` to
  `in-progress`.
  - transition is monotonic (`todo -> in-progress`)
  - already `in-progress` workspaces remain unchanged
  - failed targets must not be advanced
- 1:1 policy (`kra workspace` : `cmux workspace`):
  - when no mapping exists, create and select a new cmux workspace
  - newly created cmux workspace is labeled with `kra` / `managed by kra` (`icon=tag`, `color=#4F46E5`)
  - when mapping already exists and runtime workspace is reachable, create is skipped and operation falls back to switch
- cmux capability fallback:
  - when cmux runtime/capabilities are unavailable (`cmux_capability_missing`) and target is single workspace,
    command falls back to directory-open behavior (emit shell action `cd <workspace-path>`)
  - fallback response marks `mode=fallback-cd` and keeps `cwd_synced=true`
  - when multiple targets are requested, fallback is not applied (return `cmux_capability_missing`)

## Notes

- Parent shell cwd mutation still follows action-file protocol.
- Workspace-level command shape is `kra ws open ...`.
