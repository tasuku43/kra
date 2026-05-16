---
title: "Workspace task guide"
status: implemented
---

# Workspace task guide

This guide explains how to manage workspace-local tasks with `kra ws task`.

## What task data lives in

Structured tasks are stored in `<workspace>/tasks.md`.

- `tasks.md` is the source of truth
- `kra` is not the only editor; direct Markdown edits are allowed
- the cmux Dock can show a constantly refreshed task list, but it is only a view over `tasks.md`
- `kra ws task sync` is deprecated and no longer updates cmux task pills

Canonical task shape:

```md
## Tasks

### TASK-001 Draft docs
status: todo

Capture the main README changes.
```

Allowed statuses:

- `todo`
- `doing`
- `blocked`
- `done`

## Typical flow

```sh
kra ws task add --current --title "Draft docs"
kra ws task add --current --title "Review examples"
kra ws task list --current
kra ws task tui --current --refresh 2s
kra ws task status --current TASK-001 doing
```

`kra ws task` without a subcommand is also available as a human launcher for one active workspace.

New default workspaces include `tasks.md` and `.cmux/dock.json`. The Dock control runs `kra ws task tui --current --refresh 2s` from the workspace root so the cmux right sidebar can keep the current task state visible. When needed, the generated command prefixes the detected shell init file such as `source ~/.zshrc`. Existing roots and active workspaces can be updated with `kra root migrate --apply`; existing custom files are not overwritten.

## Common commands

- `kra ws task list` or `kra ws task ls` lists structured tasks from `tasks.md` and works for active and archived workspaces.
- `kra ws task tui` opens an interactive terminal list in `tasks.md` order. It starts in read mode; press `i` to enter write mode, then click or press Space/Enter to toggle a task done. Esc returns to read mode.
- `kra ws task tui --all --todo-only --refresh 2s` renders active workspace tasks across the root and hides completed tasks; add `--include-done` when you want completed tasks too.
- `kra ws task add --title "<text>"` lazily creates `tasks.md` and `## Tasks` when missing, and new tasks always start as `todo`.
- `kra ws task status <task-id> <todo|doing|blocked|done>` updates one task status.
- `kra ws task sync` is a deprecated no-op kept for compatibility; use `kra ws task tui` and Dock.

## Reading the list

Human output keeps one flat `Tasks:` section and uses these status icons:

- `○` = `todo`
- `●` = `doing`
- `▲` = `blocked`
- `✔` = `done`

This keeps backlog and in-progress items visible without requiring `cmux`.

## Direct edits

You can edit `tasks.md` directly as long as the structured task contract is preserved.

After direct edits, run `kra ws task tui --current --refresh 2s` or use the cmux Dock to inspect the rendered state.

## Scope and limits

- `list` can read both active and archived workspaces
- `add`, `status`, and the launcher are active-workspace operations
- `sync` is deprecated and performs no cmux updates
- phase 1 does not include task delete, reorder, rename, or nested subtasks

## Related docs

- Command overview: `docs/user/guides/COMMANDS.md`
- cmux integration guide: `docs/user/guides/CMUX.md`
- Workspace task contract: `docs/dev/spec/concepts/workspace-tasks.md`
- Command contract: `docs/dev/spec/commands/ws/task.md`
- Add contract: `docs/dev/spec/commands/ws/task/add.md`
- List contract: `docs/dev/spec/commands/ws/task/list.md`
- Status contract: `docs/dev/spec/commands/ws/task/status.md`
- Deprecated sync contract: `docs/dev/spec/commands/ws/task/sync.md`
