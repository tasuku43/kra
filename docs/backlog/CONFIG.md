---
title: "CONFIG backlog"
status: planned
---

# CONFIG Backlog

- [x] CONFIG-001: Global runtime/state paths under `~/.gionx`
  - What: relocate global runtime paths to `~/.gionx` (`state/current-context`,
    `state/root-registry.json`, `repo-pool/`) and keep tests isolated via `GIONX_HOME`.
  - Specs:
    - `docs/spec/concepts/state-store.md`
    - `docs/spec/commands/context.md`
    - `docs/spec/commands/state/registry.md`
    - `docs/spec/commands/repo/add.md`
    - `docs/spec/commands/repo/discover.md`
    - `docs/spec/commands/repo/remove.md`
  - Depends: DOC-QUALITY-005
  - Serial: yes

- [x] CONFIG-002: User config model + merge/validation foundation
  - What: define/load YAML config from global/root layers, apply precedence
    (`CLI > root > global > default`), and validate shared rules.
  - Specs:
    - `docs/spec/concepts/config.md`
    - `docs/spec/concepts/state-store.md`
  - Depends: CONFIG-001
  - Serial: yes

- [x] CONFIG-003: `init` root-config bootstrap
  - What: generate `<root>/.gionx/config.yaml` on first init without overwrite.
  - Specs:
    - `docs/spec/commands/init.md`
    - `docs/spec/concepts/config.md`
  - Depends: CONFIG-002
  - Serial: yes

- [ ] CONFIG-004: `ws create` default template from config
  - What: when `--template` is omitted, resolve default template from config
    precedence before fallback `default`.
  - Specs:
    - `docs/spec/commands/ws/create.md`
    - `docs/spec/concepts/config.md`
  - Depends: CONFIG-002, CONFIG-003
  - Serial: yes

- [ ] CONFIG-005: `ws import jira` defaults from config
  - What: resolve Jira import defaults (`space/project/type`) from config and
    keep CLI flags as highest precedence.
  - Specs:
    - `docs/spec/commands/ws/import/jira.md`
    - `docs/spec/concepts/config.md`
  - Depends: CONFIG-002, CONFIG-003
  - Serial: yes
