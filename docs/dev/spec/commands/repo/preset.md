---
title: "`kra repo preset`"
status: implemented
---

# `kra repo preset`

## Usage

```sh
kra repo preset add <name> [--yes]
kra repo preset rm <name>
kra repo preset remove <name>
kra repo preset list
kra repo preset show <name>
```

## Purpose

Manage reusable repository sets for `kra ws add-repo --preset <name>`.

This feature is intentionally separate from workspace templates:

- workspace template: directory scaffold used by `ws create`
- repo preset: repository key set used by `ws add-repo`

## Root resolution

`kra repo preset` resolves root in this order:

1. `KRA_ROOT`
2. current context (`~/.kra/state/current-context`)
3. walk-up discovery from cwd

## Storage

- source of truth: `<KRA_ROOT>/.kra/config.yaml`
- schema path:
  - `workspace.repo_presets.<name>.repos[]`
- each item in `repos[]` is a `repo_key` string.
- `repos[]` must be non-empty.
- order must preserve selection order from `preset add`.

## Commit behavior

- `preset add` and `preset rm/remove` auto-commit root-local config changes in `KRA_ROOT`.
- Commit messages:
  - add: `repo-preset-add: <name>`
  - remove: `repo-preset-remove: <name>`
- Staging allowlist:
  - `.kra/config.yaml`
- Preserve unrelated staged changes outside the allowlist.
- If the config write produces no Git diff, do not create an empty commit.

## `preset add`

- syntax: `kra repo preset add <name> [--yes]`
- candidate source:
  - current root registered repos (`repo add` registered)
- selection:
  - TTY: shared multi-select inline selector (`space` toggle, `enter` confirm)
  - non-TTY: numbered fallback (`comma numbers, /filter, empty=cancel`)
- same-name behavior:
  - TTY: ask overwrite confirmation (`y/yes`)
  - non-TTY: overwrite is allowed only with `--yes`
- empty selection aborts with non-zero exit

## `preset rm` / `preset remove`

- remove one preset from root config
- fail with `not_found` when target preset does not exist

## `preset list`

- print preset names in ascending order

## `preset show`

- print one preset with repos in stored order
- fail with `not_found` when target preset does not exist

## JSON contract

JSON mode is out of scope for current MVP.

## Related command behavior (`ws add-repo --preset`)

- `kra ws add-repo --preset <name>` resolves repo list from the preset and skips repo selector UI.
- strict validation:
  - if any preset repo is not registered in current root, fail fast with non-zero exit.
  - partial apply is not allowed for missing registration.
- if a preset repo is already bound to the target workspace, it is treated as `skipped` and execution continues.
- apply path (preflight/apply/rollback) reuses existing `ws add-repo` flow.
