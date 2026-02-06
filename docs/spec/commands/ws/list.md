---
title: "`gionx ws list`"
status: implemented
---

# `gionx ws list`

## Purpose

List workspaces with status and risk, similar in spirit to `gion manifest ls`.

## Display fields (MVP)

- `id`
- `status` (`active`/`archived`)
- `updated_at`
- `repo_count`
- `risk` (live)
- `description`

## Behavior (MVP)

- The global state store is the primary source of desired state.
- The filesystem under `GIONX_ROOT/workspaces/` is treated as actual state.
- `risk` is reported as:
  - `clean` when `repo_count == 0`
  - `unknown` otherwise (repo-level risk computation is implemented later)

### Drift repair (MVP)

- If a repo is present in the state store but its worktree is missing on disk:
  - mark it as `missing` in the state store (do not delete)
- If a workspace directory exists on disk but is missing in the state store:
  - import it automatically using the directory name as the workspace ID
