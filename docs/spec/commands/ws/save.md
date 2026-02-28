---
title: "`kra ws save`"
status: implemented
---

# `kra ws save [--id <id> | --current | --select] [-l <label>] [--no-browser-state] [--format human|json]`

## Purpose

Capture current cmux session context for one workspace so users can resume work quickly after interruption.

## Inputs

- target mode (required):
  - `--id <id>`
  - `--current`
  - `--select`
- optional:
  - `-l`, `--label <text>`
  - `--no-browser-state`
  - `--format human|json`

## Behavior (v1)

- Resolve one target workspace using shared targeting contract.
- Target must be an active workspace under `workspaces/<id>/`.
- Resolve mapped cmux workspace from `.kra/state/cmux-workspaces.json`.
- Validate runtime reachability before capture (`identify`).
- Collect session capture inputs from cmux runtime:
  - pane/surface inventory (`list-panes`, `list-pane-surfaces`)
  - focused context (`identify`)
  - terminal text snapshot (`read-screen`, bounded lines)
  - browser state (`browser state save`) unless `--no-browser-state`
- Save capture artifacts to:
  - `workspaces/<id>/artifacts/cmux/sessions/<session-id>/`
- Update session index:
  - `.kra/state/cmux-sessions.json`
- Browser state save is best-effort in default mode:
  - save continues when one or more browser surfaces fail
  - failure details are returned in `warnings[]`

## Output Contract

- Human mode:
  - print workspace id, created session id, optional label, and warning summary.
- JSON envelope:
  - `ok`
  - `action=ws.save`
  - `workspace_id`
  - `result`:
    - `session_id`
    - `label`
    - `path`
    - `saved_at`
    - `pane_count`
    - `surface_count`
    - `browser_state_saved` (bool)
  - `warnings[]`
  - `error`

## Error Code Policy

- `invalid_argument`
- `workspace_not_found`
- `workspace_not_active`
- `cmux_not_mapped`
- `cmux_runtime_unavailable`
- `state_write_failed`
- `internal_error`

## Exit Code

- `0` success (including warning-only best-effort outcomes)
- `3` business/runtime failure
- `2` usage error
