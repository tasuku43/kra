---
title: "gionx backlog"
status: planned
---

# Backlog

This file is the implementation backlog for `gionx`.

Rules:
- Each backlog item maps to one or more spec files in `docs/spec/**`.
- Dependencies are explicit so we can see what must be serial vs what can be parallel.
- When an item is complete, update the related spec file frontmatter to `status: implemented`.

Legend:
- **Serial**: on the critical path (blocks other items).
- **Parallel**: can be implemented independently once its dependencies are done.

## Foundation (critical path)

- [ ] MVP-001: Project skeleton (CLI entrypoint + subcommand routing)
  - Specs: `docs/spec/README.md`
  - Depends: -
  - Parallel: with MVP-002/MVP-003 if different owners; otherwise do serial

- [ ] MVP-002: Path resolution and root detection
  - What: resolve `GIONX_ROOT`, XDG paths for `state.db` and repo pool
  - Specs:
    - `docs/spec/concepts/state-store.md`
    - `docs/spec/concepts/layout.md`
  - Depends: MVP-001
  - Parallel: yes (with MVP-003)

- [ ] MVP-003: SQLite state store + migrations runner
  - What: open DB, `PRAGMA foreign_keys=ON`, apply `migrations/*.sql`, track `schema_migrations`
  - Specs:
    - `docs/spec/concepts/state-store.md`
    - `docs/spec/core/DATA_MODEL.md`
    - `migrations/0001_init.sql`
  - Depends: MVP-001
  - Parallel: yes (with MVP-002)

## Init

- [ ] MVP-010: `gionx init`
  - What: ensure layout dirs, create root `AGENTS.md`, create `.gitignore`, run `git init` if needed,
    initialize state store settings
  - Specs:
    - `docs/spec/commands/init.md`
    - `docs/spec/core/AGENTS.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/concepts/state-store.md`
  - Depends: MVP-002, MVP-003
  - Serial: yes (blocks most commands)

## Workspace core (no Git worktrees yet)

- [ ] MVP-020: `gionx ws create`
  - What: create workspace scaffolding + insert workspace snapshot + append `created` event
  - Specs:
    - `docs/spec/commands/ws/create.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-010
  - Parallel: yes (with MVP-021 once MVP-010 done)

- [ ] MVP-021: `gionx ws list` (without repo risk at first)
  - What: list snapshot + basic drift import/mark-missing behavior
  - Specs:
    - `docs/spec/commands/ws/list.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-010
  - Parallel: yes

## Repo pool + worktrees

- [ ] MVP-030: Repo pool (bare clone store) access
  - What: ensure bare repo for `repo_uid`, fetch, default branch detection
  - Specs:
    - `docs/spec/concepts/state-store.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-002, MVP-003
  - Parallel: yes (can start before MVP-010; used by add-repo/reopen)

- [ ] MVP-031: `gionx ws add-repo`
  - What: normalize repo spec, derive alias, prefetch, branch/base_ref prompt, create worktree,
    record `workspace_repos`
  - Specs:
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-010, MVP-030, MVP-020
  - Serial: yes (blocks close/reopen for real usage)

## Archive lifecycle (Git-managed root)

- [ ] MVP-040: `gionx ws close`
  - What: risk inspection (live), delete worktrees, atomic rename to `archive/`, commit touched paths,
    update snapshot + append event
  - Specs:
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-031
  - Serial: yes

- [ ] MVP-041: `gionx ws reopen`
  - What: atomic rename back, recreate worktrees, commit touched paths, update snapshot + append event
  - Specs:
    - `docs/spec/commands/ws/reopen.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-040
  - Serial: yes

- [ ] MVP-042: `gionx ws purge`
  - What: confirmations, delete dirs, remove snapshot row, append event, commit touched paths
  - Specs:
    - `docs/spec/commands/ws/purge.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-040
  - Parallel: with MVP-041 if we want (but usually serial to validate lifecycle first)

## Hardening / tests

- [ ] MVP-900: Test harness + non-happy-path coverage baseline
  - What: temp `GIONX_ROOT`, isolated sqlite file per test, drift scenario tests
  - Specs:
    - `docs/dev/TESTING.md`
  - Depends: MVP-003
  - Parallel: continuous (start early; extend per command)

