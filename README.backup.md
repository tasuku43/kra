# kra

`kra` is a local CLI for AI-agent-oriented, ticket-driven workspace operations.
It standardizes task workspaces on the filesystem through template-driven workspace scaffolding and per-workspace Git worktrees for the repositories each task actually needs.
The default template includes `notes/` and `artifacts/`, and you can define your own structures (for example `AGENTS.md`, `CLOUD.md`, and custom directories) to provide a predictable workspace scaffold for multi-repo execution and continuous accumulation of intermediate outputs.

`kra` works standalone for workspace lifecycle operations, and becomes even more valuable when paired with `cmux` for agent runtime/workspace integration.
Ticket providers are designed to be extensible; currently, Jira is supported.

`<KRA_ROOT>` stores task workspaces and `archive/`, while `$KRA_HOME` (default: `~/.kra/`) stores shared state such as config and the repo pool.

## One-minute mental model

- `kra ws create TASK-1234` creates a task workspace under `workspaces/TASK-1234/`.
- `kra ws add-repo --id TASK-1234` attaches only the needed repositories as worktrees under `repos/`.
- `kra ws open --id TASK-1234` opens that task-scoped execution context (and can align with `cmux` runtime workflow).
- `kra ws close --id TASK-1234` moves task outputs to `archive/` and removes workspace worktrees from active area.

## Shell integration

If you want `kra` to synchronize your parent shell `cwd` (for example fallback `cd` behavior in `ws open`), enable shell integration:

```sh
# zsh
eval "$(kra shell init zsh)"

# bash
eval "$(kra shell init bash)"

# fish
eval (kra shell init fish)
```

To persist this, add the corresponding `eval ...` line to your shell rc file (`~/.zshrc`, `~/.bashrc`, or `~/.config/fish/config.fish`).
`kra shell init <shell>` prints shell code to stdout; review it first if you prefer.

Without shell integration, `kra` still runs commands but cannot mutate your parent shell `cwd`.

## cmux integration

`kra` provides an operational framework for using `cmux` in ticket-driven, filesystem-based workflows.

In this model, `kra` acts as the glue across:

- ticket
- filesystem task workspace (`workspaces/<id>/`)
- `cmux` runtime workspace

This gives you an operational 1:1:1 mapping across ticket, task workspace, and runtime workspace, so agents can run in a consistent task-scoped workspace and continuously write intermediate outputs as work progresses.

`kra` works standalone for workspace lifecycle operations, but this `kra` + `cmux` operating model is where the overall system value becomes strongest.

`ws open` / `ws close` behavior with `cmux`:

- `kra ws open --id <id>`:
  - if no `cmux` mapping exists for the workspace, `kra` creates/selects a `cmux` workspace and persists the mapping.
  - if a mapping already exists and runtime workspace is reachable, `kra` reuses it (switch/select) instead of creating another one.
  - single-target open (for example `kra ws open --id <id>` or `--current`; `--current` resolves workspace from current directory) falls back to shell `cd` synchronization when `cmux` capabilities are unavailable.
- `kra ws close --id <id>`:
  - after archive/workspace close operations, `kra` closes the mapped `cmux` workspace on a best-effort basis.
  - `cmux` close failures do not roll back successful archive results.

Note:

- `kra` workspace and `cmux` workspace mapping is 1:1.

## worktree management

`kra` lets you manage Git worktrees per task workspace with `kra ws add-repo` / `kra ws remove-repo`.
You can attach only the repositories needed for the current task, exactly when they become necessary.

Repositories are attached under:

- `workspaces/<id>/repos/<alias>/`

This keeps execution context task-scoped and avoids mixing temporary task outputs into a single long-lived repository.

When work spans multiple repositories, this model lets you build a task-scoped, monorepo-like execution surface quickly, so agents can use the right repository context at the right time without long setup cycles.

## Why kra exists

AI-agent-driven work now produces large volumes of intermediate outputs, not only code changes but also investigations, analyses, logs, and notes.
Those artifacts need a temporary but structured home before they are distilled into final deliverables.
In practice, people often dump them into ad hoc locations first, such as random local directories or inside an active repository, and later struggle with cleanup and traceability.

In AI-agent-driven workflows that span multiple repositories and task contexts, common problems include:

- Non-code outputs end up in inconsistent locations, making task-scoped recovery harder.
- Task context is difficult to keep organized in a way that supports fast resume.
- After task switching, the exact repo/branch combination for a task is easy to lose.
- Manual workspace rebuilds drift over time (missing repos, extra repos, wrong branch context).

`kra` addresses this by making the filesystem workspace the unit of execution and traceability.
External ticket systems remain the source of truth for task management, while `kra` provides a repeatable local operating model.

## What it gives you

- A template-driven workspace scaffold for each task:
  - start with the default template (`notes/`, `artifacts/`)
  - replace or extend it with your own structure (`AGENTS.md`, `CLOUD.md`, custom directories/files)
- Per-task, per-workspace multi-repo worktree management:
  - add only repositories required for that task
  - keep them under `workspaces/<id>/repos/<alias>/` for a predictable local execution surface
- An operational bridge between planning and runtime:
  - anchor ticket context to a filesystem workspace
  - pair that workspace with `cmux` runtime workspace(s) as an operational model
- State-first lifecycle operations with explicit transitions:
  - create, open, close, reopen, purge with clear state rules
  - move completed workspaces to `archive/` by default instead of destructive deletion
- Guardrails for risky operations:
  - evaluate workspace risk (`dirty`, `unpushed`, `diverged`, `unknown`)
  - apply confirmation gates for destructive flows
- Automation-ready output contracts:
  - use a shared JSON envelope (`ok`, `action`, `workspace_id`, `result`, `error`) across supported commands

## Quickstart (5 minutes)

```sh
# 1) initialize a root (interactive)
kra init

# 2) create a workspace from your ticket id (interactive)
kra ws create TASK-1234

# 3) register repositories into the repo pool
kra repo add git@github.com:org/backend.git git@github.com:org/frontend.git

# 4) attach needed repositories to the workspace (interactive selector + prompts)
kra ws add-repo --id TASK-1234

# 5) open workspace context
kra ws open --id TASK-1234

# 6) inspect current state
kra ws dashboard

# 7) close when done (moves notes/artifacts to archive/, removes worktrees)
kra ws close --id TASK-1234
```

`kra init`, `kra ws create`, and `kra ws add-repo` will guide you with prompts in this quickstart flow.
With shell integration enabled, `kra ws open` can synchronize your shell `cwd`; without it, `kra` still runs but cannot mutate parent shell `cwd`.
You can also use interactive selection for workspace-targeted commands (for example: `kra ws open --select`, `kra ws close --select`).

After this flow, your task context and artifacts remain reviewable under `archive/<id>/`, while active workspace area stays clean.

### Resulting layout (example)

```text
# before close
<KRA_ROOT>/
├─ workspaces/
│  └─ TASK-1234/
│     ├─ notes/
│     ├─ artifacts/
│     └─ repos/
│        ├─ backend/   (git worktree)
│        └─ frontend/  (git worktree)
└─ archive/

# after close
<KRA_ROOT>/
├─ workspaces/
└─ archive/
   └─ TASK-1234/
      ├─ notes/
      ├─ artifacts/
      └─ .kra.meta.json (includes repos_restore metadata)
```

## Workspace lifecycle semantics

State model:

- `create` -> `active`
- `close` -> `archived`
- `reopen` -> `active`
- `purge` -> terminal cleanup

`close` behavior (important):

- `kra` removes worktrees under `workspaces/<id>/repos/`, then moves remaining workspace outputs (`notes/`, `artifacts/`, `.kra.meta.json`) to `archive/<id>/` (not copied).
- Non-clean repo risk (`dirty`, `unpushed`, `diverged`, `unknown`) triggers a safety gate before applying.

Risk labels:

- `dirty`: workspace worktree has local file changes.
- `unpushed`: local branch is ahead of upstream.
- `diverged`: local and upstream both advanced.
- `unknown`: risk state could not be determined.

`reopen` behavior (important):

- `kra ws reopen` restores workspace-side repository attachments from `repos_restore` in `.kra.meta.json`.
- In normal operation, `close` and `reopen` are the standard round-trip between active and archived work.

For command-specific details, see:

- `docs/spec/commands/ws/close.md`
- `docs/spec/commands/ws/reopen.md`
- `docs/spec/commands/ws/purge.md`
- `docs/spec/concepts/workspace-lifecycle.md`

## Repo pool, alias, and branch model

`kra` separates repository registration from workspace attachment:

1. `kra repo add` registers repositories into the shared repo pool.
2. `kra ws add-repo --id <id>` selects from that pool and attaches worktrees into the workspace.

Operational notes:

- `repo pool` stores managed bare repositories and root-level registration state.
- Physical shared pool location is `$KRA_HOME/repo-pool/` (default: `~/.kra/repo-pool/`; `$KRA_HOME` default is `~/.kra/`).
- `kra repo add` creates/updates shared bare mirrors in this pool, and `ws add-repo` uses them to create workspace worktrees.
- Worktree alias is derived from repository identity and must be unique per workspace.
- `ws add-repo` prompts for `base_ref` and `branch` (with defaults), so branch context is explicit and reproducible per task.

See:

- `docs/spec/commands/repo/add.md`
- `docs/spec/commands/ws/add-repo.md`

## cmux minimal recipe

`kra` works standalone, but if you use `cmux`, a minimal operating recipe is:

1. Create and prepare workspace (`ws create`, `repo add`, `ws add-repo`).
2. Open task context with `kra ws open --id <id>`.
3. Run your agent against that task workspace context.
4. Close with `kra ws close --id <id>` when done.

In this flow, `kra` handles workspace-side coordination and mapping policy.
If `cmux` capabilities are unavailable in a single-target open flow, `kra ws open` falls back to shell `cd` synchronization.

## Boundaries

`kra` is intentionally focused.

- It is not a replacement for Jira or other external ticket systems.
- It is not an agent runtime/session manager.
  - `kra` does not manage agent sessions; it maps/coordinates workspace context and performs best-effort `cmux` workspace selection/cleanup.
- It is not a GUI planning tool.

## Installation

### Homebrew (stable releases)

```sh
brew tap tasuku43/kra
brew install kra
```

### GitHub Releases (manual)

1. Download an archive for your OS/arch from GitHub Releases.
2. Extract and place `kra` on your `PATH`.
3. Verify with `kra version`.

### Jira setup (minimum)

Jira integration is optional. You can always create a workspace with a plain ID:

```sh
kra ws create TASK-1234
```

To create from Jira (`kra ws create --jira <ticket-url>`), configure:

- `KRA_JIRA_BASE_URL`
- `KRA_JIRA_EMAIL`
- `KRA_JIRA_API_TOKEN`

See `docs/spec/commands/ws/create.md` and `docs/spec/commands/ws/import/jira.md` for exact behavior.

### Build from source

Requirements:

- Go 1.24+
- Git

```sh
go build -o kra ./cmd/kra
./kra version
```

## JSON envelope example

`ws create` accepts both positional workspace ID (`kra ws create TASK-1234`) and explicit `--id`.

Example (`kra ws create --format json --id TASK-1234 --title "API retry hardening"`):

```json
{
  "ok": true,
  "action": "ws.create",
  "workspace_id": "TASK-1234",
  "result": {
    "created": 1,
    "path": "<KRA_ROOT>/workspaces/TASK-1234"
  },
  "error": null
}
```

## User guides

- Start here: `docs/START_HERE.md`
- Install guide: `docs/guides/INSTALL.md`
- Command reference: `docs/guides/COMMANDS.md`

## Development and operations references

- Product concept: `docs/concepts/product-concept.md`
- Specs: `docs/spec/README.md`
- Releasing: `docs/ops/RELEASING.md`

## Contributing

See `CONTRIBUTING.md`.

## Support

See `SUPPORT.md`.

## Security

See `SECURITY.md`.

## Code of Conduct

See `CODE_OF_CONDUCT.md`.

## License

See `LICENSE`.

## Maintainer

- @tasuku43
