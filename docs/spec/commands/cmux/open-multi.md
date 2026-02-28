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
- positional `<workspace-id>` (optional; treated as one target)
- `--format human|json` (default: `human`)

## Target Resolution

- Target IDs are resolved from:
  1. repeated `--workspace`
  2. positional `<workspace-id>`
- Empty IDs are ignored.
- Duplicate IDs are removed while preserving first-seen order.
- If multiple targets are resolved without `--multi`, command fails with `invalid_argument`.
- If no targets are resolved:
  - human mode: fallback to selector
    - `--multi`: multi-select workspace selector
    - otherwise: single-select workspace selector
  - json mode:
    - `--multi`: fail with `non_interactive_selection_required`
    - otherwise: fail with `invalid_argument`

## Execution (v2 baseline)

- Required cmux capabilities are checked once before execution.
- Targets are processed sequentially.
- Each target uses the same strict per-workspace open flow as `cmux open`:
  create -> rename -> select -> cwd sync -> mapping update.
- Mapping file is persisted after all targets finish successfully.
- On the first failure, command aborts and returns error.

## Output

- Single target without `--multi` keeps existing `cmux.open` response shape.
- Multi target (`--multi`) returns aggregated result list:
  - `result.count`
  - `result.items[]` (`kra_workspace_id`, `kra_workspace_path`, `cmux_workspace_id`, `ordinal`, `title`, `cwd_synced`)

