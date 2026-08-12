---
title: "cmux workspace mapping store"
status: implemented
---

# cmux Workspace Mapping Store

## Purpose

Persist the active 1:1 mapping from each `kra` workspace ID to its managed cmux workspace ID.

## File Location

- `KRA_ROOT/.kra/state/cmux-workspaces.json`

## Schema

```json
{
  "version": 2,
  "root_group": {
    "cmux_window_id": "UUID",
    "cmux_group_id": "UUID",
    "anchor_workspace_id": "UUID",
    "name_snapshot": "kra | project",
    "created_at": "2026-08-13T00:00:00Z",
    "last_used_at": "2026-08-13T00:00:00Z"
  },
  "workspaces": {
    "<kra-workspace-id>": {
      "next_ordinal": 2,
      "entries": [
        {
          "cmux_workspace_id": "UUID",
          "ordinal": 1,
          "title_snapshot": "MVP-020 | implement auth",
          "created_at": "2026-02-28T12:34:56Z",
          "last_used_at": "2026-02-28T13:10:00Z"
        }
      ]
    }
  },
  "todos": {
    "<kra-workspace-id>": {
      "cmux_workspace_id": "UUID",
      "items": [
        {
          "kra_task_id": "TASK-001",
          "cmux_item_id": "UUID",
          "kra_status": "todo",
          "cmux_state": "pending",
          "text": "TASK-001 implement auth"
        }
      ],
      "synced_at": "2026-08-13T00:00:00Z"
    }
  }
}
```

## Normalization Rules

- `version` must be `2`.
  - Missing version and version `1` are migrated losslessly to `2` with no root group or Todo base.
  - Unsupported version is an error.
- `workspaces` defaults to an empty map when missing.
- `entries` defaults to an empty array when missing.
- `ordinal` values less than `1` are normalized to stable 1-based values.
- `next_ordinal` values less than `1` are normalized to `1`.
- `next_ordinal` is always set to `max(entries.ordinal) + 1` when needed.
- `root_group` is optional. If present, its window, group, and anchor UUIDs are required; incomplete records are
  corruption errors and are never silently discarded.
- `todos` is an optional ignored three-way base described by `cmux-todo-reconcile.md`. Incomplete or duplicate
  managed item identities are corruption errors and are never silently discarded.

## Logical Mapping Policy

- The supported runtime policy is 1:1 (`kra workspace` : `cmux workspace`).
- Schema v1 retains `entries[]`, `ordinal`, and `next_ordinal` for backward compatibility with older mapping
  files; these fields do not authorize creating multiple managed runtimes for one KRA workspace.
- `ws open` reuses the first reachable legacy entry and collapses the in-memory mapping to that one entry, or
  replaces stale legacy entries with one newly-created runtime.
- Workspace Groups are a separate root/window-level relationship in `root_group` and must not be represented as
  additional entries in a workspace mapping. `workspaces.KRA_ROOT` mirrors the managed anchor UUID for existing
  root/current-workspace lookup compatibility.

## Persistence Rules

- Save operation must ensure parent directory exists.
- Save operation uses temp-file + rename replacement semantics.
- Mapping read-modify-write flows share one KRA-root-wide process lock at
  `.kra/state/operations/cmux-workspaces.lock`.
- A mutating flow acquires the lock before loading the latest mapping and retains it through required cmux
  side effects, atomic save, and failure compensation; lock acquisition failure must occur before any cmux
  runtime side effect.
- Temp files use writer-unique names under the ignored `.kra/state/operations/` directory, so concurrent writers
  and crash remnants cannot collide or appear in Git status.
- Read-only loads do not require the lock because successful saves publish with atomic rename.
- The mapping is runtime-local operational state, remains Git-ignored, and must not be included in command commit
  allowlists.
- On `ws open`, stale `cmux_workspace_id` entries may be relinked to a new runtime workspace handle when
  exactly one runtime workspace title matches the expected `kra`-formatted title for the same logical workspace.
- `ws open` persists mapping changes only for targets whose mandatory runtime setup completed successfully.
- A failed target restores its pre-open mapping, including `entries` and `next_ordinal`; a partial-success batch
  must not persist provisional entries from failed targets.
