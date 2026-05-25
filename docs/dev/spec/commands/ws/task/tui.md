---
title: "`kra ws status`"
status: implemented
---

# `kra ws status [--id <workspace-id>] [--current] [--select] [--all] [--todo-only] [--include-done] [--no-color]`

## Purpose

Render an interactive terminal workspace-state/task view for humans and cmux Dock, and write status
changes back to `workspace.md`.

## Behavior

- Read `<workspace>/workspace.md` with the shared workspace task parser/model.
- Render task-derived `Current Task` and `Next Task` above task progress for single-workspace views.
  - `Current Task` uses the first `doing` task in file order.
  - `Next Task` uses the first `todo` task after the current `doing` task, wrapping to the first `todo`
    task when needed.
  - Both display task `description`, falling back to task title when `description` is empty.
- Render structured tasks as a flat list in `workspace.md` file order.
- With `--all`, render active workspace task-derived `Current Task` / `Next Task` across the current
  KRA_ROOT, grouped by workspace key and title, with task progress as supplemental context.
- With `--todo-only`, hide `done` tasks.
- With `--include-done`, include `done` tasks even when `--todo-only` is present.
- Use the same status icons as `kra ws task list`:
  - `todo`: `○`
  - `doing`: `●`
  - `blocked`: `▲`
  - `done`: `✔`
- Show an empty state when `workspace.md` is missing, when `## Tasks` is missing, or when there are no
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
- Status changes use the existing workspace task transition rules and update `<workspace>/workspace.md`.

## Targeting

- Use the same explicit target modes as other `kra ws task` commands:
  - `--id <id>`
  - `--current`
  - `--select`
  - `--all`
- `--id`, `--current`, and `--select` target active workspaces because TUI can mutate task status.
- `--select` selects active workspaces.
- `--all` reads active workspaces under the current KRA_ROOT and is intended for root-level Dock.

## Updates

- TUI stays open until the user quits.
- TUI automatically follows `workspace.md` changes so direct edits can appear without restarting the
  Dock.
- `--refresh` is not part of the public status command surface.
- Legacy `--refresh <duration>` arguments may be accepted as compatibility no-ops.
- `q` or Ctrl-C exits.
- Esc returns to `read` mode when in `write` mode; in `read` mode Esc is ignored.
- File watching libraries are not required.
- If a render fails, show the error until the next update.

## Output

- Human output only; JSON output is intentionally not provided because this command is a terminal view.
- Honor `--no-color`.
- Honor non-TTY color detection.
- Follow UI color guardrails:
  - no raw ANSI color codes
  - use shared semantic style helpers

## cmux Dock

Install the global cmux Dock integration explicitly with:

```sh
kra ws task dock install
```

`kra ws task dock install --global` is accepted as a compatibility alias because global Dock is the
only supported install target.

The global Dock command is:

```sh
kra ws status --cmux-current
```

When an existing global Dock config is present, install preserves existing controls and adds or
updates the managed `id == "kra-tasks"` control. Invalid global Dock JSON fails closed.

The legacy workspace Dock command was:

```sh
kra ws status --current
```

The legacy root Dock command was:

```sh
kra ws status --all --todo-only
```

`kra root migrate --apply` can migrate managed legacy project-local Dock configs to the global Dock.
`kra ws task sync` is deprecated and no longer projects cmux task pills.
