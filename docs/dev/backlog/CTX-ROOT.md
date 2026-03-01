---
title: "CTX-ROOT backlog"
status: planned
---

# CTX-ROOT Backlog

- [x] CTX-ROOT-001: `kra init` pending spec parity
  - What: implement `--root`, non-TTY guidance, interactive root prompt, and current-context update on success.
  - Specs:
    - `docs/dev/spec/commands/init.md`
    - `docs/dev/spec/commands/context.md`
  - Depends: MVP-010, MVP-051
  - Serial: yes

- [x] CTX-ROOT-002: remove `KRA_ROOT` env-based root resolution
  - What: remove environment-variable root override from runtime resolution and use context-first behavior.
  - Specs:
    - `docs/dev/spec/commands/context.md`
    - `docs/dev/spec/concepts/layout.md`
    - `docs/dev/spec/commands/init.md`
  - Depends: CTX-ROOT-001
  - Serial: yes

- [x] CTX-ROOT-003: named context model (`name -> path`)
  - What: add Docker-like context management (`create/list/use/current`) and init wizard inputs for context name/path.
  - Specs:
    - `docs/dev/spec/commands/context.md`
    - `docs/dev/spec/commands/init.md`
  - Depends: CTX-ROOT-002
  - Serial: yes

- [x] CTX-ROOT-004: README quick-start refresh
  - What: add `init --root` and context-aware startup examples.
  - Specs:
    - `README.md`
    - `docs/dev/spec/commands/init.md`
    - `docs/dev/spec/commands/context.md`
  - Depends: CTX-ROOT-001
  - Parallel: yes
