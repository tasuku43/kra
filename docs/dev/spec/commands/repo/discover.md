---
title: "`kra repo discover`"
status: implemented
---

# `kra repo discover`

## Usage

```sh
kra repo discover --org <org> [--provider github]
```

## Purpose

Discover repositories from provider API and bulk-add selected repos into the shared repo pool.

## Root resolution

`kra repo discover` resolves root in this order:

1. `KRA_ROOT`
2. current context (`~/.kra/state/current-context`)
3. walk-up discovery from cwd

## Provider model

- `--provider` is optional (default: `github`)
- implementation uses provider interface + adapter separation
- current adapter: GitHub via `gh` CLI
- provider resolution is registry-based (not hardcoded switch-only):
  - built-ins are registered in `internal/repodiscovery`
  - future providers can be added via `RegisterProvider(name, factory)`
- unsupported provider errors must include currently supported provider names

## Discovery behavior (github)

- target scope: org repositories (`--org` required)
- include all accessible repos (private + public)
- pagination: fetch all pages

## Selection policy

- show only repos not yet registered in current root index
  - uniqueness key: `repo_uid`
- row display: `owner/repo`
- multi-select via shared inline selector component (same interaction as `ws` selectors)
  - `space`: toggle
  - `enter`: confirm
  - `esc` / `ctrl+c`: cancel

## Apply behavior

- selected repos are passed to the same pool-add path as `repo add`
- `Progress:` -> `Result:` flow (no `Plan:`)
- execution uses bounded parallel workers
- TTY: progress section is redrawn as one block while each repo state changes
- non-TTY: append progress lines as a fallback
- `Result:` prints summary (`Added <n> / <m>`) and failure details only
- one or more failures result in `exitError`
