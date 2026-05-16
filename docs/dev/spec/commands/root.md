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
  - add missing default template task Dock scaffold:
    - `.cmux/dock.json`
    - `templates/default/.cmux/dock.json`
    - `templates/default/tasks.md`
  - add the same missing scaffold to active existing workspaces:
    - `workspaces/<id>/.cmux/dock.json`
    - `workspaces/<id>/tasks.md`
  - do not touch archived workspaces
  - do not overwrite existing files
  - do not write under `workspaces/<id>/repos/`
  - do not commit automatically
  - Dock command generation may prefix the task tui command with a detected shell init file so `kra`
    is available in cmux Dock execution:
    - zsh: `source ~/.zshrc; kra ws task tui --current --refresh 2s`
    - bash: source the first existing file from `~/.bashrc`, `~/.bash_profile`, `~/.profile`
    - fish: `source ~/.config/fish/config.fish`
  - only managed `kra-tasks` Dock controls are updated; custom commands are left unchanged
  - root Dock command is `kra ws task tui --all --todo-only --refresh 2s`
  - workspace Dock command is `kra ws task tui --current --refresh 2s`

## Error handling

- invalid arguments: `exitUsage` (json code: `invalid_argument`)
- root resolution failure: `exitErr` (json code: `not_found` or `internal_error`)
- cmux open failure: `exitErr` with mapped error code
