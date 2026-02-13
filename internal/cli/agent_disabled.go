//go:build !experimental

package cli

import "fmt"

func (c *CLI) runAgent(_ []string) int {
	fmt.Fprintln(c.Err, `unknown command: "agent"`)
	c.printRootUsage(c.Err)
	return exitUsage
}
