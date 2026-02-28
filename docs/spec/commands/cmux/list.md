---
title: "`kra cmux list`"
status: implemented
---

# `kra cmux list [--workspace <workspace-id>] [--format human|json]`

## Purpose

List persisted cmux mapping entries.

## Inputs

- `--workspace <workspace-id>` (optional filter)
- `--format human|json` (default: `human`)

## Behavior

- Reads `.kra/state/cmux-workspaces.json`.
- Emits mapping rows ordered by workspace id and entry order.
- Human mode prints grouped rows.
- JSON mode returns `result.items[]`.

## JSON Response

Success:
- `ok=true`
- `action=cmux.list`
- `workspace_id` (when filter is provided)
- `result.items[]` with:
  - `workspace_id`
  - `cmux_workspace_id`
  - `ordinal`
  - `title`
  - `last_used_at`
