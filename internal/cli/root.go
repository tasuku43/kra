package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	appcmux "github.com/tasuku43/kra/internal/app/cmux"
	"github.com/tasuku43/kra/internal/infra/paths"
)

const rootCMUXMappingID = "KRA_ROOT"

func (c *CLI) runRoot(args []string) int {
	if len(args) == 0 {
		c.printRootCommandUsage(c.Err)
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printRootCommandUsage(c.Out)
		return exitOK
	case "current":
		return c.runRootCurrent(args[1:])
	case "open":
		return c.runRootOpen(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"root"}, args[0]), " "))
		c.printRootCommandUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runRootCurrent(args []string) int {
	outputFormat := "human"
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printRootCommandUsage(c.Out)
			return exitOK
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printRootCommandUsage(c.Err)
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
		c.printRootCommandUsage(c.Err)
		return exitUsage
	}
	if len(rest) > 0 {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "root.current",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: fmt.Sprintf("unexpected args for root current: %q", strings.Join(rest, " ")),
				},
			})
			return exitUsage
		}
		fmt.Fprintf(c.Err, "unexpected args for root current: %q\n", strings.Join(rest, " "))
		c.printRootCommandUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeRootRuntimeError(outputFormat, "root.current", "internal_error", fmt.Sprintf("get working dir: %v", err))
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeRootRuntimeError(outputFormat, "root.current", "not_found", fmt.Sprintf("resolve KRA_ROOT: %v", err))
	}
	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "root.current",
			Result: map[string]any{
				"root": root,
			},
		})
		return exitOK
	}
	fmt.Fprintln(c.Out, root)
	return exitOK
}

func (c *CLI) runRootOpen(args []string) int {
	outputFormat := "human"
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printRootCommandUsage(c.Out)
			return exitOK
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printRootCommandUsage(c.Err)
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
		c.printRootCommandUsage(c.Err)
		return exitUsage
	}
	if len(rest) > 0 {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "root.open",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: fmt.Sprintf("unexpected args for root open: %q", strings.Join(rest, " ")),
				},
			})
			return exitUsage
		}
		fmt.Fprintf(c.Err, "unexpected args for root open: %q\n", strings.Join(rest, " "))
		c.printRootCommandUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeRootRuntimeError(outputFormat, "root.open", "internal_error", fmt.Sprintf("get working dir: %v", err))
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeRootRuntimeError(outputFormat, "root.open", "not_found", fmt.Sprintf("resolve KRA_ROOT: %v", err))
	}

	target := appcmux.OpenTarget{
		WorkspaceID:   rootCMUXMappingID,
		WorkspacePath: root,
		Title:         "KRA_ROOT",
		StatusText:    "kra:root",
	}
	svc := appcmux.NewService(func() appcmux.Client {
		return wsOpenClientAdapter{inner: newCMUXOpenClient()}
	}, newCMUXMapStore)
	openResult, code, msg := svc.Open(context.Background(), root, []appcmux.OpenTarget{target}, 1, false)
	if code != "" {
		if code == "cmux_capability_missing" {
			return c.writeRootOpenCDFallback(outputFormat, root, msg)
		}
		return c.writeRootRuntimeError(outputFormat, "root.open", code, msg)
	}
	if len(openResult.Results) != 1 {
		return c.writeRootRuntimeError(outputFormat, "root.open", "internal_error", "unexpected open result")
	}
	item := openResult.Results[0]

	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "root.open",
			Result: map[string]any{
				"root":              root,
				"mode":              "cmux",
				"cmux_workspace_id": item.CMUXWorkspaceID,
				"reused_existing":   item.ReusedExisting,
			},
		})
		return exitOK
	}
	useColor := writerSupportsColor(c.Out)
	printResultSection(
		c.Out,
		useColor,
		styleSuccess("Opened 1 / 1", useColor),
		fmt.Sprintf("%s %s", styleSuccess("✔", useColor), root),
		styleMuted(fmt.Sprintf("cmux_workspace_id: %s", item.CMUXWorkspaceID), useColor),
	)
	return exitOK
}

func (c *CLI) writeRootOpenCDFallback(format string, root string, reason string) int {
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		trimmedReason = "workspace runtime is not available"
	}
	if err := emitShellActionCD(root); err != nil {
		return c.writeRootRuntimeError(format, "root.open", "internal_error", fmt.Sprintf("write shell action: %v", err))
	}
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "root.open",
			Result: map[string]any{
				"root":              root,
				"mode":              "fallback-cd",
				"cwd_synced":        true,
				"runtime_available": false,
				"fallback_reason":   trimmedReason,
			},
		})
		return exitOK
	}
	useColor := writerSupportsColor(c.Out)
	printResultSection(
		c.Out,
		useColor,
		styleSuccess("Opened 1 / 1", useColor),
		fmt.Sprintf("%s %s", styleSuccess("✔", useColor), root),
		styleMuted("mode: fallback-cd", useColor),
		styleMuted(fmt.Sprintf("note: %s", trimmedReason), useColor),
	)
	return exitOK
}

func (c *CLI) writeRootRuntimeError(format string, action string, code string, message string) int {
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: action,
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitError
	}
	fmt.Fprintf(c.Err, "root: %s\n", message)
	return exitError
}
