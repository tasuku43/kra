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
  list              List known roots from state registry
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
  gionx ws <subcommand> [args]

Subcommands:
  create            Create a workspace
  list              List workspaces
  add-repo          Add repo to workspace
  go                Navigate to workspace path
  close             Close workspace
  reopen            Reopen workspace
  purge             Purge workspace
  help              Show this help

Run:
  gionx ws <subcommand> --help
`)
}

func (c *CLI) printWSCreateUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws create [--no-prompt] <id>

Create a workspace directory under GIONX_ROOT/workspaces/<id>/ and record it in the state store.

Options:
  --no-prompt        Do not prompt for description (store empty)
`)
}

func (c *CLI) printRepoAddUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx repo add <repo-spec>...

Add one or more repositories into the shared repo pool and register them in the current root state DB.

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
  - not bound in current root workspace_repos
  - not referenced by other known roots (state registry scan)
  - no linked worktrees in bare repository
`)
}

func (c *CLI) printWSListUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws list [--archived] [--tree] [--format human|tsv]

List workspaces from the state store and repair basic drift from the filesystem.

Options:
  --archived        Show archived workspaces (default: active only)
  --tree            Show repo detail lines under each workspace
  --format          Output format (default: human)
`)
}

func (c *CLI) printWSAddRepoUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws add-repo [<workspace-id>]

Add repositories from the repo pool to a workspace.

Inputs:
  workspace-id       Existing active workspace ID (optional when running under workspaces/<id>/)

Behavior:
  - Select one or more repos from the existing bare repo pool.
  - For each selected repo, input base_ref and branch.
  - Show Plan, ask final confirmation, then create worktrees and bindings atomically.
`)
}

func (c *CLI) printWSGoUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws go [--archived] [--ui] [--emit-cd] [<id>]

Resolve a workspace directory target:
- active target: workspaces/<id>/
- archived target (--archived): archive/<id>/

Modes:
- direct mode: provide <id>
- selector mode: omit <id> (interactive TTY required, single selection)

Options:
  --archived        Use archived workspace scope
  --ui              Print human-readable Result section instead of shell snippet
  --emit-cd         Backward-compatible alias of default output
`)
}

func (c *CLI) printWSCloseUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws close [<id>]

Close (archive) a workspace:
- inspect repo risk (live) and prompt if not clean
- remove git worktrees under workspaces/<id>/repos/
- move workspaces/<id>/ to archive/<id>/ atomically
- commit the archive change in GIONX_ROOT

Modes:
- direct mode: provide <id>
- selector mode: omit <id> (interactive TTY required)
`)
}

func (c *CLI) printWSReopenUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws reopen [<id>]

Reopen an archived workspace:
- move archive/<id>/ to workspaces/<id>/ atomically
- recreate git worktrees under workspaces/<id>/repos/
- commit the reopen change in GIONX_ROOT

Modes:
- direct mode: provide <id>
- selector mode: omit <id> (interactive TTY required, archived scope)
`)
}

func (c *CLI) printWSPurgeUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws purge [--no-prompt --force] [<id>]

Purge (permanently delete) a workspace:
- always asks confirmation in interactive mode
- if workspace is active, inspects repo risk and asks an extra confirmation when risky
- remove git worktrees under workspaces/<id>/repos/ (if present)
- delete workspaces/<id>/ and archive/<id>/ (if present)
- commit the purge change in GIONX_ROOT (message: "purge: <id>")

Options:
  --no-prompt        Do not ask confirmations (requires --force)
  --force            Required with --no-prompt

Modes:
- direct mode: provide <id>
- selector mode: omit <id> (interactive TTY required, archived scope)
`)
}
