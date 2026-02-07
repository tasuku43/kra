---
title: "`gionx repo gc`"
status: implemented
---

# `gionx repo gc`

## Usage

```sh
gionx repo gc [<repo-key|repo-uid>...]
```

## Purpose

Garbage-collect physical bare repositories from the shared repo pool when safe.

`repo gc` is intentionally separate from `repo remove` to keep logical detach and physical deletion distinct.

## Safety gates

Before deleting a bare repo from pool, command must verify:

1. no current-root registration references (`repos`)
2. no current-root workspace bindings (`workspace_repos`)
3. no live worktrees from that bare repo (`git worktree list --porcelain`)
4. no references from other known roots (loaded from state registry entries)

If any gate fails, that repo is not included in gc candidates.

## Candidate discovery

- Scan bare repositories under `repo_pool_path` (`<host>/<owner>/<repo>.git`).
- Read `remote.origin.url` from each bare repository.
- Normalize URL and map to `repo_uid` / `repo_key`.
- Repositories that cannot be inspected/normalized are skipped safely.

## UX flow

- `Repo pool:` selection (multi-select + filter) when args are omitted
- direct mode when args are provided (`repo-key` or `repo-uid`)
- `Risk:` section with permanent deletion warning
- explicit confirmation prompt (`y/yes` only)
- `Result:` summary (`Removed <n> / <m>`)

## Exit code

- all selected repos removed: `exitOK`
- no candidates / canceled / one or more failures: `exitError`

## Scope

- default: shared repo pool path from settings/XDG
- no automatic execution from `repo remove`
