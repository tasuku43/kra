---
title: "UX-CORE backlog"
status: planned
---

# UX-CORE Backlog

- [x] UX-CORE-001: Core command `Result:` output parity (`init`/`ws create`/`context use`)
  - What: align human-readable success outputs of core commands to shared section flow and semantic color tokens.
  - Specs:
    - `docs/spec/commands/init.md`
    - `docs/spec/commands/ws/create.md`
    - `docs/spec/commands/context.md`
    - `docs/spec/concepts/ui-color.md`
  - Depends: UX-WS-014
  - Parallel: yes

- [x] UX-CORE-002: Shell integration bootstrap (`gionx shell init`)
  - What: add shell integration command that prints eval-ready wrapper function and align `ws go` default output
    to shell snippet mode.
  - Specs:
    - `docs/spec/commands/shell/init.md`
    - `docs/spec/commands/ws/go.md`
  - Depends: UX-WS-003
  - Parallel: yes

## WS Unified Entry (idea competition agreed set: 2026-02-08)
