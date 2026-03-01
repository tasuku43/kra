---
title: "MVP backlog"
status: planned
---

# MVP Backlog

- [x] MVP-001: Project skeleton (CLI entrypoint + subcommand routing)
  - Specs: `docs/dev/spec/README.md`
  - Depends: -
  - Parallel: with MVP-002/MVP-003 if different owners; otherwise do serial

- [x] MVP-002: Path resolution and root detection
  - What: resolve `KRA_ROOT`, XDG paths for `state.db` and repo pool
  - Specs:
    - `docs/dev/spec/concepts/state-store.md`
    - `docs/dev/spec/concepts/layout.md`
  - Depends: MVP-001
  - Parallel: yes (with MVP-003)

- [x] MVP-003: SQLite state store + migrations runner
  - What: open DB, `PRAGMA foreign_keys=ON`, apply `migrations/*.sql`, track `schema_migrations`
  - Specs:
    - `docs/dev/spec/concepts/state-store.md`
    - `docs/dev/spec/core/DATA_MODEL.md`
    - `migrations/0001_init.sql`
  - Depends: MVP-001
  - Parallel: yes (with MVP-002)

## Init

- [x] MVP-010: `kra init`
  - What: ensure layout dirs, create root `AGENTS.md`, create `.gitignore`, run `git init` if needed,
    initialize state store settings
  - Specs:
    - `docs/dev/spec/commands/init.md`
    - `docs/dev/spec/core/AGENTS.md`
    - `docs/dev/spec/concepts/layout.md`
    - `docs/dev/spec/concepts/state-store.md`
  - Depends: MVP-002, MVP-003
  - Serial: yes (blocks most commands)

## Workspace core (no Git worktrees yet)

- [x] MVP-020: `kra ws create`
  - What: create workspace scaffolding + insert workspace snapshot + append `created` event
  - Specs:
    - `docs/dev/spec/commands/ws/create.md`
    - `docs/dev/spec/concepts/layout.md`
    - `docs/dev/spec/core/DATA_MODEL.md`
  - Depends: MVP-010
  - Parallel: yes (with MVP-021 once MVP-010 done)

- [x] MVP-021: `kra ws list` (without repo risk at first)
  - What: list snapshot + basic drift import/mark-missing behavior
  - Specs:
    - `docs/dev/spec/commands/ws/list.md`
    - `docs/dev/spec/core/DATA_MODEL.md`
  - Depends: MVP-010
  - Parallel: yes

## Repo pool + worktrees

- [x] MVP-030: Repo pool (bare clone store) access
  - What: ensure bare repo for `repo_uid`, fetch, default branch detection
  - Specs:
    - `docs/dev/spec/concepts/state-store.md`
    - `docs/dev/spec/core/DATA_MODEL.md`
  - Depends: MVP-002, MVP-003
  - Parallel: yes (can start before MVP-010; used by add-repo/reopen)

- [x] MVP-031: `kra ws add-repo`
  - What: normalize repo spec, derive alias, prefetch, branch/base_ref prompt, create worktree,
    record `workspace_repos`
  - Specs:
    - `docs/dev/spec/commands/ws/add-repo.md`
    - `docs/dev/spec/concepts/layout.md`
    - `docs/dev/spec/core/DATA_MODEL.md`
  - Depends: MVP-010, MVP-030, MVP-020
  - Serial: yes (blocks close/reopen for real usage)

## Archive lifecycle (Git-managed root)

- [x] MVP-040: `kra ws close`
  - What: risk inspection (live), delete worktrees, atomic rename to `archive/`, commit touched paths,
    update snapshot + append event
  - Specs:
    - `docs/dev/spec/commands/ws/close.md`
    - `docs/dev/spec/concepts/layout.md`
    - `docs/dev/spec/core/DATA_MODEL.md`
  - Depends: MVP-031
  - Serial: yes

- [x] MVP-041: `kra ws reopen`
  - What: atomic rename back, recreate worktrees, commit touched paths, update snapshot + append event
  - Specs:
    - `docs/dev/spec/commands/ws/reopen.md`
    - `docs/dev/spec/concepts/layout.md`
    - `docs/dev/spec/core/DATA_MODEL.md`
  - Depends: MVP-040
  - Serial: yes

- [x] MVP-042: `kra ws purge`
  - What: confirmations, delete dirs, remove snapshot row, append event, commit touched paths
  - Specs:
    - `docs/dev/spec/commands/ws/purge.md`
    - `docs/dev/spec/core/DATA_MODEL.md`
  - Depends: MVP-040
  - Parallel: with MVP-041 if we want (but usually serial to validate lifecycle first)

## Hardening / tests

- [x] MVP-050: `kra state` foundation (registry)
  - What: introduce registry metadata for root-scoped `state.db` discovery and hygiene workflows
  - Specs:
    - `docs/dev/spec/commands/state/registry.md`
    - `docs/dev/spec/concepts/state-store.md`
  - Depends: MVP-002, MVP-003, MVP-010
  - Parallel: yes (independent from ws archive lifecycle)

- [x] MVP-051: `kra context` (root switch fallback)
  - What: add context management and root resolution fallback when `KRA_ROOT` is unset
  - Specs:
    - `docs/dev/spec/commands/context.md`
    - `docs/dev/spec/commands/state/registry.md`
  - Depends: MVP-050
  - Parallel: yes

- [x] MVP-060: `kra repo add` (shared pool upsert)
  - What: add root-scoped repo registration command that upserts shared bare pool and current root `repos` rows
    from repo specs (best-effort, conflict-safe).
  - Specs:
    - `docs/dev/spec/commands/repo/add.md`
    - `docs/dev/spec/concepts/state-store.md`
  - Depends: MVP-051
  - Parallel: yes

- [x] MVP-061: `kra repo discover` (provider-based bulk add)
  - What: add provider adapter flow (`--provider` default github) to discover org repos, exclude current-root
    registered repos, and bulk add selected repos through `repo add` path.
  - Specs:
    - `docs/dev/spec/commands/repo/discover.md`
    - `docs/dev/spec/commands/repo/add.md`
  - Depends: MVP-060
  - Parallel: yes

- [x] MVP-062: `kra repo remove` (root-local logical detach)
  - What: remove selected repos from current root `repos` registration using selector/direct mode.
    Keep physical bare repos untouched, and fail fast when selected repos are still referenced by
    `workspace_repos`.
  - Specs:
    - `docs/dev/spec/commands/repo/remove.md`
    - `docs/dev/spec/concepts/state-store.md`
  - Depends: MVP-060, MVP-061
  - Parallel: yes

- [x] MVP-063: `kra repo gc` (safe physical pool cleanup)
  - What: garbage-collect bare repos from shared pool only when safety gates pass.
  - Specs:
    - `docs/dev/spec/commands/repo/gc.md`
    - `docs/dev/spec/commands/state/registry.md`
  - Depends: MVP-062
  - Parallel: yes

- [x] MVP-900: Test harness + non-happy-path coverage baseline
  - What: temp `KRA_ROOT`, isolated sqlite file per test, drift scenario tests
  - Specs:
    - `docs/dev/TESTING.md`
  - Depends: MVP-003
  - Parallel: continuous (start early; extend per command)

- [x] MVP-901: Integration tests expansion (through MVP-042)
  - What: expand CLI-level tests to cover drift/partial-failure scenarios through the full archive lifecycle
  - Specs:
    - `docs/dev/spec/testing/integration.md`
  - Depends: MVP-042
  - Parallel: yes (recommended once lifecycle commands land)

## WS UX Polish (status-managed rollout)
