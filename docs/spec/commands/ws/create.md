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
- Do not create repos at this stage (repos are added via `ws add-repo`)
