---
title: "Layout"
status: implemented
pending:
  - init-layout
  - gitignore-worktrees
  - workspace-scaffolding
---

# Layout

## GIONX_ROOT

`GIONX_ROOT` is a user-chosen working directory that is intended to be Git-managed.

## Root detection (filesystem)

When a command needs to operate on an existing root, `gionx` detects `GIONX_ROOT` by:

1) If `$GIONX_ROOT` is set: use it (must look like a root).
2) Otherwise: walk up from the current working directory and pick the nearest directory that looks like a root.

A directory "looks like a root" when both of these exist and are directories:
- `workspaces/`
- `archive/`

## Workspace folders

At the workspace level, we separate "text-first" logs from "file-first" artifacts.

- `notes/`: investigation notes, decisions, TODOs, meeting notes, links, etc.
- `artifacts/`: files produced/collected during the task (screenshots, log dumps, curl outputs, PoC scripts, diagrams, etc.)

### Directories

```
GIONX_ROOT/
  AGENTS.md
  workspaces/
    <id>/
      AGENTS.md
      notes/
      artifacts/
      repos/
        <alias>/   # git worktree (must not be Git-tracked)
  archive/
    <id>/
      ...         # archived workspace contents (Git-tracked)
```

Notes:
- Workspace IDs are user-provided. The validation rules should follow `gion` (e.g. reject `/`).
- Repo aliases are derived from the repo URL tail (e.g. `.../sugoroku.git` -> `sugoroku`).

## Git tracking policy

`gionx` treats `GIONX_ROOT` as a Git working tree.

- Track:
  - `workspaces/<id>/` except `repos/` (notes, artifacts, AGENTS.md, and any additional files)
  - everything under `archive/<id>/`
- Ignore:
  - `workspaces/<id>/repos/**` (git worktrees)
