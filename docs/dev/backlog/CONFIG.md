---
title: "CONFIG backlog"
status: planned
---

# CONFIG Backlog

- [x] CONFIG-001: Global runtime/state paths under `~/.kra`
  - What: relocate global runtime paths to `~/.kra` (`state/current-context`,
    `state/root-registry.json`, `repo-pool/`) and keep tests isolated via `KRA_HOME`.
  - Specs:
    - `docs/dev/spec/concepts/state-store.md`
    - `docs/dev/spec/commands/context.md`
    - `docs/dev/spec/commands/state/registry.md`
    - `docs/dev/spec/commands/repo/add.md`
    - `docs/dev/spec/commands/repo/discover.md`
    - `docs/dev/spec/commands/repo/remove.md`
  - Depends: DOC-QUALITY-005
  - Serial: yes

- [x] CONFIG-002: User config model + merge/validation foundation
  - What: define/load YAML config from global/root layers, apply precedence
    (`CLI > root > global > default`), and validate shared rules.
  - Specs:
    - `docs/dev/spec/concepts/config.md`
    - `docs/dev/spec/concepts/state-store.md`
  - Depends: CONFIG-001
  - Serial: yes

- [x] CONFIG-003: `init` root-config bootstrap
  - What: generate `<root>/.kra/config.yaml` on first init without overwrite.
  - Specs:
    - `docs/dev/spec/commands/init.md`
    - `docs/dev/spec/concepts/config.md`
  - Depends: CONFIG-002
  - Serial: yes

- [x] CONFIG-004: `ws create` default template from config
  - What: when `--template` is omitted, resolve default template from config
    precedence before fallback `default`.
  - Specs:
    - `docs/dev/spec/commands/ws/create.md`
    - `docs/dev/spec/concepts/config.md`
  - Depends: CONFIG-002, CONFIG-003
  - Serial: yes

- [x] CONFIG-005: `ws import jira` defaults from config
  - What: resolve Jira import defaults (`space/project/type`) from config and
    keep CLI flags as highest precedence.
  - Specs:
    - `docs/dev/spec/commands/ws/import/jira.md`
    - `docs/dev/spec/concepts/config.md`
  - Depends: CONFIG-002, CONFIG-003
  - Serial: yes

- [x] CONFIG-006: global config scaffold bootstrap + config header comments
  - What: create `~/.kra/config.yaml` scaffold on first state-changing command
    and include precedence/order comments in global/root generated configs.
  - Specs:
    - `docs/dev/spec/concepts/config.md`
    - `docs/dev/spec/commands/init.md`
  - Depends: CONFIG-002, CONFIG-003
  - Serial: yes
