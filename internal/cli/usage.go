package cli

import (
	"fmt"
	"io"
)

func (c *CLI) printRootUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx <command> [args]

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
  close             Close workspace (not implemented yet)
  reopen            Reopen workspace (not implemented yet)
  purge             Purge workspace (not implemented yet)
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
  gionx ws list

List workspaces from the state store and repair basic drift from the filesystem.
`)
}

func (c *CLI) printWSAddRepoUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  gionx ws add-repo <workspace-id> <repo>

Add a repository to a workspace as a Git worktree.

Inputs:
  workspace-id       Existing workspace ID (must be active)
  repo               Repo spec (git@... / https://... / file://...)
`)
}
