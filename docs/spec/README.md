---
title: "gionx CLI specs"
status: implemented
pending:
  - ws_unified_launcher
  - architecture_migration
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
  - `../BACKLOG.md`: Implementation backlog (dependencies, parallelism, spec mapping)
  - `core/DATA_MODEL.md`: State store data model (tables, keys, constraints)
  - `core/AGENTS.md`: AGENTS.md generation and conventions
- Concepts
  - `concepts/layout.md`: GIONX_ROOT layout and Git tracking policy
  - `concepts/state-store.md`: Optional/rebuildable root index and registry
  - `concepts/fs-source-of-truth.md`: FS=SoT and index-store downgrade policy (planned)
  - `concepts/workspace-meta-json.md`: `.gionx.meta.json` schema and atomic update rules (planned)
  - `concepts/ui-color.md`: CLI/TUI semantic color token policy
  - `concepts/architecture.md`: Layered architecture (`cli/app/domain/infra/ui`) and migration rules
  - `concepts/workspace-lifecycle.md`: Workspace lifecycle state machine and transition policy
- Commands
  - `commands/context.md`: `gionx context`
  - `commands/init.md`: `gionx init`
  - `commands/repo/add.md`: `gionx repo add`
  - `commands/repo/discover.md`: `gionx repo discover`
  - `commands/repo/remove.md`: `gionx repo remove`
  - `commands/repo/gc.md`: `gionx repo gc`
  - `commands/shell/init.md`: `gionx shell init`
  - `commands/state/registry.md`: `gionx state` foundation (registry)
  - `commands/ws/selector.md`: Shared inline selector UI for workspace actions
  - `commands/ws/select.md`: Unified human launcher (`ws select` / context-aware `ws`)
  - `commands/ws/create.md`: `gionx ws create`
  - `commands/ws/add-repo.md`: `gionx ws add-repo`
  - `commands/ws/list.md`: `gionx ws list`
  - `commands/ws/go.md`: `gionx ws go`
  - `commands/ws/close.md`: `gionx ws close`
  - `commands/ws/reopen.md`: `gionx ws reopen`
  - `commands/ws/purge.md`: `gionx ws purge`

- Development
  - `../dev/TESTING.md`: Testing principles (developer guidance)
