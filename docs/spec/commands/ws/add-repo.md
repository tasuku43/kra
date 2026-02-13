---
title: "`kra ws --act add-repo`"
status: implemented
---

# `kra ws --act add-repo [--id <workspace-id>] [<workspace-id>] [--format human|json] [--refresh] [--no-fetch]`

## Purpose

Add repositories from the existing repo pool to a workspace as Git worktrees.

## Inputs

- `workspace-id` (optional): existing active workspace ID
- `--id <workspace-id>` (optional): explicit workspace ID flag
  - cannot be combined with positional `workspace-id`
  - if omitted, current working directory must be under `KRA_ROOT/workspaces/<id>/`
  - otherwise the command fails fast
- interactive selection is handled by `kra ws select --act add-repo`.
- JSON mode (`--format json`) is non-interactive and accepts:
  - `--repo <repo-key>` (repeatable, required)
  - `--branch <name>` (optional, highest precedence when provided)
  - `--base-ref <origin/branch>` (optional, defaults to detected default branch)
  - `--refresh` (optional; force fetch even when cache is fresh)
  - `--no-fetch` (optional; skip fetch decision/execution entirely)
  - `--yes` (required)
- human mode also accepts:
  - `--refresh`
  - `--no-fetch`

## Selection source

- Candidate repos are taken from root index (`repos`) + existing bare repos under repo pool.
- No direct repo URL input in this command.
- Repos already bound in the target workspace are excluded from candidates.
- Candidate ordering:
  1. 30-day add usage score (`repo_usage_daily` sum, descending)
  2. `repos.updated_at` descending
  3. `repo_key` ascending
- Candidate filter target is `repo_key` only.
- Candidate filter matching is fuzzy ordered-subsequence (aligned with selector behavior):
  - query characters must appear in order, but do not need to be contiguous.
  - spaces in query are ignored.
  - matching is case-insensitive.
  - example: `example-org/helmfiles` matches query `cs`.

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
    - end `Inputs:` section with exactly one trailing blank line
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
    - prompt style: `branch: <rendered-default>`
    - default text is editable (not prefix-locked)
    - default generation precedence:
      1. CLI `--branch` (JSON mode only)
      2. config `workspace.branch.template` (rendered with `workspace_id` / `repo_key` / `repo_name`)
      3. fallback `<workspace-id>`
    - empty input keeps the rendered default
    - validate via `git check-ref-format`

3. Remote sync policy (smart-fetch)
  - command evaluates each selected repo independently before preflight/apply
  - decision order:
    1. if `--no-fetch` is set: skip fetch
    2. else if `--refresh` is set: force fetch
    3. else use TTL policy (`default=5m`) with `last_fetched_at`
  - forced fetch even within TTL when either condition is true:
    - requested `base_ref` does not exist in bare repo
    - requested branch remote ref (`refs/remotes/origin/<branch>`) does not exist
  - fetch command:
    - `git fetch origin --prune`
  - on fetch success:
    - update `last_fetched_at` in state store (`repos.last_fetched_at`)
    - if state store is unavailable, update bare-local fallback metadata
  - plan output should include fetch decision per repo:
    - `fetch: skipped (fresh, age=... <= 5m)`
    - `fetch: required (stale, age=...)`
    - `fetch: required (--refresh)`
    - `fetch: skipped (--no-fetch)`

4. Preflight (all selected repos)
  - verify alias conflicts (existing bindings + selected set) do not exist
  - verify worktree target paths do not already exist
  - verify base refs exist in bare repos
  - verify branch/worktree constraints before execution
  - if any check fails: abort the whole operation (no partial apply)

5. Plan and confirmation
  - print `Plan:` as concise summary (selected repo list)
  - `target path` is not shown in default output
  - keep one blank line before `Plan:` and one trailing blank line after `Plan:` before the final confirmation prompt
  - final prompt:
    - `apply this plan? [Enter=yes / n=no]: `

6. Apply (all-or-nothing)
  - create local branches as needed
  - create worktrees under `KRA_ROOT/workspaces/<id>/repos/<alias>`
  - record workspace-repo bindings in index
  - on any failure, rollback all created branches/worktrees/bindings

7. Update usage metrics (success only)
  - increment `repo_usage_daily` for each added repo using local date key (`YYYYMMDD`)
  - touch `repos.updated_at`

8. Result
  - print `Result:`
  - summary: `Added <n> / <m>`
  - per repo success lines: `✔ <repo-key>`
  - visual emphasis policy:
    - `Added <n> / <m>` stays primary
    - success color is used on `✔` only
    - repo key text stays primary
  - end `Result:` section with exactly one trailing blank line

## Fetch error handling

- default policy is conditional fail-fast.
- fatal errors: abort immediately.
  - examples: authentication/permission denied, repository not found, remote unreachable
- retryable errors: retry once, then abort if still failing.
  - examples: transient transport/network failure
- `--no-fetch` skips this path; later preflight/apply failures still follow existing fail-fast behavior.

## Non-interactive JSON contract

- `--format json` enables machine-readable output.
- In JSON mode, command must not prompt.
- Missing required inputs (`--repo`, `--yes`) must fail with `error.code=invalid_argument` and non-zero exit.

## UI style (TTY)

- Follow shared section flow: `Repos(pool):` -> `Inputs:` -> `Plan:` -> `Result:`.
- Each section block follows the shared section atom contract (heading/body/trailing blank).
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

## FS metadata behavior

- On successful apply, command must update `workspaces/<id>/.kra.meta.json`:
  - upsert corresponding entries in `repos_restore`
  - persist `repo_uid`, `repo_key`, `remote_url`, `alias`, `branch`, `base_ref`
- `repos_restore` alias uniqueness must be validated before file replace.
- Metadata update must be atomic (`temp + rename`).

## State store additions

- `repos.last_fetched_at INTEGER NULL` (unix seconds, UTC)
- `NULL` means unknown/stale and should be treated as fetch-required unless `--no-fetch` is set.
