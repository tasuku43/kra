---
title: "`kra ws` workspace targeting options"
status: implemented
---

# `kra ws` workspace targeting options

## Purpose

Define explicit workspace targeting modes shared by workspace action commands.

## Dual-entry contract

- Targeting options apply to workspace action commands such as:
  - `kra ws open`
  - `kra ws add-repo`
  - `kra ws remove-repo`
  - `kra ws close`
  - `kra ws reopen`
  - `kra ws purge`

## Entry policy

- Action commands must use explicit target mode:
  - `--id <id>`: resolve by workspace id
  - `--current`: resolve from current path context (`workspaces/<id>/...` or `archive/<id>/...`)
  - `--select`: interactive selection-first flow
- Action command without any target mode must fail with usage error.
- `--id <id>` resolves target explicitly by id.
- `--current` resolves target from current path only when explicitly set.
- `--select` always starts from workspace selection.
- `--select --archived open|add-repo|remove-repo|close` must fail with usage error.
- `--select --multi` requires action.
- `--select --multi <close|reopen|purge>` enables multi-selection and executes the fixed action for each
  selected workspace.
- `--select --multi close` is active-scope only (`--archived` is invalid).
- `--select --multi reopen|purge` implicitly switches to archived scope.
- `--select --multi` runs lifecycle commits by default; `--no-commit` disables commits for selected action.
- `--select --multi --commit` is accepted for backward compatibility and keeps default behavior.
- `open|add-repo|remove-repo` are not supported in `--multi` mode.
- Commands must not auto-resolve workspace from current path unless `--current` is explicitly set.

## Selection flow

- Stage 1: select exactly one workspace from list scope.
- Stage 1 (multi): select one or more workspaces from list scope when `--multi` is set.
- Stage 2: select workspace for the current action command.
- Stage 3: dispatch to operation command with explicit target id.
- Stage 3 (multi): dispatch selected fixed action for each selected workspace id.

## Action routing policy

- Workspace action commands are routed by `ws <action>` subcommands.
- Read-only commands remain subcommands (`ws list`, `ws ls`) and resource creation remains `ws create`.
- Non-interactive usage should prefer explicit `--id` and must not rely on interactive selectors.
