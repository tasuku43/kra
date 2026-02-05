---
title: "Data Model"
status: planned
---

# Data Model

This document defines the state store schema at a conceptual level.
Concrete DDL should be implemented via SQLite migrations.

## Goals

- Support a single `GIONX_ROOT` (fixed root) and a global bare repo pool.
- Track workspaces and the repos attached to them.
- Support lifecycle: `ws create`, `ws add-repo`, `ws close` (archive), `ws reopen`, `ws purge`.
- Allow reusing the same workspace ID after purge, while keeping an immutable audit trail (event log).
- Support multiple Git hosts (e.g. GitHub and GitLab) without key collisions.

## Tables (MVP)

### `settings`

Single-row table.

- `root_path` (TEXT, NOT NULL)
- `repo_pool_path` (TEXT, NOT NULL)
- `created_at` (INTEGER, NOT NULL, unix epoch)
- `updated_at` (INTEGER, NOT NULL, unix epoch)

Notes:
- The application enforces "exactly 1 row" semantics.

### `workspaces`

Mutable snapshot of the current workspace state.

Primary key:
- `id` (TEXT) - user-provided workspace ID

Columns (MVP):
- `id` (TEXT, PRIMARY KEY)
- `generation` (INTEGER, NOT NULL)
- `status` (TEXT, NOT NULL) - allowed values: `active`, `archived`
- `description` (TEXT, NOT NULL) - may be empty
- `source_url` (TEXT, NOT NULL) - may be empty (e.g. Jira URL)
- `created_at` (INTEGER, NOT NULL)
- `updated_at` (INTEGER, NOT NULL)
- `archived_commit_sha` (TEXT) - optional
- `reopened_commit_sha` (TEXT) - optional

Notes:
- `generation` is required because a workspace ID can be reused after purge.
- When a workspace is purged, its row is removed. When the same ID is created again, the new row
  must use a new generation (see `workspace_events`).
- `archived_at` / `reopened_at` are not stored in the snapshot; use `workspace_events.at`.

Recommended constraints:
- `status IN ('active','archived')`

### `repos`

Global repository registry.

Primary key:
- `repo_uid` (TEXT) - host-qualified, e.g. `github.com/owner/name`

Columns (MVP):
- `repo_uid` (TEXT, PRIMARY KEY)
- `repo_key` (TEXT, NOT NULL) - canonical `owner/name`
- `remote_url` (TEXT, NOT NULL) - first-seen clone URL (fixed)
- `created_at` (INTEGER, NOT NULL)
- `updated_at` (INTEGER, NOT NULL)

Notes:
- `repo_uid` avoids collisions across hosts.
- `repo_key` supports display and grouping.

### `workspace_repos`

Workspace-to-repo bindings (and per-workspace repo metadata).

Primary key:
- (`workspace_id`, `repo_uid`)

Columns (MVP):
- `workspace_id` (TEXT, NOT NULL)
- `repo_uid` (TEXT, NOT NULL)
- `repo_key` (TEXT, NOT NULL) - redundant but convenient
- `alias` (TEXT, NOT NULL) - derived from repo tail; must be unique within a workspace
- `branch` (TEXT, NOT NULL)
- `base_ref` (TEXT, NOT NULL) - may be empty; when non-empty must be `origin/<branch>`
- `repo_spec_input` (TEXT, NOT NULL) - original user input for this workspace
- `missing_at` (INTEGER) - NULL if present, otherwise unix epoch when detected missing
- `created_at` (INTEGER, NOT NULL)
- `updated_at` (INTEGER, NOT NULL)

Recommended constraints:
- `UNIQUE(workspace_id, alias)`
- `FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE`
- `FOREIGN KEY (repo_uid) REFERENCES repos(repo_uid)`

Notes:
- `worktree_path` is not stored; it is derived from layout:
  `GIONX_ROOT/workspaces/<id>/repos/<alias>`.
- Live `risk` is not stored in the DB (computed on demand).

### `workspace_events`

Append-only immutable event log.

Columns (MVP):
- `id` (INTEGER, PRIMARY KEY AUTOINCREMENT)
- `workspace_id` (TEXT, NOT NULL)
- `workspace_generation` (INTEGER, NOT NULL)
- `event_type` (TEXT, NOT NULL) - allowed values: `created`, `archived`, `reopened`, `purged`
- `at` (INTEGER, NOT NULL) - unix epoch
- `meta` (TEXT, NOT NULL) - JSON string; in MVP it's `{}` (reserved for future)

Notes:
- Events remain even when a workspace row is purged.
- `workspace_generation` disambiguates multiple lifecycles of the same workspace ID.
- The DB does not encode lifecycle transition rules via triggers in MVP. Commands must validate transitions
  in the application layer and keep state updates atomic (event append + snapshot update in one transaction).

## Generation rules

- When creating a brand-new workspace ID (no prior events): `generation = 1`.
- When reusing a workspace ID after purge:
  - `generation = 1 + max(workspace_events.workspace_generation for that workspace_id)`

## Indexes (recommended)

- `workspace_events(workspace_id, workspace_generation, at)`
- `workspace_repos(workspace_id)`
- `workspace_repos(repo_uid)`
- `repos(repo_key)`
