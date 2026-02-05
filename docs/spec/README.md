---
title: "gionx CLI specs"
status: planned
---

# gionx specs

This directory contains incremental specifications for `gionx`.
Implementation should reference these specs. When behavior changes, update the spec first.

## Metadata rules (mirrors gion)

- Required: `title`, `status`.
- Optional: `pending` (YAML array of short tokens/ids for unimplemented pieces). If `pending` is non-empty,
  treat the spec as needing work even when `status: implemented`.
- Additional optional fields (e.g., `description`, `since`) are allowed.
- Use YAML frontmatter at the top of each spec.

### `status` values

- `planned`: spec-first discussion; not implemented yet.
- `implemented`: implemented and considered current.

## Index

- Core
  - `core/BACKLOG.md`: Implementation backlog (dependencies, parallelism, spec mapping)
  - `core/DATA_MODEL.md`: State store data model (tables, keys, constraints)
  - `core/AGENTS.md`: AGENTS.md generation and conventions
- Concepts
  - `concepts/layout.md`: GIONX_ROOT layout and Git tracking policy
  - `concepts/state-store.md`: Global state store (SQLite) and migrations
- Commands
  - `commands/init.md`: `gionx init`
  - `commands/ws/create.md`: `gionx ws create`
  - `commands/ws/add-repo.md`: `gionx ws add-repo`
  - `commands/ws/list.md`: `gionx ws list`
  - `commands/ws/close.md`: `gionx ws close`
  - `commands/ws/reopen.md`: `gionx ws reopen`
  - `commands/ws/purge.md`: `gionx ws purge`

- Development
  - `../dev/TESTING.md`: Testing principles (developer guidance)
