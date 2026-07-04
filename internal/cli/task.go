package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/tasuku43/kra/internal/app/wstask"
)

const rootTaskWorkspaceID = "root"

type taskListOptions struct {
	format string
}

type taskAddOptions struct {
	title       string
	description string
	format      string
}

type taskStatusOptions struct {
	taskID string
	status wstask.Status
	format string
}

func (c *CLI) runTask(args []string) int {
	if len(args) == 0 {
		return c.runTaskTUI(nil)
	}
	if strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printTaskUsage(c.Out)
			return exitOK
		default:
			return c.runTaskTUI(args)
		}
	}
	switch args[0] {
	case "-h", "--help", "help":
		c.printTaskUsage(c.Out)
		return exitOK
	case "tui", "view":
		return c.runTaskTUI(args[1:])
	case "list", "ls":
		return c.runTaskList(args[1:])
	case "add":
		return c.runTaskAdd(args[1:])
	case "status":
		if isHelpArgs(args[1:]) {
			c.printTaskStatusUsage(c.Out)
			return exitOK
		}
		if shouldRunTaskStatusTUI(args[1:]) {
			return c.runTaskTUI(args[1:])
		}
		return c.runTaskStatus(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"task"}, args[0]), " "))
		c.printTaskUsage(c.Err)
		return exitUsage
	}
}

func isHelpArgs(args []string) bool {
	return len(args) == 1 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help")
}

func shouldRunTaskStatusTUI(args []string) bool {
	if len(args) == 0 {
		return true
	}
	_, err := parseTaskTUIOptions(args)
	return err == nil || err == errHelpRequested
}

func (c *CLI) runTaskTUI(args []string) int {
	opts, err := parseTaskTUIOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printTaskTUIUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printTaskTUIUsage(c.Err)
		return exitUsage
	}
	root, code := c.resolveRootForRootCommand("human", "task.tui")
	if code != exitOK {
		return code
	}
	inFile, ok := c.In.(*os.File)
	if !ok || !isatty.IsTerminal(inFile.Fd()) {
		fmt.Fprintln(c.Err, "interactive task TUI requires a TTY")
		return exitError
	}
	useColor := writerSupportsColor(c.Out) && !opts.noColor
	model := newRootTaskTUIModel(root, opts, useColor)
	program := newWSTaskTUIProgram(inFile, c.Out, model)
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(c.Err, "task tui: %v\n", err)
		return exitError
	}
	return exitOK
}

func (c *CLI) runTaskList(args []string) int {
	requestedJSON := wantsJSONFormat(args)
	opts, err := parseTaskListOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printTaskListUsage(c.Out)
			return exitOK
		}
		if requestedJSON {
			return c.writeWSTaskJSONError("task.list", "", "invalid_argument", err.Error(), exitUsage)
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printTaskListUsage(c.Err)
		return exitUsage
	}
	root, code := c.resolveRootForRootCommand(opts.format, "task.list")
	if code != exitOK {
		return code
	}
	result, err := newRootTaskService().List(root, rootTaskWorkspaceID, "root")
	if err != nil {
		return c.writeWSTaskRuntimeError(opts.format, "task.list", "", err)
	}
	if opts.format == "json" {
		return writeTaskListJSON(c.Out, result)
	}
	printWSTaskListHuman(c.Out, result, writerSupportsColor(c.Out))
	return exitOK
}

func (c *CLI) runTaskAdd(args []string) int {
	requestedJSON := wantsJSONFormat(args)
	opts, err := parseTaskAddOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printTaskAddUsage(c.Out)
			return exitOK
		}
		if requestedJSON {
			return c.writeWSTaskJSONError("task.add", "", "invalid_argument", err.Error(), exitUsage)
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printTaskAddUsage(c.Err)
		return exitUsage
	}
	root, code := c.resolveRootForRootCommand(opts.format, "task.add")
	if code != exitOK {
		return code
	}
	result, err := newRootTaskService().Add(root, rootTaskWorkspaceID, opts.title, opts.description)
	if err != nil {
		return c.writeWSTaskRuntimeError(opts.format, "task.add", "", err)
	}
	if opts.format == "json" {
		return writeTaskAddJSON(c.Out, result)
	}
	useColor := writerSupportsColor(c.Out)
	printResultSection(
		c.Out,
		useColor,
		styleSuccess(fmt.Sprintf("task added: %s", result.Task.ID), useColor),
		styleMuted(fmt.Sprintf("title: %s", result.Task.Title), useColor),
		styleMuted(fmt.Sprintf("path: %s", result.Path), useColor),
	)
	return exitOK
}

func (c *CLI) runTaskStatus(args []string) int {
	requestedJSON := wantsJSONFormat(args)
	opts, err := parseTaskStatusOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printTaskStatusUsage(c.Out)
			return exitOK
		}
		if requestedJSON {
			return c.writeWSTaskJSONError("task.status", "", "invalid_argument", err.Error(), exitUsage)
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printTaskStatusUsage(c.Err)
		return exitUsage
	}
	root, code := c.resolveRootForRootCommand(opts.format, "task.status")
	if code != exitOK {
		return code
	}
	transition, err := newRootTaskService().Status(root, rootTaskWorkspaceID, opts.taskID, opts.status)
	if err != nil {
		return c.writeWSTaskRuntimeError(opts.format, "task.status", "", err)
	}
	if opts.format == "json" {
		return writeTaskStatusJSON(c.Out, transition)
	}
	printTaskStatusHuman(c.Out, transition, writerSupportsColor(c.Out))
	return exitOK
}

func parseTaskListOptions(args []string) (taskListOptions, error) {
	opts := taskListOptions{format: "human"}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return taskListOptions{}, errHelpRequested
		case arg == "--format":
			if i+1 >= len(args) {
				return taskListOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
		default:
			return taskListOptions{}, fmt.Errorf("unexpected args for task list: %q", arg)
		}
	}
	if err := validateTaskFormat(opts.format); err != nil {
		return taskListOptions{}, err
	}
	return opts, nil
}

func parseTaskAddOptions(args []string) (taskAddOptions, error) {
	opts := taskAddOptions{format: "human"}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return taskAddOptions{}, errHelpRequested
		case arg == "--title":
			if i+1 >= len(args) {
				return taskAddOptions{}, fmt.Errorf("--title requires a value")
			}
			opts.title = args[i+1]
			i++
		case arg == "--description":
			if i+1 >= len(args) {
				return taskAddOptions{}, fmt.Errorf("--description requires a value")
			}
			opts.description = args[i+1]
			i++
		case arg == "--format":
			if i+1 >= len(args) {
				return taskAddOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--title="):
			opts.title = strings.TrimPrefix(arg, "--title=")
		case strings.HasPrefix(arg, "--description="):
			opts.description = strings.TrimPrefix(arg, "--description=")
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
		default:
			return taskAddOptions{}, fmt.Errorf("unexpected args for task add: %q", arg)
		}
	}
	if strings.TrimSpace(opts.title) == "" {
		return taskAddOptions{}, fmt.Errorf("--title is required")
	}
	if err := validateTaskFormat(opts.format); err != nil {
		return taskAddOptions{}, err
	}
	return opts, nil
}

func parseTaskStatusOptions(args []string) (taskStatusOptions, error) {
	opts := taskStatusOptions{format: "human"}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return taskStatusOptions{}, errHelpRequested
		case arg == "--format":
			if i+1 >= len(args) {
				return taskStatusOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
		case strings.HasPrefix(arg, "-"):
			return taskStatusOptions{}, fmt.Errorf("unknown flag for task status: %q", arg)
		default:
			switch {
			case opts.taskID == "":
				opts.taskID = strings.TrimSpace(arg)
			case opts.status == "":
				status, err := wstask.ParseStatus(arg)
				if err != nil {
					return taskStatusOptions{}, err
				}
				opts.status = status
			default:
				return taskStatusOptions{}, fmt.Errorf("unexpected args for task status: %q", arg)
			}
		}
	}
	if strings.TrimSpace(opts.taskID) == "" {
		return taskStatusOptions{}, fmt.Errorf("task id is required")
	}
	if opts.status == "" {
		return taskStatusOptions{}, fmt.Errorf("task status is required")
	}
	if err := validateTaskFormat(opts.format); err != nil {
		return taskStatusOptions{}, err
	}
	return opts, nil
}

func parseTaskTUIOptions(args []string) (wsTaskTUIOptions, error) {
	opts := wsTaskTUIOptions{refresh: 2 * time.Second}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := strings.TrimSpace(rest[0])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return wsTaskTUIOptions{}, errHelpRequested
		case arg == "--todo-only":
			opts.todoOnly = true
			rest = rest[1:]
		case arg == "--include-done":
			opts.includeDone = true
			rest = rest[1:]
		case arg == "--no-color":
			opts.noColor = true
			rest = rest[1:]
		case arg == "--refresh":
			if len(rest) < 2 {
				return wsTaskTUIOptions{}, fmt.Errorf("--refresh requires a value")
			}
			duration, err := time.ParseDuration(strings.TrimSpace(rest[1]))
			if err != nil {
				return wsTaskTUIOptions{}, fmt.Errorf("invalid --refresh: %w", err)
			}
			opts.refresh = duration
			rest = rest[2:]
		case strings.HasPrefix(arg, "--todo-only="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--todo-only=")) != "" {
				return wsTaskTUIOptions{}, fmt.Errorf("--todo-only does not take a value")
			}
			opts.todoOnly = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--include-done="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--include-done=")) != "" {
				return wsTaskTUIOptions{}, fmt.Errorf("--include-done does not take a value")
			}
			opts.includeDone = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--no-color="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--no-color=")) != "" {
				return wsTaskTUIOptions{}, fmt.Errorf("--no-color does not take a value")
			}
			opts.noColor = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--refresh="):
			duration, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(arg, "--refresh=")))
			if err != nil {
				return wsTaskTUIOptions{}, fmt.Errorf("invalid --refresh: %w", err)
			}
			opts.refresh = duration
			rest = rest[1:]
		default:
			return wsTaskTUIOptions{}, fmt.Errorf("unknown flag for task tui: %q", arg)
		}
	}
	if len(rest) > 0 {
		return wsTaskTUIOptions{}, fmt.Errorf("unexpected args for task tui: %q", strings.Join(rest, " "))
	}
	return opts, nil
}

func newRootTaskTUIModel(root string, opts wsTaskTUIOptions, useColor bool) wsTaskTUIModel {
	return newWSTaskTUIModelWithService(
		root,
		wsTaskTarget{workspaceID: rootTaskWorkspaceID, scope: "root"},
		opts,
		newRootTaskService(),
		useColor,
	)
}

func validateTaskFormat(format string) error {
	switch format {
	case "human", "json":
		return nil
	default:
		return fmt.Errorf("unsupported --format: %q (supported: human, json)", format)
	}
}

func writeTaskListJSON(out io.Writer, result wstask.ListResult) int {
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
		OK:     true,
		Action: "task.list",
		Result: map[string]any{
			"path":       result.Path,
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

func writeTaskAddJSON(out io.Writer, result wstask.AddResult) int {
	_ = writeCLIJSON(out, cliJSONResponse{
		OK:     true,
		Action: "task.add",
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

func writeTaskStatusJSON(out io.Writer, result wstask.TransitionResult) int {
	_ = writeCLIJSON(out, cliJSONResponse{
		OK:     true,
		Action: "task.status",
		Result: map[string]any{
			"path": result.Path,
			"task": map[string]any{
				"id":              result.Task.ID,
				"title":           result.Task.Title,
				"previous_status": string(result.Task.PreviousStatus),
				"status":          string(result.Task.Status),
				"changed":         result.Task.Changed,
			},
		},
	})
	return exitOK
}

func printTaskStatusHuman(out io.Writer, result wstask.TransitionResult, useColor bool) {
	lines := []string{
		styleSuccess(fmt.Sprintf("task updated: %s", result.Task.ID), useColor),
		styleMuted(fmt.Sprintf("previous: %s", result.Task.PreviousStatus), useColor),
		styleMuted(fmt.Sprintf("status: %s", result.Task.Status), useColor),
		styleMuted(fmt.Sprintf("path: %s", result.Path), useColor),
	}
	printResultSection(out, useColor, lines...)
}

func (c *CLI) printTaskUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra task [--todo-only] [--include-done] [--no-color]
  kra task <subcommand> [args]

Subcommands:
  (no subcommand)   Open root inbox task status view
  status           Open status view, or set status with <task-id> <status>
  tui|view         Compatibility aliases for task status
  list|ls          List root inbox tasks
  add              Add one root inbox task
  help             Show this help
`)
}

func (c *CLI) printTaskTUIUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra task [--todo-only] [--include-done] [--no-color]
  kra task status [--todo-only] [--include-done] [--no-color]
  kra task tui [--todo-only] [--include-done] [--no-color]

Open an interactive terminal-friendly root inbox task status view.
`)
}

func (c *CLI) printTaskListUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra task list [--format human|json]
  kra task ls [--format human|json]

List structured root inbox tasks from <KRA_ROOT>/workspace.md.
`)
}

func (c *CLI) printTaskAddUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra task add --title "<text>" [--description "<text>"] [--format human|json]

Add one structured root inbox task to <KRA_ROOT>/workspace.md.
`)
}

func (c *CLI) printTaskStatusUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  kra task status [--todo-only] [--include-done] [--no-color]
  kra task status <task-id> <todo|doing|blocked|done> [--format human|json]

Open the root inbox status view, or set one root inbox task to the requested status.
`)
}
