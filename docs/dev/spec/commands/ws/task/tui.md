---
title: "`kra ws task tui`"
status: implemented
---

# `kra ws task tui [--id <workspace-id>] [--current] [--select] [--all] [--todo-only] [--include-done] [--refresh <duration>] [--no-color]`

## Purpose

Render an interactive terminal task view for humans and cmux Dock, and write status changes back to
`tasks.md`.

## Behavior

- Read `<workspace>/tasks.md` with the shared workspace task parser/model.
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
- On invalid task contract, fail closed by showing the error and refusing status updates.
- TUI starts in `read` mode.
- In `read` mode, click only selects a row and no task state is changed.
- Press `i` to enter `write` mode.
- Press Esc in `write` mode to return to `read` mode.
- In `write` mode, Click, Space, and Enter toggle the selected task:
  - non-`done` -> `done`
  - `done` -> `todo`
- Keyboard shortcuts may set explicit status:
  - `t`: `todo`
  - `g`: `doing`
  - `b`: `blocked`
  - `d`: `done`
- Status changes use the existing workspace task transition rules and update `<workspace>/tasks.md`.

## Targeting

- Use the same explicit target modes as other `kra ws task` commands:
  - `--id <id>`
  - `--current`
  - `--select`
  - `--all`
- `--id`, `--current`, and `--select` target active workspaces because TUI can mutate task status.
- `--select` selects active workspaces.
- `--all` reads active workspaces under the current KRA_ROOT and is intended for root-level Dock.

## Refresh

- TUI stays open until the user quits.
- TUI refreshes by polling every refresh interval so direct `tasks.md` edits can appear without
  restarting the Dock.
- Default refresh interval is `2s`.
- `--refresh` is parsed as a Go duration, such as `500ms`, `1s`, or `2s`.
- `--refresh <= 0` is invalid.
- `q` or Ctrl-C exits.
- Esc returns to `read` mode when in `write` mode; in `read` mode Esc is ignored.
- File watching libraries are not required.
- If a render fails, show the error until the next refresh.

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
kra ws task tui --current --refresh 2s
```

cmux reads project Dock configuration from `<workspace>/.cmux/dock.json`. Dock is the persistent view
over `<workspace>/tasks.md`; `kra ws task sync` is deprecated and no longer projects cmux task pills.

The root Dock command is:

```sh
kra ws task tui --all --todo-only --refresh 2s
```

Root Dock reads active workspaces only by default, and omits completed tasks unless `--include-done`
is used.
