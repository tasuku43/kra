package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	appcmux "github.com/tasuku43/kra/internal/app/cmux"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
	"github.com/tasuku43/kra/internal/infra/paths"
)

type cmuxStatusClient interface {
	ListWorkspaces(ctx context.Context) ([]cmuxctl.Workspace, error)
}

var newCMUXStatusClient = func() cmuxStatusClient { return cmuxctl.NewClient() }

func (c *CLI) runCMUXStatus(args []string) int {
	outputFormat := "human"
	workspaceID := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printCMUXStatusUsage(c.Out)
			return exitOK
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printCMUXStatusUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--workspace":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--workspace requires a value")
				c.printCMUXStatusUsage(c.Err)
				return exitUsage
			}
			workspaceID = strings.TrimSpace(args[1])
			args = args[2:]
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
			fmt.Fprintf(c.Err, "unknown flag for cmux status: %q\n", args[0])
			c.printCMUXStatusUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printCMUXStatusUsage(c.Err)
		return exitUsage
	}
	if len(args) > 0 {
		fmt.Fprintf(c.Err, "unexpected args for cmux status: %q\n", strings.Join(args, " "))
		c.printCMUXStatusUsage(c.Err)
		return exitUsage
	}
	if workspaceID != "" {
		if err := validateWorkspaceID(workspaceID); err != nil {
			return c.writeCMUXSimpleError("cmux.status", outputFormat, "invalid_argument", workspaceID, fmt.Sprintf("invalid workspace id: %v", err), exitUsage)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeCMUXSimpleError("cmux.status", outputFormat, "internal_error", workspaceID, fmt.Sprintf("get working dir: %v", err), exitError)
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeCMUXSimpleError("cmux.status", outputFormat, "internal_error", workspaceID, fmt.Sprintf("resolve KRA_ROOT: %v", err), exitError)
	}
	svc := appcmux.NewService(func() appcmux.Client {
		return cmuxStatusClientAdapter{inner: newCMUXStatusClient()}
	}, newCMUXMapStore)
	statusResult, code, msg := svc.Status(context.Background(), root, workspaceID)
	if code != "" {
		return c.writeCMUXSimpleError("cmux.status", outputFormat, code, workspaceID, msg, exitError)
	}

	type row struct {
		WorkspaceID string `json:"workspace_id"`
		CMUXID      string `json:"cmux_workspace_id"`
		Ordinal     int    `json:"ordinal"`
		Title       string `json:"title"`
		Exists      bool   `json:"exists"`
	}
	rows := make([]row, 0, len(statusResult.Rows))
	for _, r := range statusResult.Rows {
		rows = append(rows, row{
			WorkspaceID: r.WorkspaceID,
			CMUXID:      r.CMUXID,
			Ordinal:     r.Ordinal,
			Title:       r.Title,
			Exists:      r.Exists,
		})
	}

	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "cmux.status",
			WorkspaceID: workspaceID,
			Result: map[string]any{
				"items": rows,
			},
		})
		return exitOK
	}

	if len(rows) == 0 {
		fmt.Fprintln(c.Out, "no cmux mappings")
		return exitOK
	}
	current := ""
	for _, r := range rows {
		if r.WorkspaceID != current {
			current = r.WorkspaceID
			fmt.Fprintf(c.Out, "%s:\n", current)
		}
		status := "missing"
		if r.Exists {
			status = "ok"
		}
		fmt.Fprintf(c.Out, "  [%d] %s  %s  (%s)\n", r.Ordinal, r.CMUXID, r.Title, status)
	}
	return exitOK
}

type cmuxStatusClientAdapter struct {
	inner cmuxStatusClient
}

func (a cmuxStatusClientAdapter) Capabilities(context.Context) (cmuxctl.Capabilities, error) {
	return cmuxctl.Capabilities{}, fmt.Errorf("unsupported")
}
func (a cmuxStatusClientAdapter) CreateWorkspaceWithCommand(context.Context, string) (string, error) {
	return "", fmt.Errorf("unsupported")
}
func (a cmuxStatusClientAdapter) RenameWorkspace(context.Context, string, string) error {
	return fmt.Errorf("unsupported")
}
func (a cmuxStatusClientAdapter) SelectWorkspace(context.Context, string) error {
	return fmt.Errorf("unsupported")
}
func (a cmuxStatusClientAdapter) ListWorkspaces(ctx context.Context) ([]cmuxctl.Workspace, error) {
	return a.inner.ListWorkspaces(ctx)
}
func (a cmuxStatusClientAdapter) Identify(context.Context, string, string) (map[string]any, error) {
	return nil, fmt.Errorf("unsupported")
}
