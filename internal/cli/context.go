package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/stateregistry"
)

func (c *CLI) runContext(args []string) int {
	if len(args) == 0 {
		c.printContextUsage(c.Err)
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printContextUsage(c.Out)
		return exitOK
	case "current":
		return c.runContextCurrent(args[1:])
	case "list":
		return c.runContextList(args[1:])
	case "use":
		return c.runContextUse(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"context"}, args[0]), " "))
		c.printContextUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runContextCurrent(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printContextUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "unexpected args for context current: %q\n", strings.Join(args, " "))
		c.printContextUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve GIONX_ROOT: %v\n", err)
		return exitError
	}
	fmt.Fprintln(c.Out, root)
	return exitOK
}

func (c *CLI) runContextList(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printContextUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "unexpected args for context list: %q\n", strings.Join(args, " "))
		c.printContextUsage(c.Err)
		return exitUsage
	}

	registryPath, err := stateregistry.Path()
	if err != nil {
		fmt.Fprintf(c.Err, "resolve state registry path: %v\n", err)
		return exitError
	}
	entries, err := stateregistry.Load(registryPath)
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if len(entries) == 0 {
		fmt.Fprintln(c.Out, "(none)")
		return exitOK
	}

	slices.SortFunc(entries, func(a, b stateregistry.Entry) int {
		if a.LastUsedAt != b.LastUsedAt {
			if a.LastUsedAt > b.LastUsedAt {
				return -1
			}
			return 1
		}
		return strings.Compare(a.RootPath, b.RootPath)
	})

	fmt.Fprintln(c.Out, "Contexts:")
	for _, e := range entries {
		last := time.Unix(e.LastUsedAt, 0).UTC().Format(time.RFC3339)
		fmt.Fprintf(c.Out, "%s%s  last_used_at=%s\n", uiIndent, e.RootPath, last)
	}
	return exitOK
}

func (c *CLI) runContextUse(args []string) int {
	if len(args) == 0 {
		c.printContextUsage(c.Err)
		return exitUsage
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for context use: %q\n", strings.Join(args[1:], " "))
		c.printContextUsage(c.Err)
		return exitUsage
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		c.printContextUsage(c.Out)
		return exitOK
	}

	root, err := resolveContextUseRoot(args[0])
	if err != nil {
		fmt.Fprintf(c.Err, "validate root: %v\n", err)
		return exitError
	}
	if err := paths.WriteCurrentContext(root); err != nil {
		fmt.Fprintf(c.Err, "write current context: %v\n", err)
		return exitError
	}
	useColorOut := writerSupportsColor(c.Out)
	printResultSection(c.Out, useColorOut, styleSuccess(fmt.Sprintf("Context set: %s", root), useColorOut))
	return exitOK
}

func resolveContextUseRoot(raw string) (string, error) {
	root, err := filepath.Abs(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)

	fi, err := os.Stat(root)
	if err == nil {
		if !fi.IsDir() {
			return "", fmt.Errorf("not a directory: %s", root)
		}
		return root, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}

	parent := filepath.Dir(root)
	pfi, err := os.Stat(parent)
	if err != nil || !pfi.IsDir() {
		if err == nil {
			return "", fmt.Errorf("parent is not a directory: %s", parent)
		}
		return "", fmt.Errorf("parent directory missing: %s", parent)
	}
	return root, nil
}
