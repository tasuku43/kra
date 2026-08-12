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
  - ensure the root's cmux Workspace Group and focus its managed anchor
  - the group anchor is a dedicated cmux workspace at `KRA_ROOT` and is mirrored by mapping key `KRA_ROOT`
  - do not override the user's group collapse, pin, color, icon, or later name customization
  - when group capabilities are absent, open/reuse the legacy ungrouped `KRA_ROOT` target with a warning
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
  - workspace task and handoff source of truth is `<workspace>/workspace.md`
  - Dock management is retired:
    - do not create, inspect, parse, update, migrate, report, or remove `.cmux/dock.json`
    - leave existing files byte-for-byte unchanged at:
      - `<KRA_ROOT>/.cmux/dock.json`
      - `<KRA_ROOT>/templates/default/.cmux/dock.json`
      - `<KRA_ROOT>/workspaces/<id>/.cmux/dock.json`
      - `~/.config/cmux/dock.json`
    - invalid Dock JSON is also ignored because Dock files are outside this command's managed scope
  - human output includes the root, action count, and planned/applied workspace handoff actions
  - json output includes:
    - `action=root.migrate`
    - `result.root`
    - `result.mode`
    - `result.planned`
    - `result.applied`
    - `result.actions[]`

## Error handling

- invalid arguments: `exitUsage` (json code: `invalid_argument`)
- root resolution failure: `exitErr` (json code: `not_found` or `internal_error`)
- cmux open failure: `exitErr` with mapped error code
