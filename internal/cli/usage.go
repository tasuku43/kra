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
  ws                Workspace commands
  version           Print version
  help              Show this help

Run:
  gionx <command> --help
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
  gionx ws go [--archived] [--emit-cd] [<id>]

Resolve a workspace directory target:
- active target: workspaces/<id>/
- archived target (--archived): archive/<id>/

Modes:
- direct mode: provide <id>
- selector mode: omit <id> (interactive TTY required, single selection)

Options:
  --archived        Use archived workspace scope
  --emit-cd         Print shell snippet (for example: eval "$(gionx ws go --emit-cd)")
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
