---
title: "`gionx context`"
status: implemented
---

# `gionx context`

## Purpose

Manage the current `GIONX_ROOT` context when multiple roots exist.

## Resolution order

`gionx` resolves root in this order:

1. `GIONX_ROOT` environment variable (highest priority)
2. current-context file (`XDG_DATA_HOME/gionx/current-context`)
3. command-specific fallback (for commands that allow cwd-based discovery)

Notes:
- `GIONX_ROOT` behavior remains backward-compatible.
- Context is a fallback only when `GIONX_ROOT` is unset.

## Commands (MVP)

- `gionx context current`
  - print current root after applying the resolution order above
- `gionx context list`
  - show known roots from state registry with metadata (`last_used_at`)
- `gionx context use <root>`
  - validate `<root>` exists (or can be initialized via `gionx init`)
  - write `current-context` atomically
  - print success in shared section style (`Result:`)

## Error handling

- If `current-context` points to a non-existent path, show a clear recovery hint.
- If both env and context are set but differ, env wins (no warning in MVP).
- Path writes must be atomic (temp + rename).

## Out of scope

- shell integration (`eval`, auto-export helpers)
- named aliases for roots

## Output

- `context current`:
  - keep machine-friendly plain path output (`<root>`) for composability.
- `context use <root>`:
  - success output:
    - `Result:`
    - `  Context set: <root>`
  - section/title colors follow shared token rules from `docs/spec/concepts/ui-color.md`.
