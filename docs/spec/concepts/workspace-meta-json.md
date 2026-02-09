---
title: "Workspace Meta JSON"
status: implemented
---

# Workspace Meta JSON (`.gionx.meta.json`)

## Purpose

Define a single-file metadata format per workspace/archive that supports:
- workspace descriptive metadata
- repo restore metadata required by `ws reopen`

This file is canonical and stored in:
- `GIONX_ROOT/workspaces/<id>/.gionx.meta.json` (active)
- `GIONX_ROOT/archive/<id>/.gionx.meta.json` (archived)

## File format (v1)

```json
{
  "schema_version": 1,
  "workspace": {
    "id": "MVP-001",
    "title": "",
    "source_url": "",
    "status": "active",
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
  ]
}
```

## Semantics

- `workspace.id` must match directory name `<id>`.
- `workspace.status`:
  - `active`: file is under `workspaces/<id>/`
  - `archived`: file is under `archive/<id>/`
- `repos_restore` is the authoritative input for worktree reconstruction on `ws reopen`.
- Runtime-only states (`risk`, `todo`, `in-progress`) are not stored.

## Write rules

- Any update must use atomic replace (`temp file -> fsync optional -> rename`).
- Command-level writes should validate full JSON before replacing.
- On parse failure:
  - fail fast with path + recovery hint.
  - no silent fallback to partial/default values.

## Command expectations

- `ws create`:
  - create `.gionx.meta.json` with empty `repos_restore`.
- `ws add-repo`:
  - update `repos_restore` entries for added/bound repos.
- `ws close`:
  - refresh `repos_restore` from live worktrees before worktree removal.
  - set `workspace.status=archived`.
- `ws reopen`:
  - recreate worktrees from `repos_restore`.
  - set `workspace.status=active`.
- `ws purge`:
  - remove workspace/archive directories (metadata removed with them).

## Validation

- `schema_version` unknown:
  - fail with clear upgrade hint.
- Duplicate `alias` in `repos_restore`:
  - fail validation.
- Missing required keys:
  - fail validation.
