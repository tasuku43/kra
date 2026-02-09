---
title: "POST-MVP backlog"
status: planned
---

# POST-MVP Backlog

- [x] POST-MVP-001: `gionx init` pending spec parity
  - What: implement `--root`, non-TTY guidance, interactive root prompt, and current-context update on success.
  - Specs:
    - `docs/spec/commands/init.md`
    - `docs/spec/commands/context.md`
  - Depends: MVP-010, MVP-051
  - Serial: yes

- [x] POST-MVP-002: state-store concept doc sync (FS-only)
  - What: remove SQLite/migration dependency wording and align with runtime FS-only behavior.
  - Specs:
    - `docs/spec/concepts/state-store.md`
  - Depends: FS-STATE-008
  - Parallel: yes

- [x] POST-MVP-003: backlog index key-doc cleanup
  - What: replace stale `migrations/*.sql` references in backlog index with registry-oriented docs.
  - Specs:
    - `docs/backlog/README.md`
    - `docs/spec/commands/state/registry.md`
  - Depends: POST-MVP-002
  - Parallel: yes

- [x] POST-MVP-004: data model SQLite wording audit
  - What: confirm `DATA_MODEL` expresses FS canonical model and has no SQLite dependency wording.
  - Specs:
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: POST-MVP-002
  - Parallel: yes

- [x] POST-MVP-005: legacy recovery doc sync
  - What: align legacy recovery guide with SQLite-retired runtime behavior.
  - Specs:
    - `docs/dev/LEGACY_SQLITE_RECOVERY.md`
  - Depends: POST-MVP-002
  - Parallel: yes

- [x] POST-MVP-006: README quick-start refresh
  - What: add `init --root` and context-aware startup examples.
  - Specs:
    - `README.md`
    - `docs/spec/commands/init.md`
    - `docs/spec/commands/context.md`
  - Depends: POST-MVP-001
  - Parallel: yes

- [x] POST-MVP-007: remove `GIONX_ROOT` env-based root resolution
  - What: remove environment-variable root override from runtime resolution and use context-first behavior.
  - Specs:
    - `docs/spec/commands/context.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/commands/init.md`
  - Depends: POST-MVP-001
  - Serial: yes
