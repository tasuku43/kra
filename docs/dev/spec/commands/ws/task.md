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
- `list` is read-only and may target both active and archived workspaces.
- `add`, `status`, `sync`, and the launcher are valid only for active workspaces.

## Mutation safety

- Task commands must preserve all content outside the `## Tasks` section.
- Task commands must mutate only the structured task blocks they own.
- `list` treats missing `tasks.md` or missing `## Tasks` as zero structured tasks.
- `add` must lazily create `tasks.md` when the file is absent.
- `add` must lazily create `## Tasks` when the file exists but the section is absent.
- Any command that encounters duplicate structured task IDs must fail closed.
- Any command that encounters an invalid structured task-like block that starts with `### TASK-...`
  must fail closed.
- `status` and the launcher update `tasks.md` first, then invoke `sync`.

## CMUX sync projection

- `tasks.md` remains the single source of truth.
- `kra ws task sync` reconciles the current declaration into cmux sidebar status pills.
- `sync` manages only the `task:` namespace in cmux status entries.
- Projection rules:
  - `todo`, `doing`, `blocked`, and `done` are projected
- Projected cmux status shape:
  - key: `task:TASK-001`
  - icon: `checklist`
  - value prefixes:
    - `todo -> ○ TASK-001 Draft docs`
    - `doing -> ● TASK-001 Build parser`
    - `blocked -> ▲ TASK-001 Waiting review`
    - `done -> ✔ TASK-001 Shipped`
- Suggested color semantics:
  - `todo -> default text / white`
  - `doing -> info`
  - `blocked -> warning`
  - `done -> muted`
- `sync` is a full reconcile:
  - desired task pills are upserted
  - stale `task:` pills not present in `tasks.md` are cleared
- `status` and the launcher reuse the same sync behavior after mutating `tasks.md`.

## Phase 1 non-goals

- nested subtasks
- task deletion
- task reordering
- task rename/edit
- task-specific done criteria
