---
title: "`kra ws select --multi`"
status: planned
---

# `kra ws select --multi --act <close|reopen|purge> [--archived] [--format human|json] [--yes] [--continue-on-error]`

## Purpose

Add multi-select execution mode to the existing workspace selector entrypoint without introducing a new top-level command.

## Inputs

- `--multi` (required for this mode)
- `--act` (required): one of `close`, `reopen`, `purge`
- `--archived` (optional):
  - required by scope for archived actions (`reopen`, `purge`)
  - invalid with active action (`close`)
- `--yes` (optional): required in JSON mode for destructive actions
- `--continue-on-error` (optional):
  - default: fail-fast
  - enabled: execute all selected targets and report partial failures

## Validation rules

- `--multi` without `--act` must fail with usage error.
- `go`, `add-repo`, `remove-repo` are invalid in `--multi` mode.
- Scope mismatch (`--archived` + `close`, or missing `--archived` for archived-only actions) must fail fast.

## Behavior (MVP)

1. Open shared selector in multi-select mode for workspace rows.
2. Confirm selected set (empty selection aborts with non-zero exit).
3. Execute the selected `--act` per workspace using existing flow/orchestrator.
4. Aggregate per-workspace outcomes: `success`, `failed`, `skipped` (+ reason).

## Output

- Human mode:
  - `Plan:` with action + selected count
  - `Result:` summary totals and per-workspace lines
- JSON mode:
  - shared envelope (`ok`, `action`, `result`, `error`)
  - `result.items[]` with workspace ID + status + message

## Safety / commit scope

- For actions committing in `KRA_ROOT`, staging must be allowlisted by selected workspace prefixes.
- If staged paths include non-allowlisted files, abort before commit.

## Non-goals (MVP)

- Mixed-action execution in one command
- Per-workspace custom prompts/inputs during one multi run
