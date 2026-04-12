package cli

import (
	"fmt"
	"strings"
)

func (c *CLI) runWSImport(args []string) int {
	if len(args) == 0 {
		c.printWSImportUsage(c.Err)
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printWSImportUsage(c.Out)
		return exitOK
	case "all":
		return c.runWSImportAll(args[1:])
	case "github":
		return c.runWSImportGitHub(args[1:])
	case "jira":
		return c.runWSImportJira(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"ws", "import"}, args[0]), " "))
		c.printWSImportUsage(c.Err)
		return exitUsage
	}
}
