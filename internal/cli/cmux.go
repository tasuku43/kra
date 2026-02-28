package cli

import (
	"fmt"
	"strings"
)

func (c *CLI) runCMUX(args []string) int {
	if len(args) == 0 {
		c.printCMUXUsage(c.Err)
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printCMUXUsage(c.Out)
		return exitOK
	case "open":
		return c.runCMUXOpen(args[1:])
	case "switch":
		return c.runCMUXSwitch(args[1:])
	case "list":
		return c.runCMUXList(args[1:])
	case "status":
		return c.runCMUXStatus(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"cmux"}, args[0]), " "))
		c.printCMUXUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runCMUXSwitch(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printCMUXSwitchUsage(c.Out)
			return exitOK
		}
	}
	return c.notImplemented("cmux switch")
}

func (c *CLI) runCMUXList(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printCMUXListUsage(c.Out)
			return exitOK
		}
	}
	return c.notImplemented("cmux list")
}

func (c *CLI) runCMUXStatus(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printCMUXStatusUsage(c.Out)
			return exitOK
		}
	}
	return c.notImplemented("cmux status")
}
