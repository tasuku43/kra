---
title: "Integration testing (gionx)"
status: planned
---

# Integration testing (gionx)

## Purpose

Define an expanded integration test plan for `gionx` commands through the archive lifecycle (`MVP-042`).

This ticket focuses on "drift" and partial failure scenarios across:
- filesystem under `GIONX_ROOT`
- SQLite state store
- git repo pool + worktrees

## Scope (initial)

Commands implemented through `MVP-042`:
- `gionx init`
- `gionx ws create`
- `gionx ws add-repo`
- `gionx ws close`
- `gionx ws reopen`
- `gionx ws purge`

## Done definition

- Add tests that run the CLI (`CLI.Run`) and verify side effects in both FS and DB.
- Include at least some non-happy-path coverage for:
  - drift between DB and filesystem
  - git worktree constraints / repo pool issues (as applicable)
- Keep tests isolated:
  - temp `GIONX_ROOT` per test
  - isolated sqlite file per test
  - avoid using the developer’s global git config/state

## Candidate scenarios (non-exhaustive)

### `init`

- settings already initialized with different root/pool should error (drift protection)

### `ws create`

- invalid root should error
- filesystem collision should not insert DB rows
- allow re-create after `purged` (generation increments)

### `ws add-repo`

- repo pool missing/corrupted behavior
- worktree already checked out elsewhere (git worktree constraint)
- fetch failure (credentials/network) should have defined behavior (error vs continue)

### Archive lifecycle

- `ws close` removes worktrees and archives atomically (FS + DB + git)
- `ws reopen` restores archived workspace and recreates worktrees (FS + DB + git)
- `ws purge` removes workspace snapshot + files with confirmations (FS + DB)
