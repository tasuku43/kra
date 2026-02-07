---
title: "Workspace Lifecycle"
status: implemented
---

# Workspace Lifecycle

This document defines the operational lifecycle model for workspaces.

## Canonical states

- `active`: currently in progress under `workspaces/<id>/`
- `archived`: task-complete state under `archive/<id>/`
- `purged`: terminal state (no workspace snapshot row)

## State transitions

Allowed transitions:

- `create`: (none) -> `active`
- `close`: `active` -> `archived`
- `reopen`: `archived` -> `active`
- `purge`: `active` -> `purged` or `archived` -> `purged`

Disallowed transitions:

- `active` -> `active` by `create` with same id (must error)
- `archived` -> `archived` by repeated `close` (must error)
- direct restore from `purged` (must use `create`, new generation)

## Operational policy

- Normal operation should keep completed tasks in `archived` (not purged).
- `purge` is an explicit destructive cleanup operation.
- Commands must enforce transitions in the application layer and append events atomically with snapshot updates.

## UI implications

- Selector-driven commands should present state-specific lists:
  - close/go: default scope is `active`
  - reopen/purge: default scope is `archived`
- For visibility, commands may support explicit scope flags (for example `ws go --archived`).
