---
title: "`kra root`"
status: implemented
---

# `kra root`

## Purpose

Provide root-level helpers around the conceptual `KRA_ROOT`.

## Commands

- `kra root current [--format human|json]`
  - resolve conceptual `KRA_ROOT` from current execution context
  - human: prints resolved path
  - json: `action=root.current`, `result.root=<path>`

- `kra root open [--format human|json]`
  - open conceptual `KRA_ROOT` as a single cmux workspace target
  - uses cmux mapping key `KRA_ROOT` under `.kra/state/cmux-workspaces.json`
  - cmux status text must be `kra:root`
  - when cmux capability is unavailable, fallback to shell-action `cd <KRA_ROOT>`
    - json: `mode=fallback-cd`, `runtime_available=false`

- `kra root migrate [--apply] [--format human|json]`
  - plan by default; mutate only when `--apply` is set
  - add missing default template workspace scaffold:
    - `templates/default/workspace.md`
  - add the same missing workspace scaffold to active existing workspaces:
    - `workspaces/<id>/workspace.md`
  - when `tasks.md` exists and `workspace.md` is absent, convert `tasks.md` to `workspace.md`
    and remove the legacy `tasks.md`
  - do not touch archived workspaces
  - do not overwrite existing files
  - do not write under `workspaces/<id>/repos/`
  - do not commit automatically
  - do not create new project-local `.cmux/dock.json` as standard scaffold
  - workspace task and handoff source of truth is `<workspace>/workspace.md`
  - detect legacy project-local Dock config at:
    - `<KRA_ROOT>/.cmux/dock.json`
    - `<KRA_ROOT>/templates/default/.cmux/dock.json`
    - `<KRA_ROOT>/workspaces/<id>/.cmux/dock.json`
  - archived workspaces are excluded from legacy Dock migration
  - `--apply` migrates detected managed legacy project-local Dock config to global Dock config:
    - path: `~/.config/cmux/dock.json`
    - create parent directory when missing
    - create global Dock config when missing
    - preserve existing global controls
    - add or update the `id == "kra-tasks"` control
    - pretty-print JSON with stable struct field order
  - standard global `kra-tasks` control:
    - `id`: `kra-tasks`
    - `title`: `Tasks`
    - `command`: `kra ws status --cmux-current`
    - `height`: `420`
  - global Dock uses the `--cmux-current` resolver so the current cmux workspace maps to the corresponding kra workspace
  - managed legacy control detection:
    - `id == "kra-tasks"`
    - `command` contains one of:
      - `kra ws status --current`
      - `kra ws task view --current`
      - `kra ws status --all`
      - `kra ws task view --all`
    - title `Tasks` or empty is only advisory; id and command are authoritative
    - controls with a different id, such as `kra-tasks2`, are custom and never auto-managed
  - legacy project-local Dock cleanup:
    - managed-only Dock config: install/update global `kra-tasks`, remove legacy `.cmux/dock.json`, and remove `.cmux/` when it becomes empty
    - mixed managed and custom controls: install/update global `kra-tasks`, leave legacy file unchanged, and warn that cmux may prefer project-local Dock over global Dock
    - custom-only controls: leave unchanged
    - invalid project-local Dock JSON: fail closed; do not modify the invalid file and do not continue other migrations
  - invalid global Dock JSON fails closed; existing file is not modified
  - human output includes:
    - global Dock config path
    - whether global `kra-tasks` is created, updated, or skipped
    - whether managed legacy project Dock config is removed
    - whether mixed/custom project Dock config is left unchanged
    - warning that trust prompts can remain while project-local Dock config remains
  - json output includes:
    - `action=root.migrate`
    - `result.global_dock.path`
    - `result.global_dock.changed`
    - `result.global_dock.created`
    - `result.global_dock.updated`
    - `result.global_dock.skipped`
    - `result.legacy_project_docks[]`
      - `path`
      - `kind`: `root`, `template`, or `workspace`
      - `workspace_id` when applicable
      - `action`: `remove`, `leave_unchanged`, or `error`
      - `reason`
    - `result.recommendations[]`

## Error handling

- invalid arguments: `exitUsage` (json code: `invalid_argument`)
- root resolution failure: `exitErr` (json code: `not_found` or `internal_error`)
- cmux open failure: `exitErr` with mapped error code
