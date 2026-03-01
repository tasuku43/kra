---
title: "`kra doctor --fix` staged remediation"
status: implemented
---

# `kra doctor --fix --plan|--apply [--format human|json]`

## Purpose

Extend `kra doctor` from detection-only to staged remediation:

- `--plan`: show concrete remediation actions without mutation
- `--apply`: execute planned remediation actions atomically per action unit

This keeps recovery explicit and auditable while reducing operational MTTR.

## Scope (implemented phase)

- stale lock cleanup under `KRA_ROOT/.kra/locks/`
- root registry touch/repair when current root is missing from registry

## Inputs

- `--fix --plan` or `--fix --apply` (exactly one is required)
- `--format human|json` default `human`

## Behavior

1. Run normal `doctor` finding pipeline.
2. Convert fixable findings to `actions[]` with stable `action_id`.
3. `--plan`:
  - print/emit planned actions only.
  - do not mutate filesystem/registry.
4. `--apply`:
  - execute actions in deterministic order.
  - each action is atomic and reports `applied|skipped|failed`.
  - on failure, continue by default and report partial result.
5. Report summary counts:
  - `planned`
  - `applied`
  - `skipped`
  - `failed`

## Safety policy

- No remote Git operations.
- No workspace destructive operations (`close/reopen/purge`) are performed.
- Unsupported/ambiguous fix types are reported as `skipped` with reason `manual_required`.

## JSON contract

Envelope follows shared output contract:

- `ok`
- `action=doctor.fix`
- `result`:
  - `root`
  - `mode` (`plan` or `apply`)
  - `summary`
  - `actions[]` (`id`, `kind`, `target`, `status`, `reason`)
- `error` (on command-level failure)

## Exit code

- `0`: no command-level failure and `failed=0`
- `3`: one or more action failures, or runtime failure
- `2`: usage error

## Non-goals (current phase)

- `--scope` and `--workspace` selective apply
- automatic metadata rewrite for malformed `.kra.meta.json`
- automatic worktree recreation for missing repo links
