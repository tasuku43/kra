package cli

import (
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

type rootMigrationPlan struct {
	Root    string
	Actions []rootMigrateAction
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

	plan, err := planRootMigration(root)
	if err != nil {
		return c.writeRootRuntimeError(opts.format, "root.migrate", "internal_error", err.Error())
	}
	applied := 0
	if opts.apply {
		for _, action := range plan.Actions {
			if err := action.apply(); err != nil {
				return c.writeRootRuntimeError(opts.format, "root.migrate", "internal_error", fmt.Sprintf("%s: %v", action.Path, err))
			}
			applied++
		}
	}

	if opts.format == "json" {
		items := make([]map[string]any, 0, len(plan.Actions))
		for _, action := range plan.Actions {
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
				"planned": len(plan.Actions),
				"applied": applied,
				"actions": items,
			},
		})
		return exitOK
	}

	printRootMigrateHuman(c.Out, plan, opts.apply, writerSupportsColor(c.Out))
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

func planRootMigration(root string) (rootMigrationPlan, error) {
	plan := rootMigrationPlan{Root: root}
	actions := make([]rootMigrateAction, 0)

	addWorkspaceSeedActions := func(base string, label string) error {
		next, err := appendWorkspaceDocumentMigrationAction(actions, base, label)
		if err != nil {
			return err
		}
		actions = next
		return nil
	}

	defaultTemplate := workspaceTemplatePath(root, defaultWorkspaceTemplateName)
	defaultInfo, err := os.Stat(defaultTemplate)
	switch {
	case err == nil && !defaultInfo.IsDir():
		return plan, fmt.Errorf("default template path is not a directory: %s", defaultTemplate)
	case err == nil:
		if err := addWorkspaceSeedActions(defaultTemplate, "templates/default"); err != nil {
			return plan, err
		}
	case os.IsNotExist(err):
		actions = append(actions, rootMigrateAction{
			Path:        defaultTemplate,
			Description: "create default workspace template",
			apply: func() error {
				return ensureDefaultWorkspaceTemplateForRootMigrate(root)
			},
		})
	default:
		return plan, fmt.Errorf("stat default template: %w", err)
	}

	workspacesDir := filepath.Join(root, "workspaces")
	entries, err := os.ReadDir(workspacesDir)
	if err != nil {
		return plan, fmt.Errorf("read workspaces/: %w", err)
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
			return plan, err
		}
	}
	plan.Actions = actions
	return plan, nil
}

func ensureDefaultWorkspaceTemplateForRootMigrate(root string) error {
	templatesDir := workspaceTemplatesPath(root)
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		return fmt.Errorf("create templates/: %w", err)
	}
	defaultPath := workspaceTemplatePath(root, defaultWorkspaceTemplateName)
	if info, err := os.Stat(defaultPath); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("default template path is not a directory: %s", defaultPath)
		}
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat default template: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(defaultPath, "notes"), 0o755); err != nil {
		return fmt.Errorf("create default template notes/: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(defaultPath, "artifacts"), 0o755); err != nil {
		return fmt.Errorf("create default template artifacts/: %w", err)
	}
	if err := os.WriteFile(filepath.Join(defaultPath, rootAgentsFilename), []byte(defaultWorkspaceTemplateAgentsContent()), 0o644); err != nil {
		return fmt.Errorf("write default template AGENTS.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(defaultPath, rootClaudeFilename), []byte(defaultWorkspaceTemplateAgentsContent()), 0o644); err != nil {
		return fmt.Errorf("write default template CLAUDE.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(defaultPath, workspaceDocumentFilename), []byte(defaultWorkspaceDocumentContent), 0o644); err != nil {
		return fmt.Errorf("write default template workspace.md: %w", err)
	}
	return nil
}

func appendWorkspaceDocumentMigrationAction(actions []rootMigrateAction, base string, label string) ([]rootMigrateAction, error) {
	workspacePath := filepath.Join(base, workspaceDocumentFilename)
	legacyPath := filepath.Join(base, "tasks.md")
	info, err := os.Stat(workspacePath)
	if err == nil {
		if info.IsDir() {
			return append(actions, rootMigrateAction{
				Path:        workspacePath,
				Description: "invalid existing directory",
				apply: func() error {
					return fmt.Errorf("path is a directory")
				},
			}), nil
		}
		needsGuide, err := workspaceDocumentNeedsHandoffGuide(workspacePath)
		if err != nil {
			return append(actions, rootMigrateAction{
				Path:        workspacePath,
				Description: "read failed",
				apply: func() error {
					return err
				},
			}), nil
		}
		if needsGuide {
			actions = append(actions, rootMigrateAction{
				Path:        workspacePath,
				Description: fmt.Sprintf("add handoff guide to workspace.md for %s", label),
				apply: func() error {
					return addHandoffGuideToWorkspaceDocument(workspacePath)
				},
			})
		}
		return actions, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return append(actions, rootMigrateAction{
			Path:        workspacePath,
			Description: "stat failed",
			apply: func() error {
				return err
			},
		}), nil
	}
	legacyInfo, legacyErr := os.Stat(legacyPath)
	switch {
	case legacyErr == nil && legacyInfo.IsDir():
		return append(actions, rootMigrateAction{
			Path:        legacyPath,
			Description: "invalid existing directory",
			apply: func() error {
				return fmt.Errorf("path is a directory")
			},
		}), nil
	case legacyErr == nil:
		actions = append(actions, rootMigrateAction{
			Path:        workspacePath,
			Description: fmt.Sprintf("convert legacy tasks.md to workspace.md for %s", label),
			apply: func() error {
				b, err := os.ReadFile(legacyPath)
				if err != nil {
					return err
				}
				if err := os.WriteFile(workspacePath, []byte(convertLegacyTasksToWorkspaceDocument(string(b))), 0o644); err != nil {
					return err
				}
				return os.Remove(legacyPath)
			},
		})
		return actions, nil
	case legacyErr != nil && !os.IsNotExist(legacyErr):
		return append(actions, rootMigrateAction{
			Path:        legacyPath,
			Description: "stat failed",
			apply: func() error {
				return legacyErr
			},
		}), nil
	default:
		return appendMissingFileAction(actions, workspacePath, fmt.Sprintf("add workspace state source to %s", label), defaultWorkspaceDocumentContent), nil
	}
}

func workspaceDocumentNeedsHandoffGuide(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	content := string(b)
	if strings.Contains(content, "This file is the workspace handoff state. Keep it current.") {
		return false, nil
	}
	lines := splitNormalizedLinesForRootMigrate(content)
	for _, line := range lines {
		switch strings.TrimSpace(line) {
		case "## Current State", "## Tasks":
			return true, nil
		}
	}
	return false, nil
}

func addHandoffGuideToWorkspaceDocument(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := splitNormalizedLinesForRootMigrate(string(b))
	guide := []string{
		"This file is the workspace handoff state. Keep it current.",
		"",
		"- Keep `## Tasks` statuses in sync with actual progress.",
		"- Mark current work as `doing` and the next task as `todo`.",
		"- Put resumable task detail in each task description.",
		"",
	}
	out := make([]string, 0, len(lines)+len(guide)+2)
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "# Workspace" {
		out = append(out, lines[0], "")
		out = append(out, guide...)
		out = append(out, lines[1:]...)
	} else {
		out = append(out, "# Workspace", "")
		out = append(out, guide...)
		out = append(out, lines...)
	}
	rendered := strings.Join(out, "\n")
	if !strings.HasSuffix(rendered, "\n") {
		rendered += "\n"
	}
	return os.WriteFile(path, []byte(rendered), 0o644)
}

func convertLegacyTasksToWorkspaceDocument(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return defaultWorkspaceDocumentContent
	}
	lines := splitNormalizedLinesForRootMigrate(content)
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "# Workspace" {
		out := strings.Join(lines, "\n")
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		return out
	}
	outLines := []string{
		"# Workspace",
		"",
		"This file is the workspace handoff state. Keep it current.",
		"",
		"- Keep `## Tasks` statuses in sync with actual progress.",
		"- Mark current work as `doing` and the next task as `todo`.",
		"- Put resumable task detail in each task description.",
		"",
	}
	outLines = append(outLines, lines...)
	out := strings.Join(outLines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

func splitNormalizedLinesForRootMigrate(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
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

func rootMigrateMode(apply bool) string {
	if apply {
		return "apply"
	}
	return "plan"
}

func printRootMigrateHuman(out io.Writer, plan rootMigrationPlan, apply bool, useColor bool) {
	mode := rootMigrateMode(apply)
	lines := []string{
		styleSuccess(fmt.Sprintf("%s: %d action(s)", mode, len(plan.Actions)), useColor),
		styleMuted(fmt.Sprintf("root: %s", plan.Root), useColor),
	}
	if len(plan.Actions) == 0 {
		lines = append(lines, styleMuted("nothing to migrate", useColor))
	} else {
		for _, action := range plan.Actions {
			lines = append(lines, fmt.Sprintf("%s %s", styleSuccess("✔", useColor), action.Description))
			lines = append(lines, styleMuted(fmt.Sprintf("  path: %s", action.Path), useColor))
		}
		if !apply {
			lines = append(lines, styleMuted("rerun with --apply to write these files", useColor))
		}
	}
	printResultSection(out, useColor, lines...)
}
