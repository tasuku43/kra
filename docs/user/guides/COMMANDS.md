---
title: "Command reference"
status: implemented
---

# Command reference

This page is a user-oriented overview.
For exact behavior contracts, see `docs/dev/spec/commands/`.

## Quick flow

```sh
kra init
kra ws create TASK-1234
kra repo add git@github.com:org/backend.git
kra ws add-repo --id TASK-1234
kra ws open --id TASK-1234
kra ws dashboard
```

Notes:

- `kra ws create` uses the default workspace template unless you pass `--template`; the baseline template starts with `notes/` and `artifacts/`.
- Ticket provider integrations are designed to be extensible; current documented support is Jira (`kra ws create --jira`, `kra ws import jira`).

## Root commands

- `kra init` - initialize root and context.
- `kra context ...` - current/list/create/use/rename/rm context operations.
- `kra root ...` - conceptual KRA_ROOT helpers (resolve/open).
- `kra repo ...` - add/discover/remove/gc for repo pool registration.
- `kra repo preset ...` - manage reusable repo sets for `ws add-repo`.
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
- `kra ws open [--id <id> | --current | --select | --multi-select] [--concurrency <n>]`
- `kra ws doc open [--id <id> | --current | --select] [path] [--surface <id|ref|index>] [--no-focus]`
- `kra ws add-repo [--id <id> | --current | --select] [--preset <name>]`
- `kra ws remove-repo [--id <id> | --current | --select]`
- `kra ws task [--id <id> | --current | --select]`
- `kra ws task list [--id <id> | --current | --select]`
- `kra ws task add [--id <id> | --current | --select] --title "<text>"`
- `kra ws task status [--id <id> | --current | --select] <task-id> <todo|doing|blocked|done>`
- `kra ws task sync [--id <id> | --current | --select]`
- `kra ws close [--id <id> | --current | --select | --multi-select]`
- `kra ws reopen [--id <id> | --current | --select]`
- `kra ws purge [--id <id> | --current | --select]`
- `kra ws lock [--id <id> | --current | --select | --multi-select]`
- `kra ws unlock [--id <id> | --current | --select | --multi-select]`

## Task and doc flows

Use workspace-local tasks when you want a Markdown source of truth that can also project into `cmux` sidebar state.

```sh
kra ws task add --current --title "Draft docs"
kra ws task list --current
kra ws task status --current TASK-001 doing
```

Use `kra ws doc open` when you want GitHub-like Markdown viewing in `cmux` for workspace-local docs:

```sh
kra ws doc open --current
kra ws doc open --current notes/
kra ws doc open --current tasks.md --no-focus
```

## Jira import quickstart

Use this flow when you want to create multiple workspaces from Jira issues.

If `integration.jira.defaults.*` is set in `config.yaml`, most flags can be omitted:

```sh
kra ws import jira
```

1. Configure Jira base URL and credentials.
2. Run import in `--sprint` mode (by scope + sprint) or `--jql` mode (explicit query).
3. Review the plan output and use `--apply` to create workspaces.

Examples:

```sh
# Sprint mode (scope from CLI)
kra ws import jira --sprint --space PROJ

# Sprint mode (explicit sprint + project alias)
kra ws import jira --sprint "Sprint 42" --project PROJ --apply --no-prompt

# JQL mode
kra ws import jira --jql "project = PROJ AND statusCategory != Done" --limit 20
```

For setup and default behavior:

- `docs/user/guides/CONFIG.md`

## Workspace target model (`--id` / `--current` / `--select` / `--multi-select`)

Many workspace actions share a common target model:

- `--id <id>`: explicit workspace target (recommended for scripts)
- `--current`: resolve workspace from your current directory context
- `--select`: choose target interactively from the workspace selector
- `--multi-select`: choose multiple targets interactively and run one supported action for all selected workspaces

Typical examples:

- `kra ws open [--id <id> | --current | --select | --multi-select]`
- `kra ws close [--id <id> | --current | --select | --multi-select]`
- `kra ws add-repo [--id <id> | --current | --select]`
- `kra ws remove-repo [--id <id> | --current | --select]`
- `kra ws lock [--id <id> | --current | --select | --multi-select]`
- `kra ws unlock [--id <id> | --current | --select | --multi-select]`

Notes:

- Not every workspace command supports all four forms.
- `ws close` / `ws add-repo` / `ws remove-repo` support `--current` in current implementation.
- `ws add-repo --preset <name>` skips repo selector and resolves repo set from root config preset.
- `--multi-select` support is action-dependent; current support includes `open`, `close`, `lock`, `unlock`, `reopen`, `purge`.
- Positional workspace id arguments are intentionally not supported for these actions.
- For non-interactive automation, prefer explicit `--id`.

## Global flags

- `--debug` - enable debug logging under `<KRA_ROOT>/.kra/logs/`.
- `--version` - print version and exit 0.

## Further reading

- Install guide: `docs/user/guides/INSTALL.md`
- Config guide: `docs/user/guides/CONFIG.md`
- Shell integration guide: `docs/user/guides/SHELL_INTEGRATION.md`
- Workspace lifecycle guide: `docs/user/guides/WORKSPACE_LIFECYCLE.md`
- cmux integration guide: `docs/user/guides/CMUX.md`
- Workspace task guide: `docs/user/guides/TASKS.md`
- Workspace docs viewer guide: `docs/user/guides/WORKSPACE_DOCS.md`
- Repo/worktree guide: `docs/user/guides/REPO_WORKTREE.md`
- Automation JSON guide: `docs/user/guides/AUTOMATION_JSON.md`
- Specs: `docs/dev/spec/README.md`
- Release operations: `docs/dev/ops/RELEASING.md`
