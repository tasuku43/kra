---
title: "`gionx init`"
status: implemented
---

# `gionx init`

## Purpose

Initialize `GIONX_ROOT` and the global state store.

## Behavior

- Ensure `GIONX_ROOT/` exists (create if missing)
- Ensure `GIONX_ROOT/workspaces/` exists
- Ensure `GIONX_ROOT/archive/` exists
- Create `GIONX_ROOT/AGENTS.md` with a default "how to use gionx" guidance (for the no-template MVP)
  - include a short explanation of `notes/` vs `artifacts/`
- If `GIONX_ROOT` is not a Git repo, run `git init`
- Write `.gitignore` such that `workspaces/**/repos/**` is ignored
- Initialize the global state store (SQLite) if missing
- Record:
  - `root_path` (single root)
  - `repo_pool_path` (bare pool, in XDG cache by default)
  - an empty workspace list (schema only)

## Notes

- If `GIONX_ROOT` is already Git-managed, `gionx init` must not overwrite existing Git settings.
