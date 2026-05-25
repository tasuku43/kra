---
title: "Workspace task contract"
status: implemented
---

# Workspace task contract

## Purpose

Define one workspace-local, AI-editable workspace state and task contract that remains safe when
`workspace.md` is read or updated outside `kra`.

## Scope

- Applies to active and archived workspace folders.
- Canonical task document path is:
  - `workspaces/<id>/workspace.md`
  - `archive/<id>/workspace.md`
- The file is optional.
- Absence of `workspace.md` means the workspace has no recorded current state and zero structured
  tasks.

## Design goals

- AI agents and humans may read and write `workspace.md` directly.
- `kra` is not the sole editor for task data.
- `kra` defines the minimum contract, parses compliant task blocks, and writes a canonical form.
- `workspace.md` is the single source of truth for current workspace state and task state.
- Arbitrary freeform Markdown may coexist with structured tasks in the same file.
- `kra` must ignore freeform content outside the structured task section.

## File contract

- `workspace.md` may contain arbitrary Markdown before and after the structured task section.
- `kra` does not require or interpret dedicated `## Current State` or `## Next` sections.
- `kra ws status` derives `Current Task` and `Next Task` from structured tasks:
  - `Current Task` is the first `doing` task in file order.
  - `Next Task` is the first `todo` task after the current `doing` task, wrapping to the first `todo`
    task when needed.
  - Each value displays the task `description`; when `description` is empty, it displays the task
    title.
- `kra` interprets only the first level-2 heading named `Tasks`:
  - `## Tasks`
- The structured task section ends at the next level-2 heading or EOF.
- Inside `## Tasks`, one structured task begins with one level-3 heading:
  - `### TASK-<nnn> <title>`
- `<nnn>` is a zero-padded decimal identifier.
- `id` is derived from `TASK-<nnn>` and is stable once created.
- `title` is derived from the remainder of the heading text and must be non-empty.
- Each structured task block must contain one required field line:
  - `status: <todo|doing|blocked|done>`
- `description` is optional and consists of arbitrary Markdown content after the required fields
  until the next structured task heading or the end of the `## Tasks` section.
- Content inside `## Tasks` that does not match the structured task contract is ignored unless it
  starts a structured task block. A task-like block that starts with `### TASK-...` but violates the
  contract is an invalid task contract error for `kra`.

## Read / write policy

- Read policy is lenient for freeform content:
  - freeform Markdown outside `## Tasks` is ignored
  - freeform Markdown inside `## Tasks` that is not part of a structured task block is ignored
- Read policy is strict for structured task blocks:
  - duplicate task IDs are invalid
  - missing required `status` is invalid
  - invalid heading shape for a task-like block is invalid
- Write policy is canonical:
  - `kra` writes structured task blocks using a stable heading + field layout
  - `kra` must preserve content outside `## Tasks`
  - `kra` must preserve non-target structured task blocks semantically while updating only the target
    task block or inserting a new task block
- `kra` commands must not require tasks to have been created through `kra`.

## View policy

- cmux Dock is the supported persistent status view.
- `kra ws status` reads `workspace.md`, renders task-derived `Current Task` / `Next Task` and the current
  structured task state for terminal or Dock use, and writes status changes back to `workspace.md`.
- With root/all-workspace views, `kra ws status --all` emphasizes each workspace's task-derived
  `Current Task` / `Next Task` and shows task progress as supplemental context instead of listing every task.
- `kra ws status` starts in read mode; users must enter write mode before clicks or keys mutate
  task status.
- Direct `workspace.md` edits remain valid as long as the contract is preserved; users or AI agents can
  run `kra ws status` afterward or rely on Dock refresh.
- `kra ws task sync` is deprecated and no longer updates cmux task pills.

## Task model

- Task fields in phase 1:
  - `id`
  - `title`
  - `status`
  - `description` (optional)
- Tasks are flat. Nested tasks and subtasks are out of scope.
- Task-level done criteria are out of scope.
- File order is author-controlled and preserved by `kra`.

## Task transitions

- Allowed transitions are:
  - `todo -> doing|blocked|done`
  - `doing -> todo|blocked|done`
  - `blocked -> todo|doing|done`
  - `done -> todo`
- Same-state updates are idempotent.

## Task state derivation

`task_state` is derived from structured tasks only:

- `empty`: no structured tasks exist
- `done`: all structured tasks are `done`
- `blocked`: at least one structured task exists and every unfinished task is `blocked`
- `doing`: at least one task is `doing`, or at least one task is `done` while unfinished tasks remain
- `todo`: otherwise

## Relation to workspace activity

- `task_state` and workspace `activity_state` are separate concepts.
- `activity_state` continues to describe observed workspace activity (for example repo/file drift).
- Workspace overview commands may combine both views:
  - primary workspace progress remains activity-oriented
  - task information is supplemental
- A workspace may therefore be:
  - activity-active while `task_state=empty`
  - activity-active while all tasks are `todo`
  - activity-active while `task_state=done`

## Related docs

- examples:
  - `docs/dev/spec/concepts/workspace-task-examples.md`
- overview integration:
  - `docs/dev/spec/concepts/workspace-task-overview.md`
