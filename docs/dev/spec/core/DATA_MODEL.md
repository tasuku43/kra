---
title: "Data Model"
status: implemented
---

# Data Model

## Canonical model (filesystem)

`kra` data model is defined by filesystem + workspace metadata file.

Per workspace, canonical data lives in:

- `workspaces/<id>/.kra.meta.json` (active)
- `archive/<id>/.kra.meta.json` (archived)

Metadata schema (v1):

- `schema_version`
- `workspace`
  - `id`
  - `title`
  - `source_url`
  - `status` (`active` / `archived`)
  - `created_at`
  - `updated_at`
- `repos_restore[]`
  - `repo_uid`
  - `repo_key`
  - `remote_url`
  - `alias`
  - `branch`
  - `base_ref`

## Derived/logical model

The following should be computed at read time, not persisted as canonical state:

- logical work state (`todo` / `in-progress`)
- live risk (`clean` / `dirty` / `unpushed` / `diverged` / `unknown`)
- current worktree existence and drift state

## Optional index model

Optional index/cache layers are allowed for UX/performance, with these constraints:

- rebuildable from canonical filesystem metadata
- safe to delete without data loss
- commands must fail closed or rebuild when index is stale/corrupt

## Lifecycle semantics

Lifecycle transitions are:

- `active -> archived` (`ws close`)
- `archived -> active` (`ws reopen`)
- `active|archived -> purged` (`ws purge`)

Transition correctness must be validated by command logic and filesystem operations, not DB triggers.
