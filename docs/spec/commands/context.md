---
title: "`gionx context`"
status: implemented
---

# `gionx context`

## Purpose

Manage named contexts (`name -> path`) and current selection.

## Resolution order

`gionx` resolves root in this order:

1. current-context file (`~/.gionx/state/current-context`)
2. command-specific fallback (for commands that allow cwd-based discovery)

Notes:
- Root resolution is context-first.
- Environment variable based root override is not supported.

## Commands (MVP)

- `gionx context current`
  - print current context name
  - fallback: print current path when name is unavailable (legacy entry)
- `gionx context list`
  - show known contexts from registry (`name`, `path`, `last_used_at`)
- `gionx context create <name> --path <path> [--use]`
  - validate path
  - persist name/path relation into registry
  - `--use` is specified, also select it as current context
- `gionx context use [name]`
  - when `<name>` is provided:
    - resolve context by name
    - write `current-context` atomically
  - when `<name>` is omitted in TTY:
    - open shared single-select UI and choose context interactively
  - non-TTY without `<name>` must fail fast with usage guidance.
  - print success in shared section style (`Result:`)
- `gionx context rename <old> <new>`
  - rename context name in registry
  - fail when destination name already exists
- `gionx context rm <name>`
  - remove context name from registry
  - fail when target is current context (safety guard)

## Error handling

- If `current-context` points to a non-existent path, show a clear recovery hint.
- If `context create` uses an existing name for another path, fail with clear conflict error.
- Path writes must be atomic (temp + rename).

## Out of scope

- shell integration (`eval`, auto-export helpers)
- named aliases for roots

## Output

- `context current`:
  - plain output (`<name>` or `<path>` fallback)
- `context use <name>`:
  - success output:
    - `Result:`
    - `  Context selected: <name>`
    - `  path: <root>`
  - section/title colors follow shared token rules from `docs/spec/concepts/ui-color.md`.
