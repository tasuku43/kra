---
title: "UI Terminology"
status: implemented
---

# UI Terminology

## Purpose

Keep user-facing wording consistent across `ws`, `repo`, and `context` commands.

## Canonical terms

- `context`: named root selection (`name -> path`)
- `repo`: logical repository identity (`<owner>/<name>`)
- `worktree`: checked-out working directory created from repo pool
- `workspace`: task-scoped unit under `workspaces/<id>` or `archive/<id>`

## Display rules

- When planning `ws add-repo`, show both notions explicitly:
  - `repos (worktrees)`
- For selector titles and pool listings, keep concise repo-first wording:
  - `Repos(pool):`
- Do not call a context `workspace` in user-facing output.
- Do not call a repo entry `branch` or `directory` unless the output specifically describes those fields.

