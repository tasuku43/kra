---
title: "State Store"
status: implemented
---

# State Store

## Overview

`kra` is filesystem-first.

- Canonical workspace state is `workspaces/<id>/.kra.meta.json` (or `archive/<id>/.kra.meta.json`).
- Physical truth is directory layout under `KRA_ROOT/workspaces` and `KRA_ROOT/archive`.
- Optional index data may exist for performance or selector UX, but it must be rebuildable from filesystem data.

## Canonical data

Workspace metadata file (`.kra.meta.json`) must contain:

- workspace identity and lifecycle status (`active` / `archived`)
- user-facing fields (`title`, `source_url`)
- reopen restore payload (`repos_restore`)
- timestamps (`created_at`, `updated_at`)

Commands must not require SQL-only rows for lifecycle correctness.

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
