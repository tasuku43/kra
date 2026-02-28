---
title: "Testing"
status: implemented
---

# Testing

## Goals

`kra` is a workspace lifecycle tool that coordinates:
- filesystem metadata (`.kra.meta.json`)
- optional/rebuildable root index
- the filesystem under `KRA_ROOT`
- Git repositories (bare repo pool + worktrees)

The most common failures are not happy-path logic bugs, but drift / inconsistency between these layers.

This spec exists so an automation tool (e.g. Agen2MD) can extract test requirements and keep coverage honest.

## Core principles

- Always include non-happy-path tests.
- Prefer table-driven tests for drift scenarios.
- Keep side effects bounded:
  - use temporary directories for `KRA_ROOT`
  - use isolated root metadata/index files for each test
  - avoid relying on global user environment
- When a command performs multiple phases (index + FS + Git), ensure tests cover partial failure behavior.

## UI color compliance check

- Run `./scripts/lint-ui-color.sh` as part of the minimum quality gate.
- Goal: prevent ad-hoc color drift by enforcing semantic color token usage.

## Drift / inconsistency scenarios

The following are examples of "typical" drift/inconsistency scenarios.
They are not exhaustive, but should have explicit tests because they are common in practice:

### Metadata/index vs filesystem

- Workspace exists in metadata/index, but `KRA_ROOT/workspaces/<id>/` is missing.
- Workspace exists on disk, but metadata/index is stale or missing (import/reconcile path).
- Repo binding exists in metadata/index, but `workspaces/<id>/repos/<alias>` is missing.
- Files exist in `archive/<id>` but metadata says workspace is open (or vice versa).

### Git-specific

- Worktree branch is already checked out by another worktree (Git worktree constraint).
- Bare repo pool is missing or corrupted.
- `fetch` fails (network/credentials) while the user can still proceed for some operations.
