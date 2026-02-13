package cli

import (
	"fmt"
	"strings"
)

func (c *CLI) runAgent(args []string) int {
	if len(args) == 0 {
		c.printAgentUsage(c.Err)
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printAgentUsage(c.Out)
		return exitOK
	case "run":
		return c.runAgentRun(args[1:])
	case "stop":
		return c.runAgentStop(args[1:])
	case "list", "ls":
		return c.runAgentList(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"agent"}, args[0]), " "))
		c.printAgentUsage(c.Err)
		return exitUsage
	}
}
