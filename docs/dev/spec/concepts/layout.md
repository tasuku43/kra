---
title: "Layout"
status: proposed
---

# Layout

## KRA_ROOT

`KRA_ROOT` is a user-chosen working directory that is intended to be Git-managed.

## Root detection (filesystem)

When a command needs to operate on an existing root, `kra` detects root by:

1) If `current-context` is set: use it (must look like a root).
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
KRA_ROOT/
  AGENTS.md
  .kra/
    config.yaml
    logs/
    state/
      cmux-sessions.json
      cmux-workspaces.json
      root-repos.json
      operations/
        ws-close/
          <id>.json
  templates/
    <name>/        # workspace template root copied by ws create
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
- Template names follow the same validation rules as workspace IDs.
- Repo aliases are derived from the repo URL tail (e.g. `.../sugoroku.git` -> `sugoroku`).

## Git tracking policy

`kra` treats `KRA_ROOT` as a Git working tree.

- Track:
  - `.kra/config.yaml`
  - `workspaces/<id>/` except `repos/` (notes, artifacts, AGENTS.md, and any additional files)
  - everything under `archive/<id>/`
- Ignore:
  - `workspaces/<id>/repos/**` (git worktrees)
  - `.kra/logs/**`
  - `.kra/state/cmux-sessions.json`
  - `.kra/state/cmux-workspaces.json`
  - `.kra/state/root-repos.json`
  - `.kra/state/operations/**`
  - `.DS_Store`

Notes:

- Runtime operational files under `.kra/state/` are ignored by default unless a future spec explicitly promotes
  them to canonical tracked state.
- `.kra.meta.json` remains canonical workspace state and is tracked as part of each workspace/archive directory.
