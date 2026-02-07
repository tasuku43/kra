---
title: "`gionx repo remove`"
status: implemented
---

# `gionx repo remove [<repo-key>...]`

## Purpose

Remove repository registrations from the current root state DB.

This command is a logical detach from the current root context.  
It does not delete physical bare repositories from the shared repo pool.

## Root resolution

`gionx repo remove` resolves root in this order:

1. `GIONX_ROOT`
2. current context (`XDG_DATA_HOME/gionx/current-context`)
3. walk-up discovery from cwd

## Selection behavior

- Selector mode (interactive):
  - run `gionx repo remove` without args
  - use shared inline selector (`space` toggle, `enter` confirm, filter typing)
  - section title: `Repo pool:`
- Direct mode (non-interactive friendly):
  - pass one or more `repo-key` args
  - selected set is resolved from current root `repos`

## Removal policy

- Removal target is current root `repos` rows only.
- If selected repo has one or more references from `workspace_repos` in the same root,
  command must fail fast and remove nothing.
- `repo_usage_daily` cleanup relies on FK cascade from `repos` deletion.

## Output flow

- `Repo pool:` section
  - selected repo keys
- `Result:` section
  - summary: `Removed <n> / <m>`
  - per-repo lines

## Safety notes

- Physical bare repo directories under repo pool are kept.
- Physical cleanup is handled by a separate `repo gc` flow.

