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
	Commit  string
	Date    string

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
	var versionFlag bool
	args, versionFlag = c.consumeGlobalFlags(args)
	defer c.closeDebugLog()

	if versionFlag {
		fmt.Fprintln(c.Out, c.versionLine())
		return exitOK
	}

	if len(args) == 0 {
		c.printRootUsage(c.Err)
		return exitUsage
	}
	if shouldBootstrapGlobalConfig(args) {
		if err := ensureGlobalConfigScaffold(); err != nil {
			fmt.Fprintf(c.Err, "initialize global config: %v\n", err)
			return exitError
		}
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printRootUsage(c.Out)
		return exitOK
	case "version":
		fmt.Fprintln(c.Out, c.versionLine())
		return exitOK
	case "init":
		return c.runInit(args[1:])
	case "context":
		return c.runContext(args[1:])
	case "repo":
		return c.runRepo(args[1:])
	case "template":
		return c.runTemplate(args[1:])
	case "shell":
		return c.runShell(args[1:])
	case "bootstrap":
		return c.runBootstrap(args[1:])
	case "ws":
		return c.runWS(args[1:])
	case "doctor":
		return c.runDoctor(args[1:])
	case "agent":
		return c.runAgent(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", args[0])
		c.printRootUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runWS(args []string) int {
	if len(args) == 0 {
		return c.runWSLauncher(nil)
	}

	if strings.HasPrefix(args[0], "-") {
		hasSelect := false
		hasMulti := false
		for _, arg := range args {
			v := strings.TrimSpace(arg)
			if v == "--select" {
				hasSelect = true
			}
			if v == "--multi" {
				hasMulti = true
			}
		}
		if hasSelect && hasMulti {
			return c.runWSSelectMulti(args)
		}
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSUsage(c.Out)
			return exitOK
		default:
			return c.runWSLauncher(args)
		}
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printWSUsage(c.Out)
		return exitOK
	case "create":
		return c.runWSCreate(args[1:])
	case "import":
		return c.runWSImport(args[1:])
	case "list":
		return c.runWSList(args[1:])
	case "ls":
		return c.runWSList(args[1:])
	case "dashboard":
		return c.runWSDashboard(args[1:])
	case "insight":
		if !c.isExperimentEnabled(experimentInsightCapture) {
			fmt.Fprintf(c.Err, "ws insight is experimental (set %s=%s)\n", experimentsEnvKey, experimentInsightCapture)
			return exitUsage
		}
		return c.runWSInsight(args[1:])
	case "select":
		return c.runWSSelect(args[1:])
	case "lock":
		return c.runWSLock(args[1:])
	case "unlock":
		return c.runWSUnlock(args[1:])
	case "open":
		return c.runWSOpen(args[1:])
	case "switch":
		return c.runWSSwitch(args[1:])
	case "add-repo", "remove-repo", "close", "reopen", "purge":
		return c.runWSActionSubcommand(args[0], args[1:])
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

func (c *CLI) versionLine() string {
	version := strings.TrimSpace(c.Version)
	if version == "" {
		version = "dev"
	}
	parts := []string{version}
	if commit := strings.TrimSpace(c.Commit); commit != "" {
		parts = append(parts, commit)
	}
	if date := strings.TrimSpace(c.Date); date != "" {
		parts = append(parts, date)
	}
	return strings.Join(parts, " ")
}

func (c *CLI) consumeGlobalFlags(args []string) ([]string, bool) {
	filtered := make([]string, 0, len(args))
	versionFlag := false
	for _, arg := range args {
		switch arg {
		case "--debug":
			c.Debug = true
		case "--version":
			versionFlag = true
		default:
			filtered = append(filtered, arg)
		}
	}
	return filtered, versionFlag
}
