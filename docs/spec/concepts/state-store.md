---
title: "State Store"
status: implemented
---

# State Store

## Overview

`gionx` uses a global state store as the source of truth for:
- the configured `GIONX_ROOT` path (single root)
- the repo pool path (bare repo pool)
- workspace metadata (status, description, timestamps)
- workspace <-> repo bindings (repoUid, repoKey, alias, branch, etc.)

The state store also keeps traceability for workspace lifecycle operations:
- commit SHAs created by `ws close` / `ws reopen` (`archived_commit_sha`, `reopened_commit_sha`)
- lifecycle timestamps via the immutable event log (`workspace_events.at`)

## Workspace status

The workspace "current state" is stored in `workspaces.status`:
- `active`: workspace exists under `GIONX_ROOT/workspaces/<id>/`
- `archived`: workspace exists under `GIONX_ROOT/archive/<id>/`

## Event log (immutable)

In addition to the mutable `workspaces` snapshot, `gionx` stores an append-only event log.

Goal:
- allow "purge" to remove a workspace snapshot (so the same workspace ID can be reused)
- still preserve an optional audit trail / diagnostics trail

## Workspace generation

Because a workspace ID can be reused after purge, `gionx` must be able to distinguish
different "generations" of the same workspace ID in the event log.

- `workspaces.generation`: current generation number for the workspace snapshot
- `workspace_events.workspace_generation`: generation number associated with the event

When a workspace is purged, its snapshot row is removed, but its events remain.
When the same ID is created again, the new snapshot uses `generation = 1 + max(existing events)`.

Suggested table: `workspace_events`
- `workspace_id` (TEXT)
- `workspace_generation` (INTEGER)
- `event_type` (TEXT) e.g. `created`, `archived`, `reopened`, `purged`
- `at` (INTEGER unix epoch)
- `meta` (optional JSON string)

This state store is stored outside `GIONX_ROOT`.

## Repository identity

To support multiple Git hosts (e.g. GitHub and GitLab), `gionx` distinguishes:

- `repoKey`: canonical `owner/name` (derived via `gion-core/repospec`)
- `repoUid`: host-qualified repo identity, e.g. `github.com/owner/name`

`repoUid` should be the primary identifier in the state store to avoid collisions.

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
- keep the DDL simple (no triggers in MVP); validate lifecycle transitions in the application layer
  and keep the state updates atomic via a single transaction per command (event append + snapshot update)

SQLite note:
- foreign key constraints are only enforced when `PRAGMA foreign_keys = ON` is set.
  `gionx` should enable it for every connection.
