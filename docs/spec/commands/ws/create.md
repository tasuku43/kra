---
title: "`gionx ws create`"
status: planned
---

# `gionx ws create <id>`

## Purpose

Create an empty workspace with scaffolding for notes/artifacts.

## Inputs

- `id`: user-provided workspace ID
  - validation rules should follow `gion` (e.g. reject `/`)

## Behavior

- Create `GIONX_ROOT/workspaces/<id>/`
- Create:
  - `GIONX_ROOT/workspaces/<id>/notes/`
  - `GIONX_ROOT/workspaces/<id>/artifacts/`
  - `GIONX_ROOT/workspaces/<id>/AGENTS.md` with a short description of the directory meaning
    - include a short explanation of `notes/` vs `artifacts/`
- Prompt for `description` and store it in the global state store
  - if in a no-prompt mode, store an empty description
- Workspace ID collisions:
  - if `<id>` already exists as `active`, return an error and reference the existing workspace
  - if `<id>` already exists as `archived`, guide the user to `gionx ws reopen <id>`
  - if `<id>` was previously purged, allow creating it again as a new generation
- Do not create repos at this stage (repos are added via `ws add-repo`)
