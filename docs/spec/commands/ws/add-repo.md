---
title: "`gionx ws add-repo`"
status: implemented
---

# `gionx ws add-repo [<workspace-id>]`

## Purpose

Add repositories from the existing repo pool to a workspace as Git worktrees.

## Inputs

- `workspace-id` (optional): existing active workspace ID
  - if omitted, current working directory must be under `GIONX_ROOT/workspaces/<id>/`
  - otherwise the command fails fast

## Selection source

- Candidate repos are taken from the state store `repos` table + existing bare repos under repo pool.
- No direct repo URL input in this command.
- Repos already bound in the target workspace are excluded from candidates.
- Candidate ordering:
  1. 30-day add usage score (`repo_usage_daily` sum, descending)
  2. `repos.updated_at` descending
  3. `repo_key` ascending
- Candidate filter target is `repo_key` only.

## Behavior (MVP)

1. Select repos from pool (multi-select)
  - section heading: `Repos(pool):`
  - row display: `repo_key` only

2. Input per-repo branch settings
  - prompt `base_ref` for each selected repo
    - empty means default base ref detected from bare repo (typically `origin/<default>`)
    - non-empty must be `origin/<branch>`
  - prompt branch for each selected repo
    - prefill: `<workspace-id>/`
    - validate via `git check-ref-format`

3. Preflight (all selected repos)
  - verify alias conflicts (existing bindings + selected set) do not exist
  - verify worktree target paths do not already exist
  - verify base refs exist in bare repos
  - verify branch/worktree constraints before execution
  - if any check fails: abort the whole operation (no partial apply)

4. Plan and confirmation
  - print `Plan:` with per-repo:
    - alias
    - base_ref
    - branch
    - target path
  - final prompt:
    - `add selected repos to workspace? [Enter=yes / n=no]: `

5. Apply (all-or-nothing)
  - create local branches as needed
  - create worktrees under `GIONX_ROOT/workspaces/<id>/repos/<alias>`
  - record `workspace_repos` bindings
  - on any failure, rollback all created branches/worktrees/bindings

6. Update usage metrics (success only)
  - increment `repo_usage_daily` for each added repo using local date key (`YYYYMMDD`)
  - touch `repos.updated_at`

7. Result
  - print `Result:`
  - summary: `Added <n> / <m>`
  - per repo success lines

## Concurrency

- Command uses workspace-scoped lock.
- Concurrent `ws add-repo` on the same workspace must fail fast.
