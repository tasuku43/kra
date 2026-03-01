---
title: "`kra ws --multi-select`"
status: implemented
---

# `kra ws --multi-select [<action>] [--archived] [--no-commit] [--commit] [--format human|json] [--yes] [--continue-on-error]`

## Purpose

Add multi-select execution mode to the existing workspace selector entrypoint without introducing a new top-level command.

## Inputs

- `--multi-select` (required for this mode)
- action (optional): one of `open`, `close`, `reopen`, `lock`, `unlock`, `purge`
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

- when action is omitted, prompt Action selector and show only `--multi-select` supported actions for current scope.
- `add-repo`, `remove-repo` are invalid in `--multi-select` mode.
- Scope mismatch (`--archived` + `close`) must fail fast.

## Behavior (MVP)

1. Resolve action (explicit or selector) and open shared selector in multi-select mode for workspace rows.
2. Confirm selected set (empty selection aborts with non-zero exit).
3. Execute the selected action per workspace using existing flow/orchestrator.
4. Aggregate per-workspace outcomes: `success`, `failed`, `skipped` (+ reason).

## Output

- Human mode:
  - selected action prints its own sections per workspace (`Risk:`/`Result:` as defined by each action command).
  - `ws --multi-select` itself does not print an additional aggregate `Result:` block.
- JSON mode:
  - not implemented in MVP scope for `ws --multi-select`.

## Safety / commit scope

- For actions committing in `KRA_ROOT`, staging must be allowlisted by selected workspace prefixes.
- If staged paths include non-allowlisted files, abort before commit.

## Non-goals (MVP)

- Mixed-action execution in one command
- Per-workspace custom prompts/inputs during one multi run
