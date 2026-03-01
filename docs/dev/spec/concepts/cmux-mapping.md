---
title: "cmux workspace mapping store"
status: implemented
---

# cmux Workspace Mapping Store

## Purpose

Persist a 1:N mapping from `kra` workspace IDs to cmux workspace IDs.

## File Location

- `KRA_ROOT/.kra/state/cmux-workspaces.json`

## Schema

```json
{
  "version": 1,
  "workspaces": {
    "<kra-workspace-id>": {
      "next_ordinal": 1,
      "entries": [
        {
          "cmux_workspace_id": "UUID",
          "ordinal": 1,
          "title_snapshot": "MVP-020 | implement auth [1]",
          "created_at": "2026-02-28T12:34:56Z",
          "last_used_at": "2026-02-28T13:10:00Z"
        }
      ]
    }
  }
}
```

## Normalization Rules

- `version` must be `1`.
  - Missing version is normalized to `1`.
  - Unsupported version is an error.
- `workspaces` defaults to an empty map when missing.
- `entries` defaults to an empty array when missing.
- `ordinal` values less than `1` are normalized to stable 1-based values.
- `next_ordinal` values less than `1` are normalized to `1`.
- `next_ordinal` is always set to `max(entries.ordinal) + 1` when needed.

## Persistence Rules

- Save operation must ensure parent directory exists.
- Save operation uses temp-file + rename replacement semantics.
