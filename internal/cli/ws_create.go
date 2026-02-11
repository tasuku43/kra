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
	"github.com/tasuku43/gionx/internal/app/wscreate"
	"github.com/tasuku43/gionx/internal/infra/appports"
	"github.com/tasuku43/gionx/internal/infra/paths"
)

func (c *CLI) runWSCreate(args []string) int {
	var noPrompt bool
	var jiraTicketURL string
	var idFlag string
	var titleFlag string
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
		default:
			fmt.Fprintf(c.Err, "unknown flag for ws create: %q\n", args[0])
			c.printWSCreateUsage(c.Err)
			return exitUsage
		}
	}

	if jiraTicketURL != "" {
		if idFlag != "" || titleFlag != "" {
			fmt.Fprintln(c.Err, "--jira cannot be combined with --id or --title")
			c.printWSCreateUsage(c.Err)
			return exitUsage
		}
		if len(args) > 0 {
			fmt.Fprintf(c.Err, "unexpected args for ws create: %q\n", strings.Join(args, " "))
			c.printWSCreateUsage(c.Err)
			return exitUsage
		}
	} else {
		if idFlag != "" || titleFlag != "" {
			fmt.Fprintln(c.Err, "--id/--title are only supported with --jira")
			c.printWSCreateUsage(c.Err)
			return exitUsage
		}
		if len(args) == 0 {
			c.printWSCreateUsage(c.Err)
			return exitUsage
		}
		if len(args) > 1 {
			fmt.Fprintf(c.Err, "unexpected args for ws create: %q\n", strings.Join(args[1:], " "))
			c.printWSCreateUsage(c.Err)
			return exitUsage
		}
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
	if err := c.ensureDebugLog(root, "ws-create"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws create noPrompt=%t jira=%t", noPrompt, jiraTicketURL != "")

	templateName, err := c.resolveWSCreateTemplateName(root, templateNameFlag)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve template: %v\n", err)
		return exitError
	}

	ctx := context.Background()
	id := ""
	title := ""
	sourceURL := ""
	if jiraTicketURL != "" {
		svc := wscreate.NewService(appports.NewWSCreateJiraPort())
		in, err := svc.ResolveJiraWorkspaceInput(ctx, jiraTicketURL)
		if err != nil {
			fmt.Fprintf(c.Err, "resolve jira issue: %v\n", err)
			return exitError
		}
		id = in.ID
		title = in.Title
		sourceURL = in.SourceURL
	} else {
		id = args[0]
	}

	if err := validateWorkspaceID(id); err != nil {
		fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
		return exitUsage
	}
	if err := validateWorkspaceTemplateName(templateName); err != nil {
		fmt.Fprintln(c.Err, err.Error())
		return exitUsage
	}

	if err := c.touchStateRegistry(root); err != nil {
		fmt.Fprintf(c.Err, "update root registry: %v\n", err)
		return exitError
	}

	if jiraTicketURL == "" && !noPrompt {
		d, err := c.promptLine("title: ")
		if err != nil {
			fmt.Fprintf(c.Err, "read title: %v\n", err)
			return exitError
		}
		title = d
	}

	wsPath, err := c.createWorkspaceAtRoot(root, id, title, sourceURL, templateName)
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}

	useColorOut := writerSupportsColor(c.Out)
	printResultSection(
		c.Out,
		useColorOut,
		styleSuccess("Created 1 / 1", useColorOut),
		fmt.Sprintf("✔ %s", id),
		styleMuted(fmt.Sprintf("path: %s", wsPath), useColorOut),
	)
	c.debugf("ws create completed id=%s path=%s", id, wsPath)
	return exitOK
}

func (c *CLI) resolveWSCreateTemplateName(root string, templateNameFlag string) (string, error) {
	if templateName := strings.TrimSpace(templateNameFlag); templateName != "" {
		return templateName, nil
	}

	cfg, err := c.loadMergedConfig(root)
	if err != nil {
		return "", err
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
