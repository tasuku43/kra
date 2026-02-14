# kra

`kra` is a local CLI for AI-agent-driven knowledge work.
It helps you create reproducible workspaces from tickets, keep context recoverable, and run workspace lifecycle operations with automation-friendly contracts.

## Who This Is For

- Engineers and operators who work with AI agents every day
- Teams using Jira (or similar external systems) as task source-of-truth
- People who need to resume work quickly and explain outcomes later

## What You Can Do

- Create workspace directories from templates (`kra ws create`)
- Import many Jira issues as workspace plans (`kra ws import jira`)
- Track active/archived workspaces (`kra ws list`, `kra ws dashboard`)
- Run lifecycle operations with guardrails (`go`, `close`, `reopen`, `purge`)

## Install

### Homebrew (stable releases)

```sh
brew tap tasuku43/kra
brew install kra
```

### Direct download

1. Open GitHub Releases for this repository.
2. Download your OS/arch archive (`macos|linux`, `x64|arm64`).
3. Verify `checksums.txt`.
4. Place `kra` in your `PATH`.

## Quick Start

```sh
# 1) initialize root
kra init --root ~/kra

# 2) create a workspace (non-interactive)
kra ws create --no-prompt TASK-1234

# 3) go to workspace
kra ws --id TASK-1234 --act go

# 4) inspect status
kra ws dashboard
```

## Automation and JSON Contract

Commands that support machine output use a shared JSON envelope:

- top-level fields: `ok`, `action`, `workspace_id`, `result`, `error`
- stable error code classes: `invalid_argument`, `not_found`, `conflict`, `permission_denied`, `internal_error`

Example:

```sh
kra ws create --format json --id TASK-1234 --title "API retry hardening"
```

## FAQ

### Does kra replace Jira?

No. Jira (or other external systems) remains the task source-of-truth. `kra` is the local execution and traceability layer.

### Where does kra keep state?

Workspace data is filesystem-first under your selected root (`workspaces/`, `archive/`, metadata files). Additional registries are maintained under `.kra/`.

### Is the `agent` command GA?

Not yet. `agent` capabilities are intentionally EXP for now; public-release readiness focuses on stable core workspace workflows.

## Docs

- Start here: `docs/START_HERE.md`
- Product concept: `docs/concepts/product-concept.md`
- Specs: `docs/spec/`
- Backlog: `docs/backlog/README.md`
- Releasing: `docs/ops/RELEASING.md`
