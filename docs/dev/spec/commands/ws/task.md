---
title: "`kra ws task`"
status: implemented
---

# `kra ws task`

## Purpose

Manage structured workspace-local tasks stored in `workspace.md` without requiring `kra` to be the only
editor.

## Namespace

- `kra ws task`
- `kra ws task list`
- `kra ws status`
- `kra ws task add`
- `kra ws task status`
- `kra ws task sync`

## Source of truth

- Task commands use the task contracts defined in:
  - `docs/dev/spec/concepts/workspace-tasks-yaml.md`
  - `docs/dev/spec/concepts/workspace-tasks.md`
- Source priority is `<workspace>/workspace.yaml`, then legacy `<workspace>/workspace.md`.
- Task commands must work when the file was created or edited outside `kra`, as long as the minimum
  contract is satisfied.

## Targeting

- Targeting follows `docs/dev/spec/commands/ws/select.md`.
- Commands must use explicit target mode:
  - `--id <id>`
  - `--current`
  - `--select`
- Commands must not auto-resolve workspace from current path unless `--current` is explicitly set.
- `kra ws task` without a subcommand is a human launcher:
  - resolve one active workspace target
  - select one task
  - select one allowed next status
  - update the task and exit
- `list` is read-only and may target both active and archived workspaces.
- `tui`, `add`, `status`, and the launcher are valid only for active workspaces because they may
  mutate task status.
- `sync` is the explicit permission boundary for cmux native Todo reconciliation and is valid only for active
  mapped workspaces.

## Mutation safety

- Task commands must preserve all content outside the `## Tasks` section.
- Task commands must mutate only the structured task blocks they own.
- `list` treats missing `workspace.md` or missing `## Tasks` as zero structured tasks.
- `tui` treats missing `workspace.md`, missing `## Tasks`, or zero structured tasks as an empty view.
- `add` must lazily create `workspace.md` when the file is absent.
- `add` must lazily create `## Tasks` when the file exists but the section is absent.
- Any command that encounters duplicate structured task IDs must fail closed.
- Any command that encounters an invalid structured task-like block that starts with `### TASK-...`
  must fail closed.
- `status` and the launcher update `workspace.md` only.

## cmux native Todo view

- The selected local task document remains the structured source of truth; cmux Todo is a state projection.
- `kra ws task sync` reconciles KRA-managed Todo items and preserves foreign Todo items.
- The command supports `--prefer kra|cmux` for explicit conflict resolution as defined by
  `cmux-todo-reconcile.md`.
- `kra ws status` remains the full terminal view for descriptions, blockers, and dependencies that cmux Todo
  cannot represent.
- Legacy status pills and the managed cmux Dock are not used.

## Phase 1 non-goals

- nested subtasks
- task deletion
- task reordering
- task rename/edit
- task-specific done criteria
