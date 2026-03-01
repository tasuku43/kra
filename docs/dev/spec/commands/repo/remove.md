---
title: "`kra repo remove`"
status: implemented
---

# `kra repo remove [--format human|json] [<repo-key>...]`

## Purpose

Remove repository registrations from the current root index.

This command is a logical detach from the current root context.  
It does not delete physical bare repositories from the shared repo pool.

## Root resolution

`kra repo remove` resolves root in this order:

1. `KRA_ROOT`
2. current context (`~/.kra/state/current-context`)
3. walk-up discovery from cwd

## Selection behavior

- Selector mode (interactive):
  - run `kra repo remove` without args
  - use shared inline selector (`space` toggle, `enter` confirm, filter typing)
  - section title: `Repo pool:`
- Direct mode (non-interactive friendly):
  - pass one or more `repo-key` args
  - selected set is resolved from current root `repos`
  - `--format json` requires direct mode (one or more repo keys)

## Removal policy

- Removal target is current root `repos` rows only.
- If selected repo has one or more workspace references in the same root,
  command must fail fast and remove nothing.
- `repo_usage_daily` cleanup relies on FK cascade from `repos` deletion.

## Output flow

- `Repo pool:` section
  - selected repo keys
- `Result:` section
  - summary: `Removed <n> / <m>`
  - summary color semantics follow shared `Result:` rules:
    - all success: success token
    - all failed: error token
    - mixed/partial: warning token
  - per-repo lines

## Safety notes

- Physical bare repo directories under repo pool are kept.
- Physical cleanup is handled by a separate `repo gc` flow.

## JSON mode (`--format json`)

- output envelope follows `docs/dev/spec/concepts/output-contract.md`
- action: `repo.remove`
- success result:
  - `removed`
  - `total`
  - `repos[]`
- blocked removal (workspace refs) returns `ok=false`, `error.code=conflict`.
