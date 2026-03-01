---
title: "`kra ws lock` / `kra ws unlock`"
status: implemented
---

# `kra ws lock <id> [--format human|json]`
# `kra ws unlock <id> [--format human|json]`

## Purpose

Protect workspaces from accidental purge by using one dedicated guard.

This command family manages purge guard only:

- `ws purge`

## Data model

Purge guard state is stored in canonical workspace metadata:

- `workspaces/<id>/.kra.meta.json`
- `archive/<id>/.kra.meta.json`

Field path:

- `protection.purge_guard.enabled` (`bool`)
- `protection.purge_guard.updated_at` (`unix`, optional)

Defaults:

- `ws create` must initialize `protection.purge_guard.enabled=true`.
- `ws close` and `ws reopen` must preserve current guard value.

## Behavior

### `ws lock`

- fail if target workspace does not exist
- set `protection.purge_guard.enabled=true`
- idempotent success when already `true`

### `ws unlock`

- fail if target workspace does not exist
- set `protection.purge_guard.enabled=false`
- idempotent success when already `false`

## Integration with existing actions

- `ws purge` must fail with `error.code=conflict` when `protection.purge_guard.enabled=true`.
- `ws purge` remains archived-only in phase 1.
- failure message must include recovery hint:
  - `hint: run 'kra ws unlock <id>' before purge`
- launcher integration (phase 1):
  - archived launcher (`kra ws --archived`) includes `unlock` action.
  - active launcher does not add `lock/unlock` actions.

## JSON envelope

- `ok`
- `action`: `ws.lock` or `ws.unlock`
- `workspace_id`
- `result`:
  - `purge_guard_enabled` (bool)
- `error`

## Exit code

- `0` success
- `3` business/runtime failure
- `2` usage error
