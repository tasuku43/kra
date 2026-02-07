package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	exitOK             = 0
	exitNotImplemented = 1
	exitUsage          = 2
	exitError          = 3
)

type CLI struct {
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	Version string

	inReader *bufio.Reader
	Debug    bool

	debugSession debugSession
}

func New(out io.Writer, err io.Writer) *CLI {
	return &CLI{
		In:      os.Stdin,
		Out:     out,
		Err:     err,
		Version: "dev",
	}
}

func (c *CLI) Run(args []string) int {
	c.inReader = bufio.NewReader(c.In)
	c.Debug = false
	args = c.consumeGlobalDebugFlag(args)
	defer c.closeDebugLog()

	if len(args) == 0 {
		c.printRootUsage(c.Err)
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printRootUsage(c.Out)
		return exitOK
	case "version":
		fmt.Fprintln(c.Out, c.Version)
		return exitOK
	case "init":
		return c.runInit(args[1:])
	case "ws":
		return c.runWS(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", args[0])
		c.printRootUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runWS(args []string) int {
	if len(args) == 0 {
		c.printWSUsage(c.Err)
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printWSUsage(c.Out)
		return exitOK
	case "create":
		return c.runWSCreate(args[1:])
	case "list":
		return c.runWSList(args[1:])
	case "add-repo":
		return c.runWSAddRepo(args[1:])
	case "close":
		return c.runWSClose(args[1:])
	case "reopen":
		return c.runWSReopen(args[1:])
	case "purge":
		return c.runWSPurge(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"ws"}, args[0]), " "))
		c.printWSUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) notImplemented(name string) int {
	fmt.Fprintf(c.Err, "not implemented: %s\n", name)
	return exitNotImplemented
}

func (c *CLI) consumeGlobalDebugFlag(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--debug" {
			c.Debug = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}
