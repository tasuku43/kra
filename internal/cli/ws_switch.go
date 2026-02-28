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

	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "--cmux" {
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--cmux requires a value")
				c.printWSSwitchUsage(c.Err)
				return exitUsage
			}
			i++
			continue
		}
		if strings.HasPrefix(arg, "--cmux=") {
			continue
		}
		filtered = append(filtered, arg)
	}
	return c.runWSOpen(filtered)
}
