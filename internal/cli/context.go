package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/app/contextcmd"
	"github.com/tasuku43/kra/internal/core/workspacerisk"
	"github.com/tasuku43/kra/internal/infra/appports"
	"github.com/tasuku43/kra/internal/infra/paths"
)

func writeContextJSONError(out io.Writer, action string, code string, message string) {
	_ = writeCLIJSON(out, cliJSONResponse{
		OK:     false,
		Action: action,
		Error: &cliJSONError{
			Code:    code,
			Message: message,
		},
	})
}

func contextErrorCode(err error) string {
	msg := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		return "not_found"
	case strings.Contains(msg, "already exists"):
		return "conflict"
	case strings.Contains(msg, "cannot remove current context"):
		return "conflict"
	case strings.Contains(msg, "validate root:"):
		return "invalid_argument"
	default:
		return "internal_error"
	}
}

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
	case "create":
		return c.runContextCreate(args[1:])
	case "use":
		return c.runContextUse(args[1:])
	case "rename":
		return c.runContextRename(args[1:])
	case "rm", "remove":
		return c.runContextRemove(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"context"}, args[0]), " "))
		c.printContextUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runContextCurrent(args []string) int {
	outputFormat := "human"
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printContextUsage(c.Out)
			return exitOK
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printContextUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(arg, "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
				continue
			}
			rest = append(rest, arg)
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printContextUsage(c.Err)
		return exitUsage
	}
	if len(rest) > 0 {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.current", "invalid_argument", fmt.Sprintf("unexpected args for context current: %q", strings.Join(rest, " ")))
			return exitUsage
		}
		fmt.Fprintf(c.Err, "unexpected args for context current: %q\n", strings.Join(rest, " "))
		c.printContextUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.current", "internal_error", fmt.Sprintf("get working dir: %v", err))
			return exitError
		}
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	svc := contextcmd.NewService(appports.NewContextPort(resolveContextUseRoot))
	current, err := svc.Current(wd)
	if err != nil {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.current", contextErrorCode(err), fmt.Sprintf("resolve current context: %v", err))
			return exitError
		}
		fmt.Fprintf(c.Err, "resolve current context: %v\n", err)
		return exitError
	}
	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "context.current",
			Result: map[string]any{
				"current": current,
			},
		})
		return exitOK
	}
	fmt.Fprintln(c.Out, current)
	return exitOK
}

func (c *CLI) runContextList(args []string) int {
	outputFormat := "human"
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printContextUsage(c.Out)
			return exitOK
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printContextUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(arg, "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
				continue
			}
			rest = append(rest, arg)
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printContextUsage(c.Err)
		return exitUsage
	}
	if len(rest) > 0 {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.list", "invalid_argument", fmt.Sprintf("unexpected args for context list: %q", strings.Join(rest, " ")))
			return exitUsage
		}
		fmt.Fprintf(c.Err, "unexpected args for context list: %q\n", strings.Join(rest, " "))
		c.printContextUsage(c.Err)
		return exitUsage
	}

	svc := contextcmd.NewService(appports.NewContextPort(resolveContextUseRoot))
	entries, err := svc.List()
	if err != nil {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.list", contextErrorCode(err), err.Error())
			return exitError
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	currentRoot, _, _ := paths.ReadCurrentContext()
	if outputFormat == "json" {
		items := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			items = append(items, map[string]any{
				"name":         strings.TrimSpace(e.ContextName),
				"path":         strings.TrimSpace(e.RootPath),
				"last_used_at": e.LastUsedAt,
				"current":      strings.TrimSpace(e.RootPath) != "" && strings.TrimSpace(e.RootPath) == strings.TrimSpace(currentRoot),
			})
		}
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "context.list",
			Result: map[string]any{
				"contexts": items,
			},
		})
		return exitOK
	}
	if len(entries) == 0 {
		fmt.Fprintln(c.Out, "(none)")
		return exitOK
	}

	useColorOut := writerSupportsColor(c.Out)
	fmt.Fprintln(c.Out, styleBold("Contexts:", useColorOut))

	ordered := make([]contextcmd.Entry, 0, len(entries))
	for _, e := range entries {
		if strings.TrimSpace(e.RootPath) == strings.TrimSpace(currentRoot) {
			ordered = append(ordered, e)
		}
	}
	for _, e := range entries {
		if strings.TrimSpace(e.RootPath) != strings.TrimSpace(currentRoot) {
			ordered = append(ordered, e)
		}
	}

	for _, e := range ordered {
		isCurrent := strings.TrimSpace(e.RootPath) != "" && strings.TrimSpace(e.RootPath) == strings.TrimSpace(currentRoot)
		last := time.Unix(e.LastUsedAt, 0).UTC().Format(time.RFC3339)
		name := strings.TrimSpace(e.ContextName)
		if name == "" {
			name = "(unnamed)"
		}
		prefix := "○"
		if isCurrent {
			prefix = styleAccent("●", useColorOut)
		}
		title := name
		if isCurrent {
			title = styleAccent(title, useColorOut)
		}
		currentLabel := ""
		if isCurrent {
			currentLabel = " " + styleAccent("[current]", useColorOut)
		}
		fmt.Fprintf(c.Out, "%s%s %s%s\n", uiIndent, prefix, title, currentLabel)
		fmt.Fprintf(c.Out, "%s├─ %s%s\n", uiIndent, styleMuted("path: ", useColorOut), e.RootPath)
		fmt.Fprintf(c.Out, "%s└─ %s%s\n", uiIndent, styleMuted("last used: ", useColorOut), styleMuted(last, useColorOut))
	}
	return exitOK
}

func (c *CLI) runContextUse(args []string) int {
	outputFormat := "human"
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printContextUsage(c.Out)
			return exitOK
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printContextUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(arg, "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
				continue
			}
			rest = append(rest, arg)
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printContextUsage(c.Err)
		return exitUsage
	}

	if len(rest) == 0 {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.use", "invalid_argument", "context use --format json requires <name>")
			return exitUsage
		}
		svc := contextcmd.NewService(appports.NewContextPort(resolveContextUseRoot))
		entries, err := svc.List()
		if err != nil {
			fmt.Fprintf(c.Err, "%v\n", err)
			return exitError
		}
		if len(entries) == 0 {
			fmt.Fprintln(c.Err, "no contexts available")
			return exitError
		}
		currentRoot, _, _ := paths.ReadCurrentContext()
		candidates := make([]workspaceSelectorCandidate, 0, len(entries))
		for _, e := range entries {
			name := strings.TrimSpace(e.ContextName)
			if name == "" {
				continue
			}
			title := e.RootPath
			if strings.TrimSpace(e.RootPath) == strings.TrimSpace(currentRoot) {
				title += " [current]"
			}
			candidates = append(candidates, workspaceSelectorCandidate{
				ID:    name,
				Title: title,
				Risk:  workspacerisk.WorkspaceRiskClean,
			})
		}
		if len(candidates) == 0 {
			fmt.Fprintln(c.Err, "no named contexts available")
			return exitError
		}
		selected, err := c.promptWorkspaceSelectorWithOptionsAndMode("active", "select", "Contexts:", "context", candidates, true)
		if err != nil {
			if err == errSelectorCanceled {
				fmt.Fprintln(c.Err, "aborted")
				return exitError
			}
			if strings.Contains(err.Error(), "requires a TTY") {
				fmt.Fprintln(c.Err, "context use without <name> requires a TTY")
				c.printContextUsage(c.Err)
				return exitUsage
			}
			fmt.Fprintf(c.Err, "run context selector: %v\n", err)
			return exitError
		}
		rest = []string{selected[0]}
	}
	if len(rest) > 1 {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.use", "invalid_argument", fmt.Sprintf("unexpected args for context use: %q", strings.Join(rest[1:], " ")))
			return exitUsage
		}
		fmt.Fprintf(c.Err, "unexpected args for context use: %q\n", strings.Join(rest[1:], " "))
		c.printContextUsage(c.Err)
		return exitUsage
	}
	name := strings.TrimSpace(rest[0])

	svc := contextcmd.NewService(appports.NewContextPort(resolveContextUseRoot))
	root, err := svc.Use(name)
	if err != nil {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.use", contextErrorCode(err), err.Error())
			return exitError
		}
		switch {
		case strings.HasPrefix(err.Error(), "resolve context by name:"):
			fmt.Fprintf(c.Err, "%v\n", err)
		case strings.HasPrefix(err.Error(), "context not found:"):
			fmt.Fprintf(c.Err, "%v\n", err)
		case strings.HasPrefix(err.Error(), "write current context:"):
			fmt.Fprintf(c.Err, "%v\n", err)
		default:
			fmt.Fprintf(c.Err, "run context use usecase: %v\n", err)
		}
		return exitError
	}
	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "context.use",
			Result: map[string]any{
				"context_name": name,
				"path":         root,
			},
		})
		return exitOK
	}
	useColorOut := writerSupportsColor(c.Out)
	printResultSection(c.Out, useColorOut, styleSuccess(fmt.Sprintf("Context selected: %s", name), useColorOut), styleMuted(fmt.Sprintf("path: %s", root), useColorOut))
	return exitOK
}

func (c *CLI) runContextCreate(args []string) int {
	if len(args) == 0 {
		c.printContextUsage(c.Err)
		return exitUsage
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		c.printContextUsage(c.Out)
		return exitOK
	}
	name := strings.TrimSpace(args[0])
	if name == "" {
		fmt.Fprintln(c.Err, "context name is required")
		c.printContextUsage(c.Err)
		return exitUsage
	}
	rawPath := ""
	useNow := false
	outputFormat := "human"
	rest := args[1:]
	for len(rest) > 0 {
		switch rest[0] {
		case "--path":
			if len(rest) < 2 {
				fmt.Fprintln(c.Err, "--path requires a value")
				c.printContextUsage(c.Err)
				return exitUsage
			}
			rawPath = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case "--use":
			useNow = true
			rest = rest[1:]
		case "--format":
			if len(rest) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printContextUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			if strings.HasPrefix(rest[0], "--path=") {
				rawPath = strings.TrimSpace(strings.TrimPrefix(rest[0], "--path="))
				rest = rest[1:]
				continue
			}
			if strings.HasPrefix(rest[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(rest[0], "--format="))
				rest = rest[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unexpected args for context create: %q\n", strings.Join(rest, " "))
			c.printContextUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printContextUsage(c.Err)
		return exitUsage
	}
	if rawPath == "" {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.create", "invalid_argument", "context create requires --path <path>")
			return exitUsage
		}
		fmt.Fprintln(c.Err, "context create requires --path <path>")
		c.printContextUsage(c.Err)
		return exitUsage
	}

	svc := contextcmd.NewService(appports.NewContextPort(resolveContextUseRoot))
	root, err := svc.Create(name, rawPath, useNow)
	if err != nil {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.create", contextErrorCode(err), err.Error())
			return exitError
		}
		switch {
		case strings.HasPrefix(err.Error(), "validate root:"):
			fmt.Fprintf(c.Err, "%v\n", err)
		case strings.HasPrefix(err.Error(), "write context registry:"):
			fmt.Fprintf(c.Err, "%v\n", err)
		case strings.HasPrefix(err.Error(), "write current context:"):
			fmt.Fprintf(c.Err, "%v\n", err)
		default:
			fmt.Fprintf(c.Err, "run context create usecase: %v\n", err)
		}
		return exitError
	}
	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "context.create",
			Result: map[string]any{
				"context_name": name,
				"path":         root,
				"selected":     useNow,
			},
		})
		return exitOK
	}

	useColorOut := writerSupportsColor(c.Out)
	if useNow {
		printResultSection(c.Out, useColorOut, styleSuccess(fmt.Sprintf("Context created and selected: %s", name), useColorOut), styleMuted(fmt.Sprintf("path: %s", root), useColorOut))
		return exitOK
	}
	printResultSection(c.Out, useColorOut, styleSuccess(fmt.Sprintf("Context created: %s", name), useColorOut), styleMuted(fmt.Sprintf("path: %s", root), useColorOut))
	return exitOK
}

func (c *CLI) runContextRename(args []string) int {
	outputFormat := "human"
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printContextUsage(c.Out)
			return exitOK
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printContextUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(arg, "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
				continue
			}
			rest = append(rest, arg)
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printContextUsage(c.Err)
		return exitUsage
	}
	if len(rest) != 2 {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.rename", "invalid_argument", "usage: kra context rename <old-name> <new-name>")
			return exitUsage
		}
		fmt.Fprintf(c.Err, "usage: kra context rename <old-name> <new-name>\n")
		return exitUsage
	}

	oldName := strings.TrimSpace(rest[0])
	newName := strings.TrimSpace(rest[1])
	svc := contextcmd.NewService(appports.NewContextPort(resolveContextUseRoot))
	root, err := svc.Rename(oldName, newName)
	if err != nil {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.rename", contextErrorCode(err), err.Error())
			return exitError
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "context.rename",
			Result: map[string]any{
				"old_name": oldName,
				"new_name": newName,
				"path":     root,
			},
		})
		return exitOK
	}

	useColorOut := writerSupportsColor(c.Out)
	printResultSection(c.Out, useColorOut, styleSuccess(fmt.Sprintf("Context renamed: %s -> %s", oldName, newName), useColorOut), styleMuted(fmt.Sprintf("path: %s", root), useColorOut))
	return exitOK
}

func (c *CLI) runContextRemove(args []string) int {
	outputFormat := "human"
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printContextUsage(c.Out)
			return exitOK
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printContextUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(arg, "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
				continue
			}
			rest = append(rest, arg)
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printContextUsage(c.Err)
		return exitUsage
	}

	if len(rest) == 0 {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.remove", "invalid_argument", "context rm --format json requires <name>")
			return exitUsage
		}
		svc := contextcmd.NewService(appports.NewContextPort(resolveContextUseRoot))
		entries, err := svc.List()
		if err != nil {
			fmt.Fprintf(c.Err, "%v\n", err)
			return exitError
		}
		if len(entries) == 0 {
			fmt.Fprintln(c.Err, "no contexts available")
			return exitError
		}
		currentRoot, _, _ := paths.ReadCurrentContext()
		candidates := make([]workspaceSelectorCandidate, 0, len(entries))
		for _, e := range entries {
			name := strings.TrimSpace(e.ContextName)
			if name == "" {
				continue
			}
			title := e.RootPath
			if strings.TrimSpace(e.RootPath) == strings.TrimSpace(currentRoot) {
				title += " [current]"
			}
			candidates = append(candidates, workspaceSelectorCandidate{
				ID:    name,
				Title: title,
				Risk:  workspacerisk.WorkspaceRiskClean,
			})
		}
		if len(candidates) == 0 {
			fmt.Fprintln(c.Err, "no named contexts available")
			return exitError
		}
		selected, err := c.promptWorkspaceSelectorWithOptionsAndMode("active", "remove", "Contexts:", "context", candidates, true)
		if err != nil {
			if err == errSelectorCanceled {
				fmt.Fprintln(c.Err, "aborted")
				return exitError
			}
			if strings.Contains(err.Error(), "requires a TTY") {
				fmt.Fprintln(c.Err, "context rm without <name> requires a TTY")
				c.printContextUsage(c.Err)
				return exitUsage
			}
			fmt.Fprintf(c.Err, "run context selector: %v\n", err)
			return exitError
		}
		rest = []string{selected[0]}
	}
	if len(rest) != 1 {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.remove", "invalid_argument", "usage: kra context rm <name>")
			return exitUsage
		}
		fmt.Fprintf(c.Err, "usage: kra context rm <name>\n")
		return exitUsage
	}

	name := strings.TrimSpace(rest[0])
	svc := contextcmd.NewService(appports.NewContextPort(resolveContextUseRoot))
	root, ok, err := svc.ResolveRootByName(name)
	if err != nil {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.remove", contextErrorCode(err), err.Error())
			return exitError
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if !ok {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.remove", "not_found", fmt.Sprintf("context not found: %s", name))
			return exitError
		}
		fmt.Fprintf(c.Err, "context not found: %s\n", name)
		return exitError
	}
	currentRoot, _, _ := paths.ReadCurrentContext()
	if strings.TrimSpace(root) != "" && strings.TrimSpace(root) == strings.TrimSpace(currentRoot) {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.remove", "conflict", fmt.Sprintf("cannot remove current context: %s", name))
			return exitError
		}
		fmt.Fprintf(c.Err, "cannot remove current context: %s\n", name)
		return exitError
	}
	removedRoot, err := svc.Remove(name)
	if err != nil {
		if outputFormat == "json" {
			writeContextJSONError(c.Out, "context.remove", contextErrorCode(err), err.Error())
			return exitError
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "context.remove",
			Result: map[string]any{
				"context_name": name,
				"path":         removedRoot,
			},
		})
		return exitOK
	}

	useColorOut := writerSupportsColor(c.Out)
	printResultSection(c.Out, useColorOut, styleSuccess(fmt.Sprintf("Context removed: %s", name), useColorOut), styleMuted(fmt.Sprintf("path: %s", removedRoot), useColorOut))
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
