---
title: "`gionx repo add`"
status: implemented
---

# `gionx repo add`

## Usage

```sh
gionx repo add <repo-spec>...
```

## Purpose

Register repositories into the shared bare repo pool and the current root state DB.

## Root resolution

`gionx repo add` resolves root in this order:

1. `GIONX_ROOT`
2. current context (`XDG_DATA_HOME/gionx/current-context`)
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
  - if bare missing: `git clone --bare`
  - always run fetch update (`fetch --prune`) via existing bare sync path
3. upsert current root `repos` row:
  - insert on first seen
  - update `updated_at` when already present

Conflict policy:
- if same `repo_uid` exists with different `remote_url`, treat as failure for that repo

## Output flow

- `Repo pool:` section
- `Result:` section
  - summary: `Added <n> / <m>`
  - per repo lines with reason on failure

## Exit code

- all success: `exitOK`
- one or more failures: `exitError`
