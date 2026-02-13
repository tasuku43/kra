---
title: "`kra doctor`"
status: implemented
---

# `kra doctor [--format human|json]`
# `kra doctor --fix --plan|--apply [--format human|json]`

## Purpose

Provide a non-destructive health report for current `KRA_ROOT` to detect operational drifts early.

## Scope (MVP)

- Validate root layout essentials (`workspaces/`, `archive/`).
- Validate workspace metadata readability (`.kra.meta.json` parse/required fields).
- Detect broken workspace repo links:
  - binding exists but worktree path missing
  - worktree exists but binding/metadata missing
- Detect stale workspace action lock files under `.kra/locks/` when owner PID is not alive.
- Detect obvious registry drift where current root is missing from `~/.kra/state/root-registry.json`.

## Output

- Human mode:
  - `Result:` section with summary counts: `ok`, `warn`, `error`.
  - List findings grouped by severity.
- JSON mode:
  - stable envelope (`ok`, `action`, `result`, `error`) aligned with shared output contract.
  - `result` includes `root`, counts (`ok`,`warn`,`error`), and `findings[]`.

## Exit code

- `0` when no `error` findings (warnings allowed).
- `3` when at least one `error` finding exists, or command runtime fails.
- `2` for usage errors (including unsupported `--fix` in MVP).

## `--fix` policy

- Staged remediation mode is defined in:
  - `docs/spec/commands/doctor-fix.md`
- This command keeps detection/reporting behavior as default.
- Remediation is opt-in and explicit via `--fix --plan|--apply`.

## Non-goals (MVP)

- No automatic mutation of filesystem or state store.
- No remote Git operations (`fetch`, network checks).
- No cross-root global registry repair.
