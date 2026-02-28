package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
	"github.com/tasuku43/kra/internal/infra/paths"
)

type cmuxOpenClient interface {
	Capabilities(ctx context.Context) (cmuxctl.Capabilities, error)
	CreateWorkspace(ctx context.Context) (string, error)
	RenameWorkspace(ctx context.Context, workspace string, title string) error
	SelectWorkspace(ctx context.Context, workspace string) error
	SendText(ctx context.Context, workspace string, surface string, text string) error
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

	client := newCMUXOpenClient()
	ctx := context.Background()
	caps, err := client.Capabilities(ctx)
	if err != nil {
		return c.writeCMUXOpenError(outputFormat, "cmux_capability_missing", workspaceHint, fmt.Sprintf("read cmux capabilities: %v", err), exitError)
	}
	required := []string{"workspace.create", "workspace.rename", "workspace.select", "surface.send_text"}
	for _, method := range required {
		if _, ok := caps.Methods[method]; !ok {
			return c.writeCMUXOpenError(outputFormat, "cmux_capability_missing", workspaceHint, fmt.Sprintf("cmux capability missing: %s", method), exitError)
		}
	}

	store := newCMUXMapStore(root)
	mapping, err := store.Load()
	if err != nil {
		return c.writeCMUXOpenError(outputFormat, "state_write_failed", workspaceHint, fmt.Sprintf("load cmux mapping: %v", err), exitError)
	}

	results := make([]cmuxOpenResult, 0, len(targetIDs))
	failures := make([]cmuxOpenFailure, 0)
	if multi && concurrency > 1 {
		results, failures = c.runCMUXOpenConcurrent(ctx, root, targetIDs, concurrency, &mapping)
	} else {
		results, failures = c.runCMUXOpenSequential(ctx, client, root, targetIDs, &mapping)
	}
	if len(results) > 0 {
		if err := store.Save(mapping); err != nil {
			workspaceID := ""
			if len(results) > 0 {
				workspaceID = results[len(results)-1].WorkspaceID
			}
			return c.writeCMUXOpenError(outputFormat, "state_write_failed", workspaceID, fmt.Sprintf("save cmux mapping: %v", err), exitError)
		}
	}
	return c.writeCMUXOpenResult(outputFormat, multi, results, failures)
}

type cmuxOpenResult struct {
	WorkspaceID     string
	WorkspacePath   string
	CMUXWorkspaceID string
	Ordinal         int
	Title           string
}

type cmuxOpenFailure struct {
	WorkspaceID string
	Code        string
	Message     string
}

type cmuxOpenTarget struct {
	WorkspaceID   string
	WorkspacePath string
	Title         string
}

func (c *CLI) runCMUXOpenSequential(ctx context.Context, client cmuxOpenClient, root string, targetIDs []string, mapping *cmuxmap.File) ([]cmuxOpenResult, []cmuxOpenFailure) {
	results := make([]cmuxOpenResult, 0, len(targetIDs))
	failures := make([]cmuxOpenFailure, 0)
	var mapMu sync.Mutex
	for _, workspaceID := range targetIDs {
		target, code, msg := resolveCMUXOpenTarget(root, workspaceID)
		if code != "" {
			failures = append(failures, cmuxOpenFailure{WorkspaceID: workspaceID, Code: code, Message: msg})
			return results, failures
		}
		result, code, msg := c.openOneCMUXWorkspace(ctx, client, target, mapping, &mapMu)
		if code != "" {
			failures = append(failures, cmuxOpenFailure{WorkspaceID: workspaceID, Code: code, Message: msg})
			return results, failures
		}
		results = append(results, result)
	}
	return results, failures
}

func (c *CLI) runCMUXOpenConcurrent(ctx context.Context, root string, targetIDs []string, concurrency int, mapping *cmuxmap.File) ([]cmuxOpenResult, []cmuxOpenFailure) {
	type item struct {
		index  int
		target cmuxOpenTarget
	}
	tasks := make([]item, 0, len(targetIDs))
	failures := make([]cmuxOpenFailure, 0)
	for i, workspaceID := range targetIDs {
		target, code, msg := resolveCMUXOpenTarget(root, workspaceID)
		if code != "" {
			failures = append(failures, cmuxOpenFailure{WorkspaceID: workspaceID, Code: code, Message: msg})
			continue
		}
		tasks = append(tasks, item{index: i, target: target})
	}
	if len(tasks) == 0 {
		return nil, failures
	}

	type jobResult struct {
		index  int
		result cmuxOpenResult
		fail   *cmuxOpenFailure
	}
	jobs := make(chan item)
	out := make(chan jobResult, len(tasks))
	var workers sync.WaitGroup
	var mapMu sync.Mutex
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			client := newCMUXOpenClient()
			for job := range jobs {
				result, code, msg := c.openOneCMUXWorkspace(ctx, client, job.target, mapping, &mapMu)
				if code != "" {
					out <- jobResult{
						index: job.index,
						fail: &cmuxOpenFailure{
							WorkspaceID: job.target.WorkspaceID,
							Code:        code,
							Message:     msg,
						},
					}
					continue
				}
				out <- jobResult{index: job.index, result: result}
			}
		}()
	}
	go func() {
		for _, task := range tasks {
			jobs <- task
		}
		close(jobs)
		workers.Wait()
		close(out)
	}()

	collected := make([]jobResult, 0, len(tasks))
	for item := range out {
		collected = append(collected, item)
	}
	sort.Slice(collected, func(i, j int) bool { return collected[i].index < collected[j].index })

	results := make([]cmuxOpenResult, 0, len(tasks))
	for _, item := range collected {
		if item.fail != nil {
			failures = append(failures, *item.fail)
			continue
		}
		results = append(results, item.result)
	}
	return results, failures
}

func resolveCMUXOpenTarget(root string, workspaceID string) (cmuxOpenTarget, string, string) {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	fi, err := os.Stat(wsPath)
	if err != nil || !fi.IsDir() {
		return cmuxOpenTarget{}, "workspace_not_found", fmt.Sprintf("workspace not found: %s", workspaceID)
	}
	title := ""
	if meta, err := loadWorkspaceMetaFile(wsPath); err == nil {
		title = strings.TrimSpace(meta.Workspace.Title)
	}
	return cmuxOpenTarget{
		WorkspaceID:   workspaceID,
		WorkspacePath: wsPath,
		Title:         title,
	}, "", ""
}

func (c *CLI) openOneCMUXWorkspace(ctx context.Context, client cmuxOpenClient, target cmuxOpenTarget, mapping *cmuxmap.File, mapMu *sync.Mutex) (cmuxOpenResult, string, string) {
	cmuxWorkspaceID, err := client.CreateWorkspace(ctx)
	if err != nil {
		return cmuxOpenResult{}, "cmux_create_failed", fmt.Sprintf("create cmux workspace: %v", err)
	}

	mapMu.Lock()
	ordinal, err := cmuxmap.AllocateOrdinal(mapping, target.WorkspaceID)
	mapMu.Unlock()
	if err != nil {
		return cmuxOpenResult{}, "state_write_failed", fmt.Sprintf("allocate cmux ordinal: %v", err)
	}
	cmuxTitle, err := cmuxmap.FormatWorkspaceTitle(target.WorkspaceID, target.Title, ordinal)
	if err != nil {
		return cmuxOpenResult{}, "cmux_rename_failed", fmt.Sprintf("format cmux workspace title: %v", err)
	}

	if err := client.RenameWorkspace(ctx, cmuxWorkspaceID, cmuxTitle); err != nil {
		return cmuxOpenResult{}, "cmux_rename_failed", fmt.Sprintf("rename cmux workspace: %v", err)
	}
	if err := client.SelectWorkspace(ctx, cmuxWorkspaceID); err != nil {
		return cmuxOpenResult{}, "cmux_select_failed", fmt.Sprintf("select cmux workspace: %v", err)
	}
	if err := client.SendText(ctx, cmuxWorkspaceID, "", fmt.Sprintf("cd %s\n", shellQuoteSingle(target.WorkspacePath))); err != nil {
		return cmuxOpenResult{}, "cmux_cwd_sync_failed", fmt.Sprintf("sync cmux cwd: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	mapMu.Lock()
	ws := mapping.Workspaces[target.WorkspaceID]
	ws.Entries = append(ws.Entries, cmuxmap.Entry{
		CMUXWorkspaceID: cmuxWorkspaceID,
		Ordinal:         ordinal,
		TitleSnapshot:   cmuxTitle,
		CreatedAt:       now,
		LastUsedAt:      now,
	})
	mapping.Workspaces[target.WorkspaceID] = ws
	mapMu.Unlock()
	return cmuxOpenResult{
		WorkspaceID:     target.WorkspaceID,
		WorkspacePath:   target.WorkspacePath,
		CMUXWorkspaceID: cmuxWorkspaceID,
		Ordinal:         ordinal,
		Title:           cmuxTitle,
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
		fmt.Fprintln(c.Out, "opened cmux workspace")
		fmt.Fprintf(c.Out, "  kra: %s\n", result.WorkspaceID)
		fmt.Fprintf(c.Out, "  cmux: %s\n", result.CMUXWorkspaceID)
		fmt.Fprintf(c.Out, "  title: %s\n", result.Title)
		fmt.Fprintf(c.Out, "  cwd: %s\n", result.WorkspacePath)
		return exitOK
	}
	fmt.Fprintf(c.Out, "opened cmux workspaces: %d succeeded / %d total\n", len(results), len(results)+len(failures))
	sort.Slice(results, func(i, j int) bool { return results[i].WorkspaceID < results[j].WorkspaceID })
	for _, result := range results {
		fmt.Fprintf(c.Out, "  - %s => %s (%s)\n", result.WorkspaceID, result.CMUXWorkspaceID, result.Title)
	}
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

func shellQuoteSingle(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
