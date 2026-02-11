package cli

import (
	"fmt"
	"strings"
)

func (c *CLI) runTemplate(args []string) int {
	if len(args) == 0 {
		c.printTemplateUsage(c.Err)
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printTemplateUsage(c.Out)
		return exitOK
	case "validate":
		return c.runTemplateValidate(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"template"}, args[0]), " "))
		c.printTemplateUsage(c.Err)
		return exitUsage
	}
}
