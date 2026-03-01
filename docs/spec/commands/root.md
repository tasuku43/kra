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

## Error handling

- invalid arguments: `exitUsage` (json code: `invalid_argument`)
- root resolution failure: `exitErr` (json code: `not_found` or `internal_error`)
- cmux open failure: `exitErr` with mapped error code
