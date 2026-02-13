---
title: "CLI output contract (human/json)"
status: planned
---

# Output Contract

## Purpose

Define one shared machine-readable response envelope for commands that support `--format json`.

## JSON envelope (target)

- `ok` (`bool`): overall command success.
- `action` (`string`): stable action identifier (for example: `ws.add-repo`, `repo.gc`).
- `workspace_id` (`string`, optional): populated when command targets a single workspace context.
- `result` (`object`, optional): success payload.
- `error` (`object`, optional):
  - `code` (`string`): stable machine code.
  - `message` (`string`): human-readable diagnostic.

## Error code policy

- Command-specific internals should map to stable top-level classes:
  - `invalid_argument`
  - `not_found`
  - `conflict`
  - `permission_denied`
  - `internal_error`
- Keep `message` detailed, but clients should branch on `error.code`.

## Human mode relation

- Human `Result:` sections are not required to mirror JSON field names.
- Semantics (success/failure totals and target IDs) should remain equivalent.

## Rollout plan

1. Document current deviations per command.
2. Align selected high-traffic commands first.
3. Backward-compat policy:
  - preserve existing keys for one transition window when needed.
