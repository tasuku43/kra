package cli

import (
	"fmt"
	"strings"
)

func (c *CLI) runRepo(args []string) int {
	if len(args) == 0 {
		c.printRepoUsage(c.Err)
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printRepoUsage(c.Out)
		return exitOK
	case "add":
		return c.runRepoAdd(args[1:])
	case "discover":
		return c.runRepoDiscover(args[1:])
	case "remove":
		return c.runRepoRemove(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"repo"}, args[0]), " "))
		c.printRepoUsage(c.Err)
		return exitUsage
	}
}
