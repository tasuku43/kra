package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/tasuku43/kra/internal/app/wscreate"
	"github.com/tasuku43/kra/internal/config"
	"github.com/tasuku43/kra/internal/infra/appports"
	"github.com/tasuku43/kra/internal/infra/paths"
)

func (c *CLI) runWSCreate(args []string) int {
	var noPrompt bool
	var jiraTicketURL string
	var idFlag string
	var titleFlag string
	outputFormat := "human"
	templateNameFlag := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSCreateUsage(c.Out)
			return exitOK
		case "--no-prompt":
			noPrompt = true
			args = args[1:]
		case "--jira":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--jira requires a ticket URL")
				c.printWSCreateUsage(c.Err)
				return exitUsage
			}
			jiraTicketURL = strings.TrimSpace(args[1])
			args = args[2:]
		case "--id":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--id requires a value")
				c.printWSCreateUsage(c.Err)
				return exitUsage
			}
			idFlag = strings.TrimSpace(args[1])
			args = args[2:]
		case "--title":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--title requires a value")
				c.printWSCreateUsage(c.Err)
				return exitUsage
			}
			titleFlag = args[1]
			args = args[2:]
		case "--template":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--template requires a value")
				c.printWSCreateUsage(c.Err)
				return exitUsage
			}
			templateNameFlag = strings.TrimSpace(args[1])
			args = args[2:]
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printWSCreateUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for ws create: %q\n", args[0])
			c.printWSCreateUsage(c.Err)
			return exitUsage
		}
	}

	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printWSCreateUsage(c.Err)
		return exitUsage
	}

	writeUsageError := func(message string) int {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "ws.create",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: message,
				},
			})
			return exitUsage
		}
		fmt.Fprintln(c.Err, message)
		c.printWSCreateUsage(c.Err)
		return exitUsage
	}
	writeRuntimeError := func(code string, message string) int {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "ws.create",
				Error: &cliJSONError{
					Code:    code,
					Message: message,
				},
			})
			return exitError
		}
		fmt.Fprintln(c.Err, message)
		return exitError
	}

	if jiraTicketURL != "" {
		if idFlag != "" || titleFlag != "" {
			return writeUsageError("--jira cannot be combined with --id or --title")
		}
		if len(args) > 0 {
			return writeUsageError(fmt.Sprintf("unexpected args for ws create: %q", strings.Join(args, " ")))
		}
	} else {
		if idFlag != "" && len(args) > 0 {
			return writeUsageError("--id cannot be combined with positional <id>")
		}
		if len(args) > 1 {
			return writeUsageError(fmt.Sprintf("unexpected args for ws create: %q", strings.Join(args[1:], " ")))
		}
		if idFlag == "" && len(args) == 0 {
			if outputFormat == "json" {
				return writeUsageError("ws create --format json requires --id <id> or positional <id>")
			}
			c.printWSCreateUsage(c.Err)
			return exitUsage
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return writeRuntimeError("internal_error", fmt.Sprintf("get working dir: %v", err))
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return writeRuntimeError("not_found", fmt.Sprintf("resolve KRA_ROOT: %v", err))
	}
	if err := c.ensureDebugLog(root, "ws-create"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws create noPrompt=%t jira=%t", noPrompt, jiraTicketURL != "")

	cfg, err := c.loadMergedConfig(root)
	if err != nil {
		return writeRuntimeError("internal_error", fmt.Sprintf("load config: %v", err))
	}

	templateName, err := c.resolveWSCreateTemplateName(cfg, templateNameFlag)
	if err != nil {
		return writeRuntimeError("internal_error", fmt.Sprintf("resolve template: %v", err))
	}

	ctx := context.Background()
	id := ""
	title := ""
	sourceURL := ""
	if jiraTicketURL != "" {
		svc := wscreate.NewService(appports.NewWSCreateJiraPortWithBaseURL(cfg.Integration.Jira.BaseURL))
		in, err := svc.ResolveJiraWorkspaceInput(ctx, jiraTicketURL)
		if err != nil {
			return writeRuntimeError("not_found", fmt.Sprintf("resolve jira issue: %v", err))
		}
		id = in.ID
		title = in.Title
		sourceURL = in.SourceURL
	} else {
		if strings.TrimSpace(idFlag) != "" {
			id = strings.TrimSpace(idFlag)
		} else if len(args) == 1 {
			id = strings.TrimSpace(args[0])
		}
		title = titleFlag
	}

	if err := validateWorkspaceID(id); err != nil {
		return writeUsageError(fmt.Sprintf("invalid workspace id: %v", err))
	}
	if err := validateWorkspaceTemplateName(templateName); err != nil {
		return writeUsageError(err.Error())
	}

	if err := c.touchStateRegistry(root); err != nil {
		return writeRuntimeError("internal_error", fmt.Sprintf("update root registry: %v", err))
	}

	if jiraTicketURL == "" && title == "" && !noPrompt && outputFormat == "human" {
		d, err := c.promptLine("title: ")
		if err != nil {
			return writeRuntimeError("internal_error", fmt.Sprintf("read title: %v", err))
		}
		title = d
	}

	wsPath, err := c.createWorkspaceAtRoot(root, id, title, sourceURL, templateName)
	if err != nil {
		return writeRuntimeError(classifyWSCreateErrorCode(err), err.Error())
	}

	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "ws.create",
			WorkspaceID: id,
			Result: map[string]any{
				"created":  1,
				"path":     wsPath,
				"template": templateName,
			},
		})
		c.debugf("ws create completed id=%s path=%s format=json", id, wsPath)
		return exitOK
	}

	useColorOut := writerSupportsColor(c.Out)
	printResultSection(
		c.Out,
		useColorOut,
		styleSuccess("Created 1 / 1", useColorOut),
		fmt.Sprintf("%s %s", styleSuccess("✔", useColorOut), id),
		styleMuted(fmt.Sprintf("path: %s", wsPath), useColorOut),
	)
	c.debugf("ws create completed id=%s path=%s", id, wsPath)
	return exitOK
}

func classifyWSCreateErrorCode(err error) string {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "already exists"),
		strings.Contains(msg, "ws --act reopen"),
		strings.Contains(msg, "cannot"):
		return "conflict"
	case strings.Contains(msg, "not found"),
		strings.Contains(msg, "no such file or directory"),
		strings.Contains(msg, "missing"):
		return "not_found"
	case strings.Contains(msg, "invalid"),
		strings.Contains(msg, "requires"):
		return "invalid_argument"
	default:
		return "internal_error"
	}
}

func (c *CLI) resolveWSCreateTemplateName(cfg config.Config, templateNameFlag string) (string, error) {
	if templateName := strings.TrimSpace(templateNameFlag); templateName != "" {
		return templateName, nil
	}
	if templateName := strings.TrimSpace(cfg.Workspace.Defaults.Template); templateName != "" {
		return templateName, nil
	}
	return defaultWorkspaceTemplateName, nil
}

func (c *CLI) promptLine(prompt string) (string, error) {
	inFile, inOK := c.In.(*os.File)
	errFile, errOK := c.Err.(*os.File)
	if inOK && errOK && isatty.IsTerminal(inFile.Fd()) && isatty.IsTerminal(errFile.Fd()) {
		return runInlineTextInput(inFile, c.Err, prompt)
	}

	if prompt != "" {
		fmt.Fprint(c.Err, prompt)
	}
	if c.inReader == nil {
		c.inReader = bufio.NewReader(c.In)
	}
	line, err := c.inReader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func validateWorkspaceID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.ContainsAny(id, "/\\") {
		return fmt.Errorf("must not contain path separators")
	}
	return nil
}
