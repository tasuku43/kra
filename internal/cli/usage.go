package cli

import (
	"fmt"
	"io"
	"strings"
)

func (c *CLI) printRootUsage(w io.Writer) {
	commands := []string{
		"  init              Initialize KRA_ROOT",
		"  bootstrap         Bootstrap project-local runtime scaffolds",
		"  context           Context commands",
		"  repo              Repository pool commands",
		"  template          Workspace template commands",
		"  shell             Shell integration commands",
		"  ws                Workspace commands",
		"  doctor            Diagnose KRA_ROOT health",
	}
	commands = append(commands,
		"  version           Print version",
		"  help              Show this help",
	)

	fmt.Fprintf(
		w,
		"Usage:\n  kra [global-flags] <command> [args]\n  kra --version\n\nCommands:\n%s\n\nGlobal flags:\n  --debug            Enable debug logging to <KRA_ROOT>/.kra/logs/\n  --version          Print version and exit\n  --help, -h         Show this help\n\nRun:\n  kra <command> --help\n",
		strings.Join(commands, "\n"),
	)
}

func (c *CLI) printBootstrapUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra bootstrap <subcommand> [args]

Subcommands:
  agent-skills      Bootstrap project-local agent skills references
  help              Show this help
`)
}

func (c *CLI) printBootstrapAgentSkillsUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra bootstrap agent-skills [--format human|json]

Bootstrap project-local skill references for the current context root.

Rules:
  - target root is resolved from current context only
  - --root/--context are not supported
`)
}

func (c *CLI) printContextUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra context <subcommand> [args]

Subcommands:
  current [--format human|json]
                   Print current context name (or path fallback)
  list [--format human|json]
                   List contexts (name/path) from root registry
  create <name> --path <path> [--use] [--format human|json]
                   Create a named context
  use [name] [--format human|json]
                   Select context by name (or interactive selector)
  rename <old> <new> [--format human|json]
                   Rename context
  rm [name] [--format human|json]
                   Remove context (or interactive selector; cannot remove current context)
  help              Show this help

Notes:
  - context use/rm in --format json mode require explicit <name> (non-interactive)
`)
}

func (c *CLI) printRepoUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra repo <subcommand> [args]

Subcommands:
  add               Add repositories into shared repo pool
  discover          Discover repositories from provider and add selected
  remove            Remove repositories from current root registration
  gc                Garbage-collect removable bare repos from shared pool
  help              Show this help
`)
}

func (c *CLI) printShellUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra shell <subcommand> [args]

Subcommands:
  init [shell]      Print shell integration function for eval
  help              Show this help

Examples:
  eval "$(kra shell init zsh)"
  eval "$(kra shell init bash)"
  eval (kra shell init fish)
`)
}

func (c *CLI) printTemplateUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra template <subcommand> [args]

Subcommands:
  validate          Validate workspace templates under current root
  help              Show this help
`)
}

func (c *CLI) printTemplateValidateUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra template validate [--name <template>]

Validate templates under <current-root>/templates.

Options:
  --name            Validate only one template (default: validate all templates)
`)
}

func (c *CLI) printDoctorUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra doctor [--format human|json]
  kra doctor --fix --plan [--format human|json]
  kra doctor --fix --apply [--format human|json]

Diagnose current KRA_ROOT health and optionally run staged remediation.

Options:
  --format          Output format (default: human)
  --fix             Enable remediation mode
  --plan            Print remediation actions without mutation (requires --fix)
  --apply           Apply remediation actions (requires --fix)
`)
}

func (c *CLI) printInitUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra init [--root <path>] [--context <name>] [--bootstrap agent-skills] [--format human|json]

Initialize KRA_ROOT and set current context.

Root selection order:
- --root <path> (explicit)
- interactive prompt in TTY (default: ~/kra)
- non-TTY without --root: fail

Context name:
- --context <name> (explicit)
- interactive prompt in TTY (default: cwd basename)
- non-TTY without --context: fail

JSON mode:
- requires --root and --context (no interactive prompt)

Bootstrap:
- --bootstrap supports only "agent-skills" in this phase
`)
}

func (c *CLI) printWSUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws [--id <id> | --current | --select]
  kra ws open [--id <id> | --current | --select] [--multi] [--concurrency <n>] [--format human|json]
  kra ws <add-repo|remove-repo|close|reopen|purge> [--id <id> | --current | --select] [action-args...]
  kra ws --select [--archived] [open|close|add-repo|remove-repo|reopen|unlock|purge]
  kra ws --select --multi [--archived] <close|reopen|purge> [--no-commit]
  kra ws create [--no-prompt] [--template <name>] [--format human|json] <id>
  kra ws create [--no-prompt] [--template <name>] [--format human|json] --id <id> [--title "<title>"]
  kra ws create --jira <ticket-url> [--template <name>] [--format human|json]
  kra ws import jira (--sprint [<id|name>] [--space <key>|--project <key>] | --jql "<expr>") [--limit <n>] [--apply] [--no-prompt] [--format human|json]
  kra ws list|ls [--archived] [--tree] [--format human|tsv|json]
  kra ws dashboard [--archived] [--workspace <id>] [--format human|json]
  kra ws lock <id> [--format human|json]
  kra ws unlock <id> [--format human|json]

Subcommands:
  create            Create a workspace
  import            Import workspaces from external systems
  open              Open cmux workspace(s) for selected workspace target
  add-repo          Add repositories to a workspace
  remove-repo       Remove repositories from a workspace
  close             Archive a workspace
  reopen            Restore an archived workspace
  purge             Permanently delete an archived workspace
  list              List workspaces
  ls                Alias of list
  dashboard         Show workspace operational dashboard
  lock              Enable purge guard on workspace metadata
  unlock            Disable purge guard on workspace metadata
  help              Show this help

Run:
  kra ws <subcommand> --help

Notes:
- edit actions are routed by ws <action> subcommands.
- active actions: open, add-repo, remove-repo, close
- archived actions: reopen, unlock, purge (applies archived scope automatically)
- ws --archived with add-repo|remove-repo|close is invalid.
- kra ws requires explicit target mode: --id, --current, or --select.
- kra ws does not resolve workspace implicitly from current path unless --current is set.
- kra ws --select always opens workspace selection first.
- ws --select --multi requires an action positional argument.
- ws --select --multi supports only close/reopen/purge.
- ws --select --multi reopen|purge implies archived scope.
- ws --select --multi commits by default; use --no-commit to disable lifecycle commits.
- invalid action/scope combinations fail with usage.
`)
}

func (c *CLI) printWSOpenUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws open [--id <id> | --current | --select] [--multi] [--concurrency <n>] [--format human|json]

Open flow for cmux workspace integration.
`)
}

func (c *CLI) printWSSwitchUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws switch [same options as "kra ws open"]

Alias of "kra ws open" for backward compatibility.
`)
}

// Legacy internal helpers kept for shared parser/renderer reuse.
func (c *CLI) printCMUXOpenUsage(w io.Writer)   { c.printWSOpenUsage(w) }
func (c *CLI) printCMUXSwitchUsage(w io.Writer) { c.printWSSwitchUsage(w) }

func (c *CLI) printCMUXListUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws list [--format human|json]

List mapped cmux workspaces.
`)
}

func (c *CLI) printCMUXStatusUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws status [--format human|json]

Show mapping status for cmux integration.
`)
}

func (c *CLI) printWSLockUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws lock <id> [--format human|json]

Enable purge guard for the target workspace.
`)
}

func (c *CLI) printWSInsightUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws insight <subcommand> [args]

Subcommands:
  add               Save one approved insight into workspace worklog
  help              Show this help
`)
}

func (c *CLI) printWSInsightAddUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws insight add --id <workspace-id> --ticket <ticket> --session-id <session-id> --what "<text>" --approved [--context "<text>"] [--why "<text>"] [--next "<text>"] [--tag <tag> ...] [--format human|json]

Save one approved insight document into worklog/insights.
`)
}

func (c *CLI) printWSDashboardUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws dashboard [--archived] [--workspace <id>] [--format human|json]

Show operational dashboard for workspaces.
`)
}

func (c *CLI) printWSUnlockUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws unlock <id> [--format human|json]

Disable purge guard for the target workspace.
`)
}

func (c *CLI) printWSCreateUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws create [--no-prompt] [--template <name>] [--format human|json] <id>
  kra ws create [--no-prompt] [--template <name>] [--format human|json] --id <id> [--title "<title>"]
  kra ws create --jira <ticket-url> [--template <name>] [--format human|json]

Create a workspace directory from template and write .kra.meta.json.

Options:
  --no-prompt        Do not prompt for title (store empty)
  --id               Explicit workspace id (automation-friendly alternative to positional <id>)
  --title            Workspace title for non-Jira create (skips title prompt)
  --template         Template name under <current-root>/templates (default: default)
  --jira             Resolve workspace id/title from Jira issue URL (email/token env required; base URL supports config)
  --format           Output format (human or json; default: human)
`)
}

func (c *CLI) printWSImportUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws import <source> [args]

Sources:
  jira              Import workspaces from Jira issues
  help              Show this help
`)
}

func (c *CLI) printWSImportJiraUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws import jira --sprint [<id|name>] --space <key> [--limit <n>] [--apply] [--no-prompt] [--format human|json]
  kra ws import jira --sprint [<id|name>] --project <key> [--limit <n>] [--apply] [--no-prompt] [--format human|json]
  kra ws import jira --jql "<expr>" [--limit <n>] [--apply] [--no-prompt] [--format human|json]

Plan-first bulk workspace creation from Jira.

Rules:
  --sprint and --jql are mutually exclusive.
  If both are omitted, mode is resolved from config defaults.
  --space (or --project) is required with --sprint.
  --space and --project cannot be combined.
  --board is not supported (use --space/--project with --sprint).
  --limit default is 30 (range: 1..200).
`)
}

func (c *CLI) printRepoAddUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra repo add [--format human|json] <repo-spec>...

Add one or more repositories into the shared repo pool and register them in the current root index.

Accepted repo-spec formats:
  - git@<host>:<owner>/<repo>.git
  - https://<host>/<owner>/<repo>[.git]
  - file://.../<host>/<owner>/<repo>.git
`)
}

func (c *CLI) printRepoDiscoverUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra repo discover --org <org> [--provider github]

Discover repositories from provider, select multiple repos, and add them into the shared repo pool.

Options:
  --org             Organization name (required)
  --provider        Provider name (default: github)
`)
}

func (c *CLI) printRepoRemoveUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra repo remove [--format human|json] [<repo-key>...]

Remove repositories from the current root registry (logical detach from this root only).

Modes:
  - selector mode: omit args (interactive TTY required)
  - direct mode:   pass one or more repo keys

Notes:
  - Physical bare repos in the shared pool are NOT deleted by this command.
  - Repos still bound to any workspace in this root cannot be removed.
  - --format json mode requires one or more repo keys.
`)
}

func (c *CLI) printRepoGCUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra repo gc [--format human|json] [--yes] [<repo-key|repo-uid>...]

Garbage-collect bare repositories from shared repo pool when safety gates pass.

Modes:
  - selector mode: omit args (interactive TTY required)
  - direct mode:   pass repo keys or repo_uids from gc candidates

Safety gates:
  - not registered in current root repos
  - not referenced by current root workspace metadata
  - not referenced by other known roots (root registry scan)
  - no linked worktrees in bare repository

Notes:
  - --format json mode requires explicit targets and --yes.
`)
}

func (c *CLI) printWSListUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws list [--archived] [--tree] [--format human|tsv|json]
  kra ws ls [--archived] [--tree] [--format human|tsv|json]

List workspaces from filesystem metadata and repair basic drift.

Options:
  --archived        Show archived workspaces (default: active only)
  --tree            Show repo detail lines under each workspace
  --format          Output format (default: human)
`)
}

func (c *CLI) printWSAddRepoUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws add-repo [--id <workspace-id> | --current | --select] [<workspace-id>] [--format human|json] [--refresh] [--no-fetch]
  kra ws add-repo --format json --id <workspace-id> --repo <repo-key> [--repo <repo-key> ...] [--branch <name>] [--base-ref <origin/branch>] [--refresh] [--no-fetch] [--yes]

Add repositories from the repo pool to a workspace.

Inputs:
  workspace-id       Existing active workspace ID (optional when running under workspaces/<id>/)
  --id               Explicit workspace ID

Behavior:
  - Select one or more repos from the existing bare repo pool.
  - For each selected repo, input base_ref and branch.
  - base_ref accepts: origin/<branch>, <branch>, /<branch>.
  - Smart fetch runs for selected repos only (TTL=5m; --refresh forces, --no-fetch skips).
  - Show Plan, ask final confirmation, then create worktrees and bindings atomically.
`)
}

func (c *CLI) printWSRemoveRepoUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws remove-repo [--id <workspace-id> | --current | --select] [<workspace-id>] [--format human|json]
  kra ws remove-repo --format json --id <workspace-id> --repo <repo-key> [--repo <repo-key> ...] [--yes] [--force]

Remove repositories from a workspace (binding + worktree).

Inputs:
  workspace-id       Existing active workspace ID (optional when running under workspaces/<id>/)
  --id               Explicit workspace ID

Behavior:
  - Select one or more repos already bound to the workspace.
  - Show Plan and ask confirmation.
  - Remove workspace bindings and corresponding worktrees.
  - Repo pool entries/bare repositories are kept.
`)
}

func (c *CLI) printWSGoUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws go [--archived] [--id <id> | --current | --select] [--ui] [--format human|json] [<id>]

Resolve a workspace directory target:
- active target: workspaces/<id>/
- archived target (--archived): archive/<id>/

Options:
  --archived        Use archived workspace scope
  --id              Explicit workspace ID
  --ui              Print human-readable Result section
`)
}

func (c *CLI) printWSCloseUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws close [--id <id> | --current | --select] [--force] [--format human|json] [--no-commit] [<id>]
  kra ws close --dry-run --format json [--id <id>|<id>]

Close (archive) a workspace:
- inspect repo risk (live) and prompt if not clean
- remove git worktrees under workspaces/<id>/repos/
- move workspaces/<id>/ to archive/<id>/ atomically
- by default, lifecycle commits run automatically (pre-close + archive).
- --no-commit: disable lifecycle commits for this command

If ID is omitted, current directory must resolve to an active workspace.
`)
}

func (c *CLI) printWSReopenUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws reopen [--id <id> | --current | --select] [--no-commit] [<id>]
  kra ws reopen --dry-run --format json [--id <id>|<id>]

Reopen an archived workspace:
- move archive/<id>/ to workspaces/<id>/ atomically
- recreate git worktrees under workspaces/<id>/repos/
- by default, lifecycle commits run automatically (pre-reopen + reopen).
- --no-commit: disable lifecycle commits for this command

Use kra ws select --archived for interactive selection.
`)
}

func (c *CLI) printWSPurgeUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws purge [--id <id> | --current | --select] [--no-prompt --force] [--no-commit] [<id>]
  kra ws purge --dry-run --format json [--id <id>|<id>]

Purge (permanently delete) a workspace:
- always asks confirmation in interactive mode
- if workspace is active, inspects repo risk and asks an extra confirmation when risky
- remove git worktrees under workspaces/<id>/repos/ (if present)
- delete workspaces/<id>/ and archive/<id>/ (if present)
- by default, lifecycle commits run automatically (pre-purge + purge).
- --no-commit: disable lifecycle commits for this command

Options:
  --no-prompt        Do not ask confirmations (requires --force)
  --force            Required with --no-prompt

Use kra ws select --archived for interactive selection.
`)
}

func (c *CLI) printAgentUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra agent <subcommand> [args]

Subcommands:
  run               Start an agent activity
  stop              Stop a running agent activity
  logs              Show logs for an agent activity
  list              List agent activities
  ls                Alias of list
  help              Show this help
`)
}

func (c *CLI) printAgentRunUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra agent run --workspace <id> --kind <agent-kind> [--repo <repo-key>] [--task <summary>] [--instruction <summary>] [--status <running|waiting_user|thinking|blocked>] [--log-path <path>]

Start/replace tracked running agent activity for one workspace.

Options:
  --workspace       Workspace ID (required)
  --kind            Agent kind label (required)
  --repo            Optional repository key in workspace scope
  --task            Optional short work summary
  --instruction     Optional short instruction summary
  --status          Initial live status (default: running)
  --log-path        Optional log path for operator navigation
`)
}

func (c *CLI) printAgentStopUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra agent stop --workspace <id> [--status succeeded|failed|unknown]

Stop tracked running agent activity for one workspace.

Options:
  --workspace       Workspace ID (required)
  --status          Final status (default: failed)
`)
}

func (c *CLI) printAgentListUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra agent list [--workspace <id>] [--format human|tsv]
  kra agent ls [--workspace <id>] [--format human|tsv]

List tracked agent activities managed by kra in current KRA_ROOT.

Options:
  --workspace       Filter by workspace ID
  --format          Output format (default: human; tsv columns include repo/task/instruction summaries)
`)
}

func (c *CLI) printAgentLogsUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra agent logs --workspace <id> [--tail <n>] [--follow]

Show logs for one workspace's current tracked agent activity.

Options:
  --workspace       Workspace ID (required)
  --tail            Show only the last N lines (default: 100)
  --follow          Keep streaming appended lines
`)
}
