---
title: "`kra ws task`"
status: implemented
---

# `kra ws task`

## Purpose

Manage structured workspace-local tasks stored in `tasks.md` without requiring `kra` to be the only
editor.

## Namespace

- `kra ws task`
- `kra ws task list`
- `kra ws task view`
- `kra ws task add`
- `kra ws task status`
- `kra ws task sync`

## Source of truth

- Task commands use the workspace task contract defined in:
  - `docs/dev/spec/concepts/workspace-tasks.md`
- The canonical structured task document is `<workspace>/tasks.md`.
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
- `list` and `view` are read-only and may target both active and archived workspaces when addressed by
  `--id` or `--current`; `--select` selects active workspaces.
- `add`, `status`, and the launcher are valid only for active workspaces.
- `sync` is a deprecated compatibility no-op; it still resolves active workspace targets so invalid
  targets fail normally.

## Mutation safety

- Task commands must preserve all content outside the `## Tasks` section.
- Task commands must mutate only the structured task blocks they own.
- `list` treats missing `tasks.md` or missing `## Tasks` as zero structured tasks.
- `view` treats missing `tasks.md`, missing `## Tasks`, or zero structured tasks as an empty read-only
  view.
- `add` must lazily create `tasks.md` when the file is absent.
- `add` must lazily create `## Tasks` when the file exists but the section is absent.
- Any command that encounters duplicate structured task IDs must fail closed.
- Any command that encounters an invalid structured task-like block that starts with `### TASK-...`
  must fail closed.
- `status` and the launcher update `tasks.md` only.

## CMUX task view

- `tasks.md` remains the single source of truth.
- `kra ws task view` is the supported human-facing display for terminals and cmux Dock.
- cmux Dock configs should run `kra ws task view --current --watch --refresh 2s` for one
  workspace or `kra ws task view --all --todo-only --watch --refresh 2s` from the root.
- `kra ws task sync` no longer projects cmux task pills. It returns a deprecated/skipped result and
  does not update cmux state.

## Phase 1 non-goals

- nested subtasks
- task deletion
- task reordering
- task rename/edit
- task-specific done criteria
