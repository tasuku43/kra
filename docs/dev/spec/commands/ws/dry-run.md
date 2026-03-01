---
title: "`kra ws <close|reopen|purge> --dry-run`"
status: implemented
---

# `kra ws <close|reopen|purge> --dry-run [--id <id>|<id>] --format json`

## Purpose

Provide a non-mutating preflight contract for destructive/reversible lifecycle actions.

The output must be machine-readable and consistent across:

- `ws close`
- `ws reopen`
- `ws purge`

## Inputs

- `--dry-run` (required in this mode)
- target workspace id (`--id` or positional)
- `--format json` required

## Behavior

For selected action, evaluate:

- target existence and lifecycle state
- risk signals (`clean|unpushed|diverged|dirty|unknown`)
- precondition failures (conflicts, missing paths, lock violations)
- lifecycle commit allowlist/staging feasibility in default mode (or when `--commit` is specified)

No file, git, or metadata mutation is allowed.

## Unified JSON envelope

- `ok`
- `action`: `ws.close.dry-run` / `ws.reopen.dry-run` / `ws.purge.dry-run`
- `workspace_id`
- `result`:
  - `executable` (bool)
  - `checks[]` (`name`, `status=pass|warn|fail`, `message`)
  - `risk` (`workspace`, `repos[]`)
  - `planned_effects[]` (path and state transitions)
  - `requires_confirmation` (bool)
  - `requires_force` (bool)
  - `commit_enabled` (bool)
- `error` for usage/runtime failures

## Error code policy

- `invalid_argument`
- `not_found`
- `conflict`
- `permission_denied`
- `internal_error`

## Exit code

- `0`: dry-run succeeded (even with warnings)
- `3`: command-level failure or `result.executable=false` due to hard fail checks
- `2`: usage error

## Non-goals (phase 1)

- human mode dry-run UX redesign
- batch dry-run across multi-select in one call
