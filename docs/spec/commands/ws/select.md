---
title: "`gionx ws` selection entrypoint policy"
status: implemented
---

# `gionx ws` / `gionx ws list --select`

## Purpose

Unify interactive selection into a single entrypoint while keeping operation commands explicit for automation.

## Entry policy

- `gionx ws` (no subcommand) is removed as launcher.
- Interactive selection must start from:
  - `gionx ws list --select`
  - `gionx ws list --select --archived` (archived scope)

## Selection flow

- Stage 1: select exactly one workspace from list scope.
- Stage 2: select action for selected workspace.
  - active scope: `go`, `add-repo`, `close`
  - archived scope: `reopen`, `purge`
- Stage 3: dispatch to operation command with explicit `<id>`.

## Operation command policy

- `ws go/close/reopen/purge` require explicit `<id>`.
- Operation-level `--select` is not supported.
- `ws add-repo` keeps direct/cwd resolution behavior and does not provide `--select`.
