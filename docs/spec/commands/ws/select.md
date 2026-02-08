---
title: "`gionx ws select` unified human launcher"
status: planned
---

# `gionx ws select` / `gionx ws` (context-aware launcher)

## Purpose

Provide a single human-oriented entrypoint for workspace operations while keeping operation-fixed commands for
automation (`ws go`, `ws close`, `ws add-repo`).

## Command forms

- Canonical human launcher: `gionx ws select`
- Context-aware shortcut: `gionx ws`

## Role boundary

- Human launcher (`ws select` / `ws`) is interactive-first.
- Agent/automation entry remains operation-fixed commands (`ws go`, `ws close`, `ws add-repo`).
- No behavior semantics should diverge between launcher path and direct command path.

## Launcher behavior

### `gionx ws select` (always explicit)

1. Show active workspace selector in single-select mode.
2. After one workspace is selected, show action selector:
  - `go`
  - `add-repo`
  - `close`
3. Dispatch to existing command flow for the selected action.

### `gionx ws` (context-aware)

- If current working directory is outside `GIONX_ROOT/workspaces/<id>/...`:
  - behave as `gionx ws select`
- If current working directory is inside `GIONX_ROOT/workspaces/<id>/...`:
  - skip workspace selection
  - infer target workspace id from current path
  - show action selector with fixed choices/order:
    - `add-repo`
    - `close`
  - `go` must not be shown in this in-workspace action menu

## Selection model

- Single-select only for workspace selection in launcher flow.
- Multi-select behavior is out of this phase and may be introduced later via explicit design.

## TTY and non-interactive policy

- Launcher commands require interactive TTY.
- Non-TTY invocation of `ws select` (or `ws` in paths that require interaction) must fail fast.
- No automatic prompt fallback is allowed for launcher flow.

## Relationship with existing commands

- `ws go`, `ws close`, `ws add-repo` remain first-class and directly invokable.
- Launcher must delegate to shared internal execution flow to avoid drift.
- Existing command-specific safety gates and validations remain authoritative.

## Out of scope

- `ws current` command is not part of initial launcher scope.

