---
title: "ARCH backlog"
status: planned
---

# ARCH Backlog

- [x] ARCH-001: Layered architecture spec and migration policy
  - What: define target package structure (`cli/app/domain/infra/ui`), dependency direction, and migration rules.
  - Specs:
    - `docs/spec/concepts/architecture.md`
    - `docs/spec/README.md`
  - Depends: -
  - Serial: yes

- [x] ARCH-002: Package skeleton + dependency guard tests
  - What: add package skeleton and guard tests preventing new direct infra usage from `cli`.
  - Specs:
    - `docs/spec/concepts/architecture.md`
  - Depends: ARCH-001
  - Serial: yes

- [x] ARCH-003: WS use case interfaces in `internal/app/ws`
  - What: define request/response contracts for launcher and operation commands (`go/close/add-repo/reopen/purge`).
  - Specs:
    - `docs/spec/concepts/architecture.md`
    - `docs/spec/commands/ws/select.md`
  - Depends: ARCH-002
  - Serial: yes

- [x] ARCH-004: Migrate `ws` launcher + `ws open` to app layer
  - What: route launcher and go through shared app use case path.
  - Specs:
    - `docs/spec/concepts/architecture.md`
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/commands/ws/open.md`
  - Depends: ARCH-003
  - Serial: yes

- [x] ARCH-005: Migrate `ws close/add-repo/reopen/purge` to app layer
  - What: route all WS operations through app layer, keeping existing behavior and safety gates.
  - Specs:
    - `docs/spec/concepts/architecture.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/commands/ws/reopen.md`
    - `docs/spec/commands/ws/purge.md`
  - Depends: ARCH-004
  - Serial: yes

- [x] ARCH-006: Migrate `init/context` to app layer
  - What: move root/context orchestration out of cli handlers into app use cases.
  - Specs:
    - `docs/spec/concepts/architecture.md`
    - `docs/spec/commands/init.md`
    - `docs/spec/commands/context.md`
  - Depends: ARCH-002
  - Parallel: yes

- [x] ARCH-007: Migrate `repo` command family to app layer
  - What: move repo add/discover/remove/gc orchestration into app use cases.
  - Specs:
    - `docs/spec/concepts/architecture.md`
    - `docs/spec/commands/repo/add.md`
    - `docs/spec/commands/repo/discover.md`
    - `docs/spec/commands/repo/remove.md`
    - `docs/spec/commands/repo/gc.md`
  - Depends: ARCH-002
  - Parallel: yes

- [x] ARCH-008: Migrate shell integration to app-aware action protocol adapter
  - What: isolate shell action protocol interaction as infra/ui adapter with app-level action emission.
  - Specs:
    - `docs/spec/concepts/architecture.md`
    - `docs/spec/commands/shell/init.md`
  - Depends: ARCH-004
  - Parallel: yes

- [x] ARCH-009: Remove direct infra access from `internal/cli`
  - What: complete migration and remove remaining direct `paths/statestore/gitutil` calls from cli package.
  - Specs:
    - `docs/spec/concepts/architecture.md`
  - Depends: ARCH-005, ARCH-006, ARCH-007, ARCH-008
  - Serial: yes

- [x] ARCH-010: Architecture finalization (`status: implemented`)
  - What: finalize docs and guards, and mark architecture spec as implemented.
  - Specs:
    - `docs/spec/concepts/architecture.md`
    - `docs/spec/README.md`
  - Depends: ARCH-009
  - Serial: yes

## External Integrations
