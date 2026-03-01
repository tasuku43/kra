---
title: "`kra context`"
status: implemented
---

# `kra context`

## Purpose

Manage named contexts (`name -> path`) and current selection.

## Resolution order

`kra` resolves root in this order:

1. current-context file (`~/.kra/state/current-context`)
2. command-specific fallback (for commands that allow cwd-based discovery)

Notes:
- Root resolution is context-first.
- Environment variable based root override is not supported.

## Commands (MVP)

- `kra context current`
  - print current context name
  - fallback: print current path when name is unavailable (legacy entry)
  - `--format json` supported (`action=context.current`)
- `kra context list`
  - show known contexts from registry (`name`, `path`, `last_used_at`)
  - `--format json` supported (`action=context.list`)
- `kra context create <name> --path <path> [--use]`
  - validate path
  - persist name/path relation into registry
  - `--use` is specified, also select it as current context
  - `--format json` supported (`action=context.create`)
- `kra context use [name]`
  - when `<name>` is provided:
    - resolve context by name
    - write `current-context` atomically
  - when `<name>` is omitted in TTY:
    - open shared single-select UI and choose context interactively
  - non-TTY without `<name>` must fail fast with usage guidance.
  - `--format json` is non-interactive and requires explicit `<name>` (`action=context.use`)
  - print success in shared section style (`Result:`)
- `kra context rename <old> <new>`
  - rename context name in registry
  - fail when destination name already exists
  - `--format json` supported (`action=context.rename`)
- `kra context rm [name]`
  - when `<name>` is provided:
    - remove context name from registry
  - when `<name>` is omitted in TTY:
    - open shared single-select UI and choose context interactively
  - non-TTY without `<name>` must fail fast with usage guidance.
  - `--format json` is non-interactive and requires explicit `<name>` (`action=context.remove`)
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
  - section/title colors follow shared token rules from `docs/dev/spec/concepts/ui-color.md`.
