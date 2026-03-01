---
title: "FS-STATE backlog"
status: planned
---

# FS-STATE Backlog

- [x] FS-STATE-001: FS=SoT migration spec baseline
  - What: define filesystem as source of truth and position index data as rebuildable.
  - Specs:
    - `docs/spec/concepts/fs-source-of-truth.md`
    - `docs/spec/README.md`
  - Depends: -
  - Serial: yes (foundation for migration items)

- [x] FS-STATE-002: `ws create` writes `.kra.meta.json`
  - What: create workspace-local canonical metadata file with schema version and workspace core fields.
  - Specs:
    - `docs/spec/concepts/workspace-meta-json.md`
    - `docs/spec/commands/ws/create.md`
  - Depends: FS-STATE-001
  - Serial: yes

- [x] FS-STATE-003: `ws add-repo` persists restore metadata in `.kra.meta.json`
  - What: update `repos_restore` on successful bindings and keep alias/branch/base_ref deterministic.
  - Specs:
    - `docs/spec/concepts/workspace-meta-json.md`
    - `docs/spec/commands/ws/add-repo.md`
  - Depends: FS-STATE-002
  - Serial: yes

- [x] FS-STATE-004: `ws close` snapshots restore metadata from live worktrees
  - What: refresh `.kra.meta.json.repos_restore` before removing worktrees and archiving directory.
  - Specs:
    - `docs/spec/concepts/workspace-meta-json.md`
    - `docs/spec/commands/ws/close.md`
  - Depends: FS-STATE-003
  - Serial: yes

- [x] FS-STATE-005: `ws reopen` restores from `.kra.meta.json` (no DB binding dependency)
  - What: recreate worktrees exclusively from `repos_restore` and keep close/reopen reversible.
  - Specs:
    - `docs/spec/concepts/workspace-meta-json.md`
    - `docs/spec/commands/ws/reopen.md`
  - Depends: FS-STATE-004
  - Serial: yes

- [x] FS-STATE-006: logical work-state (`todo`/`in-progress`) in `ws list`/`ws open`
  - What: derive logical state at read time from filesystem + git signals, without DB persistence.
  - Specs:
    - `docs/spec/commands/ws/list.md`
    - `docs/spec/commands/ws/open.md`
    - `docs/spec/concepts/fs-source-of-truth.md`
  - Depends: FS-STATE-005
  - Parallel: yes

- [x] FS-STATE-007: `repo gc` safety gates read FS metadata + registry
  - What: evaluate references from workspace/archive metadata and known roots without relying on SQL joins.
  - Specs:
    - `docs/spec/commands/repo/gc.md`
    - `docs/spec/concepts/fs-source-of-truth.md`
  - Depends: FS-STATE-005
  - Parallel: yes (with FS-STATE-006)

- [x] FS-STATE-008: state-store downgrade/deprecation plan
  - What: redefine SQLite as optional/rebuildable index (or remove), and align docs/tests accordingly.
  - Specs:
    - `docs/spec/concepts/state-store.md`
    - `docs/spec/core/DATA_MODEL.md`
    - `docs/dev/TESTING.md`
  - Depends: FS-STATE-006, FS-STATE-007
  - Serial: yes (finalization step)

## Foundation (critical path)
