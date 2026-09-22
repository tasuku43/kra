---
title: "`kra repo gc`"
status: implemented
---

# `kra repo gc`

## Usage

```sh
kra repo gc [--format human|json] [--yes] [<repo-key|repo-uid>...]
```

## Purpose

Garbage-collect physical bare repositories from the current root's repo pool when safe.

`repo gc` is intentionally separate from `repo remove` to keep logical detach and physical deletion distinct.

## Safety gates

Before deleting a bare repo from pool, command must verify:

1. no current-root registration references (`repos`)
2. no current-root workspace metadata references
3. no live worktrees from that bare repo (`git worktree list --porcelain`)
If any gate fails, that repo is not included in gc candidates. Other roots are independent because
their pools are physically separate.

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

JSON mode:
- `--format json` requires explicit targets and `--yes` (no interactive prompt)
- action: `repo.gc`
- success result includes `removed`, `total`, `items[]`
- ineligible selection returns `ok=false`, `error.code=conflict`

## Exit code

- all selected repos removed: `exitOK`
- no candidates / canceled / one or more failures: `exitError`

## Scope

- default: `<KRA_ROOT>/.kra/repo-pool/`
- no automatic execution from `repo remove`

## FS metadata safety behavior

- Safety gate evaluation must include FS-based references from workspace/archive `.kra.meta.json`.
- Missing/corrupt index data must not permit unsafe deletion; command should rebuild index or fail closed.
