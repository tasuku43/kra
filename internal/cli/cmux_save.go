package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	appcmuxsession "github.com/tasuku43/kra/internal/app/cmuxsession"
	"github.com/tasuku43/kra/internal/infra/paths"
)

func (c *CLI) runCMUXSave(args []string) int {
	outputFormat := "human"
	workspaceID := ""
	label := ""
	noBrowserState := false

	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSSaveUsage(c.Out)
			return exitOK
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printWSSaveUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--workspace":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--workspace requires a value")
				c.printWSSaveUsage(c.Err)
				return exitUsage
			}
			workspaceID = strings.TrimSpace(args[1])
			args = args[2:]
		case "-l", "--label":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--label requires a value")
				c.printWSSaveUsage(c.Err)
				return exitUsage
			}
			label = strings.TrimSpace(args[1])
			args = args[2:]
		case "--no-browser-state":
			noBrowserState = true
			args = args[1:]
		default:
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--workspace=") {
				workspaceID = strings.TrimSpace(strings.TrimPrefix(args[0], "--workspace="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--label=") {
				label = strings.TrimSpace(strings.TrimPrefix(args[0], "--label="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "-l=") {
				label = strings.TrimSpace(strings.TrimPrefix(args[0], "-l="))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for ws save: %q\n", args[0])
			c.printWSSaveUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printWSSaveUsage(c.Err)
		return exitUsage
	}
	if len(args) > 0 {
		fmt.Fprintf(c.Err, "unexpected args for ws save: %q\n", strings.Join(args, " "))
		c.printWSSaveUsage(c.Err)
		return exitUsage
	}
	if workspaceID != "" {
		if err := validateWorkspaceID(workspaceID); err != nil {
			return c.writeWSSaveError(outputFormat, "invalid_argument", workspaceID, fmt.Sprintf("invalid workspace id: %v", err), exitUsage)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeWSSaveError(outputFormat, "internal_error", workspaceID, fmt.Sprintf("get working dir: %v", err), exitError)
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeWSSaveError(outputFormat, "internal_error", workspaceID, fmt.Sprintf("resolve KRA_ROOT: %v", err), exitError)
	}
	if workspaceID == "" {
		if outputFormat == "json" {
			return c.writeWSSaveError(outputFormat, "invalid_argument", "", "ws save requires explicit workspace in --format json mode", exitUsage)
		}
		id, selErr := c.selectWorkspaceIDByStatus(root, "active", "save")
		if selErr != nil {
			return c.writeWSSaveError(outputFormat, "workspace_not_found", "", selErr.Error(), exitError)
		}
		workspaceID = id
	}

	svc := appcmuxsession.NewService(
		func() appcmuxsession.Client {
			return cmuxSessionClientAdapter{inner: newCMUXSessionClient()}
		},
		func(root string) appcmuxsession.MappingStore {
			return newCMUXMapStore(root)
		},
		newCMUXSessionStore,
	)
	saveResult, code, msg := svc.Save(context.Background(), appcmuxsession.SaveRequest{
		Root:                root,
		WorkspaceID:         workspaceID,
		Label:               label,
		IncludeBrowserState: !noBrowserState,
	})
	if code != "" {
		return c.writeWSSaveError(outputFormat, code, workspaceID, msg, exitError)
	}
	return c.writeWSSaveResult(outputFormat, workspaceID, saveResult)
}

func (c *CLI) writeWSSaveResult(format string, workspaceID string, result appcmuxsession.SaveResult) int {
	warnings := make([]map[string]any, 0, len(result.Warnings))
	for _, w := range result.Warnings {
		warnings = append(warnings, map[string]any{
			"code":    w.Code,
			"message": w.Message,
		})
	}
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "ws.save",
			WorkspaceID: workspaceID,
			Result: map[string]any{
				"session_id":          result.SessionID,
				"label":               result.Label,
				"path":                result.Path,
				"saved_at":            result.SavedAt,
				"pane_count":          result.PaneCount,
				"surface_count":       result.SurfaceCount,
				"browser_state_saved": result.BrowserStateSaved,
			},
			Warnings: warnings,
		})
		return exitOK
	}
	useColor := writerSupportsColor(c.Out)
	body := []string{
		fmt.Sprintf("%s%s", uiIndent, styleSuccess("Saved 1 session", useColor)),
		fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("workspace", useColor), workspaceID),
		fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("session", useColor), result.SessionID),
	}
	if strings.TrimSpace(result.Label) != "" {
		body = append(body, fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("label", useColor), result.Label))
	}
	body = append(body,
		fmt.Sprintf("%s%s %s: %d panes / %d surfaces", uiIndent, styleMuted("•", useColor), styleMuted("captured", useColor), result.PaneCount, result.SurfaceCount),
	)
	if len(result.Warnings) > 0 {
		body = append(body, fmt.Sprintf("%s%s %s", uiIndent, styleWarn("warning:", useColor), fmt.Sprintf("%d item(s) were partially captured", len(result.Warnings))))
	}
	printSection(c.Out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
	return exitOK
}

func (c *CLI) writeWSSaveError(format string, code string, workspaceID string, message string, exitCode int) int {
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "ws.save",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitCode
	}
	if workspaceID != "" {
		fmt.Fprintf(c.Err, "ws save (%s): %s\n", workspaceID, message)
	} else {
		fmt.Fprintf(c.Err, "ws save: %s\n", message)
	}
	return exitCode
}
