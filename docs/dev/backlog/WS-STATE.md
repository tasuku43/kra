---
title: "WS-STATE backlog"
status: planned
---

# WS-STATE Backlog

- [x] WS-STATE-001: Workspace work-state baseline and auto-commit lifecycle
  - What: define and implement `todo/in-progress` derivation with hybrid baseline signals
    (`repos/**` git signals + non-repo FS hash), and align baseline lifecycle with workspace lifecycle.
    `ws create` must auto-commit creation scope and initialize baseline.
  - Specs:
    - `docs/dev/spec/commands/ws/create.md`
    - `docs/dev/spec/commands/ws/list.md`
    - `docs/dev/spec/commands/ws/selector.md`
    - `docs/dev/spec/concepts/fs-source-of-truth.md`
  - Depends: UX-WS-006, UX-WS-019
  - Serial: yes
