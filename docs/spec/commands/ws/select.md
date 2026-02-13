---
title: "`kra ws` selection entrypoint policy"
status: implemented
---

# `kra ws` / `kra ws select`

## Purpose

Unify interactive selection into a single entrypoint while keeping operation commands explicit for automation.

## Dual-entry contract

- Human facade (interactive):
  - `kra ws`
  - `kra ws select`
- Non-interactive execution path (operation-fixed):
  - `kra ws --act <go|add-repo|remove-repo|close|reopen|purge> ...`
- Both facades must converge to the same operation core behavior for each action.
- Parent-shell side effects (for example `cd`) are applied only through action-file protocol.

## Entry policy

- `kra ws` is context-aware launcher.
- `kra ws --id <id>` resolves launcher target explicitly by id.
- `kra ws select` always starts from workspace selection.
- `kra ws select --act <go|close|add-repo|remove-repo|reopen|unlock|purge>` skips action menu and executes fixed action.
- `kra ws select --act reopen|unlock|purge` implicitly switches to archived scope.
- `kra ws select --archived --act go|add-repo|remove-repo|close` must fail with usage error.
- `kra ws select --multi` requires `--act`.
- `kra ws select --multi --act <close|reopen|purge>` enables multi-selection and executes the fixed action for each
  selected workspace.
- `kra ws select --multi --act close` is active-scope only (`--archived` is invalid).
- `kra ws select --multi --act reopen|purge` implicitly switches to archived scope.
- `kra ws select --multi --commit` enables per-workspace commit behavior for selected action.
- `go|add-repo|remove-repo` are not supported in `--multi` mode.
- `kra ws` must not auto-fallback to workspace list selection when current path cannot resolve workspace.
  unresolved invocation should fail and instruct users to run `kra ws select`.
- `kra ws` must resolve target workspace by either:
  - explicit `--id <id>`
  - current workspace context path (`workspaces/<id>/...` or `archive/<id>/...`)

## Selection flow

- Stage 1: select exactly one workspace from list scope.
- Stage 1 (multi): select one or more workspaces from list scope when `--multi` is set.
- Stage 2: select action for selected workspace.
  - active scope: `go`, `add-repo`, `remove-repo`, `close`
  - archived scope: `reopen`, `purge`
- Stage 3: dispatch to operation command with explicit `<id>`.
- Stage 3 (multi): dispatch selected fixed action for each selected workspace id.

## Action routing policy

- Edit operations for existing workspace resources are routed by `--act`.
- Read-only commands remain subcommands (`ws list`, `ws ls`) and resource creation remains `ws create`.
- `ws go|close|add-repo|remove-repo|reopen|purge` subcommands are not part of supported entrypoints.
- Non-interactive usage should prefer explicit `--id` and must not rely on interactive selectors.
