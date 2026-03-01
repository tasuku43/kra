---
title: "DOC-QUALITY backlog"
status: planned
---

# DOC-QUALITY Backlog

- [x] DOC-QUALITY-001: state-store concept doc sync (FS-only)
  - What: remove SQLite/migration dependency wording and align with runtime FS-only behavior.
  - Specs:
    - `docs/dev/spec/concepts/state-store.md`
  - Depends: FS-STATE-008
  - Parallel: yes

- [x] DOC-QUALITY-002: backlog index key-doc cleanup
  - What: replace stale `migrations/*.sql` references in backlog index with registry-oriented docs.
  - Specs:
    - `docs/dev/backlog/README.md`
    - `docs/dev/spec/commands/state/registry.md`
  - Depends: DOC-QUALITY-001
  - Parallel: yes

- [x] DOC-QUALITY-003: data model SQLite wording audit
  - What: confirm `DATA_MODEL` expresses FS canonical model and has no SQLite dependency wording.
  - Specs:
    - `docs/dev/spec/core/DATA_MODEL.md`
  - Depends: DOC-QUALITY-001
  - Parallel: yes

- [x] DOC-QUALITY-004: legacy recovery doc sync
  - What: align legacy recovery guide with SQLite-retired runtime behavior.
  - Specs:
    - `docs/dev/LEGACY_SQLITE_RECOVERY.md`
  - Depends: DOC-QUALITY-001
  - Parallel: yes

- [x] DOC-QUALITY-005: CLI UI regression test matrix (E2E + component golden)
  - What: add comprehensive UI regression coverage across core commands and shared renderers.
    Include command-level E2E golden snapshots and component-level golden snapshots to detect
    layout/section/prompt drift early.
  - Specs:
    - `docs/dev/spec/testing/ui-regression.md`
    - `docs/dev/spec/testing/integration.md`
  - Depends: CTX-ROOT-003
  - Parallel: yes
