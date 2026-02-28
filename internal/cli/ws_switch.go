package cli

import (
	"fmt"
	"strings"
)

func (c *CLI) runWSSwitch(args []string) int {
	if len(args) > 0 {
		head := strings.TrimSpace(args[0])
		if head == "-h" || head == "--help" || head == "help" {
			c.printWSSwitchUsage(c.Out)
			return exitOK
		}
	}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "--cmux" || strings.HasPrefix(arg, "--cmux=") {
			fmt.Fprintln(c.Err, "--cmux is no longer supported; use kra ws open with workspace target only")
			c.printWSSwitchUsage(c.Err)
			return exitUsage
		}
	}
	return c.runWSOpen(args)
}
