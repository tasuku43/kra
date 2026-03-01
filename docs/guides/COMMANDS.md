---
title: "Command reference"
status: implemented
---

# Command reference

This page is a user-oriented overview.
For exact behavior contracts, see `docs/spec/commands/`.

## Quick flow

```sh
kra init --root ~/kra --context main
kra ws create --no-prompt TASK-1234
kra ws open --id TASK-1234
kra ws dashboard
```

## Root commands

- `kra init` - initialize root and context.
- `kra context ...` - current/list/create/use/rename/rm context operations.
- `kra repo ...` - add/discover/remove/gc for repo pool registration.
- `kra template validate` - validate workspace templates.
- `kra shell init` - print shell integration helper.
- `kra shell completion` - print shell completion script.
- `kra ws ...` - workspace lifecycle operations.
- `kra doctor` - root diagnostics and optional staged remediation.
- `kra version` / `kra --version` - print build version.

## Common workspace commands

- `kra ws create [--no-prompt] <id>`
- `kra ws list --format human|tsv|json`
- `kra ws dashboard --format human|json`
- `kra ws open [--id <id> | --current | --select]`
- `kra ws add-repo ...`
- `kra ws remove-repo ...`
- `kra ws close <id>`
- `kra ws reopen <id>`
- `kra ws purge <id>`

## Global flags

- `--debug` - enable debug logging under `<KRA_ROOT>/.kra/logs/`.
- `--version` - print version and exit 0.

## Further reading

- Install guide: `docs/guides/INSTALL.md`
- Specs: `docs/spec/README.md`
- Release operations: `docs/ops/RELEASING.md`
