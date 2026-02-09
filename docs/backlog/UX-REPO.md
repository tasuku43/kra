---
title: "UX-REPO backlog"
status: planned
---

# UX-REPO Backlog

- [x] UX-REPO-001: Result color semantics parity (`repo remove`)
  - What: apply shared `Result:` summary color semantics to `repo remove` for consistency with `repo add/gc`.
  - Specs:
    - `docs/spec/commands/repo/remove.md`
    - `docs/spec/concepts/ui-color.md`
  - Depends: UX-WS-014
  - Parallel: yes

- [x] UX-REPO-002: `repo gc` summary condition cleanup
  - What: use consistent denominator (`eligibleSelected`) for summary color condition to avoid future drift.
  - Specs:
    - `docs/spec/commands/repo/gc.md`
  - Depends: MVP-063
  - Parallel: yes

