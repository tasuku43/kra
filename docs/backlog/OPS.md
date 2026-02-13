---
title: "OPS backlog"
status: planned
---

# OPS Backlog

- [x] OPS-001: `kra doctor` baseline (state/fs/worktree health checks)
  - What: add a non-destructive health check command to detect common operational drifts before running workspace actions.
  - Specs:
    - `docs/spec/commands/doctor.md`
    - `docs/spec/concepts/state-store.md`
    - `docs/spec/concepts/layout.md`
  - Depends: none
  - Serial: yes

- [x] OPS-002: Workspace multi-select action (`ws select --multi`)
  - What: extend existing selector to support multi-selection with explicit action binding.
  - Specs:
    - `docs/spec/commands/ws/select-multi.md`
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/concepts/workspace-lifecycle.md`
  - Depends: OPS-001
  - Serial: yes

- [ ] OPS-003: Machine-readable output parity across major commands
  - What: align `--format json` contracts across core commands so automation can rely on common envelope and error semantics.
  - Specs:
    - `docs/spec/concepts/output-contract.md`
    - `docs/spec/commands/init.md`
    - `docs/spec/commands/context.md`
    - `docs/spec/commands/repo/add.md`
    - `docs/spec/commands/repo/remove.md`
    - `docs/spec/commands/repo/gc.md`
    - `docs/spec/commands/ws/list.md`
  - Depends: none
  - Parallel: yes
