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

type rootMigrationPlan struct {
	Root               string
	Actions            []rootMigrateAction
	GlobalDock         rootMigrateGlobalDockResult
	LegacyProjectDocks []rootMigrateLegacyDockResult
	Recommendations    []string
	InstallGlobalDock  bool
	GlobalDockCommand  string
}

type rootMigrateGlobalDockResult struct {
	Path    string `json:"path"`
	Changed bool   `json:"changed"`
	Created bool   `json:"created"`
	Updated bool   `json:"updated"`
	Skipped bool   `json:"skipped"`
}

type rootMigrateLegacyDockResult struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
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
		if plan.InstallGlobalDock {
			if err := applyGlobalDockMigration(plan.GlobalDock.Path, plan.GlobalDockCommand); err != nil {
				return c.writeRootRuntimeError(opts.format, "root.migrate", "internal_error", fmt.Sprintf("%s: %v", plan.GlobalDock.Path, err))
			}
		}
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
				"root":                 root,
				"mode":                 rootMigrateMode(opts.apply),
				"planned":              len(plan.Actions),
				"applied":              applied,
				"actions":              items,
				"global_dock":          plan.GlobalDock,
				"legacy_project_docks": plan.LegacyProjectDocks,
				"recommendations":      plan.Recommendations,
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
	if err := appendLegacyDockMigrationPlan(root, &plan); err != nil {
		return plan, err
	}
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
		needsNext, err := workspaceDocumentNeedsNextSection(workspacePath)
		if err != nil {
			return append(actions, rootMigrateAction{
				Path:        workspacePath,
				Description: "read failed",
				apply: func() error {
					return err
				},
			}), nil
		}
		if needsNext {
			actions = append(actions, rootMigrateAction{
				Path:        workspacePath,
				Description: fmt.Sprintf("add Next section to workspace.md for %s", label),
				apply: func() error {
					return addNextSectionToWorkspaceDocument(workspacePath)
				},
			})
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

func workspaceDocumentNeedsNextSection(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	lines := splitNormalizedLinesForRootMigrate(string(b))
	looksLikeWorkspaceDocument := false
	for _, line := range lines {
		switch strings.TrimSpace(line) {
		case "## Next":
			return false, nil
		case "## Current State", "## Tasks":
			looksLikeWorkspaceDocument = true
		}
	}
	return looksLikeWorkspaceDocument, nil
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
		"- Update `## Current State` when the situation changes.",
		"- Update `## Next` before stopping or handing off.",
		"- Keep `## Tasks` statuses in sync with actual progress.",
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

func addNextSectionToWorkspaceDocument(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(b)
	lines := splitNormalizedLinesForRootMigrate(content)
	insert := []string{
		"## Next",
		"",
		"Record the next concrete step here before handing off or stopping.",
		"",
	}
	taskStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "## Tasks" {
			taskStart = i
			break
		}
	}
	out := make([]string, 0, len(lines)+len(insert)+2)
	if taskStart >= 0 {
		out = append(out, lines[:taskStart]...)
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, insert...)
		out = append(out, lines[taskStart:]...)
	} else {
		out = append(out, lines...)
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, insert...)
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
		"- Update `## Current State` when the situation changes.",
		"- Update `## Next` before stopping or handing off.",
		"- Keep `## Tasks` statuses in sync with actual progress.",
		"",
		"## Current State",
		"",
		"This workspace was migrated from tasks.md. Update this section with the current work state.",
		"",
		"## Next",
		"",
		"Record the next concrete step here before handing off or stopping.",
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

func isManagedKraTasksDockCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	for _, marker := range []string{
		"kra ws task tui --current",
		"kra ws task view --current",
		"kra ws task tui --all",
		"kra ws task view --all",
		"kra ws status --current",
		"kra ws status --all",
	} {
		if strings.Contains(trimmed, marker) {
			return true
		}
	}
	return false
}

func appendLegacyDockMigrationPlan(root string, plan *rootMigrationPlan) error {
	globalPath, err := globalCMUXDockPath()
	if err != nil {
		return err
	}
	plan.GlobalDock.Path = globalPath

	candidates := []rootMigrateLegacyDockResult{{
		Path: filepath.Join(root, ".cmux", "dock.json"),
		Kind: "root",
	}, {
		Path: filepath.Join(root, "templates", "default", ".cmux", "dock.json"),
		Kind: "template",
	}}
	workspacesDir := filepath.Join(root, "workspaces")
	entries, err := os.ReadDir(workspacesDir)
	if err != nil {
		return fmt.Errorf("read workspaces/: %w", err)
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		id := strings.TrimSpace(ent.Name())
		if err := validateWorkspaceID(id); err != nil {
			continue
		}
		candidates = append(candidates, rootMigrateLegacyDockResult{
			Path:        filepath.Join(workspacesDir, id, ".cmux", "dock.json"),
			Kind:        "workspace",
			WorkspaceID: id,
		})
	}

	legacyUsesTUI := false
	for _, candidate := range candidates {
		result, usesTUI, err := inspectLegacyProjectDock(candidate)
		if err != nil {
			return err
		}
		if result.Action == "" {
			continue
		}
		if usesTUI {
			legacyUsesTUI = true
		}
		if result.Action == "remove" {
			path := result.Path
			plan.Actions = append(plan.Actions, rootMigrateAction{
				Path:        path,
				Description: "remove managed legacy project Dock config",
				apply: func() error {
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
						return err
					}
					return removeDirIfEmpty(filepath.Dir(path))
				},
			})
		}
		if result.Action == "leave_unchanged" && strings.Contains(result.Reason, "mixed") {
			plan.Recommendations = append(plan.Recommendations, "project-local Dock config remains and cmux may prefer it over global Dock config; move custom controls to the global config or remove the project-local .cmux/dock.json to fully migrate")
		}
		plan.LegacyProjectDocks = append(plan.LegacyProjectDocks, result)
	}

	plan.GlobalDockCommand = standardGlobalDockCommand(legacyUsesTUI)
	state, err := inspectGlobalDock(globalPath, plan.GlobalDockCommand, hasManagedLegacyDock(plan.LegacyProjectDocks))
	if err != nil {
		return err
	}
	plan.GlobalDock = state
	plan.InstallGlobalDock = state.Changed
	if plan.GlobalDock.Path == "" {
		plan.GlobalDock.Path = globalPath
	}
	return nil
}

func inspectLegacyProjectDock(result rootMigrateLegacyDockResult) (rootMigrateLegacyDockResult, bool, error) {
	info, err := os.Stat(result.Path)
	if os.IsNotExist(err) {
		return result, false, nil
	}
	if err != nil {
		return result, false, fmt.Errorf("stat legacy Dock config %s: %w", result.Path, err)
	}
	if info.IsDir() {
		result.Action = "error"
		result.Reason = "path is a directory"
		return result, false, fmt.Errorf("legacy Dock config path is a directory: %s", result.Path)
	}
	b, err := os.ReadFile(result.Path)
	if err != nil {
		return result, false, fmt.Errorf("read legacy Dock config %s: %w", result.Path, err)
	}
	var config cmuxDockConfig
	if err := json.Unmarshal(b, &config); err != nil {
		result.Action = "error"
		result.Reason = "invalid JSON"
		return result, false, fmt.Errorf("invalid legacy project Dock JSON %s: %w", result.Path, err)
	}
	managed := 0
	custom := 0
	usesTUI := false
	for _, control := range config.Controls {
		if control.ID == "kra-tasks" && isManagedKraTasksDockCommand(control.Command) {
			managed++
			if strings.Contains(control.Command, "kra ws task tui ") || strings.Contains(control.Command, "kra ws status ") {
				usesTUI = true
			}
			continue
		}
		custom++
	}
	switch {
	case managed > 0 && custom == 0:
		result.Action = "remove"
		result.Reason = "managed legacy project Dock config"
	case managed > 0 && custom > 0:
		result.Action = "leave_unchanged"
		result.Reason = "mixed managed and custom project Dock controls"
	case managed == 0 && custom > 0:
		result.Action = "leave_unchanged"
		result.Reason = "custom project Dock config left unchanged"
	default:
		result.Action = "leave_unchanged"
		result.Reason = "empty project Dock config left unchanged"
	}
	return result, usesTUI, nil
}

func hasManagedLegacyDock(results []rootMigrateLegacyDockResult) bool {
	for _, result := range results {
		if result.Action == "remove" || strings.Contains(result.Reason, "mixed managed") {
			return true
		}
	}
	return false
}

func globalCMUXDockPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("resolve home dir: empty")
	}
	return filepath.Join(home, ".config", "cmux", "dock.json"), nil
}

func standardGlobalDockCommand(_ bool) string {
	return "kra ws status --cmux-current"
}

func inspectGlobalDock(path string, desiredCommand string, createIfMissing bool) (rootMigrateGlobalDockResult, error) {
	result := rootMigrateGlobalDockResult{Path: path}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if createIfMissing {
			result.Changed = true
			result.Created = true
		} else {
			result.Skipped = true
		}
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("stat global Dock config: %w", err)
	}
	if info.IsDir() {
		return result, fmt.Errorf("global Dock config path is a directory: %s", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("read global Dock config: %w", err)
	}
	var config cmuxDockConfig
	if err := json.Unmarshal(b, &config); err != nil {
		return result, fmt.Errorf("invalid global Dock JSON %s: %w", path, err)
	}
	updated := upsertGlobalKraTasksControl(&config, desiredCommand, false)
	if updated {
		result.Changed = true
		result.Updated = true
	} else {
		result.Skipped = true
	}
	return result, nil
}

func applyGlobalDockMigration(path string, desiredCommand string) error {
	config := cmuxDockConfig{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &config); err != nil {
			return fmt.Errorf("invalid global Dock JSON: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	upsertGlobalKraTasksControl(&config, desiredCommand, true)
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func upsertGlobalKraTasksControl(config *cmuxDockConfig, desiredCommand string, createIfMissing bool) bool {
	desired := cmuxDockControl{
		ID:      "kra-tasks",
		Title:   "Status",
		Command: desiredCommand,
		Height:  420,
	}
	for i := range config.Controls {
		if config.Controls[i].ID != "kra-tasks" {
			continue
		}
		if config.Controls[i] == desired {
			return false
		}
		config.Controls[i] = desired
		return true
	}
	if !createIfMissing {
		return false
	}
	config.Controls = append(config.Controls, desired)
	return true
}

func removeDirIfEmpty(path string) error {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return nil
	}
	return os.Remove(path)
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
		styleMuted(fmt.Sprintf("global Dock config: %s", plan.GlobalDock.Path), useColor),
	}
	if plan.InstallGlobalDock {
		switch {
		case plan.GlobalDock.Created:
			lines = append(lines, "global kra-tasks control: create")
		case plan.GlobalDock.Updated:
			lines = append(lines, "global kra-tasks control: update")
		case plan.GlobalDock.Skipped:
			lines = append(lines, "global kra-tasks control: skip")
		}
	} else {
		lines = append(lines, "global kra-tasks control: skip")
	}
	for _, legacy := range plan.LegacyProjectDocks {
		lines = append(lines, fmt.Sprintf("legacy project Dock: %s", legacy.Action))
		lines = append(lines, styleMuted(fmt.Sprintf("  path: %s", legacy.Path), useColor))
		lines = append(lines, styleMuted(fmt.Sprintf("  reason: %s", legacy.Reason), useColor))
	}
	for _, recommendation := range plan.Recommendations {
		lines = append(lines, styleWarn("warning: "+recommendation, useColor))
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
