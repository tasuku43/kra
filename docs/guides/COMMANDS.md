---
title: "Command reference"
status: implemented
---

# Command reference

This page is a user-oriented overview.
For exact behavior contracts, see `docs/spec/commands/`.

## Quick flow

```sh
kra init
kra ws create TASK-1234
kra repo add git@github.com:org/backend.git
kra ws add-repo --id TASK-1234
kra ws open --id TASK-1234
kra ws dashboard
```

## Root commands

- `kra init` - initialize root and context.
- `kra context ...` - current/list/create/use/rename/rm context operations.
- `kra root ...` - conceptual KRA_ROOT helpers (resolve/open).
- `kra repo ...` - add/discover/remove/gc for repo pool registration.
- `kra template create` - create a workspace template scaffold.
- `kra template remove` - remove a workspace template.
- `kra template validate` - validate workspace templates.
- `kra shell init` - print shell integration helper.
- `kra shell completion` - print shell completion script.
- `kra ws ...` - workspace lifecycle operations.
- `kra doctor` - root diagnostics and optional staged remediation.
- `kra version` / `kra --version` - print build version.

## Common workspace commands

- `kra ws create [--no-prompt] [--template <name>] <id>`
- `kra ws create --jira <ticket-url>`
- `kra ws import jira [--sprint ... | --jql ...]`
- `kra ws list --format human|tsv|json`
- `kra ws dashboard --format human|json`
- `kra ws open [--id <id> | --current | --select] [--multi] [--concurrency <n>]`
- `kra ws add-repo [--id <id> | --current | --select]`
- `kra ws remove-repo [--id <id> | --current | --select]`
- `kra ws close [--id <id> | --current | --select]`
- `kra ws reopen [--id <id> | --current | --select]`
- `kra ws purge [--id <id> | --current | --select]`
- `kra ws lock [--id <id> | --current | --select]`
- `kra ws unlock [--id <id> | --current | --select]`

## Workspace target model (`--id` / `--current` / `--select`)

Many workspace actions share a common target model:

- `--id <id>`: explicit workspace target (recommended for scripts)
- `--current`: resolve workspace from your current directory context
- `--select`: choose target interactively from the workspace selector

Typical examples:

- `kra ws open [--id <id> | --current | --select]`
- `kra ws close [--id <id> | --current | --select]`
- `kra ws add-repo [--id <id> | --current | --select]`
- `kra ws remove-repo [--id <id> | --current | --select]`
- `kra ws lock [--id <id> | --current | --select]`
- `kra ws unlock [--id <id> | --current | --select]`

Notes:

- Not every workspace command supports all three forms.
- `ws close` / `ws add-repo` / `ws remove-repo` support `--current` in current implementation.
- Positional workspace id arguments are intentionally not supported for these actions.
- For non-interactive automation, prefer explicit `--id`.

## Global flags

- `--debug` - enable debug logging under `<KRA_ROOT>/.kra/logs/`.
- `--version` - print version and exit 0.

## Further reading

- Install guide: `docs/guides/INSTALL.md`
- Shell integration guide: `docs/guides/SHELL_INTEGRATION.md`
- Workspace lifecycle guide: `docs/guides/WORKSPACE_LIFECYCLE.md`
- cmux integration guide: `docs/guides/CMUX.md`
- Repo/worktree guide: `docs/guides/REPO_WORKTREE.md`
- Automation JSON guide: `docs/guides/AUTOMATION_JSON.md`
- Specs: `docs/spec/README.md`
- Release operations: `docs/ops/RELEASING.md`
