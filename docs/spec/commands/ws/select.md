---
title: "`gionx ws` selection entrypoint policy"
status: implemented
---

# `gionx ws` / `gionx ws select`

## Purpose

Unify interactive selection into a single entrypoint while keeping operation commands explicit for automation.

## Entry policy

- `gionx ws` is context-aware launcher.
- `gionx ws --id <id>` resolves launcher target explicitly by id.
- `gionx ws select` always starts from workspace selection.
- `gionx ws select --act <go|close|add-repo|reopen|purge>` skips action menu and executes fixed action.
- `gionx ws list --select` remains compatibility path for selection-first flow.
- `gionx ws` must not auto-fallback to workspace list selection when current path cannot resolve workspace.
  unresolved invocation should fail and instruct users to run `gionx ws select`.

## Selection flow

- Stage 1: select exactly one workspace from list scope.
- Stage 2: select action for selected workspace.
  - active scope: `go`, `add-repo`, `close`
  - archived scope: `reopen`, `purge`
- Stage 3: dispatch to operation command with explicit `<id>`.

## Operation command policy

- `ws go/close/reopen/purge` require explicit `<id>`.
- Operation-level `--select` is not supported.
- `ws add-repo` keeps direct/cwd resolution behavior and supports `--id`.
- `ws close` supports `--id` and cwd resolution fallback.
