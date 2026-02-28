package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	appcmux "github.com/tasuku43/kra/internal/app/cmux"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
	"github.com/tasuku43/kra/internal/infra/paths"
)

type cmuxOpenClient interface {
	Capabilities(ctx context.Context) (cmuxctl.Capabilities, error)
	CreateWorkspaceWithCommand(ctx context.Context, command string) (string, error)
	RenameWorkspace(ctx context.Context, workspace string, title string) error
	SelectWorkspace(ctx context.Context, workspace string) error
	Identify(ctx context.Context, workspace string, surface string) (map[string]any, error)
}

var newCMUXOpenClient = func() cmuxOpenClient { return cmuxctl.NewClient() }
var newCMUXMapStore = func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) }

func (c *CLI) runCMUXOpen(args []string) int {
	outputFormat := "human"
	multi := false
	concurrency := 1
	targetIDs := make([]string, 0, 4)
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printCMUXOpenUsage(c.Out)
			return exitOK
		case "--multi":
			multi = true
			args = args[1:]
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printCMUXOpenUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--concurrency":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--concurrency requires a value")
				c.printCMUXOpenUsage(c.Err)
				return exitUsage
			}
			n, perr := parseIntArg(strings.TrimSpace(args[1]), "--concurrency")
			if perr != nil {
				fmt.Fprintln(c.Err, perr.Error())
				c.printCMUXOpenUsage(c.Err)
				return exitUsage
			}
			concurrency = n
			args = args[2:]
		case "--workspace":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--workspace requires a value")
				c.printCMUXOpenUsage(c.Err)
				return exitUsage
			}
			targetIDs = append(targetIDs, strings.TrimSpace(args[1]))
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--workspace=") {
				targetIDs = append(targetIDs, strings.TrimSpace(strings.TrimPrefix(args[0], "--workspace=")))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--concurrency=") {
				n, perr := parseIntArg(strings.TrimSpace(strings.TrimPrefix(args[0], "--concurrency=")), "--concurrency")
				if perr != nil {
					fmt.Fprintln(c.Err, perr.Error())
					c.printCMUXOpenUsage(c.Err)
					return exitUsage
				}
				concurrency = n
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for cmux open: %q\n", args[0])
			c.printCMUXOpenUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printCMUXOpenUsage(c.Err)
		return exitUsage
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for cmux open: %q\n", strings.Join(args[1:], " "))
		c.printCMUXOpenUsage(c.Err)
		return exitUsage
	}
	if len(args) == 1 {
		targetIDs = append(targetIDs, strings.TrimSpace(args[0]))
	}
	targetIDs = dedupeNonEmpty(targetIDs)
	if concurrency < 1 {
		return c.writeCMUXOpenError(outputFormat, "invalid_argument", "", "--concurrency must be >= 1", exitUsage)
	}
	if concurrency > 1 && !multi {
		return c.writeCMUXOpenError(outputFormat, "invalid_argument", "", "--concurrency requires --multi", exitUsage)
	}
	if !multi && len(targetIDs) > 1 {
		return c.writeCMUXOpenError(outputFormat, "invalid_argument", "", "multiple targets require --multi", exitUsage)
	}
	if multi && len(targetIDs) == 0 && outputFormat == "json" {
		return c.writeCMUXOpenError(outputFormat, "non_interactive_selection_required", "", "open --multi requires explicit targets in --format json mode", exitUsage)
	}
	for _, workspaceID := range targetIDs {
		if err := validateWorkspaceID(workspaceID); err != nil {
			return c.writeCMUXOpenError(outputFormat, "invalid_argument", workspaceID, fmt.Sprintf("invalid workspace id: %v", err), exitUsage)
		}
	}
	workspaceHint := ""
	if len(targetIDs) == 1 {
		workspaceHint = targetIDs[0]
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeCMUXOpenError(outputFormat, "internal_error", workspaceHint, fmt.Sprintf("get working dir: %v", err), exitError)
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeCMUXOpenError(outputFormat, "internal_error", workspaceHint, fmt.Sprintf("resolve KRA_ROOT: %v", err), exitError)
	}

	if len(targetIDs) == 0 {
		if outputFormat == "json" {
			return c.writeCMUXOpenError(outputFormat, "invalid_argument", "", "workspace id is required in --format json mode", exitUsage)
		}
		selectedIDs, selErr := c.selectWorkspacesForCMUXOpen(root, multi)
		if selErr != nil {
			return c.writeCMUXOpenError(outputFormat, "workspace_not_found", "", selErr.Error(), exitError)
		}
		targetIDs = selectedIDs
	}

	targets := make([]appcmux.OpenTarget, 0, len(targetIDs))
	preFailures := make([]cmuxOpenFailure, 0)
	for _, id := range targetIDs {
		target, code, msg := resolveCMUXOpenTarget(root, id)
		if code != "" {
			if multi && concurrency > 1 {
				preFailures = append(preFailures, cmuxOpenFailure{
					WorkspaceID: id,
					Code:        code,
					Message:     msg,
				})
				continue
			}
			return c.writeCMUXOpenError(outputFormat, code, id, msg, exitError)
		}
		targets = append(targets, target)
	}

	svc := appcmux.NewService(func() appcmux.Client {
		return cmuxOpenClientAdapter{inner: newCMUXOpenClient()}
	}, newCMUXMapStore)
	openResult, code, msg := svc.Open(context.Background(), root, targets, concurrency, multi)
	if code != "" {
		return c.writeCMUXOpenError(outputFormat, code, workspaceHint, msg, exitError)
	}
	results := make([]cmuxOpenResult, 0, len(openResult.Results))
	for _, r := range openResult.Results {
		results = append(results, cmuxOpenResult{
			WorkspaceID:     r.WorkspaceID,
			WorkspacePath:   r.WorkspacePath,
			CMUXWorkspaceID: r.CMUXWorkspaceID,
			Ordinal:         r.Ordinal,
			Title:           r.Title,
			ReusedExisting:  r.ReusedExisting,
		})
	}
	failures := make([]cmuxOpenFailure, 0, len(openResult.Failures))
	for _, f := range openResult.Failures {
		failures = append(failures, cmuxOpenFailure{
			WorkspaceID: f.WorkspaceID,
			Code:        f.Code,
			Message:     f.Message,
		})
	}
	failures = append(failures, preFailures...)
	return c.writeCMUXOpenResult(outputFormat, multi, results, failures)
}

type cmuxOpenResult struct {
	WorkspaceID     string
	WorkspacePath   string
	CMUXWorkspaceID string
	Ordinal         int
	Title           string
	ReusedExisting  bool
}

type cmuxOpenFailure struct {
	WorkspaceID string
	Code        string
	Message     string
}

func resolveCMUXOpenTarget(root string, workspaceID string) (appcmux.OpenTarget, string, string) {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	fi, err := os.Stat(wsPath)
	if err != nil || !fi.IsDir() {
		return appcmux.OpenTarget{}, "workspace_not_found", fmt.Sprintf("workspace not found: %s", workspaceID)
	}
	title := ""
	if meta, err := loadWorkspaceMetaFile(wsPath); err == nil {
		title = strings.TrimSpace(meta.Workspace.Title)
	}
	return appcmux.OpenTarget{
		WorkspaceID:   workspaceID,
		WorkspacePath: wsPath,
		Title:         title,
	}, "", ""
}

func (c *CLI) writeCMUXOpenResult(format string, multi bool, results []cmuxOpenResult, failures []cmuxOpenFailure) int {
	if format == "json" {
		if !multi && len(results) == 1 && len(failures) == 0 {
			result := results[0]
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          true,
				Action:      "cmux.open",
				WorkspaceID: result.WorkspaceID,
				Result: map[string]any{
					"kra_workspace_id":   result.WorkspaceID,
					"kra_workspace_path": result.WorkspacePath,
					"cmux_workspace_id":  result.CMUXWorkspaceID,
					"ordinal":            result.Ordinal,
					"title":              result.Title,
					"cwd_synced":         true,
					"reused_existing":    result.ReusedExisting,
				},
			})
			return exitOK
		}
		successes := make([]map[string]any, 0, len(results))
		for _, result := range results {
			successes = append(successes, map[string]any{
				"kra_workspace_id":   result.WorkspaceID,
				"kra_workspace_path": result.WorkspacePath,
				"cmux_workspace_id":  result.CMUXWorkspaceID,
				"ordinal":            result.Ordinal,
				"title":              result.Title,
				"cwd_synced":         true,
				"reused_existing":    result.ReusedExisting,
			})
		}
		failureItems := make([]map[string]any, 0, len(failures))
		for _, fail := range failures {
			failureItems = append(failureItems, map[string]any{
				"kra_workspace_id": fail.WorkspaceID,
				"code":             fail.Code,
				"message":          fail.Message,
			})
		}
		payload := cliJSONResponse{
			OK:     len(failures) == 0,
			Action: "cmux.open",
			Result: map[string]any{
				"count":     len(successes) + len(failureItems),
				"succeeded": len(successes),
				"failed":    len(failureItems),
				"items":     successes,
				"failures":  failureItems,
			},
		}
		if len(failures) > 0 {
			payload.Error = &cliJSONError{
				Code:    "partial_failure",
				Message: "some workspaces failed to open",
			}
			_ = writeCLIJSON(c.Out, payload)
			return exitError
		}
		_ = writeCLIJSON(c.Out, payload)
		return exitOK
	}

	if !multi && len(results) == 1 && len(failures) == 0 {
		result := results[0]
		useColor := writerSupportsColor(c.Out)
		body := []string{
			fmt.Sprintf("%s%s", uiIndent, styleSuccess("Opened 1 / 1", useColor)),
			fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("mode", useColor), map[bool]string{true: "switched", false: "created"}[result.ReusedExisting]),
			fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleAccent("kra", useColor), result.WorkspaceID),
			fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleAccent("cmux", useColor), result.CMUXWorkspaceID),
			fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("title", useColor), result.Title),
			fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("cwd", useColor), result.WorkspacePath),
		}
		printSection(c.Out, renderResultTitle(useColor), body, sectionRenderOptions{
			blankAfterHeading: false,
			trailingBlank:     true,
		})
		return exitOK
	}
	useColor := writerSupportsColor(c.Out)
	body := []string{
		fmt.Sprintf("%s%s %d / %d", uiIndent, styleSuccess("Opened", useColor), len(results), len(results)+len(failures)),
	}
	sort.Slice(results, func(i, j int) bool { return results[i].WorkspaceID < results[j].WorkspaceID })
	for _, result := range results {
		body = append(body, fmt.Sprintf("%s%s %s => %s", uiIndent, styleSuccess("✔", useColor), result.WorkspaceID, result.CMUXWorkspaceID))
		body = append(body, fmt.Sprintf("%s%s %s", uiIndent+uiIndent, styleMuted("mode:", useColor), map[bool]string{true: "switched", false: "created"}[result.ReusedExisting]))
		body = append(body, fmt.Sprintf("%s%s %s", uiIndent+uiIndent, styleMuted("title:", useColor), result.Title))
		body = append(body, fmt.Sprintf("%s%s %s", uiIndent+uiIndent, styleMuted("cwd:", useColor), result.WorkspacePath))
	}
	if len(failures) > 0 {
		body = append(body, fmt.Sprintf("%s%s %d", uiIndent, styleWarn("failed:", useColor), len(failures)))
		for _, fail := range failures {
			body = append(body, fmt.Sprintf("%s%s %s (%s)", uiIndent, styleWarn("•", useColor), fail.WorkspaceID, fail.Code))
		}
	}
	printSection(c.Out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
	for _, fail := range failures {
		fmt.Fprintf(c.Err, "cmux open (%s): %s\n", fail.WorkspaceID, fail.Message)
	}
	if len(failures) > 0 {
		return exitError
	}
	return exitOK
}

func parseIntArg(raw string, name string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%s requires a value", name)
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("%s must be an integer", name)
		}
		n = n*10 + int(r-'0')
	}
	if n < 1 {
		return 0, fmt.Errorf("%s must be >= 1", name)
	}
	return n, nil
}

func dedupeNonEmpty(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (c *CLI) selectWorkspacesForCMUXOpen(root string, multi bool) ([]string, error) {
	candidates, err := listWorkspaceCandidatesByStatus(context.Background(), root, "active")
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no active workspaces available")
	}
	if multi {
		ids, err := c.promptWorkspaceSelector("active", "open", candidates)
		if err != nil {
			return nil, err
		}
		return dedupeNonEmpty(ids), nil
	}
	ids, err := c.promptWorkspaceSelectorSingle("active", "open", candidates)
	if err != nil {
		return nil, err
	}
	if len(ids) != 1 {
		return nil, fmt.Errorf("cmux open requires exactly one workspace selected")
	}
	return []string{strings.TrimSpace(ids[0])}, nil
}

type cmuxOpenClientAdapter struct {
	inner cmuxOpenClient
}

func (a cmuxOpenClientAdapter) Capabilities(ctx context.Context) (cmuxctl.Capabilities, error) {
	return a.inner.Capabilities(ctx)
}

func (a cmuxOpenClientAdapter) CreateWorkspaceWithCommand(ctx context.Context, command string) (string, error) {
	return a.inner.CreateWorkspaceWithCommand(ctx, command)
}

func (a cmuxOpenClientAdapter) RenameWorkspace(ctx context.Context, workspace string, title string) error {
	return a.inner.RenameWorkspace(ctx, workspace, title)
}

func (a cmuxOpenClientAdapter) SelectWorkspace(ctx context.Context, workspace string) error {
	return a.inner.SelectWorkspace(ctx, workspace)
}

func (a cmuxOpenClientAdapter) ListWorkspaces(context.Context) ([]cmuxctl.Workspace, error) {
	return nil, fmt.Errorf("unsupported")
}

func (a cmuxOpenClientAdapter) Identify(ctx context.Context, workspace string, surface string) (map[string]any, error) {
	return a.inner.Identify(ctx, workspace, surface)
}

func (c *CLI) writeCMUXOpenError(format string, code string, workspaceID string, message string, exitCode int) int {
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "cmux.open",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitCode
	}
	if workspaceID != "" {
		fmt.Fprintf(c.Err, "cmux open (%s): %s\n", workspaceID, message)
	} else {
		fmt.Fprintf(c.Err, "cmux open: %s\n", message)
	}
	return exitCode
}
