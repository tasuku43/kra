package cli

import (
	"fmt"
	"io"
)

func (c *CLI) printRootUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx [--debug] <command> [args]

Commands:
  init              Initialize GIONX_ROOT
  context           Context commands
  repo              Repository pool commands
  shell             Shell integration commands
  ws                Workspace commands
  version           Print version
  help              Show this help

Run:
  gionx <command> --help
`)
}

func (c *CLI) printContextUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx context <subcommand> [args]

Subcommands:
  current           Print current root
  list              List known roots from root registry
  use <root>        Set current root context
  help              Show this help
`)
}

func (c *CLI) printRepoUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx repo <subcommand> [args]

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
  gionx shell <subcommand> [args]

Subcommands:
  init [shell]      Print shell integration function for eval
  help              Show this help

Examples:
  eval "$(gionx shell init zsh)"
  eval "$(gionx shell init bash)"
  eval (gionx shell init fish)
`)
}

func (c *CLI) printInitUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx init

Initialize GIONX_ROOT (current directory by default, or $GIONX_ROOT if set).
`)
}

func (c *CLI) printWSUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws [--id <id>] [--act <action>] [action-args...]
  gionx ws select [--archived] [--act <go|close|add-repo|reopen|purge>]
  gionx ws create [--no-prompt] <id>
  gionx ws list|ls [--archived] [--tree] [--format human|tsv]

Subcommands:
  create            Create a workspace
  select            Select workspace (then action or fixed action)
  ls                Alias of list
  list              List workspaces
  help              Show this help

Run:
  gionx ws <subcommand> --help

Notes:
- edit actions for existing workspaces are routed by --act.
- active actions: go, add-repo, close
- archived actions: reopen, purge (applies archived scope automatically)
- ws --archived --act go|add-repo|close is invalid.
- gionx ws opens action launcher when --act is omitted.
- gionx ws resolves workspace from --id or current workspace context.
- gionx ws select always opens workspace selection first.
- invalid --act/scope combinations fail with usage.
`)
}

func (c *CLI) printWSCreateUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws create [--no-prompt] <id>

Create a workspace directory under GIONX_ROOT/workspaces/<id>/ and write .gionx.meta.json.

Options:
  --no-prompt        Do not prompt for title (store empty)
`)
}

func (c *CLI) printRepoAddUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx repo add <repo-spec>...

Add one or more repositories into the shared repo pool and register them in the current root index.

Accepted repo-spec formats:
  - git@<host>:<owner>/<repo>.git
  - https://<host>/<owner>/<repo>[.git]
  - file://.../<host>/<owner>/<repo>.git
`)
}

func (c *CLI) printRepoDiscoverUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx repo discover --org <org> [--provider github]

Discover repositories from provider, select multiple repos, and add them into the shared repo pool.

Options:
  --org             Organization name (required)
  --provider        Provider name (default: github)
`)
}

func (c *CLI) printRepoRemoveUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx repo remove [<repo-key>...]

Remove repositories from the current root registry (logical detach from this root only).

Modes:
  - selector mode: omit args (interactive TTY required)
  - direct mode:   pass one or more repo keys

Notes:
  - Physical bare repos in the shared pool are NOT deleted by this command.
  - Repos still bound to any workspace in this root cannot be removed.
`)
}

func (c *CLI) printRepoGCUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx repo gc [<repo-key|repo-uid>...]

Garbage-collect bare repositories from shared repo pool when safety gates pass.

Modes:
  - selector mode: omit args (interactive TTY required)
  - direct mode:   pass repo keys or repo_uids from gc candidates

Safety gates:
  - not registered in current root repos
  - not referenced by current root workspace metadata
  - not referenced by other known roots (root registry scan)
  - no linked worktrees in bare repository
`)
}

func (c *CLI) printWSListUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws list [--archived] [--tree] [--format human|tsv]
  gionx ws ls [--archived] [--tree] [--format human|tsv]

List workspaces from filesystem metadata and repair basic drift.

Options:
  --archived        Show archived workspaces (default: active only)
  --tree            Show repo detail lines under each workspace
  --format          Output format (default: human)
`)
}

func (c *CLI) printWSAddRepoUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws --act add-repo [--id <workspace-id>] [<workspace-id>] [--format human|json]
  gionx ws --act add-repo --format json --id <workspace-id> --repo <repo-key> [--repo <repo-key> ...] [--branch <name>] [--base-ref <origin/branch>] [--yes]

Add repositories from the repo pool to a workspace.

Inputs:
  workspace-id       Existing active workspace ID (optional when running under workspaces/<id>/)
  --id               Explicit workspace ID

Behavior:
  - Select one or more repos from the existing bare repo pool.
  - For each selected repo, input base_ref and branch.
  - Show Plan, ask final confirmation, then create worktrees and bindings atomically.
`)
}

func (c *CLI) printWSGoUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws --act go [--archived] [--id <id>] [--ui] [--format human|json] [<id>]

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
  gionx ws --act close [--id <id>] [--force] [--format human|json] [<id>]

Close (archive) a workspace:
- inspect repo risk (live) and prompt if not clean
- remove git worktrees under workspaces/<id>/repos/
- move workspaces/<id>/ to archive/<id>/ atomically
- commit the archive change in GIONX_ROOT

If ID is omitted, current directory must resolve to an active workspace.
`)
}

func (c *CLI) printWSReopenUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws --act reopen <id>

Reopen an archived workspace:
- move archive/<id>/ to workspaces/<id>/ atomically
- recreate git worktrees under workspaces/<id>/repos/
- commit the reopen change in GIONX_ROOT

Use gionx ws select --archived for interactive selection.
`)
}

func (c *CLI) printWSPurgeUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws --act purge [--no-prompt --force] <id>

Purge (permanently delete) a workspace:
- always asks confirmation in interactive mode
- if workspace is active, inspects repo risk and asks an extra confirmation when risky
- remove git worktrees under workspaces/<id>/repos/ (if present)
- delete workspaces/<id>/ and archive/<id>/ (if present)
- commit the purge change in GIONX_ROOT (message: "purge: <id>")

Options:
  --no-prompt        Do not ask confirmations (requires --force)
  --force            Required with --no-prompt

Use gionx ws select --archived for interactive selection.
`)
}
