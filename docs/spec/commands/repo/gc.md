---
title: "`gionx repo gc`"
status: planned
---

# `gionx repo gc`

## Purpose

Garbage-collect physical bare repositories from the shared repo pool when safe.

`repo gc` is intentionally separate from `repo remove` to keep logical detach and physical deletion distinct.

## Safety gates (planned)

Before deleting a bare repo from pool, command must verify:

1. no current-root registration references (`repos`)
2. no current-root workspace bindings (`workspace_repos`)
3. no live worktrees from that bare repo (`git worktree list`)
4. optional registry-based cross-root checks (best-effort; future hardening)

If any gate fails, that repo is skipped and reported.

## UX outline (planned)

- `Repo pool:` selection (multi-select + filter)
- `Risk:` section with permanent deletion warning
- explicit confirmation
- `Progress:` and `Result:` summary

## Scope

- default: shared repo pool path from settings/XDG
- no automatic execution from `repo remove`

