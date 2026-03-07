---
title: "Data Model"
status: proposed
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
  - `work_state` (`todo` / `in-progress`, monotonic)
  - `created_at`
  - `updated_at`
- `repos_restore[]`
  - `repo_uid`
  - `repo_key`
  - `remote_url`
  - `alias`
  - `branch`
  - `base_ref`
- `baseline`
  - `version`
  - `created_at`
  - `repos.<alias>.baseline_head`
  - `fs.<path>`

## Derived/logical model

The following should be computed at read time, not persisted as canonical state:

- live risk (`clean` / `dirty` / `unpushed` / `diverged` / `unknown`)
- current worktree existence and drift state
- output coverage (`empty` / `notes-only` / `artifacts-only` / `documented`)
- lifecycle recovery state (`none` / `resume_ready` / `manual_required`)

`output coverage` excludes scaffolding-only files such as:

- `AGENTS.md`
- `CLAUDE.md`
- `.kra.meta.json`
- `.claude/settings.local.json`

`README.md` is a summary artifact but does not by itself satisfy `documented`.

## Optional index model

Optional index/cache layers are allowed for UX/performance, with these constraints:

- rebuildable from canonical filesystem metadata
- safe to delete without data loss
- commands must fail closed or rebuild when index is stale/corrupt

## Runtime operational model

Runtime operational state may exist under `KRA_ROOT/.kra/state/`.

Examples:

- cmux mapping/session stores
- root repo registry
- lifecycle journals under `operations/ws-close/<id>.json`

These files are not canonical workspace data. They support recovery and UX, but must not silently replace
workspace metadata as the source of truth.

## Lifecycle semantics

Lifecycle transitions are:

- `active -> archived` (`ws close`)
- `archived -> active` (`ws reopen`)
- `active|archived -> purged` (`ws purge`)

Transition correctness must be validated by command logic and filesystem operations, not DB triggers.
