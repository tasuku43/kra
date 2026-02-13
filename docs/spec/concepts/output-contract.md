---
title: "CLI output contract (human/json)"
status: implemented
---

# Output Contract

## Purpose

Define one shared machine-readable response envelope for commands that support `--format json`.

## JSON envelope

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

Implemented code classes in this phase:
- `invalid_argument`
- `not_found`
- `conflict`
- `internal_error`

## Human mode relation

- Human `Result:` sections are not required to mirror JSON field names.
- Semantics (success/failure totals and target IDs) should remain equivalent.

## TSV naming policy

- For commands with machine-readable TSV output, column names should use stable `snake_case`.
- When JSON field names exist for the same data, TSV column names should align with those JSON names.

## Applied commands (OPS-003)

- `init`
- `context` (`current`, `list`, `create`, `use`, `rename`, `rm`)
- `repo add`
- `repo remove`
- `repo gc`
- `ws list`
