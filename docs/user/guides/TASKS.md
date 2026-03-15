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
- `kra ws task sync` projects the current task state into `cmux` sidebar `task:` entries

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
kra ws task status --current TASK-001 doing
kra ws task sync --current
```

`kra ws task` without a subcommand is also available as a human launcher for one active workspace.

## Common commands

- `kra ws task list` or `kra ws task ls` lists structured tasks from `tasks.md` and works for active and archived workspaces.
- `kra ws task add --title "<text>"` lazily creates `tasks.md` and `## Tasks` when missing, and new tasks always start as `todo`.
- `kra ws task status <task-id> <todo|doing|blocked|done>` updates one task status and then runs the same reconcile behavior as `kra ws task sync`.
- `kra ws task sync` refreshes the `cmux` sidebar task pills from the current `tasks.md`.

## Reading the list

Human output keeps one flat `Tasks:` section and uses these status icons:

- `○` = `todo`
- `●` = `doing`
- `▲` = `blocked`
- `✔` = `done`

This keeps backlog and in-progress items visible without requiring `cmux`.

## Direct edits

You can edit `tasks.md` directly as long as the structured task contract is preserved.

Typical pattern:

1. update `tasks.md`
2. run `kra ws task sync --current`

This is useful when an agent or a user wants to batch-edit descriptions in Markdown first and refresh the sidebar afterward.

## Scope and limits

- `list` can read both active and archived workspaces
- `add`, `status`, `sync`, and the launcher are active-workspace operations
- phase 1 does not include task delete, reorder, rename, or nested subtasks

## Related docs

- Command overview: `docs/user/guides/COMMANDS.md`
- cmux integration guide: `docs/user/guides/CMUX.md`
- Workspace task contract: `docs/dev/spec/concepts/workspace-tasks.md`
- Command contract: `docs/dev/spec/commands/ws/task.md`
- Add contract: `docs/dev/spec/commands/ws/task/add.md`
- List contract: `docs/dev/spec/commands/ws/task/list.md`
- Status contract: `docs/dev/spec/commands/ws/task/status.md`
- Sync contract: `docs/dev/spec/commands/ws/task/sync.md`
