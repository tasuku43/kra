---
title: "TEMPLATE-WS backlog"
status: planned
---

# TEMPLATE-WS Backlog

- [x] TEMPLATE-WS-001: Workspace template model and `ws create` spec alignment
  - What: define root-local template model, reserved path policy, and make `ws create`
    template-first (`--template`, default=`default`, preflight validation, no scaffold fallback).
  - Specs:
    - `docs/spec/concepts/workspace-template.md`
    - `docs/spec/commands/ws/create.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/core/AGENTS.md`
  - Depends: DOC-QUALITY-005
  - Serial: yes

- [x] TEMPLATE-WS-002: `init` default template bootstrap
  - What: generate `<current-root>/templates/default/` on first init (no overwrite),
    with baseline `notes/`, `artifacts/`, and `AGENTS.md`.
  - Specs:
    - `docs/spec/commands/init.md`
  - Depends: TEMPLATE-WS-001
  - Serial: yes

- [x] TEMPLATE-WS-003: `gionx template validate` command
  - What: add template validation command with shared validator reuse and all-violations report.
  - Specs:
    - `docs/spec/commands/template/validate.md`
    - `docs/spec/concepts/workspace-template.md`
  - Depends: TEMPLATE-WS-001
  - Serial: yes

- [x] TEMPLATE-WS-004: Integration test coverage update
  - What: add/create tests for template creation/validation paths and update testing spec mapping.
  - Specs:
    - `docs/spec/testing/integration.md`
  - Depends: TEMPLATE-WS-002, TEMPLATE-WS-003
  - Serial: yes

