---
title: "Workspace task contract"
status: planned
---

# Workspace task contract

## Purpose

Define one workspace-local, AI-editable structured task contract that remains safe when
`tasks.md` is read or updated outside `kra`.

## Scope

- Applies to active and archived workspace folders.
- Canonical task document path is:
  - `workspaces/<id>/tasks.md`
  - `archive/<id>/tasks.md`
- The file is optional.
- Absence of `tasks.md` means the workspace has zero structured tasks.

## Design goals

- AI agents and humans may read and write `tasks.md` directly.
- `kra` is not the sole editor for task data.
- `kra` defines the minimum contract, parses compliant task blocks, and writes a canonical form.
- Arbitrary freeform Markdown may coexist with structured tasks in the same file.
- `kra` must ignore freeform content outside the structured task section.

## File contract

- `tasks.md` may contain arbitrary Markdown before and after the structured task section.
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

## Task model

- Task fields in phase 1:
  - `id`
  - `title`
  - `status`
  - `description` (optional)
- Tasks are flat. Nested tasks and subtasks are out of scope.
- Task-level done criteria are out of scope.
- File order is author-controlled and preserved by `kra`.

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
