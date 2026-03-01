# kra

`kra` is a local CLI for ticket-driven development workflows on your filesystem.
It creates an isolated workspace per task, attaches only the repositories you need as Git worktrees, and closes work safely into `archive/` when done.
It is useful standalone for workspace lifecycle operations, and becomes more valuable with `cmux` by aligning ticket, `kra` workspace, and `cmux` workspace in a 1:1:1 operating model.
The default workspace template starts with `notes/` and `artifacts/`, and you can extend it with your own files (for example `AGENTS.md`) and directories.
`<KRA_ROOT>` stores active/archived task workspaces, while `$KRA_HOME` (default: `~/.kra/`) stores shared state such as config and the repo pool.

## Filesystem Model (At a Glance)

`kra` is built around filesystem-based workspace management under `<KRA_ROOT>`.

### ASCII (3-layer view)

```text
[Layer 1: Root]
<KRA_ROOT>/
├── templates/default/
├── workspaces/
│   └── <TASK_ID>/
└── archive/
    └── <TASK_ID>/

[Layer 2: Active Workspace (example when using template: default)]
<KRA_ROOT>/workspaces/<TASK_ID>/
├── notes/
├── artifacts/
└── repos/
    ├── app/      (git worktree)
    └── infra/    (git worktree)

[Layer 3: Shared State]
$KRA_HOME/ (default: ~/.kra/)
├── config.yaml
└── repo-pool/    (shared repo pool)
```

- `kra ws create <ID>`: creates `<KRA_ROOT>/workspaces/<ID>/`
- `kra ws close --id <ID>`: archives to `<KRA_ROOT>/archive/<ID>/`
- `kra ws reopen --id <ID>`: moves `<ID>` from `archive/` back to `workspaces/`
- `kra ws purge --id <ID>`: permanently removes an archived workspace

## Quickstart

```sh
kra init
kra ws create TASK-1234
kra ws open --id TASK-1234
```

When using `cmux`, `kra ws open` creates and opens the corresponding `cmux` workspace if it does not exist, and moves to it if it already exists.
In single-target open, if `cmux` capabilities are unavailable, `kra` falls back to directory-open behavior; with shell integration, it can also sync parent shell `cwd`.

For day-to-day operations (`repo add`, `ws add-repo`, `ws close`), see:

- `docs/user/guides/INSTALL.md`
- `docs/user/guides/COMMANDS.md`
- `docs/user/guides/CONFIG.md`

## Shell Integration (Recommended)

If you use `kra ws open` / `kra ws close` as your primary workspace navigation flow, enable shell integration to sync your parent shell `cwd`.

This prevents staying in a deleted directory after closing a workspace from inside that workspace path.

Setup and details:

- `docs/user/guides/SHELL_INTEGRATION.md`

## Core Features

### 1) Filesystem-based workspace lifecycle
Manage a task workspace through explicit state transitions (`create`, `close`, `reopen`, `purge`), while preserving task outputs in `archive/` on close.

Example:

```sh
kra ws close --id TASK-1234
```

Guide: `docs/user/guides/WORKSPACE_LIFECYCLE.md`

### 2) cmux integration
When using `cmux`, `kra ws open` creates and opens the corresponding `cmux` workspace if it does not exist, and moves to it if it already exists. `kra ws close` closes mapped `cmux` workspace(s) on a best-effort basis after archive operations.

Example:

```sh
kra ws open --id TASK-1234
```

Guide: `docs/user/guides/CMUX.md`

### 3) Per-task worktree attach/remove
Register repositories in the shared repo pool, then attach and remove only the repositories needed per task as worktrees so each workspace keeps only the required repo context.

Example:

```sh
kra ws add-repo --id TASK-1234
```

Details:

- `docs/user/guides/REPO_WORKTREE.md`

## Installation

### Homebrew (stable releases)

```sh
brew tap tasuku43/kra
brew install kra
```

### mise

`mise` can install `kra` from GitHub Releases.

```sh
# pin a version
mise use -g github:tasuku43/kra@v0.1.0

# track latest
mise use -g github:tasuku43/kra@latest
```

Verify:

- `kra version` (or `kra --version`)

### GitHub Releases (manual)

1. Download the archive for your OS/arch from GitHub Releases.
2. Extract it and place `kra` on your `PATH`.
3. Verify with `kra version` (or `kra --version`).

### Build from source

Requirements:

- Go 1.24+
- Git

```sh
go build -o kra ./cmd/kra
./kra version
```

### Jira setup (optional)

Ticket providers are designed to be extensible; current documented support is Jira.

You can always create a workspace with a plain ID:

```sh
kra ws create TASK-1234
```

To create from Jira (`kra ws create --jira <ticket-url>`), configure:

- base URL:
  - `KRA_JIRA_BASE_URL`, or
  - `.kra/config.yaml` / `~/.kra/config.yaml` -> `integration.jira.base_url`
- credentials (env-only):
  - `KRA_JIRA_EMAIL`
  - `KRA_JIRA_API_TOKEN`

Details:

- `docs/user/guides/COMMANDS.md`

## Documentation

- Install guide: `docs/user/guides/INSTALL.md`
- Command reference: `docs/user/guides/COMMANDS.md`
- Config guide: `docs/user/guides/CONFIG.md`
- Workspace lifecycle guide: `docs/user/guides/WORKSPACE_LIFECYCLE.md`
- cmux integration guide: `docs/user/guides/CMUX.md`
- Shell integration guide: `docs/user/guides/SHELL_INTEGRATION.md`
- Repo/worktree guide: `docs/user/guides/REPO_WORKTREE.md`
- Automation JSON guide: `docs/user/guides/AUTOMATION_JSON.md`
- Docs index: `docs/README.md`
- Developer specs (implementation contracts): `docs/dev/spec/README.md`

## Project Meta

- Contributing: `CONTRIBUTING.md`
- Support: `SUPPORT.md`
- Security: `SECURITY.md`
- Code of Conduct: `CODE_OF_CONDUCT.md`
- License: `LICENSE`
- Maintainer: `@tasuku43`
