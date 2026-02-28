---
title: "cmux session capture store"
status: implemented
---

# cmux Session Capture Store

## Purpose

Persist saved cmux session metadata used by `kra ws save` and `kra ws resume`.

## File Location

- `KRA_ROOT/.kra/state/cmux-sessions.json`

## Schema (v1)

```json
{
  "version": 1,
  "workspaces": {
    "WS-101": {
      "sessions": [
        {
          "session_id": "20260228T164501Z-review-before-merge",
          "label": "review before merge",
          "created_at": "2026-02-28T16:45:01Z",
          "path": "workspaces/WS-101/artifacts/cmux/sessions/20260228T164501Z-review-before-merge",
          "pane_count": 2,
          "surface_count": 3,
          "browser_state_saved": true
        }
      ]
    }
  }
}
```

## Normalization Rules

- `version` must be `1`.
  - missing version is normalized to `1`
  - unsupported version is an error
- `workspaces` defaults to empty map when missing.
- `sessions` defaults to empty array when missing.
- session ordering is newest-first by `created_at` (stable sort fallback by `session_id`).
- duplicate `session_id` entries in one workspace are invalid and must fail save.

## Persistence Rules

- Save operation must ensure parent directory exists.
- Save operation uses temp-file + rename replacement semantics.
- Index update and artifact directory creation are part of one logical capture operation:
  - if index write fails, command must report `state_write_failed`
  - partial artifact leftovers may remain; command reports warning with cleanup hint.
