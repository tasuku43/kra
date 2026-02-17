---
title: "`kra ws select --multi`"
status: implemented
---

# `kra ws select --multi --act <close|reopen|purge> [--archived] [--no-commit] [--commit] [--format human|json] [--yes] [--continue-on-error]`

## Purpose

Add multi-select execution mode to the existing workspace selector entrypoint without introducing a new top-level command.

## Inputs

- `--multi` (required for this mode)
- `--act` (required): one of `close`, `reopen`, `purge`
- `--archived` (optional):
  - implied automatically by archived actions (`reopen`, `purge`)
  - invalid with active action (`close`)
- default mode is commit-enabled for lifecycle actions.
- `--no-commit` (optional):
  - disable lifecycle commits for selected action executions.
- `--commit` (optional):
  - accepted for backward compatibility and keeps default behavior.
- `--yes` (optional): required in JSON mode for destructive actions
- `--continue-on-error` (optional):
  - default: fail-fast
  - enabled: execute all selected targets and report partial failures

## Validation rules

- `--multi` without `--act` must fail with usage error.
- `go`, `add-repo`, `remove-repo` are invalid in `--multi` mode.
- Scope mismatch (`--archived` + `close`) must fail fast.

## Behavior (MVP)

1. Open shared selector in multi-select mode for workspace rows.
2. Confirm selected set (empty selection aborts with non-zero exit).
3. Execute the selected `--act` per workspace using existing flow/orchestrator.
4. Aggregate per-workspace outcomes: `success`, `failed`, `skipped` (+ reason).

## Output

- Human mode:
  - selected action prints its own sections per workspace (`Risk:`/`Result:` as defined by each action command).
  - `ws select --multi` itself does not print an additional aggregate `Result:` block.
- JSON mode:
  - not implemented in MVP scope for `ws select --multi`.

## Safety / commit scope

- For actions committing in `KRA_ROOT`, staging must be allowlisted by selected workspace prefixes.
- If staged paths include non-allowlisted files, abort before commit.

## Non-goals (MVP)

- Mixed-action execution in one command
- Per-workspace custom prompts/inputs during one multi run
