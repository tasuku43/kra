---
title: "UX-REPO backlog"
status: planned
---

# UX-REPO Backlog

- [x] UX-REPO-001: Result color semantics parity (`repo remove`)
  - What: apply shared `Result:` summary color semantics to `repo remove` for consistency with `repo add/gc`.
  - Specs:
    - `docs/dev/spec/commands/repo/remove.md`
    - `docs/dev/spec/concepts/ui-color.md`
  - Depends: UX-WS-014
  - Parallel: yes

- [x] UX-REPO-002: `repo gc` summary condition cleanup
  - What: use consistent denominator (`eligibleSelected`) for summary color condition to avoid future drift.
  - Specs:
    - `docs/dev/spec/commands/repo/gc.md`
  - Depends: MVP-063
  - Parallel: yes

- [x] UX-REPO-003: Repo preset for `ws add-repo`
  - What: add root-local repo presets (`kra repo preset ...`) and `kra ws add-repo --preset <name>` to skip repetitive repo selection.
  - Specs:
    - `docs/dev/spec/commands/repo/preset.md`
    - `docs/dev/spec/commands/ws/add-repo.md`
    - `docs/dev/spec/concepts/config.md`
  - Depends: UX-WS-018, MVP-030
  - Parallel: yes

- [x] UX-REPO-004: User-configured clone protocol for `repo discover`
  - What: make GitHub discovery follow the host-specific `gh` clone protocol preference, preserving
    Git's normal `url.<base>.insteadOf` rewriting and falling back to HTTPS when unset.
  - Specs:
    - `docs/dev/spec/commands/repo/discover.md`
    - `docs/dev/spec/commands/repo/add.md`
  - Depends: MVP-061
  - Parallel: yes
