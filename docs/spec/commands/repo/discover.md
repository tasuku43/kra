---
title: "`gionx repo discover`"
status: implemented
---

# `gionx repo discover`

## Usage

```sh
gionx repo discover --org <org> [--provider github]
```

## Purpose

Discover repositories from provider API and bulk-add selected repos into the shared repo pool.

## Root resolution

`gionx repo discover` resolves root in this order:

1. `GIONX_ROOT`
2. current context (`XDG_DATA_HOME/gionx/current-context`)
3. walk-up discovery from cwd

## Provider model

- `--provider` is optional (default: `github`)
- implementation uses provider interface + adapter separation
- current adapter: GitHub via `gh` CLI

## Discovery behavior (github)

- target scope: org repositories (`--org` required)
- include all accessible repos (private + public)
- pagination: fetch all pages

## Selection policy

- show only repos not yet registered in current root state DB
  - uniqueness key: `repo_uid`
- row display: `owner/repo`
- multi-select via inline selector

## Apply behavior

- selected repos are passed to the same pool-add path as `repo add`
- `Repo pool:` -> `Result:` flow (no `Plan:`)
- one or more failures result in `exitError`
