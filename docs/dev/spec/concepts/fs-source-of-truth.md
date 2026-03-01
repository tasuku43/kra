---
title: "FS Source of Truth"
status: implemented
---

# FS Source of Truth

## Purpose

Define the next architecture direction:
- filesystem is the canonical source of truth (SoT)
- JSON files are primary durable metadata
- index-like data is derived/rebuildable

This spec defines the current canonical model.

## Principles

- Canonical state lives under `KRA_ROOT` as files/directories.
- Runtime-derived values (risk, logical todo/in-progress) are not persisted.
- Root-external data under XDG is treated as cache/index unless explicitly required otherwise.
- Commands must continue to be safe under partial failure (atomic writes, strict allowlists, rollback where possible).

## Scope split

### Canonical (must survive by itself)

- workspace existence and physical state:
  - `workspaces/<id>/`
  - `archive/<id>/`
- workspace metadata and repo restore metadata:
  - `.kra.meta.json` stored inside workspace/archive directories

### Rebuildable (can be recreated)

- ranking/usage hints
- selector acceleration indexes
- cross-root scan caches
- workspace logical-state baseline/cache:
  - `.kra/state/workspace-baselines/<id>.json`
  - `.kra/state/workspace-workstate.json`

If rebuildable data is missing/corrupt, commands should either:
- rebuild automatically, or
- fail with a clear "reindex" recovery hint.

## Migration policy

- During migration window, command behavior is defined by command specs with explicit precedence.
- Target steady-state:
  - workspace lifecycle and reopen restore do not require SQL tables.
  - workspace action/list surfaces operate from filesystem + meta JSON.

## Non-goals (this phase)

- introduce remote server/database
- preserve strict event log parity from legacy schema if it conflicts with FS-first simplicity
