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
	SetStatus(ctx context.Context, workspace string, label string, text string, icon string, color string) error
	Identify(ctx context.Context, workspace string, surface string) (map[string]any, error)
}

var newCMUXOpenClient = func() cmuxOpenClient { return cmuxctl.NewClient() }
var newCMUXMapStore = func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) }

func (c *CLI) runWSOpenRuntime(args []string) int {
	outputFormat := "human"
	multi := false
	concurrency := 1
	targetIDs := make([]string, 0, 4)
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSOpenUsage(c.Out)
			return exitOK
		case "--multi":
			multi = true
			args = args[1:]
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printWSOpenUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--concurrency":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--concurrency requires a value")
				c.printWSOpenUsage(c.Err)
				return exitUsage
			}
			n, perr := parseIntArg(strings.TrimSpace(args[1]), "--concurrency")
			if perr != nil {
				fmt.Fprintln(c.Err, perr.Error())
				c.printWSOpenUsage(c.Err)
				return exitUsage
			}
			concurrency = n
			args = args[2:]
		case "--workspace":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--workspace requires a value")
				c.printWSOpenUsage(c.Err)
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
					c.printWSOpenUsage(c.Err)
					return exitUsage
				}
				concurrency = n
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for ws open: %q\n", args[0])
			c.printWSOpenUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printWSOpenUsage(c.Err)
		return exitUsage
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws open: %q\n", strings.Join(args[1:], " "))
		c.printWSOpenUsage(c.Err)
		return exitUsage
	}
	if len(args) == 1 {
		targetIDs = append(targetIDs, strings.TrimSpace(args[0]))
	}
	targetIDs = dedupeNonEmpty(targetIDs)
	if concurrency < 1 {
		return c.writeWSOpenError(outputFormat, "invalid_argument", "", "--concurrency must be >= 1", exitUsage)
	}
	if concurrency > 1 && !multi {
		return c.writeWSOpenError(outputFormat, "invalid_argument", "", "--concurrency requires --multi", exitUsage)
	}
	if !multi && len(targetIDs) > 1 {
		return c.writeWSOpenError(outputFormat, "invalid_argument", "", "multiple targets require --multi", exitUsage)
	}
	if multi && len(targetIDs) == 0 && outputFormat == "json" {
		return c.writeWSOpenError(outputFormat, "non_interactive_selection_required", "", "open --multi requires explicit targets in --format json mode", exitUsage)
	}
	for _, workspaceID := range targetIDs {
		if err := validateWorkspaceID(workspaceID); err != nil {
			return c.writeWSOpenError(outputFormat, "invalid_argument", workspaceID, fmt.Sprintf("invalid workspace id: %v", err), exitUsage)
		}
	}
	workspaceHint := ""
	if len(targetIDs) == 1 {
		workspaceHint = targetIDs[0]
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeWSOpenError(outputFormat, "internal_error", workspaceHint, fmt.Sprintf("get working dir: %v", err), exitError)
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeWSOpenError(outputFormat, "internal_error", workspaceHint, fmt.Sprintf("resolve KRA_ROOT: %v", err), exitError)
	}

	if len(targetIDs) == 0 {
		if outputFormat == "json" {
			return c.writeWSOpenError(outputFormat, "invalid_argument", "", "workspace id is required in --format json mode", exitUsage)
		}
		selectedIDs, selErr := c.selectWorkspacesForWSOpen(root, multi)
		if selErr != nil {
			return c.writeWSOpenError(outputFormat, "workspace_not_found", "", selErr.Error(), exitError)
		}
		targetIDs = selectedIDs
	}

	targets := make([]appcmux.OpenTarget, 0, len(targetIDs))
	preFailures := make([]wsOpenFailure, 0)
	for _, id := range targetIDs {
		target, code, msg := resolveWSOpenTarget(root, id)
		if code != "" {
			if multi && concurrency > 1 {
				preFailures = append(preFailures, wsOpenFailure{
					WorkspaceID: id,
					Code:        code,
					Message:     msg,
				})
				continue
			}
			return c.writeWSOpenError(outputFormat, code, id, msg, exitError)
		}
		targets = append(targets, target)
	}

	svc := appcmux.NewService(func() appcmux.Client {
		return wsOpenClientAdapter{inner: newCMUXOpenClient()}
	}, newCMUXMapStore)
	openResult, code, msg := svc.Open(context.Background(), root, targets, concurrency, multi)
	if code != "" {
		if code == "cmux_capability_missing" {
			return c.writeWSOpenCDFallback(outputFormat, workspaceHint, targets, multi, msg)
		}
		return c.writeWSOpenError(outputFormat, code, workspaceHint, msg, exitError)
	}
	results := make([]wsOpenResult, 0, len(openResult.Results))
	for _, r := range openResult.Results {
		results = append(results, wsOpenResult{
			WorkspaceID:     r.WorkspaceID,
			WorkspacePath:   r.WorkspacePath,
			CMUXWorkspaceID: r.CMUXWorkspaceID,
			Ordinal:         r.Ordinal,
			Title:           r.Title,
			ReusedExisting:  r.ReusedExisting,
		})
	}
	failures := make([]wsOpenFailure, 0, len(openResult.Failures))
	for _, f := range openResult.Failures {
		failures = append(failures, wsOpenFailure{
			WorkspaceID: f.WorkspaceID,
			Code:        f.Code,
			Message:     f.Message,
		})
	}
	failures = append(failures, preFailures...)
	return c.writeWSOpenResult(outputFormat, multi, results, failures)
}

func (c *CLI) writeWSOpenCDFallback(format string, workspaceHint string, targets []appcmux.OpenTarget, multi bool, reason string) int {
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		trimmedReason = "workspace runtime is not available"
	}
	if len(targets) != 1 {
		message := fmt.Sprintf("%s; directory fallback supports a single target only", trimmedReason)
		return c.writeWSOpenError(format, "cmux_capability_missing", workspaceHint, message, exitError)
	}
	target := targets[0]
	if err := emitShellActionCD(target.WorkspacePath); err != nil {
		return c.writeWSOpenError(format, "internal_error", target.WorkspaceID, fmt.Sprintf("write shell action: %v", err), exitError)
	}

	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "ws.open",
			WorkspaceID: target.WorkspaceID,
			Result: map[string]any{
				"kra_workspace_id":   target.WorkspaceID,
				"kra_workspace_path": target.WorkspacePath,
				"mode":               "fallback-cd",
				"cwd_synced":         true,
				"runtime_available":  false,
				"fallback_reason":    trimmedReason,
			},
		})
		return exitOK
	}

	useColor := writerSupportsColor(c.Out)
	body := []string{
		fmt.Sprintf("%s%s", uiIndent, styleSuccess("Opened 1 / 1", useColor)),
		fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("mode", useColor), "fallback-cd"),
		fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleAccent("kra", useColor), target.WorkspaceID),
		fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("cwd", useColor), target.WorkspacePath),
		fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("note", useColor), trimmedReason),
	}
	printSection(c.Out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
	return exitOK
}

type wsOpenResult struct {
	WorkspaceID     string
	WorkspacePath   string
	CMUXWorkspaceID string
	Ordinal         int
	Title           string
	ReusedExisting  bool
}

type wsOpenFailure struct {
	WorkspaceID string
	Code        string
	Message     string
}

func resolveWSOpenTarget(root string, workspaceID string) (appcmux.OpenTarget, string, string) {
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
		StatusText:    "kra:workspace",
	}, "", ""
}

func (c *CLI) writeWSOpenResult(format string, multi bool, results []wsOpenResult, failures []wsOpenFailure) int {
	if format == "json" {
		if !multi && len(results) == 1 && len(failures) == 0 {
			result := results[0]
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          true,
				Action:      "ws.open",
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
			Action: "ws.open",
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
			fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleAccent("runtime", useColor), result.CMUXWorkspaceID),
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
		fmt.Fprintf(c.Err, "ws open (%s): %s\n", fail.WorkspaceID, fail.Message)
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

func (c *CLI) selectWorkspacesForWSOpen(root string, multi bool) ([]string, error) {
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
		return nil, fmt.Errorf("ws open requires exactly one workspace selected")
	}
	return []string{strings.TrimSpace(ids[0])}, nil
}

type wsOpenClientAdapter struct {
	inner cmuxOpenClient
}

func (a wsOpenClientAdapter) Capabilities(ctx context.Context) (cmuxctl.Capabilities, error) {
	return a.inner.Capabilities(ctx)
}

func (a wsOpenClientAdapter) CreateWorkspaceWithCommand(ctx context.Context, command string) (string, error) {
	return a.inner.CreateWorkspaceWithCommand(ctx, command)
}

func (a wsOpenClientAdapter) RenameWorkspace(ctx context.Context, workspace string, title string) error {
	return a.inner.RenameWorkspace(ctx, workspace, title)
}

func (a wsOpenClientAdapter) SelectWorkspace(ctx context.Context, workspace string) error {
	return a.inner.SelectWorkspace(ctx, workspace)
}

func (a wsOpenClientAdapter) SetStatus(ctx context.Context, workspace string, label string, text string, icon string, color string) error {
	return a.inner.SetStatus(ctx, workspace, label, text, icon, color)
}

func (a wsOpenClientAdapter) ListWorkspaces(context.Context) ([]cmuxctl.Workspace, error) {
	return nil, fmt.Errorf("unsupported")
}

func (a wsOpenClientAdapter) Identify(ctx context.Context, workspace string, surface string) (map[string]any, error) {
	return a.inner.Identify(ctx, workspace, surface)
}

func (c *CLI) writeWSOpenError(format string, code string, workspaceID string, message string, exitCode int) int {
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "ws.open",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitCode
	}
	if workspaceID != "" {
		fmt.Fprintf(c.Err, "ws open (%s): %s\n", workspaceID, message)
	} else {
		fmt.Fprintf(c.Err, "ws open: %s\n", message)
	}
	return exitCode
}
