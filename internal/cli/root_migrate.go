package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type rootMigrateOptions struct {
	apply  bool
	format string
}

type rootMigrateAction struct {
	Path        string
	Description string
	apply       func() error
}

func (c *CLI) runRootMigrate(args []string) int {
	opts, err := parseRootMigrateOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printRootMigrateUsage(c.Out)
			return exitOK
		}
		fmt.Fprintln(c.Err, err)
		c.printRootMigrateUsage(c.Err)
		return exitUsage
	}

	root, code := c.resolveRootForRootCommand(opts.format, "root.migrate")
	if code != exitOK {
		return code
	}

	actions, err := planRootMigration(root)
	if err != nil {
		return c.writeRootRuntimeError(opts.format, "root.migrate", "internal_error", err.Error())
	}
	applied := 0
	if opts.apply {
		for _, action := range actions {
			if err := action.apply(); err != nil {
				return c.writeRootRuntimeError(opts.format, "root.migrate", "internal_error", fmt.Sprintf("%s: %v", action.Path, err))
			}
			applied++
		}
	}

	if opts.format == "json" {
		items := make([]map[string]any, 0, len(actions))
		for _, action := range actions {
			items = append(items, map[string]any{
				"path":        action.Path,
				"description": action.Description,
			})
		}
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "root.migrate",
			Result: map[string]any{
				"root":    root,
				"mode":    rootMigrateMode(opts.apply),
				"planned": len(actions),
				"applied": applied,
				"actions": items,
			},
		})
		return exitOK
	}

	printRootMigrateHuman(c.Out, root, actions, opts.apply, writerSupportsColor(c.Out))
	return exitOK
}

func parseRootMigrateOptions(args []string) (rootMigrateOptions, error) {
	opts := rootMigrateOptions{format: "human"}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return rootMigrateOptions{}, errHelpRequested
		case arg == "--apply":
			opts.apply = true
		case arg == "--format":
			if i+1 >= len(args) {
				return rootMigrateOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
		case strings.HasPrefix(arg, "--apply="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--apply=")) != "" {
				return rootMigrateOptions{}, fmt.Errorf("--apply does not take a value")
			}
			opts.apply = true
		case arg == "":
		default:
			return rootMigrateOptions{}, fmt.Errorf("unexpected args for root migrate: %q", arg)
		}
	}
	switch opts.format {
	case "human", "json":
	default:
		return rootMigrateOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.format)
	}
	return opts, nil
}

func planRootMigration(root string) ([]rootMigrateAction, error) {
	actions := make([]rootMigrateAction, 0)
	if err := validateRootMigrateDir(filepath.Join(root, ".cmux")); err != nil {
		return nil, err
	}
	actions = appendRootDockJSONMigrationAction(actions, filepath.Join(root, ".cmux", "dock.json"), "add cmux Dock config to KRA_ROOT")

	addWorkspaceSeedActions := func(base string, label string) error {
		if err := validateRootMigrateDir(filepath.Join(base, ".cmux")); err != nil {
			return err
		}
		actions = appendDockJSONMigrationAction(actions, filepath.Join(base, ".cmux", "dock.json"), fmt.Sprintf("add cmux Dock config to %s", label))
		actions = appendMissingFileAction(actions, filepath.Join(base, "tasks.md"), fmt.Sprintf("add workspace task source to %s", label), defaultWorkspaceTasksContent)
		return nil
	}

	defaultTemplate := workspaceTemplatePath(root, defaultWorkspaceTemplateName)
	defaultInfo, err := os.Stat(defaultTemplate)
	switch {
	case err == nil && !defaultInfo.IsDir():
		return nil, fmt.Errorf("default template path is not a directory: %s", defaultTemplate)
	case err == nil:
		if err := addWorkspaceSeedActions(defaultTemplate, "templates/default"); err != nil {
			return nil, err
		}
	case os.IsNotExist(err):
		actions = append(actions, rootMigrateAction{
			Path:        defaultTemplate,
			Description: "create default workspace template",
			apply: func() error {
				return ensureDefaultWorkspaceTemplate(root)
			},
		})
	default:
		return nil, fmt.Errorf("stat default template: %w", err)
	}

	workspacesDir := filepath.Join(root, "workspaces")
	entries, err := os.ReadDir(workspacesDir)
	if err != nil {
		return nil, fmt.Errorf("read workspaces/: %w", err)
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		id := strings.TrimSpace(ent.Name())
		if err := validateWorkspaceID(id); err != nil {
			continue
		}
		wsPath := filepath.Join(workspacesDir, id)
		if err := addWorkspaceSeedActions(wsPath, "workspace "+id); err != nil {
			return nil, err
		}
	}
	return actions, nil
}

func appendRootDockJSONMigrationAction(actions []rootMigrateAction, path string, description string) []rootMigrateAction {
	return appendDockJSONMigrationActionWithCommand(actions, path, description, defaultRootCMUXDockCommand(), defaultRootCMUXDockContent())
}

func validateRootMigrateDir(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path is not a directory: %s", path)
		}
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("stat %s: %w", path, err)
}

func appendMissingFileAction(actions []rootMigrateAction, path string, description string, content string) []rootMigrateAction {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			actions = append(actions, rootMigrateAction{
				Path:        path,
				Description: "invalid existing directory",
				apply: func() error {
					return fmt.Errorf("path is a directory")
				},
			})
		}
		return actions
	}
	if err != nil && !os.IsNotExist(err) {
		actions = append(actions, rootMigrateAction{
			Path:        path,
			Description: "stat failed",
			apply: func() error {
				return err
			},
		})
		return actions
	}
	actions = append(actions, rootMigrateAction{
		Path:        path,
		Description: description,
		apply: func() error {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			return os.WriteFile(path, []byte(content), 0o644)
		},
	})
	return actions
}

func appendDockJSONMigrationAction(actions []rootMigrateAction, path string, description string) []rootMigrateAction {
	return appendDockJSONMigrationActionWithCommand(actions, path, description, defaultWorkspaceCMUXDockCommand(), defaultWorkspaceCMUXDockContent())
}

func appendDockJSONMigrationActionWithCommand(actions []rootMigrateAction, path string, description string, desiredCommand string, desiredContent string) []rootMigrateAction {
	desired := desiredContent
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			actions = append(actions, rootMigrateAction{
				Path:        path,
				Description: "invalid existing directory",
				apply: func() error {
					return fmt.Errorf("path is a directory")
				},
			})
			return actions
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			actions = append(actions, rootMigrateAction{
				Path:        path,
				Description: "read existing cmux Dock config",
				apply: func() error {
					return readErr
				},
			})
			return actions
		}
		updated, changed, managed := migrateManagedDockConfig(b, desiredCommand)
		if !managed || !changed {
			return actions
		}
		actions = append(actions, rootMigrateAction{
			Path:        path,
			Description: "update managed kra task Dock command",
			apply: func() error {
				return os.WriteFile(path, updated, 0o644)
			},
		})
		return actions
	}
	if err != nil && !os.IsNotExist(err) {
		actions = append(actions, rootMigrateAction{
			Path:        path,
			Description: "stat failed",
			apply: func() error {
				return err
			},
		})
		return actions
	}
	actions = append(actions, rootMigrateAction{
		Path:        path,
		Description: description,
		apply: func() error {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			return os.WriteFile(path, []byte(desired), 0o644)
		},
	})
	return actions
}

func migrateManagedDockConfig(raw []byte, desiredCommand string) ([]byte, bool, bool) {
	var config cmuxDockConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, false, false
	}
	managed := false
	changed := false
	for i := range config.Controls {
		if config.Controls[i].ID != "kra-tasks" {
			continue
		}
		if !isManagedKraTasksDockCommand(config.Controls[i].Command) {
			return nil, false, false
		}
		managed = true
		if config.Controls[i].Command != desiredCommand {
			config.Controls[i].Command = desiredCommand
			changed = true
		}
		if config.Controls[i].Height == 360 {
			config.Controls[i].Height = 420
			changed = true
		}
	}
	if !managed || !changed {
		return nil, managed, false
	}
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, false, false
	}
	return append(b, '\n'), true, true
}

func isManagedKraTasksDockCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	if trimmed == defaultWorkspaceCMUXDockBaseCommand {
		return true
	}
	if trimmed == defaultRootCMUXDockBaseCommand {
		return true
	}
	if trimmed == "kra ws task tui --current" {
		return true
	}
	if trimmed == "kra ws task tui --all --todo-only" {
		return true
	}
	if trimmed == "kra ws task view --current --watch --refresh 2s" {
		return true
	}
	if trimmed == "kra ws task view --all --todo-only --watch --refresh 2s" {
		return true
	}
	if trimmed == "while true; do clear; kra ws task list --current; sleep 2; done" {
		return true
	}
	if strings.HasSuffix(trimmed, "; kra ws task view --current --watch --refresh 2s") {
		return true
	}
	if strings.HasSuffix(trimmed, "; kra ws task view --all --todo-only --watch --refresh 2s") {
		return true
	}
	if strings.HasSuffix(trimmed, "; kra ws task tui --current") {
		return true
	}
	if strings.HasSuffix(trimmed, "; kra ws task tui --all --todo-only") {
		return true
	}
	return strings.HasSuffix(trimmed, "; "+defaultWorkspaceCMUXDockBaseCommand) ||
		strings.HasSuffix(trimmed, "; "+defaultRootCMUXDockBaseCommand)
}

func rootMigrateMode(apply bool) string {
	if apply {
		return "apply"
	}
	return "plan"
}

func printRootMigrateHuman(out io.Writer, root string, actions []rootMigrateAction, apply bool, useColor bool) {
	mode := rootMigrateMode(apply)
	lines := []string{
		styleSuccess(fmt.Sprintf("%s: %d action(s)", mode, len(actions)), useColor),
		styleMuted(fmt.Sprintf("root: %s", root), useColor),
	}
	if len(actions) == 0 {
		lines = append(lines, styleMuted("nothing to migrate", useColor))
	} else {
		for _, action := range actions {
			lines = append(lines, fmt.Sprintf("%s %s", styleSuccess("✔", useColor), action.Description))
			lines = append(lines, styleMuted(fmt.Sprintf("  path: %s", action.Path), useColor))
		}
		if !apply {
			lines = append(lines, styleMuted("rerun with --apply to write these files", useColor))
		}
	}
	printResultSection(out, useColor, lines...)
}
