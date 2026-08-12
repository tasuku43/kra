package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tasuku43/kra/internal/app/cmuxtodo"
)

type cmuxTodoReconciler interface {
	Reconcile(context.Context, cmuxtodo.Request) (cmuxtodo.Result, error)
}

type cmuxTodoSyncOptions struct {
	target wsTaskTargetOptions
	prefer cmuxtodo.Preference
	format string
	dryRun bool
}

type cmuxTodoSyncOutcome struct {
	WorkspaceID string
	Result      cmuxtodo.Result
	State       string
	Warning     string
	Err         error
}

func (c *CLI) runWSTaskSync(args []string) int {
	requestedJSON := wantsJSONFormat(args)
	opts, err := parseWSTaskSyncOptions(args)
	if err != nil {
		if errors.Is(err, errHelpRequested) {
			c.printWSTaskSyncUsage(c.Out)
			return exitOK
		}
		if requestedJSON {
			return c.writeWSTaskJSONError("ws.task.sync", "", "invalid_argument", err.Error(), exitUsage)
		}
		fmt.Fprintln(c.Err, err)
		c.printWSTaskSyncUsage(c.Err)
		return exitUsage
	}

	reconciler := newCMUXTodoReconciler()
	if reconciler == nil {
		return c.writeCMUXTodoSyncFatal(opts.format, "ws.task.sync", "", fmt.Errorf("cmux Todo reconciler is not initialized"))
	}
	if opts.target.useAll {
		root, code := c.resolveRootForRootCommand(opts.format, "ws.task.sync")
		if code != exitOK {
			return code
		}
		return c.runWSTaskSyncAll(root, opts, reconciler)
	}

	target, root, _, code := c.resolveWSTaskTarget(opts.target, "sync", opts.format, false, "active")
	if code != exitOK {
		return code
	}
	if target.scope == wsTaskTargetScopeAll {
		return c.runWSTaskSyncAll(root, opts, reconciler)
	}
	outcome := reconcileCMUXTodo(
		context.Background(),
		reconciler,
		cmuxtodo.Request{
			Root:        root,
			WorkspaceID: target.workspaceID,
			Scope:       target.scope,
			Target:      cmuxtodo.TargetWorkspace,
			Prefer:      opts.prefer,
			DryRun:      opts.dryRun,
		},
	)
	return c.writeCMUXTodoSingle(opts.format, "ws.task.sync", outcome)
}

func (c *CLI) runWSTaskSyncAll(root string, opts cmuxTodoSyncOptions, reconciler cmuxTodoReconciler) int {
	rows, err := listRowsFromFilesystem(context.Background(), root, "active", false)
	if err != nil {
		return c.writeCMUXTodoSyncFatal(opts.format, "ws.task.sync", "", err)
	}
	outcomes := make([]cmuxTodoSyncOutcome, 0, len(rows))
	for _, row := range rows {
		outcomes = append(outcomes, reconcileCMUXTodo(
			context.Background(),
			reconciler,
			cmuxtodo.Request{
				Root:        root,
				WorkspaceID: row.ID,
				Scope:       "active",
				Target:      cmuxtodo.TargetWorkspace,
				Prefer:      opts.prefer,
				DryRun:      opts.dryRun,
			},
		))
	}
	return c.writeCMUXTodoBatch(opts.format, "ws.task.sync", outcomes)
}

func (c *CLI) runTaskSync(args []string) int {
	requestedJSON := wantsJSONFormat(args)
	opts, err := parseTaskSyncOptions(args)
	if err != nil {
		if errors.Is(err, errHelpRequested) {
			c.printTaskSyncUsage(c.Out)
			return exitOK
		}
		if requestedJSON {
			return c.writeWSTaskJSONError("task.sync", "", "invalid_argument", err.Error(), exitUsage)
		}
		fmt.Fprintln(c.Err, err)
		c.printTaskSyncUsage(c.Err)
		return exitUsage
	}
	root, code := c.resolveRootForRootCommand(opts.format, "task.sync")
	if code != exitOK {
		return code
	}
	reconciler := newCMUXTodoReconciler()
	if reconciler == nil {
		return c.writeCMUXTodoSyncFatal(opts.format, "task.sync", "", fmt.Errorf("cmux Todo reconciler is not initialized"))
	}
	outcome := reconcileCMUXTodo(
		context.Background(),
		reconciler,
		cmuxtodo.Request{
			Root:        root,
			WorkspaceID: cmuxtodo.RootWorkspaceID,
			Scope:       "root",
			Target:      cmuxtodo.TargetRoot,
			Prefer:      opts.prefer,
			DryRun:      opts.dryRun,
		},
	)
	return c.writeCMUXTodoSingle(opts.format, "task.sync", outcome)
}

func parseWSTaskSyncOptions(args []string) (cmuxTodoSyncOptions, error) {
	opts := cmuxTodoSyncOptions{format: "human"}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return cmuxTodoSyncOptions{}, errHelpRequested
		case arg == "--id":
			if i+1 >= len(args) {
				return cmuxTodoSyncOptions{}, fmt.Errorf("--id requires a value")
			}
			opts.target.workspaceID = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--id="):
			opts.target.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--id="))
		case arg == "--current":
			opts.target.useCurrent = true
		case arg == "--cmux-current":
			opts.target.useCMUXCurrent = true
		case arg == "--select":
			opts.target.useSelect = true
		case arg == "--all":
			opts.target.useAll = true
		case arg == "--dry-run":
			opts.dryRun = true
		case arg == "--prefer":
			if i+1 >= len(args) {
				return cmuxTodoSyncOptions{}, fmt.Errorf("--prefer requires a value")
			}
			opts.prefer = cmuxtodo.Preference(strings.TrimSpace(args[i+1]))
			i++
		case strings.HasPrefix(arg, "--prefer="):
			opts.prefer = cmuxtodo.Preference(strings.TrimSpace(strings.TrimPrefix(arg, "--prefer=")))
		case arg == "--format":
			if i+1 >= len(args) {
				return cmuxTodoSyncOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
		default:
			return cmuxTodoSyncOptions{}, fmt.Errorf("unexpected args for ws task sync: %q", arg)
		}
	}
	if opts.target.useAll {
		if opts.target.workspaceID != "" || opts.target.useCurrent || opts.target.useCMUXCurrent || opts.target.useSelect {
			return cmuxTodoSyncOptions{}, fmt.Errorf("--all cannot be combined with another workspace target")
		}
	} else if err := validateWSTaskTargetOptions(opts.target); err != nil {
		return cmuxTodoSyncOptions{}, err
	}
	if !opts.prefer.Valid() {
		return cmuxTodoSyncOptions{}, fmt.Errorf("unsupported --prefer: %q (supported: kra, cmux)", opts.prefer)
	}
	if opts.format != "human" && opts.format != "json" {
		return cmuxTodoSyncOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.format)
	}
	return opts, nil
}

func parseTaskSyncOptions(args []string) (cmuxTodoSyncOptions, error) {
	opts := cmuxTodoSyncOptions{format: "human"}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return cmuxTodoSyncOptions{}, errHelpRequested
		case arg == "--dry-run":
			opts.dryRun = true
		case arg == "--prefer":
			if i+1 >= len(args) {
				return cmuxTodoSyncOptions{}, fmt.Errorf("--prefer requires a value")
			}
			opts.prefer = cmuxtodo.Preference(strings.TrimSpace(args[i+1]))
			i++
		case strings.HasPrefix(arg, "--prefer="):
			opts.prefer = cmuxtodo.Preference(strings.TrimSpace(strings.TrimPrefix(arg, "--prefer=")))
		case arg == "--format":
			if i+1 >= len(args) {
				return cmuxTodoSyncOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
		default:
			return cmuxTodoSyncOptions{}, fmt.Errorf("unexpected args for task sync: %q", arg)
		}
	}
	if !opts.prefer.Valid() {
		return cmuxTodoSyncOptions{}, fmt.Errorf("unsupported --prefer: %q (supported: kra, cmux)", opts.prefer)
	}
	if opts.format != "human" && opts.format != "json" {
		return cmuxTodoSyncOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.format)
	}
	return opts, nil
}

func reconcileCMUXTodo(ctx context.Context, reconciler cmuxTodoReconciler, request cmuxtodo.Request) cmuxTodoSyncOutcome {
	result, err := reconciler.Reconcile(ctx, request)
	outcome := cmuxTodoSyncOutcome{WorkspaceID: request.WorkspaceID, Result: result, Err: err}
	switch {
	case errors.Is(err, cmuxtodo.ErrNotMapped):
		outcome.Err = nil
		outcome.State = "skipped"
		outcome.Warning = "no mapped cmux workspace; run kra ws open first"
	case err != nil:
		outcome.State = "conflict"
	case request.DryRun:
		outcome.State = "planned"
	case result.Changed:
		outcome.State = "synced"
	default:
		outcome.State = "unchanged"
	}
	return outcome
}

func (c *CLI) writeCMUXTodoSingle(format string, action string, outcome cmuxTodoSyncOutcome) int {
	if format == "json" {
		payload := cliJSONResponse{
			OK:          outcome.Err == nil,
			Action:      action,
			WorkspaceID: workspaceIDForSyncJSON(action, outcome.WorkspaceID),
			Result:      map[string]any{"sync": cmuxTodoSyncJSON(outcome)},
		}
		if outcome.Err != nil {
			payload.Error = &cliJSONError{Code: cmuxTodoErrorCode(outcome.Err), Message: outcome.Err.Error()}
		}
		_ = writeCLIJSON(c.Out, payload)
		if outcome.Err != nil {
			return exitErrorIf(payload.Error.Code)
		}
		return exitOK
	}
	printCMUXTodoSyncHuman(c.Out, outcome, writerSupportsColor(c.Out))
	if outcome.Err != nil {
		fmt.Fprintf(c.Err, "%s sync: %v\n", strings.TrimSuffix(action, ".sync"), outcome.Err)
		return exitErrorIf(cmuxTodoErrorCode(outcome.Err))
	}
	return exitOK
}

func (c *CLI) writeCMUXTodoBatch(format string, action string, outcomes []cmuxTodoSyncOutcome) int {
	summary := summarizeCMUXTodo(outcomes)
	failed := summary["failed"].(int)
	if format == "json" {
		items := make([]map[string]any, 0, len(outcomes))
		failures := make([]map[string]any, 0)
		for _, outcome := range outcomes {
			items = append(items, cmuxTodoSyncJSON(outcome))
			if outcome.Err != nil {
				failures = append(failures, map[string]any{
					"workspace_id": outcome.WorkspaceID,
					"code":         cmuxTodoErrorCode(outcome.Err),
					"message":      outcome.Err.Error(),
				})
			}
		}
		summary["workspaces"] = items
		summary["failures"] = failures
		payload := cliJSONResponse{OK: failed == 0, Action: action, Result: summary}
		if failed > 0 {
			payload.Error = &cliJSONError{Code: "partial_failure", Message: fmt.Sprintf("%d workspace Todo reconciliation(s) failed", failed)}
		}
		_ = writeCLIJSON(c.Out, payload)
		if failed > 0 {
			return exitError
		}
		return exitOK
	}
	for _, outcome := range outcomes {
		printCMUXTodoSyncHuman(c.Out, outcome, writerSupportsColor(c.Out))
		if outcome.Err != nil {
			fmt.Fprintf(c.Err, "ws task sync (%s): %v\n", outcome.WorkspaceID, outcome.Err)
		}
	}
	fmt.Fprintf(
		c.Out,
		"Todo sync: %d succeeded, %d skipped, %d failed\n",
		summary["succeeded"],
		summary["skipped"],
		failed,
	)
	if failed > 0 {
		return exitError
	}
	return exitOK
}

func (c *CLI) writeCMUXTodoSyncFatal(format string, action string, workspaceID string, err error) int {
	code := cmuxTodoErrorCode(err)
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      action,
			WorkspaceID: workspaceID,
			Error:       &cliJSONError{Code: code, Message: err.Error()},
		})
	} else {
		fmt.Fprintf(c.Err, "%s: %v\n", strings.ReplaceAll(action, ".", " "), err)
	}
	return exitErrorIf(code)
}

func cmuxTodoSyncJSON(outcome cmuxTodoSyncOutcome) map[string]any {
	counts := countCMUXTodoOperations(outcome.Result)
	conflicts := make([]map[string]any, 0, len(outcome.Result.Conflicts))
	for _, conflict := range outcome.Result.Conflicts {
		conflicts = append(conflicts, map[string]any{
			"task_id": conflict.TaskID,
			"kind":    string(conflict.Kind),
			"detail":  conflict.Detail,
		})
	}
	return map[string]any{
		"workspace_id":      outcome.WorkspaceID,
		"cmux_workspace_id": outcome.Result.CMUXWorkspaceID,
		"state":             outcome.State,
		"pushed":            counts.pushed,
		"pulled":            counts.pulled,
		"created":           counts.created,
		"removed":           0,
		"conflicts":         conflicts,
		"managed":           outcome.Result.ManagedCount,
		"foreign":           outcome.Result.ForeignCount,
		"truncated":         outcome.Result.Truncated,
		"warning":           outcome.Warning,
	}
}

type cmuxTodoOperationCounts struct {
	pushed  int
	pulled  int
	created int
}

func countCMUXTodoOperations(result cmuxtodo.Result) cmuxTodoOperationCounts {
	counts := cmuxTodoOperationCounts{}
	for _, operation := range result.Operations {
		switch operation.Kind {
		case cmuxtodo.OperationAdd:
			counts.created++
		case cmuxtodo.OperationEdit, cmuxtodo.OperationSetState:
			counts.pushed++
		case cmuxtodo.OperationPullStatus:
			counts.pulled++
		}
	}
	return counts
}

func summarizeCMUXTodo(outcomes []cmuxTodoSyncOutcome) map[string]any {
	summary := map[string]any{
		"count":     len(outcomes),
		"succeeded": 0,
		"failed":    0,
		"skipped":   0,
		"pushed":    0,
		"pulled":    0,
		"created":   0,
		"removed":   0,
		"conflicts": 0,
	}
	for _, outcome := range outcomes {
		if outcome.Err != nil {
			summary["failed"] = summary["failed"].(int) + 1
		} else if outcome.State == "skipped" {
			summary["skipped"] = summary["skipped"].(int) + 1
		} else {
			summary["succeeded"] = summary["succeeded"].(int) + 1
		}
		counts := countCMUXTodoOperations(outcome.Result)
		summary["pushed"] = summary["pushed"].(int) + counts.pushed
		summary["pulled"] = summary["pulled"].(int) + counts.pulled
		summary["created"] = summary["created"].(int) + counts.created
		summary["conflicts"] = summary["conflicts"].(int) + len(outcome.Result.Conflicts)
	}
	return summary
}

func printCMUXTodoSyncHuman(out io.Writer, outcome cmuxTodoSyncOutcome, useColor bool) {
	counts := countCMUXTodoOperations(outcome.Result)
	status := styleSuccess("Todo "+outcome.State, useColor)
	if outcome.Err != nil {
		status = styleError("Todo conflict", useColor)
	} else if outcome.State == "skipped" {
		status = styleMuted("Todo skipped", useColor)
	}
	lines := []string{
		status,
		styleMuted(fmt.Sprintf("workspace: %s", outcome.WorkspaceID), useColor),
		styleMuted(fmt.Sprintf("created: %d, pushed: %d, pulled: %d, conflicts: %d", counts.created, counts.pushed, counts.pulled, len(outcome.Result.Conflicts)), useColor),
	}
	if outcome.Warning != "" {
		lines = append(lines, styleMuted("warning: "+outcome.Warning, useColor))
	}
	for _, conflict := range outcome.Result.Conflicts {
		lines = append(lines, styleError(fmt.Sprintf("%s %s: %s", conflict.TaskID, conflict.Kind, conflict.Detail), useColor))
	}
	printResultSection(out, useColor, lines...)
}

func cmuxTodoErrorCode(err error) string {
	switch {
	case errors.Is(err, cmuxtodo.ErrRecovery):
		return "recovery_failed"
	case errors.Is(err, cmuxtodo.ErrInvalidRequest):
		return "invalid_argument"
	case errors.Is(err, cmuxtodo.ErrInvalidDocument), errors.Is(err, cmuxtodo.ErrInvalidBase), errors.Is(err, cmuxtodo.ErrConflict), errors.Is(err, cmuxtodo.ErrCapacity), errors.Is(err, cmuxtodo.ErrConcurrentTodo):
		return "conflict"
	case errors.Is(err, cmuxtodo.ErrNotMapped):
		return "not_found"
	default:
		return "internal_error"
	}
}

func workspaceIDForSyncJSON(action string, workspaceID string) string {
	if action == "task.sync" {
		return ""
	}
	return workspaceID
}

func (c *CLI) printWSTaskSyncUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra ws task sync [--id <workspace-id> | --current | --cmux-current | --select | --all] [--prefer kra|cmux] [--dry-run] [--format human|json]

Reconcile KRA workspace tasks with the mapped cmux native Todo checklist.
Foreign Todo items are preserved. Conflicts require an explicit --prefer retry.
`)
}

func (c *CLI) printTaskSyncUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra task sync [--prefer kra|cmux] [--dry-run] [--format human|json]

Reconcile the root inbox with native Todo on the cmux Workspace Group anchor.
`)
}
