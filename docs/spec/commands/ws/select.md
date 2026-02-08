---
title: "`gionx ws` launcher and shared `--select` option"
status: planned
---

# `gionx ws` (no subcommand launcher) + `ws <op> --select`

## Purpose

Keep operation-fixed commands for automation while providing one human launcher path.

## Command forms

- Human launcher:
  - `gionx ws`
- Shared selector option:
  - `gionx ws go --select`
  - `gionx ws close --select`
  - `gionx ws add-repo --select`
  - `gionx ws reopen --select`
  - `gionx ws purge --select`

## Shared selector rule (`--select`)

- `--select` means: select workspace first, then run the specified command.
- Workspace selector uses single-select mode.
- Default scope is `active`.
- If command supports `--archived`, selector scope follows the flag.
- `--select` and direct `<id>` cannot be used together.

## Launcher behavior (`gionx ws`)

### When current directory resolves to workspace context

- If inside `workspaces/<id>/...`:
  - skip workspace selection
  - show `Action:` menu for current workspace:
    - `add-repo`
    - `close`
- If inside `archive/<id>/...`:
  - skip workspace selection
  - show `Action:` menu for current workspace:
    - `reopen`
    - `purge`

### When current directory does not resolve workspace context

- show workspace selector first (single-select)
- then show `Action:` menu for selected workspace

## Action menu

- heading: `Action:`
- first line: `workspace: <id>`
- interaction: vertical list (`Up/Down`, `Enter`, `Esc`)
- `Esc` behavior:
  - in action menu: go back to workspace selection stage (if it exists)
  - in first stage: cancel launcher without side effects

## Role boundary

- Human: launcher (`gionx ws`)
- Agent/automation: operation-fixed commands without interactive selection by default
- Launcher must dispatch to existing operation flows (no behavior drift).

