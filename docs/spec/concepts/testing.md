# Testing

## Goals

`gionx` is a workspace lifecycle tool that coordinates:
- the global state store (SQLite)
- the filesystem under `GIONX_ROOT`
- Git repositories (bare repo pool + worktrees)

The most common failures are not happy-path logic bugs, but drift / inconsistency between these layers.

This spec exists so an agent (e.g. Agen2MD) can extract test requirements and keep coverage honest.

## Core principles

- Always include non-happy-path tests.
- Prefer table-driven tests for drift scenarios.
- Keep side effects bounded:
  - use temporary directories for `GIONX_ROOT`
  - use an isolated SQLite file for each test
  - avoid relying on global user environment
- When a command performs multiple phases (DB + FS + Git), ensure tests cover partial failure behavior.

## Drift / inconsistency scenarios (must-have)

The following are considered "typical" and should have explicit tests:

### State store vs filesystem

- Workspace exists in DB, but `GIONX_ROOT/workspaces/<id>/` is missing.
- Workspace exists on disk, but not in DB (import path).
- Repo binding exists in DB, but `workspaces/<id>/repos/<alias>` is missing.
- Files exist in `archive/<id>` but DB says workspace is open (or vice versa).

### Git-specific

- Worktree branch is already checked out by another worktree (Git worktree constraint).
- Bare repo pool is missing or corrupted.
- `fetch` fails (network/credentials) while the user can still proceed for some operations.

## Command-level test matrix (MVP)

### `gionx init`

Non-happy path:
- state store path is not writable
- `GIONX_ROOT` exists but is not writable
- `GIONX_ROOT` is already a Git repo (must not overwrite)

### `gionx ws create`

Non-happy path:
- invalid workspace id (validation follows `gion`)
- workspace already exists and is open (error with reference)
- workspace already exists and is closed (guide to `ws reopen`)

### `gionx ws add-repo`

Non-happy path:
- workspace does not exist
- alias conflict in the same workspace
- remote fetch fails
- branch name invalid (`git check-ref-format` fails)
- branch already checked out by another worktree (expect an error)

### `gionx ws list`

Non-happy path:
- DB says repo exists, but worktree is missing (mark as missing, do not delete)
- filesystem has workspace not in DB (auto-import using directory name)

### `gionx ws close`

Non-happy path:
- repos are dirty/unpushed/diverged/unknown -> must prompt/confirm
- archive directory already exists -> error (avoid overwrite)
- commit should stage only touched paths (do not include unrelated changes)

### `gionx ws reopen`

Non-happy path:
- archive missing -> error
- workspace already exists -> error
- recorded branch cannot be checked out due to worktree constraint -> error
- commit should stage only touched paths

