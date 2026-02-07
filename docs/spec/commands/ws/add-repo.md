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
  - TTY: use shared inline selector (`space` toggle, `enter` confirm, text filter input)
  - non-TTY: fallback to numbered prompt input (`comma numbers, /filter, empty=cancel`)

2. Input per-repo branch settings
  - print `Inputs:` section first (`workspace`, `repos`)
  - section spacing:
    - keep one blank line before `Inputs:`
    - no blank line between `Inputs:` heading and first body line
  - for each selected repo, show a tree row and prompt inline under that repo
  - Inputs section is redrawn incrementally while values are fixed:
    - after base_ref only: repo node has only `base_ref` detail
    - while branch input is active: same repo node shows both `base_ref` and `branch` (branch line prefilled with current editable value)
    - after branch fixed: same repo node keeps both `base_ref` and `branch`
    - when next repo starts, previous repo keeps finalized two-line detail tree
  - prompt `base_ref` for each selected repo
    - prompt style: `base_ref: <default>`
    - empty means default base ref detected from bare repo (typically `origin/<default>`)
    - non-empty accepts:
      - `origin/<branch>` (as-is)
      - `<branch>` (normalized to `origin/<branch>`)
      - `/branch` (normalized to `origin/branch`)
  - prompt branch for each selected repo
    - prompt style: `branch: <workspace-id>`
    - default text is editable (not prefix-locked)
    - empty means `<workspace-id>`
    - validate via `git check-ref-format`

3. Preflight (all selected repos)
  - verify alias conflicts (existing bindings + selected set) do not exist
  - verify worktree target paths do not already exist
  - verify base refs exist in bare repos
  - verify branch/worktree constraints before execution
  - if any check fails: abort the whole operation (no partial apply)

4. Plan and confirmation
  - print `Plan:` as concise summary (selected repo list)
  - `target path` is not shown in default output
  - keep one blank line before `Plan:` and one blank line before the final confirmation prompt
  - final prompt:
    - `apply this plan? [Enter=yes / n=no]: `

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

## UI style (TTY)

- Follow shared section flow: `Repos(pool):` -> `Inputs:` -> `Plan:` -> `Result:`.
- Use shared style tokens:
  - `muted`: bullets/tree connectors
  - `accent`: field labels (`workspace`, `repos`, `alias`, `base_ref`, `branch`)
- Keep repo key/value text primary (normal contrast); avoid dense one-line key/value packing.

## Concurrency

- Command uses workspace-scoped lock.
- Concurrent `ws add-repo` on the same workspace must fail fast.
- Lock file stores owner PID metadata.
- If an existing lock is stale (owner process no longer alive), command must auto-recover by removing stale lock
  and retrying once.
