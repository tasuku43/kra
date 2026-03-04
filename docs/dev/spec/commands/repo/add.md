---
title: "`kra repo add`"
status: implemented
---

# `kra repo add`

## Usage

```sh
kra repo add [--format human|json] <repo-spec>...
```

## Purpose

Register repositories into the shared bare repo pool and the current root index.

## Root resolution

`kra repo add` resolves root in this order:

1. `KRA_ROOT`
2. current context (`~/.kra/state/current-context`)
3. walk-up discovery from cwd

## Inputs

- one or more `repo-spec` values
- accepted formats:
  - `git@<host>:<owner>/<repo>.git`
  - `https://<host>/<owner>/<repo>[.git]`
  - `file://.../<host>/<owner>/<repo>.git`

## Behavior

Per input repo (best-effort):

1. normalize `repo-spec` into `repo_uid` / `repo_key`
2. upsert shared bare pool state:
  - if bare missing: `git clone --bare` only (no immediate fetch)
  - if bare already exists: reuse it without remote sync (no fetch)
3. upsert current root `repos` row:
  - insert on first seen
  - update `updated_at` when already present
4. idempotent skip in same root:
  - if the same `repo_uid` + `remote_url` is already registered in current root
    and the bare repo already exists, treat as skipped (success, no clone/fetch)

Conflict policy:
- if same `repo_uid` exists with different `remote_url`, treat as failure for that repo

## Output flow

- `Repo pool:` section
- `Progress:` section
  - TTY: redraw a single progress block (no duplicated lines per repo)
  - non-TTY: append progress lines as a fallback
  - show running/completed state per repo
- `Result:` section
  - summary: `Added <n> / <m>`
  - when skipped items exist, print one skip summary reason (`already added in current root`)
  - skipped repo detail lines list repo keys only (no repeated reason text)
  - success details are not repeated here
  - failure lines include reason (`! <repo> (reason: ...)`)

## Exit code

- all success: `exitOK`
- one or more failures: `exitError`

## JSON mode (`--format json`)

- output envelope follows `docs/dev/spec/concepts/output-contract.md`
- action: `repo.add`
- success result:
  - `added`
  - `skipped`
  - `total`
  - `items[]` (`repo_key`, `success`, `skipped`, `reason`)
- failure returns `ok=false` with `error.code=conflict` and includes partial result counts.
