---
title: "`kra ws resume`"
status: implemented
---

# `kra ws resume [--id <id> | --current | --select] [--latest] [--strict] [--no-browser] [--format human|json]`

## Purpose

Restore a previously saved cmux session context for one workspace.

## Inputs

- target mode (required):
  - `--id <id>`
  - `--current`
  - `--select`
- restore policy:
  - `--latest`
  - `--strict`
  - `--no-browser`
- output:
  - `--format human|json`

## Selection UX

- `--select` uses two-stage selection:
  1) select workspace
  2) select session from that workspace
- `--id` / `--current` skip stage 1 and start from session selection.
- `--latest` skips session selection and picks the latest session in resolved workspace.
- v1 does not expose session id as user-facing CLI argument.

## Behavior (v1)

- Resolve one target workspace using shared targeting contract.
- Target must be an active workspace under `workspaces/<id>/`.
- Resolve mapped cmux workspace from `.kra/state/cmux-workspaces.json`.
- Resolve selected session from `.kra/state/cmux-sessions.json`.
- Run restore flow:
  - select cmux workspace
  - restore focus target if available (`focus-pane` / `tab-action`)
  - restore browser state when saved and `--no-browser` is not set
- Default mode is best-effort:
  - partial restore is allowed
  - unresolved items are returned in `warnings[]`
- Strict mode (`--strict`):
  - unresolved required items fail command (`ok=false`)

## Non-Interactive JSON Rules

- `--format json` is non-interactive.
- JSON mode requires explicit target (`--id` or `--current`) and `--latest`.
- JSON mode with selector-required path (`--select` or missing `--latest`) must fail with `invalid_argument`.

## Output Contract

- Human mode:
  - print workspace id, selected session summary, restored items, warning summary.
- JSON envelope:
  - `ok`
  - `action=ws.resume`
  - `workspace_id`
  - `result`:
    - `session_id`
    - `session_label`
    - `resumed_at`
    - `restored`:
      - `workspace_selected` (bool)
      - `focus_restored` (bool)
      - `browser_restored` (bool)
  - `warnings[]`
  - `error`

## Error Code Policy

- `invalid_argument`
- `workspace_not_found`
- `workspace_not_active`
- `cmux_not_mapped`
- `cmux_runtime_unavailable`
- `session_not_found`
- `session_restore_partial` (strict mode)
- `internal_error`

## Exit Code

- `0` success (including warning-only best-effort outcomes)
- `3` business/runtime failure
- `2` usage error
