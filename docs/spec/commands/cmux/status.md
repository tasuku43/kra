---
title: "`kra cmux status`"
status: implemented
---

# `kra cmux status [--workspace <workspace-id>] [--format human|json]`

## Purpose

Show mapping health by comparing persisted mappings with current cmux workspace list.

## Inputs

- `--workspace <workspace-id>` (optional filter)
- `--format human|json` (default: `human`)

## Behavior

- Loads persisted mapping from `.kra/state/cmux-workspaces.json`.
- Queries current cmux workspaces.
- Marks each mapped entry as:
  - `exists=true` when found in cmux runtime
  - `exists=false` when missing

## JSON Response

Success:
- `ok=true`
- `action=cmux.status`
- `workspace_id` (when filter is provided)
- `result.items[]` with:
  - `workspace_id`
  - `cmux_workspace_id`
  - `ordinal`
  - `title`
  - `exists`
