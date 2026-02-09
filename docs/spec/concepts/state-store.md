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

`gionx` maintains `XDG_DATA_HOME/gionx/registry.json` for known-root discovery.

- entry fields:
  - `root_path` (absolute canonical path, unique)
  - `first_seen_at`
  - `last_used_at` (monotonic non-decreasing)
- missing registry file is treated as empty and created lazily.
- malformed registry must fail with a recovery hint.

## Locations (defaults)

- Root registry (data): `~/.local/share/gionx/registry.json`
- Repo pool (cache): `~/.cache/gionx/repo-pool/`

XDG environment variables may override these defaults:

- `$XDG_DATA_HOME` (default: `~/.local/share`)
- `$XDG_CACHE_HOME` (default: `~/.cache`)

## Legacy compatibility

- Existing SQLite files created in earlier versions are treated as legacy index data.
- Runtime behavior should continue to work when legacy index data is absent, stale, or removed.
