package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	appws "github.com/tasuku43/kra/internal/app/ws"
	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/infra/paths"
)

var promptWSTaskSelection = func(c *CLI, title string, candidates []workspaceSelectorCandidate) ([]string, error) {
	return c.promptWorkspaceSelectorWithOptionsAndMode("active", "task", title, "task", candidates, true)
}

var promptWSTaskStatusSelection = func(c *CLI, title string, candidates []workspaceSelectorCandidate) ([]string, error) {
	return c.promptWorkspaceSelectorWithOptionsAndMode("active", "status", title, "status", candidates, true)
}

type wsTaskTargetOptions struct {
	workspaceID string
	useCurrent  bool
	useSelect   bool
	useAll      bool
}

type wsTaskTarget struct {
	workspaceID string
	scope       string
}

type wsTaskLauncherOptions struct {
	target wsTaskTargetOptions
}

type wsTaskListOptions struct {
	target wsTaskTargetOptions
	format string
}

type wsTaskAddOptions struct {
	target      wsTaskTargetOptions
	title       string
	description string
	format      string
}

type wsTaskStatusOptions struct {
	target wsTaskTargetOptions
	taskID string
	status wstask.Status
	format string
}

type wsTaskSyncOptions struct {
	target wsTaskTargetOptions
	format string
}

type wsTaskSyncBatchItem struct {
	WorkspaceID string
	Sync        wstask.SyncResult
	Error       error
}

type wsTaskStatusExecution struct {
	Transition wstask.TransitionResult
	Sync       wstask.SyncResult
	SyncErr    error
}

func (c *CLI) runWSTask(args []string) int {
	if len(args) == 0 {
		c.printWSTaskUsage(c.Err)
		return exitUsage
	}

	switch first := strings.TrimSpace(args[0]); first {
	case "-h", "--help", "help":
		c.printWSTaskUsage(c.Out)
		return exitOK
	case "list", "ls":
		return c.runWSTaskList(args[1:])
	case "add":
		return c.runWSTaskAdd(args[1:])
	case "status":
		return c.runWSTaskStatus(args[1:])
	case "sync":
		return c.runWSTaskSync(args[1:])
	default:
		if strings.HasPrefix(first, "-") {
			return c.runWSTaskLauncher(args)
		}
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"ws", "task"}, args[0]), " "))
		c.printWSTaskUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runWSTaskLauncher(args []string) int {
	opts, err := parseWSTaskLauncherOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printWSTaskUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSTaskUsage(c.Err)
		return exitUsage
	}

	target, root, _, code := c.resolveWSTaskTarget(opts.target, "run", "human", false, "active")
	if code != exitOK {
		return code
	}

	listResult, err := newWorkspaceTaskService().List(root, target.workspaceID, target.scope)
	if err != nil {
		return c.writeWSTaskRuntimeError("human", "ws.task", target.workspaceID, err)
	}
	if len(listResult.Overview.Items) == 0 {
		fmt.Fprintln(c.Err, "no tasks available in workspace; use 'kra ws task add --title ...'")
		return exitError
	}

	selectedTask, err := c.promptWSTaskItem(listResult.Overview.Items)
	if err != nil {
		return c.writeWSTaskSelectorError(target.workspaceID, err)
	}
	nextStatus, err := c.promptWSTaskNextStatus(selectedTask)
	if err != nil {
		return c.writeWSTaskSelectorError(target.workspaceID, err)
	}

	execResult, err := c.executeWSTaskStatus(root, target.workspaceID, selectedTask.ID, nextStatus)
	if err != nil {
		return c.writeWSTaskRuntimeError("human", "ws.task.status", target.workspaceID, err)
	}
	printWSTaskStatusHuman(c.Out, target.workspaceID, execResult, writerSupportsColor(c.Out))
	return exitOK
}

func (c *CLI) runWSTaskList(args []string) int {
	requestedJSON := wantsJSONFormat(args)
	opts, err := parseWSTaskListOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printWSTaskListUsage(c.Out)
			return exitOK
		}
		if requestedJSON {
			return c.writeWSTaskJSONError("ws.task.list", "", "invalid_argument", err.Error(), exitUsage)
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSTaskListUsage(c.Err)
		return exitUsage
	}

	target, root, _, code := c.resolveWSTaskTarget(opts.target, "list", opts.format, true, "active")
	if code != exitOK {
		return code
	}

	result, err := newWorkspaceTaskService().List(root, target.workspaceID, target.scope)
	if err != nil {
		return c.writeWSTaskRuntimeError(opts.format, "ws.task.list", target.workspaceID, err)
	}

	switch opts.format {
	case "json":
		return writeWSTaskListJSON(c.Out, target.workspaceID, result)
	default:
		printWSTaskListHuman(c.Out, result, writerSupportsColor(c.Out))
		return exitOK
	}
}

func (c *CLI) runWSTaskAdd(args []string) int {
	requestedJSON := wantsJSONFormat(args)
	opts, err := parseWSTaskAddOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printWSTaskAddUsage(c.Out)
			return exitOK
		}
		if requestedJSON {
			return c.writeWSTaskJSONError("ws.task.add", "", "invalid_argument", err.Error(), exitUsage)
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSTaskAddUsage(c.Err)
		return exitUsage
	}

	target, root, _, code := c.resolveWSTaskTarget(opts.target, "add", opts.format, false, "active")
	if code != exitOK {
		return code
	}

	result, err := newWorkspaceTaskService().Add(root, target.workspaceID, opts.title, opts.description)
	if err != nil {
		return c.writeWSTaskRuntimeError(opts.format, "ws.task.add", target.workspaceID, err)
	}

	if opts.format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "ws.task.add",
			WorkspaceID: target.workspaceID,
			Result: map[string]any{
				"path": result.Path,
				"task": map[string]any{
					"id":     result.Task.ID,
					"title":  result.Task.Title,
					"status": string(result.Task.Status),
				},
			},
		})
		return exitOK
	}

	useColor := writerSupportsColor(c.Out)
	printResultSection(
		c.Out,
		useColor,
		styleSuccess(fmt.Sprintf("task added: %s", result.Task.ID), useColor),
		styleMuted(fmt.Sprintf("workspace: %s", target.workspaceID), useColor),
		styleMuted(fmt.Sprintf("title: %s", result.Task.Title), useColor),
		styleMuted(fmt.Sprintf("path: %s", result.Path), useColor),
	)
	return exitOK
}

func (c *CLI) runWSTaskStatus(args []string) int {
	requestedJSON := wantsJSONFormat(args)
	opts, err := parseWSTaskStatusOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printWSTaskStatusUsage(c.Out)
			return exitOK
		}
		if requestedJSON {
			return c.writeWSTaskJSONError("ws.task.status", "", "invalid_argument", err.Error(), exitUsage)
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSTaskStatusUsage(c.Err)
		return exitUsage
	}

	target, root, _, code := c.resolveWSTaskTarget(opts.target, "status", opts.format, false, "active")
	if code != exitOK {
		return code
	}

	result, err := c.executeWSTaskStatus(root, target.workspaceID, opts.taskID, opts.status)
	if err != nil {
		return c.writeWSTaskRuntimeError(opts.format, "ws.task.status", target.workspaceID, err)
	}

	if opts.format == "json" {
		return writeWSTaskStatusJSON(c.Out, target.workspaceID, result)
	}
	printWSTaskStatusHuman(c.Out, target.workspaceID, result, writerSupportsColor(c.Out))
	return exitOK
}

func (c *CLI) runWSTaskSync(args []string) int {
	requestedJSON := wantsJSONFormat(args)
	opts, err := parseWSTaskSyncOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printWSTaskSyncUsage(c.Out)
			return exitOK
		}
		if requestedJSON {
			return c.writeWSTaskJSONError("ws.task.sync", "", "invalid_argument", err.Error(), exitUsage)
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSTaskSyncUsage(c.Err)
		return exitUsage
	}
	if opts.target.useAll {
		return c.runWSTaskSyncAll(opts.format)
	}

	target, root, _, code := c.resolveWSTaskTarget(opts.target, "sync", opts.format, false, "active")
	if code != exitOK {
		return code
	}

	result, err := newWorkspaceTaskService().Sync(context.Background(), root, target.workspaceID)
	if err != nil {
		return c.writeWSTaskRuntimeError(opts.format, "ws.task.sync", target.workspaceID, err)
	}

	if opts.format == "json" {
		return writeWSTaskSyncJSON(c.Out, target.workspaceID, result)
	}
	printWSTaskSyncHuman(c.Out, target.workspaceID, result, writerSupportsColor(c.Out))
	return exitOK
}

func (c *CLI) runWSTaskSyncAll(format string) int {
	wd, err := os.Getwd()
	if err != nil {
		return c.writeWSTaskRuntimeError(format, "ws.task.sync", "", fmt.Errorf("get working dir: %w", err))
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeWSTaskRuntimeError(format, "ws.task.sync", "", fmt.Errorf("resolve KRA_ROOT: %w", err))
	}
	workspaceIDs, err := listWorkspaceIDsByStatus(context.Background(), root, "active")
	if err != nil {
		return c.writeWSTaskRuntimeError(format, "ws.task.sync", "", fmt.Errorf("list workspaces: %w", err))
	}
	if len(workspaceIDs) == 0 {
		return c.writeWSTaskRuntimeError(format, "ws.task.sync", "", errNoActiveWorkspaces)
	}

	items := executeWSTaskSyncBatch(context.Background(), root, workspaceIDs)
	if format == "json" {
		return writeWSTaskSyncBatchJSON(c.Out, items)
	}
	return printWSTaskSyncBatchHuman(c.Out, c.Err, items, writerSupportsColor(c.Out))
}

func (c *CLI) executeWSTaskStatus(root string, workspaceID string, taskID string, next wstask.Status) (wsTaskStatusExecution, error) {
	service := newWorkspaceTaskService()
	transition, err := service.Status(root, workspaceID, taskID, next)
	if err != nil {
		return wsTaskStatusExecution{}, err
	}
	syncResult, syncErr := service.Sync(context.Background(), root, workspaceID)
	return wsTaskStatusExecution{
		Transition: transition,
		Sync:       syncResult,
		SyncErr:    syncErr,
	}, nil
}

func (c *CLI) promptWSTaskItem(items []wstask.Item) (wstask.Item, error) {
	candidates := make([]workspaceSelectorCandidate, 0, len(items))
	for _, item := range items {
		candidates = append(candidates, workspaceSelectorCandidate{
			ID:          item.ID,
			Title:       fmt.Sprintf("%s %s", renderWSTaskStatusMarker(item.Status, false), item.Title),
			Description: fmt.Sprintf("status: %s", item.Status),
		})
	}
	ids, err := promptWSTaskSelection(c, "Tasks:", candidates)
	if err != nil {
		return wstask.Item{}, err
	}
	item, ok := findWSTaskItemByID(items, ids[0])
	if !ok {
		return wstask.Item{}, fmt.Errorf("%w: %s", wstask.ErrTaskNotFound, ids[0])
	}
	return item, nil
}

func (c *CLI) promptWSTaskNextStatus(item wstask.Item) (wstask.Status, error) {
	nextStatuses := wstask.AllowedTransitions(item.Status)
	if len(nextStatuses) == 0 {
		return "", fmt.Errorf("no allowed task transitions for %s", item.ID)
	}
	candidates := make([]workspaceSelectorCandidate, 0, len(nextStatuses))
	for _, next := range nextStatuses {
		candidates = append(candidates, workspaceSelectorCandidate{
			ID:          string(next),
			Title:       fmt.Sprintf("%s set task to %s", renderWSTaskStatusMarker(next, false), next),
			Description: fmt.Sprintf("%s %s", item.ID, item.Title),
		})
	}
	title := fmt.Sprintf("Task: %s %s", item.ID, item.Title)
	ids, err := promptWSTaskStatusSelection(c, title, candidates)
	if err != nil {
		return "", err
	}
	return wstask.ParseStatus(ids[0])
}

func findWSTaskItemByID(items []wstask.Item, taskID string) (wstask.Item, bool) {
	for _, item := range items {
		if item.ID == taskID {
			return item, true
		}
	}
	return wstask.Item{}, false
}

func parseWSTaskLauncherOptions(args []string) (wsTaskLauncherOptions, error) {
	opts := wsTaskLauncherOptions{}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := strings.TrimSpace(rest[0])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return wsTaskLauncherOptions{}, errHelpRequested
		case arg == "--current":
			opts.target.useCurrent = true
			rest = rest[1:]
		case arg == "--select":
			opts.target.useSelect = true
			rest = rest[1:]
		case arg == "--all":
			opts.target.useAll = true
			rest = rest[1:]
		case arg == "--id":
			if len(rest) < 2 {
				return wsTaskLauncherOptions{}, fmt.Errorf("--id requires a value")
			}
			opts.target.workspaceID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--id="):
			opts.target.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--id="))
			rest = rest[1:]
		case strings.HasPrefix(arg, "--current="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--current=")) != "" {
				return wsTaskLauncherOptions{}, fmt.Errorf("--current does not take a value")
			}
			opts.target.useCurrent = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--select="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--select=")) != "" {
				return wsTaskLauncherOptions{}, fmt.Errorf("--select does not take a value")
			}
			opts.target.useSelect = true
			rest = rest[1:]
		default:
			return wsTaskLauncherOptions{}, fmt.Errorf("unknown flag for ws task: %q", arg)
		}
	}
	if len(rest) > 0 {
		return wsTaskLauncherOptions{}, fmt.Errorf("unexpected args for ws task: %q", strings.Join(rest, " "))
	}
	if err := validateWSTaskTargetOptions(opts.target); err != nil {
		return wsTaskLauncherOptions{}, err
	}
	return opts, nil
}

func parseWSTaskListOptions(args []string) (wsTaskListOptions, error) {
	opts := wsTaskListOptions{format: "human"}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := strings.TrimSpace(rest[0])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return wsTaskListOptions{}, errHelpRequested
		case arg == "--current":
			opts.target.useCurrent = true
			rest = rest[1:]
		case arg == "--select":
			opts.target.useSelect = true
			rest = rest[1:]
		case arg == "--all":
			opts.target.useAll = true
			rest = rest[1:]
		case arg == "--id":
			if len(rest) < 2 {
				return wsTaskListOptions{}, fmt.Errorf("--id requires a value")
			}
			opts.target.workspaceID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case arg == "--format":
			if len(rest) < 2 {
				return wsTaskListOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--id="):
			opts.target.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--id="))
			rest = rest[1:]
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
			rest = rest[1:]
		case strings.HasPrefix(arg, "--current="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--current=")) != "" {
				return wsTaskListOptions{}, fmt.Errorf("--current does not take a value")
			}
			opts.target.useCurrent = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--select="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--select=")) != "" {
				return wsTaskListOptions{}, fmt.Errorf("--select does not take a value")
			}
			opts.target.useSelect = true
			rest = rest[1:]
		default:
			return wsTaskListOptions{}, fmt.Errorf("unknown flag for ws task list: %q", arg)
		}
	}
	if len(rest) > 0 {
		return wsTaskListOptions{}, fmt.Errorf("unexpected args for ws task list: %q", strings.Join(rest, " "))
	}
	if err := validateWSTaskTargetOptions(opts.target); err != nil {
		return wsTaskListOptions{}, err
	}
	switch opts.format {
	case "human", "json":
	default:
		return wsTaskListOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.format)
	}
	return opts, nil
}

func parseWSTaskAddOptions(args []string) (wsTaskAddOptions, error) {
	opts := wsTaskAddOptions{format: "human"}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := strings.TrimSpace(rest[0])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return wsTaskAddOptions{}, errHelpRequested
		case arg == "--current":
			opts.target.useCurrent = true
			rest = rest[1:]
		case arg == "--select":
			opts.target.useSelect = true
			rest = rest[1:]
		case arg == "--all":
			opts.target.useAll = true
			rest = rest[1:]
		case arg == "--id":
			if len(rest) < 2 {
				return wsTaskAddOptions{}, fmt.Errorf("--id requires a value")
			}
			opts.target.workspaceID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case arg == "--title":
			if len(rest) < 2 {
				return wsTaskAddOptions{}, fmt.Errorf("--title requires a value")
			}
			opts.title = rest[1]
			rest = rest[2:]
		case arg == "--description":
			if len(rest) < 2 {
				return wsTaskAddOptions{}, fmt.Errorf("--description requires a value")
			}
			opts.description = rest[1]
			rest = rest[2:]
		case arg == "--format":
			if len(rest) < 2 {
				return wsTaskAddOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--id="):
			opts.target.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--id="))
			rest = rest[1:]
		case strings.HasPrefix(arg, "--title="):
			opts.title = strings.TrimPrefix(arg, "--title=")
			rest = rest[1:]
		case strings.HasPrefix(arg, "--description="):
			opts.description = strings.TrimPrefix(arg, "--description=")
			rest = rest[1:]
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
			rest = rest[1:]
		case strings.HasPrefix(arg, "--current="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--current=")) != "" {
				return wsTaskAddOptions{}, fmt.Errorf("--current does not take a value")
			}
			opts.target.useCurrent = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--select="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--select=")) != "" {
				return wsTaskAddOptions{}, fmt.Errorf("--select does not take a value")
			}
			opts.target.useSelect = true
			rest = rest[1:]
		default:
			return wsTaskAddOptions{}, fmt.Errorf("unknown flag for ws task add: %q", arg)
		}
	}
	if len(rest) > 0 {
		return wsTaskAddOptions{}, fmt.Errorf("unexpected args for ws task add: %q", strings.Join(rest, " "))
	}
	if err := validateWSTaskTargetOptions(opts.target); err != nil {
		return wsTaskAddOptions{}, err
	}
	if strings.TrimSpace(opts.title) == "" {
		return wsTaskAddOptions{}, fmt.Errorf("--title is required")
	}
	switch opts.format {
	case "human", "json":
	default:
		return wsTaskAddOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.format)
	}
	return opts, nil
}

func parseWSTaskStatusOptions(args []string) (wsTaskStatusOptions, error) {
	opts := wsTaskStatusOptions{format: "human"}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return wsTaskStatusOptions{}, errHelpRequested
		case arg == "--current":
			opts.target.useCurrent = true
		case arg == "--select":
			opts.target.useSelect = true
		case arg == "--id":
			if i+1 >= len(args) {
				return wsTaskStatusOptions{}, fmt.Errorf("--id requires a value")
			}
			opts.target.workspaceID = strings.TrimSpace(args[i+1])
			i++
		case arg == "--format":
			if i+1 >= len(args) {
				return wsTaskStatusOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--id="):
			opts.target.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--id="))
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
		case strings.HasPrefix(arg, "--current="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--current=")) != "" {
				return wsTaskStatusOptions{}, fmt.Errorf("--current does not take a value")
			}
			opts.target.useCurrent = true
		case strings.HasPrefix(arg, "--select="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--select=")) != "" {
				return wsTaskStatusOptions{}, fmt.Errorf("--select does not take a value")
			}
			opts.target.useSelect = true
		case strings.HasPrefix(arg, "-"):
			return wsTaskStatusOptions{}, fmt.Errorf("unknown flag for ws task status: %q", arg)
		default:
			switch {
			case opts.taskID == "":
				opts.taskID = strings.TrimSpace(arg)
			case opts.status == "":
				status, err := wstask.ParseStatus(arg)
				if err != nil {
					return wsTaskStatusOptions{}, err
				}
				opts.status = status
			default:
				return wsTaskStatusOptions{}, fmt.Errorf("unexpected args for ws task status: %q", arg)
			}
		}
	}
	if strings.TrimSpace(opts.taskID) == "" {
		return wsTaskStatusOptions{}, fmt.Errorf("task id is required")
	}
	if opts.status == "" {
		return wsTaskStatusOptions{}, fmt.Errorf("task status is required")
	}
	if err := validateWSTaskTargetOptions(opts.target); err != nil {
		return wsTaskStatusOptions{}, err
	}
	switch opts.format {
	case "human", "json":
	default:
		return wsTaskStatusOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.format)
	}
	return opts, nil
}

func parseWSTaskSyncOptions(args []string) (wsTaskSyncOptions, error) {
	opts := wsTaskSyncOptions{format: "human"}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := strings.TrimSpace(rest[0])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return wsTaskSyncOptions{}, errHelpRequested
		case arg == "--current":
			opts.target.useCurrent = true
			rest = rest[1:]
		case arg == "--select":
			opts.target.useSelect = true
			rest = rest[1:]
		case arg == "--all":
			opts.target.useAll = true
			rest = rest[1:]
		case arg == "--id":
			if len(rest) < 2 {
				return wsTaskSyncOptions{}, fmt.Errorf("--id requires a value")
			}
			opts.target.workspaceID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case arg == "--format":
			if len(rest) < 2 {
				return wsTaskSyncOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--id="):
			opts.target.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--id="))
			rest = rest[1:]
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
			rest = rest[1:]
		case strings.HasPrefix(arg, "--current="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--current=")) != "" {
				return wsTaskSyncOptions{}, fmt.Errorf("--current does not take a value")
			}
			opts.target.useCurrent = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--all="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--all=")) != "" {
				return wsTaskSyncOptions{}, fmt.Errorf("--all does not take a value")
			}
			opts.target.useAll = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--select="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--select=")) != "" {
				return wsTaskSyncOptions{}, fmt.Errorf("--select does not take a value")
			}
			opts.target.useSelect = true
			rest = rest[1:]
		default:
			return wsTaskSyncOptions{}, fmt.Errorf("unknown flag for ws task sync: %q", arg)
		}
	}
	if len(rest) > 0 {
		return wsTaskSyncOptions{}, fmt.Errorf("unexpected args for ws task sync: %q", strings.Join(rest, " "))
	}
	if opts.target.useAll {
		switch opts.format {
		case "human", "json":
		default:
			return wsTaskSyncOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.format)
		}
		if opts.target.workspaceID != "" {
			return wsTaskSyncOptions{}, fmt.Errorf("--id and --all cannot be used together")
		}
		if opts.target.useCurrent {
			return wsTaskSyncOptions{}, fmt.Errorf("--current and --all cannot be used together")
		}
		if opts.target.useSelect {
			return wsTaskSyncOptions{}, fmt.Errorf("--select and --all cannot be used together")
		}
		return opts, nil
	}
	if err := validateWSTaskTargetOptions(opts.target); err != nil {
		return wsTaskSyncOptions{}, err
	}
	switch opts.format {
	case "human", "json":
	default:
		return wsTaskSyncOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.format)
	}
	return opts, nil
}

func validateWSTaskTargetOptions(opts wsTaskTargetOptions) error {
	if opts.workspaceID != "" && opts.useAll {
		return fmt.Errorf("--id and --all cannot be used together")
	}
	if opts.workspaceID != "" && opts.useCurrent {
		return fmt.Errorf("--id and --current cannot be used together")
	}
	if opts.workspaceID != "" && opts.useSelect {
		return fmt.Errorf("--id and --select cannot be used together")
	}
	if opts.useCurrent && opts.useAll {
		return fmt.Errorf("--current and --all cannot be used together")
	}
	if opts.useCurrent && opts.useSelect {
		return fmt.Errorf("--current and --select cannot be used together")
	}
	if opts.useSelect && opts.useAll {
		return fmt.Errorf("--select and --all cannot be used together")
	}
	if opts.workspaceID == "" && !opts.useCurrent && !opts.useSelect {
		return fmt.Errorf("one of --id <id>, --current, or --select is required")
	}
	if opts.workspaceID != "" {
		if err := validateWorkspaceID(opts.workspaceID); err != nil {
			return fmt.Errorf("invalid workspace id: %w", err)
		}
	}
	return nil
}

func (c *CLI) resolveWSTaskTarget(
	opts wsTaskTargetOptions,
	action string,
	format string,
	allowArchived bool,
	selectScope string,
) (wsTaskTarget, string, string, int) {
	wd, err := os.Getwd()
	if err != nil {
		return wsTaskTarget{}, "", "", c.writeWSTaskRuntimeError(format, "ws.task."+action, "", fmt.Errorf("get working dir: %w", err))
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return wsTaskTarget{}, "", "", c.writeWSTaskRuntimeError(format, "ws.task."+action, "", fmt.Errorf("resolve KRA_ROOT: %w", err))
	}

	target := wsTaskTarget{}
	switch {
	case opts.useCurrent:
		resolved, ok := detectWorkspaceFromCWD(root, wd)
		if !ok {
			return wsTaskTarget{}, "", "", c.writeWSTaskRuntimeError(format, "ws.task."+action, "", fmt.Errorf("ws task --current requires current path under workspaces/<id>/... or archive/<id>/..."))
		}
		target.workspaceID = resolved.ID
		target.scope = resolved.Status
	case opts.useSelect:
		selected, err := c.selectWorkspaceByScope(root, selectScope, action)
		if err != nil {
			return wsTaskTarget{}, "", "", c.writeWSTaskRuntimeError(format, "ws.task."+action, "", err)
		}
		target.workspaceID = selected.workspaceID
		target.scope = selected.scope
	default:
		scope, ok, err := lookupWorkspaceStatusByID(context.Background(), root, opts.workspaceID)
		if err != nil {
			return wsTaskTarget{}, "", "", c.writeWSTaskRuntimeError(format, "ws.task."+action, opts.workspaceID, err)
		}
		if !ok {
			return wsTaskTarget{}, "", "", c.writeWSTaskRuntimeError(format, "ws.task."+action, opts.workspaceID, fmt.Errorf("%w: %s", wstask.ErrWorkspaceNotFound, opts.workspaceID))
		}
		target.workspaceID = opts.workspaceID
		target.scope = scope
	}

	if target.scope == string(appws.ScopeArchived) && !allowArchived {
		return wsTaskTarget{}, "", "", c.writeWSTaskRuntimeError(format, "ws.task."+action, target.workspaceID, fmt.Errorf("%w: %s", wstask.ErrWorkspaceArchived, target.workspaceID))
	}
	return target, root, wd, exitOK
}

func (c *CLI) selectWorkspaceByScope(root string, scope string, action string) (wsTaskTarget, error) {
	selectedID, err := c.selectWorkspaceIDByStatus(root, scope, action)
	if err != nil {
		return wsTaskTarget{}, err
	}
	return wsTaskTarget{
		workspaceID: selectedID,
		scope:       scope,
	}, nil
}

func (c *CLI) writeWSTaskSelectorError(workspaceID string, err error) int {
	if errors.Is(err, errSelectorCanceled) {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}
	return c.writeWSTaskRuntimeError("human", "ws.task", workspaceID, err)
}

func (c *CLI) writeWSTaskRuntimeError(format string, action string, workspaceID string, err error) int {
	code := "internal_error"
	switch {
	case errors.Is(err, wstask.ErrWorkspaceNotFound), strings.Contains(err.Error(), "resolve KRA_ROOT"), strings.Contains(err.Error(), "requires current path"):
		code = "not_found"
	case errors.Is(err, wstask.ErrTaskNotFound):
		code = "not_found"
	case errors.Is(err, wstask.ErrConflict), errors.Is(err, wstask.ErrWorkspaceArchived):
		code = "conflict"
	case strings.Contains(err.Error(), "requires a TTY"):
		code = "invalid_argument"
	}
	if format == "json" {
		return c.writeWSTaskJSONError(action, workspaceID, code, err.Error(), exitErrorIf(code))
	}
	fmt.Fprintf(c.Err, "%v\n", err)
	return exitErrorIf(code)
}

func exitErrorIf(code string) int {
	if code == "invalid_argument" {
		return exitUsage
	}
	return exitError
}

func (c *CLI) writeWSTaskJSONError(action string, workspaceID string, code string, message string, exitCode int) int {
	_ = writeCLIJSON(c.Out, cliJSONResponse{
		OK:          false,
		Action:      action,
		WorkspaceID: workspaceID,
		Error: &cliJSONError{
			Code:    code,
			Message: message,
		},
	})
	return exitCode
}

func writeWSTaskListJSON(out io.Writer, workspaceID string, result wstask.ListResult) int {
	items := make([]map[string]any, 0, len(result.Overview.Items))
	for _, item := range result.Overview.Items {
		items = append(items, map[string]any{
			"id":          item.ID,
			"title":       item.Title,
			"status":      string(item.Status),
			"description": item.Description,
		})
	}
	_ = writeCLIJSON(out, cliJSONResponse{
		OK:          true,
		Action:      "ws.task.list",
		WorkspaceID: workspaceID,
		Result: map[string]any{
			"task_state": string(result.Overview.TaskState),
			"counts": map[string]any{
				"total":   result.Overview.Counts.Total,
				"doing":   result.Overview.Counts.Doing,
				"blocked": result.Overview.Counts.Blocked,
				"todo":    result.Overview.Counts.Todo,
				"done":    result.Overview.Counts.Done,
			},
			"items": items,
		},
	})
	return exitOK
}

func writeWSTaskStatusJSON(out io.Writer, workspaceID string, result wsTaskStatusExecution) int {
	payload := cliJSONResponse{
		OK:          true,
		Action:      "ws.task.status",
		WorkspaceID: workspaceID,
		Result: map[string]any{
			"task": map[string]any{
				"id":              result.Transition.Task.ID,
				"title":           result.Transition.Task.Title,
				"previous_status": string(result.Transition.Task.PreviousStatus),
				"status":          string(result.Transition.Task.Status),
				"changed":         result.Transition.Task.Changed,
			},
			"sync": syncResultMap(result.Sync),
		},
	}
	if result.SyncErr != nil {
		payload.Warnings = []string{fmt.Sprintf("task sync failed: %v", result.SyncErr)}
	}
	_ = writeCLIJSON(out, payload)
	return exitOK
}

func writeWSTaskSyncJSON(out io.Writer, workspaceID string, result wstask.SyncResult) int {
	_ = writeCLIJSON(out, cliJSONResponse{
		OK:          true,
		Action:      "ws.task.sync",
		WorkspaceID: workspaceID,
		Result: map[string]any{
			"sync": syncResultMap(result),
		},
	})
	return exitOK
}

func writeWSTaskSyncBatchJSON(out io.Writer, items []wsTaskSyncBatchItem) int {
	successes := make([]map[string]any, 0, len(items))
	failures := make([]map[string]any, 0)
	skipped := 0
	totalTargets := 0
	totalSet := 0
	totalCleared := 0
	for _, item := range items {
		if item.Error != nil {
			failures = append(failures, map[string]any{
				"workspace_id": item.WorkspaceID,
				"code":         "internal_error",
				"message":      item.Error.Error(),
			})
			continue
		}
		if item.Sync.State == wstask.SyncStateSkipped {
			skipped++
		}
		totalTargets += item.Sync.Targets
		totalSet += item.Sync.SetCount
		totalCleared += item.Sync.ClearCount
		successes = append(successes, map[string]any{
			"workspace_id": item.WorkspaceID,
			"sync":         syncResultMap(item.Sync),
		})
	}

	payload := cliJSONResponse{
		OK:     len(failures) == 0,
		Action: "ws.task.sync",
		Result: map[string]any{
			"count":      len(items),
			"succeeded":  len(successes),
			"failed":     len(failures),
			"skipped":    skipped,
			"targets":    totalTargets,
			"set":        totalSet,
			"cleared":    totalCleared,
			"workspaces": successes,
			"failures":   failures,
		},
	}
	if len(failures) > 0 {
		payload.Error = &cliJSONError{
			Code:    "partial_failure",
			Message: "some workspaces failed to sync tasks",
		}
		_ = writeCLIJSON(out, payload)
		return exitError
	}
	_ = writeCLIJSON(out, payload)
	return exitOK
}

func syncResultMap(result wstask.SyncResult) map[string]any {
	out := map[string]any{
		"state":   string(result.State),
		"targets": result.Targets,
		"set":     result.SetCount,
		"cleared": result.ClearCount,
	}
	if strings.TrimSpace(result.WarningText) != "" {
		out["warning"] = result.WarningText
	}
	return out
}

func printWSTaskListHuman(out io.Writer, result wstask.ListResult, useColor bool) {
	body := make([]string, 0, len(result.Overview.Items))
	if len(result.Overview.Items) == 0 {
		body = append(body, fmt.Sprintf("%s(none)", uiIndent))
	} else {
		for _, item := range result.Overview.Items {
			body = append(body, renderWSTaskListRow(item, useColor))
		}
	}
	printSection(out, styleBold("Tasks:", useColor), body, sectionRenderOptions{
		blankAfterHeading: true,
		trailingBlank:     true,
	})
}

func printWSTaskStatusHuman(out io.Writer, workspaceID string, result wsTaskStatusExecution, useColor bool) {
	lines := []string{
		styleSuccess(fmt.Sprintf("task updated: %s", result.Transition.Task.ID), useColor),
		styleMuted(fmt.Sprintf("workspace: %s", workspaceID), useColor),
		styleMuted(fmt.Sprintf("previous: %s", result.Transition.Task.PreviousStatus), useColor),
		styleMuted(fmt.Sprintf("status: %s", result.Transition.Task.Status), useColor),
	}
	if result.SyncErr != nil {
		lines = append(lines, styleWarn(fmt.Sprintf("task sync failed: %v", result.SyncErr), useColor))
	} else {
		lines = append(lines, renderWSTaskSyncSummary(result.Sync, useColor))
	}
	printResultSection(out, useColor, lines...)
}

func printWSTaskSyncHuman(out io.Writer, workspaceID string, result wstask.SyncResult, useColor bool) {
	printResultSection(
		out,
		useColor,
		renderWSTaskSyncSummary(result, useColor),
		styleMuted(fmt.Sprintf("workspace: %s", workspaceID), useColor),
	)
}

func renderWSTaskSyncSummary(result wstask.SyncResult, useColor bool) string {
	if result.State == wstask.SyncStateSkipped {
		if strings.TrimSpace(result.WarningText) != "" {
			return styleMuted(fmt.Sprintf("task sync skipped: %s", result.WarningText), useColor)
		}
		return styleMuted("task sync skipped", useColor)
	}
	return styleMuted(
		fmt.Sprintf("task sync: applied %d (set %d, cleared %d)", result.Targets, result.SetCount, result.ClearCount),
		useColor,
	)
}

func executeWSTaskSyncBatch(ctx context.Context, root string, workspaceIDs []string) []wsTaskSyncBatchItem {
	type indexedResult struct {
		index int
		item  wsTaskSyncBatchItem
	}

	workerLimit := defaultWSOpenConcurrency(len(workspaceIDs))
	if workerLimit < 1 {
		workerLimit = 1
	}

	service := newWorkspaceTaskService()
	jobs := make(chan int)
	results := make(chan indexedResult, len(workspaceIDs))
	var wg sync.WaitGroup
	for i := 0; i < workerLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				workspaceID := workspaceIDs[idx]
				result, err := service.Sync(ctx, root, workspaceID)
				results <- indexedResult{
					index: idx,
					item: wsTaskSyncBatchItem{
						WorkspaceID: workspaceID,
						Sync:        result,
						Error:       err,
					},
				}
			}
		}()
	}
	for i := range workspaceIDs {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	close(results)

	items := make([]wsTaskSyncBatchItem, len(workspaceIDs))
	for result := range results {
		items[result.index] = result.item
	}
	sort.Slice(items, func(i, j int) bool { return items[i].WorkspaceID < items[j].WorkspaceID })
	return items
}

func printWSTaskSyncBatchHuman(out io.Writer, errOut io.Writer, items []wsTaskSyncBatchItem, useColor bool) int {
	totalTargets := 0
	totalSet := 0
	totalCleared := 0
	skipped := 0
	failed := 0
	lines := make([]string, 0, len(items)+2)
	for _, item := range items {
		if item.Error != nil {
			failed++
			lines = append(lines, fmt.Sprintf("%s %s", styleWarn("✖", useColor), item.WorkspaceID))
			fmt.Fprintf(errOut, "ws task sync (%s): %v\n", item.WorkspaceID, item.Error)
			continue
		}
		totalTargets += item.Sync.Targets
		totalSet += item.Sync.SetCount
		totalCleared += item.Sync.ClearCount
		if item.Sync.State == wstask.SyncStateSkipped {
			skipped++
		}
		lines = append(lines, fmt.Sprintf("%s %s", styleMuted("•", useColor), item.WorkspaceID))
		lines = append(lines, fmt.Sprintf("%s%s", uiIndent, renderWSTaskSyncSummary(item.Sync, useColor)))
	}
	printResultSection(
		out,
		useColor,
		append([]string{
			styleMuted(fmt.Sprintf("task sync: applied %d workspaces (set %d, cleared %d)", totalTargets, totalSet, totalCleared), useColor),
			styleMuted(fmt.Sprintf("workspace: all (succeeded %d, skipped %d, failed %d)", len(items)-failed, skipped, failed), useColor),
		}, lines...)...,
	)
	if failed > 0 {
		return exitError
	}
	return exitOK
}

func renderWSTaskListRow(item wstask.Item, useColor bool) string {
	return fmt.Sprintf("%s%s %s: %s", uiIndent, renderWSTaskStatusMarker(item.Status, useColor), item.ID, item.Title)
}

func renderWSTaskStatusMarker(status wstask.Status, useColor bool) string {
	icon := "○"
	switch status {
	case wstask.StatusDoing:
		icon = "●"
		if useColor {
			return styleInfo(icon, useColor)
		}
	case wstask.StatusBlocked:
		icon = "▲"
		if useColor {
			return styleWarn(icon, useColor)
		}
	case wstask.StatusDone:
		icon = "✔"
		if useColor {
			return styleSuccess(icon, useColor)
		}
	default:
		if useColor {
			return styleMuted(icon, useColor)
		}
	}
	return icon
}

func wantsJSONFormat(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "--format" && i+1 < len(args) && strings.TrimSpace(args[i+1]) == "json" {
			return true
		}
		if strings.HasPrefix(arg, "--format=") && strings.TrimSpace(strings.TrimPrefix(arg, "--format=")) == "json" {
			return true
		}
	}
	return false
}
