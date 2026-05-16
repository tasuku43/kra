---
title: "`kra ws task view`"
status: implemented
---

# `kra ws task view [--id <workspace-id>] [--current] [--select] [--all] [--todo-only] [--include-done] [--watch] [--refresh <duration>] [--no-color]`

## Purpose

Render a terminal-friendly, read-only view of structured workspace tasks for humans and cmux Dock.

## Behavior

- Read `<workspace>/tasks.md` with the shared workspace task parser/model.
- Do not mutate `tasks.md`.
- Render structured tasks as a flat list in `tasks.md` file order.
- With `--all`, render active workspace tasks across the current KRA_ROOT, grouped by workspace key
  and title.
- With `--todo-only`, hide `done` tasks.
- With `--include-done`, include `done` tasks even when `--todo-only` is present.
- Use the same status icons as `kra ws task list`:
  - `todo`: `○`
  - `doing`: `●`
  - `blocked`: `▲`
  - `done`: `✔`
- Show an empty state when `tasks.md` is missing, when `## Tasks` is missing, or when there are no
  structured tasks.
- On invalid task contract, fail closed.

## Targeting

- Use the same explicit target modes as other `kra ws task` commands:
  - `--id <id>`
  - `--current`
  - `--select`
  - `--all`
- `--id` and `--current` may read active or archived workspaces.
- `--select` selects active workspaces.
- `--all` reads active workspaces under the current KRA_ROOT and is intended for root-level Dock.

## Watch mode

- Without `--watch`, render once and exit.
- With `--watch`, re-render by polling every refresh interval.
- Watch mode compares the rendered task state between polls and does not redraw when nothing changed.
- Default refresh interval is `2s`.
- `--refresh` is parsed as a Go duration, such as `500ms`, `1s`, or `2s`.
- `--refresh <= 0` is invalid.
- Ctrl-C exits watch mode.
- File watching libraries are not required.
- If a render fails in watch mode, show the error until the next refresh.

## Output

- Human output only; JSON output is intentionally not provided because this command is a terminal view.
- Honor `--no-color`.
- Honor non-TTY color detection.
- Follow UI color guardrails:
  - no raw ANSI color codes
  - use shared semantic style helpers

## cmux Dock

The default Dock command is:

```sh
kra ws task view --current --watch --refresh 2s
```

cmux reads project Dock configuration from `<workspace>/.cmux/dock.json`. Dock is the persistent view
over `<workspace>/tasks.md`; `kra ws task sync` is deprecated and no longer projects cmux task pills.

The root Dock command is:

```sh
kra ws task view --all --todo-only --watch --refresh 2s
```

Root Dock reads active workspaces only by default, and omits completed tasks unless `--include-done`
is used.
