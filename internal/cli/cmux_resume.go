package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	appcmuxsession "github.com/tasuku43/kra/internal/app/cmuxsession"
	sessionstore "github.com/tasuku43/kra/internal/cmuxsession"
	"github.com/tasuku43/kra/internal/infra/paths"
)

func (c *CLI) runCMUXResume(args []string) int {
	outputFormat := "human"
	workspaceID := ""
	latest := false
	strict := false
	noBrowser := false

	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSResumeUsage(c.Out)
			return exitOK
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printWSResumeUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--workspace":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--workspace requires a value")
				c.printWSResumeUsage(c.Err)
				return exitUsage
			}
			workspaceID = strings.TrimSpace(args[1])
			args = args[2:]
		case "--latest":
			latest = true
			args = args[1:]
		case "--strict":
			strict = true
			args = args[1:]
		case "--no-browser":
			noBrowser = true
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
			fmt.Fprintf(c.Err, "unknown flag for ws resume: %q\n", args[0])
			c.printWSResumeUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printWSResumeUsage(c.Err)
		return exitUsage
	}
	if len(args) > 0 {
		fmt.Fprintf(c.Err, "unexpected args for ws resume: %q\n", strings.Join(args, " "))
		c.printWSResumeUsage(c.Err)
		return exitUsage
	}
	if workspaceID != "" {
		if err := validateWorkspaceID(workspaceID); err != nil {
			return c.writeWSResumeError(outputFormat, "invalid_argument", workspaceID, fmt.Sprintf("invalid workspace id: %v", err), exitUsage)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeWSResumeError(outputFormat, "internal_error", workspaceID, fmt.Sprintf("get working dir: %v", err), exitError)
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeWSResumeError(outputFormat, "internal_error", workspaceID, fmt.Sprintf("resolve KRA_ROOT: %v", err), exitError)
	}
	if workspaceID == "" {
		if outputFormat == "json" {
			return c.writeWSResumeError(outputFormat, "invalid_argument", "", "ws resume requires explicit workspace in --format json mode", exitUsage)
		}
		id, selErr := c.selectWorkspaceIDByStatus(root, "active", "resume")
		if selErr != nil {
			return c.writeWSResumeError(outputFormat, "workspace_not_found", "", selErr.Error(), exitError)
		}
		workspaceID = id
	}
	if outputFormat == "json" && !latest {
		return c.writeWSResumeError(outputFormat, "invalid_argument", workspaceID, "ws resume --format json requires --latest", exitUsage)
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
	sessions, code, msg := svc.ListSessions(root, workspaceID)
	if code != "" {
		return c.writeWSResumeError(outputFormat, code, workspaceID, msg, exitError)
	}
	if len(sessions) == 0 {
		return c.writeWSResumeError(outputFormat, "session_not_found", workspaceID, "no saved sessions found", exitError)
	}
	sessionID, selCode, selMsg := c.resolveResumeSessionID(outputFormat, workspaceID, latest, sessions)
	if selCode != "" {
		exitCode := exitError
		if selCode == "invalid_argument" {
			exitCode = exitUsage
		}
		return c.writeWSResumeError(outputFormat, selCode, workspaceID, selMsg, exitCode)
	}
	resumeResult, rcode, rmsg := svc.Resume(context.Background(), appcmuxsession.ResumeRequest{
		Root:        root,
		WorkspaceID: workspaceID,
		SessionID:   sessionID,
		Strict:      strict,
		SkipBrowser: noBrowser,
	})
	if rcode != "" {
		return c.writeWSResumeResultOrError(outputFormat, workspaceID, resumeResult, rcode, rmsg, exitError)
	}
	return c.writeWSResumeResultOrError(outputFormat, workspaceID, resumeResult, "", "", exitOK)
}

func (c *CLI) resolveResumeSessionID(format string, workspaceID string, latest bool, sessions []sessionstore.Entry) (string, string, string) {
	if latest {
		return strings.TrimSpace(sessions[0].SessionID), "", ""
	}
	if format == "json" {
		return "", "invalid_argument", "ws resume --format json requires --latest"
	}
	items := make([]workspaceSelectorCandidate, 0, len(sessions))
	for _, entry := range sessions {
		title := strings.TrimSpace(entry.Label)
		if title == "" {
			title = strings.TrimSpace(entry.SessionID)
		}
		desc := strings.TrimSpace(entry.CreatedAt)
		if entry.PaneCount > 0 || entry.SurfaceCount > 0 {
			desc = fmt.Sprintf("%s  (%d panes / %d surfaces)", desc, entry.PaneCount, entry.SurfaceCount)
		}
		items = append(items, workspaceSelectorCandidate{
			ID:          strings.TrimSpace(entry.SessionID),
			Title:       title,
			Description: strings.TrimSpace(desc),
		})
	}
	title := fmt.Sprintf("Sessions:\n%s%s: %s", uiIndent, "workspace", workspaceID)
	selected, err := c.promptWorkspaceSelectorWithOptionsAndMode("active", "resume", title, "session", items, true)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "tty") {
			return "", "invalid_argument", "interactive session selection requires a TTY"
		}
		return "", "session_not_found", err.Error()
	}
	if len(selected) != 1 {
		return "", "session_not_found", "resume requires exactly one session selected"
	}
	return strings.TrimSpace(selected[0]), "", ""
}

func (c *CLI) writeWSResumeResultOrError(format string, workspaceID string, result appcmuxsession.ResumeResult, code string, message string, exitCode int) int {
	warnings := make([]map[string]any, 0, len(result.Warnings))
	for _, w := range result.Warnings {
		warnings = append(warnings, map[string]any{
			"code":    w.Code,
			"message": w.Message,
		})
	}
	if format == "json" {
		payload := cliJSONResponse{
			OK:          code == "",
			Action:      "ws.resume",
			WorkspaceID: workspaceID,
			Result: map[string]any{
				"session_id":    result.SessionID,
				"session_label": result.SessionLabel,
				"resumed_at":    result.ResumedAt,
				"restored": map[string]any{
					"workspace_selected": result.WorkspaceSelect,
					"focus_restored":     result.FocusRestored,
					"browser_restored":   result.BrowserRestored,
				},
			},
			Warnings: warnings,
		}
		if code != "" {
			payload.Error = &cliJSONError{
				Code:    code,
				Message: message,
			}
		}
		_ = writeCLIJSON(c.Out, payload)
		return exitCode
	}
	if code != "" {
		return c.writeWSResumeError(format, code, workspaceID, message, exitCode)
	}
	useColor := writerSupportsColor(c.Out)
	body := []string{
		fmt.Sprintf("%s%s", uiIndent, styleSuccess("Resumed 1 session", useColor)),
		fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("workspace", useColor), workspaceID),
		fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("session", useColor), result.SessionID),
	}
	if strings.TrimSpace(result.SessionLabel) != "" {
		body = append(body, fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("label", useColor), result.SessionLabel))
	}
	body = append(body,
		fmt.Sprintf("%s%s %s: workspace=%t focus=%t browser=%t", uiIndent, styleMuted("•", useColor), styleMuted("restored", useColor), result.WorkspaceSelect, result.FocusRestored, result.BrowserRestored),
	)
	if len(result.Warnings) > 0 {
		body = append(body, fmt.Sprintf("%s%s %s", uiIndent, styleWarn("warning:", useColor), fmt.Sprintf("%d item(s) were partially restored", len(result.Warnings))))
	}
	printSection(c.Out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
	return exitOK
}

func (c *CLI) writeWSResumeError(format string, code string, workspaceID string, message string, exitCode int) int {
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "ws.resume",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitCode
	}
	if workspaceID != "" {
		fmt.Fprintf(c.Err, "ws resume (%s): %s\n", workspaceID, message)
	} else {
		fmt.Fprintf(c.Err, "ws resume: %s\n", message)
	}
	return exitCode
}
