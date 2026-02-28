---
title: "`kra cmux open`"
status: implemented
---

# `kra cmux open [<workspace-id>] [--format human|json]`

## Purpose

Open a new cmux workspace for one `kra` workspace using strict integration flow.

## Inputs

- `<workspace-id>` (optional in human mode; required in JSON mode)
- `--format human|json` (default: `human`)

## Behavior

- If `<workspace-id>` is omitted:
  - TTY + human mode: fallback to interactive workspace selection.
  - JSON mode: fail with `invalid_argument`.
- Required cmux capabilities:
  - `workspace.create`
  - `workspace.rename`
  - `workspace.select`
  - `surface.send_text`
- Strict execution order:
  1. create cmux workspace
  2. allocate ordinal from mapping store
  3. format title (`<kra-id> | <kra-title> [<n>]`)
  4. rename cmux workspace
  5. select cmux workspace
  6. sync cwd via `surface.send_text` (`cd '<path>'`)
  7. persist mapping entry

## Failure Policy

- Any failed step aborts the command (`strict`).
- JSON mode returns shared error envelope with stable error codes.

## JSON Response

Success:
- `ok=true`
- `action=cmux.open`
- `workspace_id`
- `result.kra_workspace_id`
- `result.kra_workspace_path`
- `result.cmux_workspace_id`
- `result.ordinal`
- `result.title`
- `result.cwd_synced=true`

Error:
- `ok=false`
- `action=cmux.open`
- `workspace_id` (if resolved)
- `error.code`
- `error.message`
