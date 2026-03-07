---
title: "Workspace Meta JSON"
status: implemented
---

# Workspace Meta JSON (`.kra.meta.json`)

## Purpose

Define a single-file metadata format per workspace/archive that supports:
- workspace descriptive metadata
- repo restore metadata required by `ws reopen`
- workspace-local baseline metadata required by logical work-state derivation

This file is canonical and stored in:
- `KRA_ROOT/workspaces/<id>/.kra.meta.json` (active)
- `KRA_ROOT/archive/<id>/.kra.meta.json` (archived)

## File format (v1)

```json
{
  "schema_version": 1,
  "workspace": {
    "id": "MVP-001",
    "title": "",
    "source_url": "",
    "status": "active",
    "work_state": "todo",
    "created_at": 1730000000,
    "updated_at": 1730000000
  },
  "repos_restore": [
    {
      "repo_uid": "github.com/owner/repo",
      "repo_key": "owner/repo",
      "remote_url": "git@github.com:owner/repo.git",
      "alias": "repo",
      "branch": "feature/MVP-001",
      "base_ref": "origin/main"
    }
  ],
  "baseline": {
    "version": 1,
    "created_at": 1730000000,
    "repos": {
      "repo": {
        "baseline_head": "0123456789abcdef"
      }
    },
    "fs": {
      "notes/README.md": "sha256:..."
    }
  },
  "protection": {
    "purge_guard": {
      "enabled": true,
      "updated_at": 1730000000
    }
  }
}
```

## Semantics

- `workspace.id` must match directory name `<id>`.
- `workspace.status`:
  - `active`: file is under `workspaces/<id>/`
  - `archived`: file is under `archive/<id>/`
- `workspace.work_state`:
  - `todo` or `in-progress`
  - monotonic transition only (`todo -> in-progress`)
  - `in-progress` is treated as stable for canonical state
  - read-only commands must not rewrite this field during inspection
- `repos_restore` is the authoritative input for worktree reconstruction on `ws reopen`.
- `baseline` stores workspace-local snapshot data used to derive `workspace.work_state` for
  `todo` workspaces and repair missing canonical state.
  - `repos.<alias>.baseline_head`
  - `fs.<path> = sha256:<digest>`
  - baseline is canonical workspace metadata, not a root-level cache
  - legacy `.kra/state/workspace-baselines/<id>.json` may be migrated into this field by `doctor --fix`
- `protection.purge_guard.enabled` controls whether purge is blocked.
- Runtime-only `risk` is not stored.

## Write rules

- Any update must use atomic replace (`temp file -> fsync optional -> rename`).
- Command-level writes should validate full JSON before replacing.
- Metadata read/write helpers are centralized in `internal/app/workspacemeta`.
- On parse failure:
  - fail fast with path + recovery hint.
  - no silent fallback to partial/default values.

## Command expectations

- `ws create`:
  - create `.kra.meta.json` with empty `repos_restore`.
  - initialize `workspace.work_state=todo`.
  - initialize `baseline` from created workspace contents.
- `ws add-repo`:
  - update `repos_restore` entries for added/bound repos.
- `ws list` / `ws dashboard`:
  - may derive transient state in memory
  - must not create missing baseline data
  - must not normalize `workspace.work_state` by writing metadata
- `ws close`:
  - refresh `repos_restore` from live worktrees before worktree removal.
  - set `workspace.status=archived`.
- `ws reopen`:
  - recreate worktrees from `repos_restore`.
  - set `workspace.status=active`.
  - reset `workspace.work_state=todo`.
  - refresh `baseline` from reopened workspace contents.
- `doctor --fix`:
  - may create a missing baseline for an active workspace
  - may normalize missing `workspace.work_state` when deterministic
- `ws purge`:
  - remove workspace/archive directories (metadata removed with them).

## Validation

- `schema_version` unknown:
  - fail with clear upgrade hint.
- Duplicate `alias` in `repos_restore`:
  - fail validation.
- Missing required keys:
  - fail validation.
- Invalid `workspace.work_state`:
  - fail validation.
