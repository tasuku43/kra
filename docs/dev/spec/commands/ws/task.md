---
title: "`kra ws task`"
status: planned
---

# `kra ws task`

## Purpose

Manage structured workspace-local tasks stored in `tasks.md` without requiring `kra` to be the only
editor.

## Namespace

- `kra ws task list`
- `kra ws task add`
- `kra ws task start`
- `kra ws task block`
- `kra ws task done`

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
- `list` is read-only and may target both active and archived workspaces.
- `add`, `start`, `block`, and `done` are mutating commands and are valid only for active workspaces.

## Mutation safety

- Task commands must preserve all content outside the `## Tasks` section.
- Task commands must mutate only the structured task blocks they own.
- `list` treats missing `tasks.md` or missing `## Tasks` as zero structured tasks.
- `add` must lazily create `tasks.md` when the file is absent.
- `add` must lazily create `## Tasks` when the file exists but the section is absent.
- Any command that encounters duplicate structured task IDs must fail closed.
- Any command that encounters an invalid structured task-like block that starts with `### TASK-...`
  must fail closed.

## Phase 1 non-goals

- nested subtasks
- task deletion
- task reordering
- task rename/edit
- reverting task state back to `todo`
- task-specific done criteria
