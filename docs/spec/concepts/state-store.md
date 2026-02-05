# State Store

## Overview

`gionx` uses a global state store as the source of truth for:
- the configured `GIONX_ROOT` path (single root)
- the repo pool path (bare repo pool)
- workspace metadata (status, description, timestamps)
- workspace <-> repo bindings (repoKey, alias, branch, worktree path, etc.)

The state store also keeps traceability for workspace lifecycle operations:
- `archived_at`, `archived_commit_sha`
- `reopened_at`, `reopened_commit_sha`

## Workspace status

The workspace "current state" is stored in `workspaces.status`:
- `active`: workspace exists under `GIONX_ROOT/workspaces/<id>/`
- `archived`: workspace exists under `GIONX_ROOT/archive/<id>/`

## Event log (immutable)

In addition to the mutable `workspaces` snapshot, `gionx` stores an append-only event log.

Goal:
- allow "purge" to remove a workspace snapshot (so the same workspace ID can be reused)
- still preserve an optional audit trail / diagnostics trail

Suggested table: `workspace_events`
- `workspace_id` (TEXT)
- `event_type` (TEXT) e.g. `created`, `archived`, `reopened`, `purged`
- `at` (timestamp)
- `meta` (optional JSON/text)

This state store is stored outside `GIONX_ROOT`.

## Locations (defaults)

- State store (data): `~/.local/share/gionx/state.db`
- Repo pool (cache): `~/.cache/gionx/repo-pool/`

XDG environment variables may override these defaults:
- `$XDG_DATA_HOME` (default: `~/.local/share`)
- `$XDG_CACHE_HOME` (default: `~/.cache`)

## SQLite + migrations

Migrations are applied in order from `migrations/*.sql`.

Recommended approach:
- keep a `schema_migrations` table storing applied migration identifiers
- at startup, apply any missing migrations inside a transaction
