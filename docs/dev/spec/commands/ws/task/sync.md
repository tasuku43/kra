---
title: "`kra ws task sync`"
status: implemented
---

# `kra ws task sync [--id <workspace-id> | --current | --cmux-current | --select | --all] [--prefer kra|cmux] [--dry-run] [--format human|json]`

## Purpose

Explicitly reconcile KRA structured tasks with the mapped cmux workspace's native Todo checklist.

## Behavior

- Resolve target workspace using the shared `ws` targeting contract.
- `--all` resolves all active workspaces non-interactively.
- `--cmux-current` resolves one mapped workspace; when invoked from the managed `KRA_ROOT` group anchor, it
  resolves all active workspaces, matching the shared task-view targeting contract.
- Command is valid only for active workspaces.
- Read the selected local task document before accessing cmux; invalid content fails closed.
- Reconcile local state, live Todo, and the ignored last-sync base according to `cmux-todo-reconcile.md`.
- Preserve foreign Todo items and never pin cmux `workspace.status`.
- `--prefer kra|cmux` is valid only for resolving reported managed-item conflicts.
- `--dry-run` computes and reports the reconciliation plan without changing the local document, live Todo, or
  ignored base.
- No mapped cmux workspace returns success with `state=skipped` and a warning.
- In `--all` mode, isolate failures per workspace and return aggregate partial-failure output.

## Human output

- Render applied/pushed/pulled/created/removed/conflict counts and the workspace ID.
- Conflicts name the task ID and require an explicit retry with a preference.
- In `--all` mode, render an aggregate summary plus per-workspace results.

## JSON envelope

- `ok`
- `action=ws.task.sync`
- `workspace_id`
- `result`:
  - `sync`:
    - `state`
    - `pushed`
    - `pulled`
    - `created`
    - `removed`
    - `conflicts`
    - `warning` (optional)
- In `--all` mode, `result` is batch-shaped:
  - `count`
  - `succeeded`
  - `failed`
  - `skipped`
  - `pushed`
  - `pulled`
  - `created`
  - `removed`
  - `conflicts`
  - `workspaces[]`
  - `failures[]`
- `error`
