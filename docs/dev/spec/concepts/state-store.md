---
title: "State Store"
status: proposed
---

# State Store

## Overview

`kra` is filesystem-first.

- Canonical workspace state is `workspaces/<id>/.kra.meta.json` (or `archive/<id>/.kra.meta.json`).
- Physical truth is directory layout under `KRA_ROOT/workspaces` and `KRA_ROOT/archive`.
- Runtime operational files may exist under `KRA_ROOT/.kra/state/`.
- Optional index data may exist for performance or selector UX, but it must be rebuildable from canonical filesystem data.

## Canonical data

Workspace metadata file (`.kra.meta.json`) must contain:

- workspace identity and lifecycle status (`active` / `archived`)
- logical work-state (`todo` / `in-progress`) with monotonic transition
- user-facing fields (`title`, `source_url`)
- reopen restore payload (`repos_restore`)
- timestamps (`created_at`, `updated_at`)

Commands must not require SQL-only rows for lifecycle correctness.

Read-only commands must not mutate canonical data as part of inspection or degraded recovery.

## Runtime operational data

Root-local runtime state may exist under `KRA_ROOT/.kra/state/`.

Examples:

- `cmux-workspaces.json`
- `cmux-sessions.json`
- `root-repos.json`
- `operations/ws-close/<id>.json`

Rules:

- runtime operational files are not canonical workspace state
- commands must degrade gracefully when runtime files are missing or stale
- lifecycle journals under `operations/` should be preserved until resolved
- read-only commands must not "repair" canonical metadata by writing through runtime observations

## Optional index data

Optional index/cache layers are allowed for UX/performance, with these constraints:

- rebuildable from canonical filesystem metadata
- safe to delete without data loss
- commands must fail closed or rebuild when index is stale/corrupt
- runtime-only files under `.kra/state/` must not become hidden canonical dependencies

## Root registry

`kra` maintains `~/.kra/state/root-registry.json` for known-root discovery.

- entry fields:
  - `root_path` (absolute canonical path, unique)
  - `first_seen_at`
  - `last_used_at` (monotonic non-decreasing)
- missing registry file is treated as empty and created lazily.
- malformed registry must fail with a recovery hint.

## Locations (defaults)

- Global config: `~/.kra/config.yaml`
- Current context pointer: `~/.kra/state/current-context`
- Root registry: `~/.kra/state/root-registry.json`
- Repo pool: `~/.kra/repo-pool/`

Environment override:

- `$KRA_HOME` (default: `~/.kra`)

## Legacy compatibility

- SQLite state store and SQL migrations are retired.
- Runtime behavior must not depend on any SQLite file.
- If legacy SQLite files exist from older versions, they are ignored.
- Legacy root-local files such as `workspace-workstate.json` and `workspace-baselines/<id>.json`
  are recovery inputs only and should be reported by `doctor` until migrated or removed.
