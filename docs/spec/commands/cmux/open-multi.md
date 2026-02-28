---
title: "`kra cmux open --multi`"
status: implemented
---

# `kra cmux open --multi`

## Purpose

Open new cmux workspaces for multiple `kra` workspaces in one command.

## Inputs

- `--multi` (required for multi-target open)
- `--workspace <id>` (repeatable)
- `--concurrency <n>` (optional, default: `1`)
- positional `<workspace-id>` (optional; treated as one target)
- `--format human|json` (default: `human`)

## Target Resolution

- Target IDs are resolved from:
  1. repeated `--workspace`
  2. positional `<workspace-id>`
- Empty IDs are ignored.
- Duplicate IDs are removed while preserving first-seen order.
- If multiple targets are resolved without `--multi`, command fails with `invalid_argument`.
- `--concurrency` must be `>=1`.
- `--concurrency > 1` requires `--multi`.
- If no targets are resolved:
  - human mode: fallback to selector
    - `--multi`: multi-select workspace selector
    - otherwise: single-select workspace selector
  - json mode:
    - `--multi`: fail with `non_interactive_selection_required`
    - otherwise: fail with `invalid_argument`

## Execution

- Required cmux capabilities are checked once before execution.
- `--concurrency=1`: targets are processed sequentially (fail-fast).
- `--concurrency>1`: targets are processed with bounded parallel workers.
- Each target uses the same strict per-workspace open flow as `cmux open`:
  create -> rename -> select -> cwd sync -> mapping update.
- Mapping file is persisted once after execution when at least one target succeeded.
- Parallel mode does not stop on first failure; it aggregates per-target failures.

## Output

- Single target without `--multi` keeps existing `cmux.open` response shape.
- Multi target (`--multi`) returns aggregated result list:
  - `result.count`, `result.succeeded`, `result.failed`
  - `result.items[]` (`kra_workspace_id`, `kra_workspace_path`, `cmux_workspace_id`, `ordinal`, `title`, `cwd_synced`)
  - `result.failures[]` (`kra_workspace_id`, `code`, `message`)
- If any target fails in multi mode:
  - JSON: `ok=false`, `error.code=partial_failure`
  - Human: failed targets are printed to stderr
  - Exit code: `exitError`
