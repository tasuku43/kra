---
title: "State Store"
status: implemented
---

# State Store

## Overview

`gionx` is filesystem-first.

- Canonical workspace state is `workspaces/<id>/.gionx.meta.json` (or `archive/<id>/.gionx.meta.json`).
- Physical truth is directory layout under `GIONX_ROOT/workspaces` and `GIONX_ROOT/archive`.
- Optional index data may exist for performance or selector UX, but it must be rebuildable from filesystem data.

## Canonical data

Workspace metadata file (`.gionx.meta.json`) must contain:

- workspace identity and lifecycle status (`active` / `archived`)
- user-facing fields (`title`, `source_url`)
- reopen restore payload (`repos_restore`)
- timestamps (`created_at`, `updated_at`)

Commands must not require SQL-only rows for lifecycle correctness.

## Root registry

`gionx` maintains `~/.gionx/state/root-registry.json` for known-root discovery.

- entry fields:
  - `root_path` (absolute canonical path, unique)
  - `first_seen_at`
  - `last_used_at` (monotonic non-decreasing)
- missing registry file is treated as empty and created lazily.
- malformed registry must fail with a recovery hint.

## Locations (defaults)

- Global config: `~/.gionx/config.yaml`
- Current context pointer: `~/.gionx/state/current-context`
- Root registry: `~/.gionx/state/root-registry.json`
- Repo pool: `~/.gionx/repo-pool/`

Environment override:

- `$GIONX_HOME` (default: `~/.gionx`)

## Legacy compatibility

- SQLite state store and SQL migrations are retired.
- Runtime behavior must not depend on any SQLite file.
- If legacy SQLite files exist from older versions, they are ignored.
