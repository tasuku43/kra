---
title: "`gionx ws` selection entrypoint policy"
status: implemented
---

# `gionx ws` / `gionx ws select`

## Purpose

Unify interactive selection into a single entrypoint while keeping operation commands explicit for automation.

## Dual-entry contract

- Human facade (interactive):
  - `gionx ws`
  - `gionx ws select`
- Non-interactive execution path (operation-fixed):
  - `gionx ws --act <go|add-repo|close|reopen|purge> ...`
- Both facades must converge to the same operation core behavior for each action.
- Parent-shell side effects (for example `cd`) are applied only through action-file protocol.

## Entry policy

- `gionx ws` is context-aware launcher.
- `gionx ws --id <id>` resolves launcher target explicitly by id.
- `gionx ws select` always starts from workspace selection.
- `gionx ws select --act <go|close|add-repo|reopen|purge>` skips action menu and executes fixed action.
- `gionx ws select --act reopen|purge` implicitly switches to archived scope.
- `gionx ws select --archived --act go|add-repo|close` must fail with usage error.
- `gionx ws` must not auto-fallback to workspace list selection when current path cannot resolve workspace.
  unresolved invocation should fail and instruct users to run `gionx ws select`.
- `gionx ws` must resolve target workspace by either:
  - explicit `--id <id>`
  - current workspace context path (`workspaces/<id>/...` or `archive/<id>/...`)

## Selection flow

- Stage 1: select exactly one workspace from list scope.
- Stage 2: select action for selected workspace.
  - active scope: `go`, `add-repo`, `close`
  - archived scope: `reopen`, `purge`
- Stage 3: dispatch to operation command with explicit `<id>`.

## Action routing policy

- Edit operations for existing workspace resources are routed by `--act`.
- Read-only commands remain subcommands (`ws list`, `ws ls`) and resource creation remains `ws create`.
- `ws go|close|add-repo|reopen|purge` subcommands are not part of supported entrypoints.
- Non-interactive usage should prefer explicit `--id` and must not rely on interactive selectors.
