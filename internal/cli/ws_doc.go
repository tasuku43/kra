package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	appws "github.com/tasuku43/kra/internal/app/ws"
	appwsdoc "github.com/tasuku43/kra/internal/app/wsdoc"
	"github.com/tasuku43/kra/internal/infra/paths"
)

var promptWSDocSelection = func(c *CLI, title string, candidates []workspaceSelectorCandidate) ([]string, error) {
	return c.promptWorkspaceSelectorWithOptionsAndMode("active", "open", title, "document", candidates, true)
}

type wsDocTargetOptions struct {
	workspaceID string
	useCurrent  bool
	useSelect   bool
}

type wsDocOpenOptions struct {
	target      wsDocTargetOptions
	path        string
	surfaceHint string
	noFocus     bool
	format      string
}

type wsDocTarget struct {
	workspaceID string
	scope       string
	root        string
	workspace   string
}

func (c *CLI) runWSDoc(args []string) int {
	if len(args) == 0 {
		c.printWSDocUsage(c.Err)
		return exitUsage
	}

	switch first := strings.TrimSpace(args[0]); first {
	case "-h", "--help", "help":
		c.printWSDocUsage(c.Out)
		return exitOK
	case "open":
		return c.runWSDocOpen(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"ws", "doc"}, args[0]), " "))
		c.printWSDocUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runWSDocOpen(args []string) int {
	requestedJSON := wantsJSONFormat(args)
	opts, err := parseWSDocOpenOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printWSDocOpenUsage(c.Out)
			return exitOK
		}
		return c.writeWSDocError(opts.format, "", "invalid_argument", err.Error(), exitUsage)
	}
	if requestedJSON && opts.format != "json" {
		opts.format = "json"
	}

	target, code := c.resolveWSDocTarget(opts.target, opts.format)
	if code != exitOK {
		return code
	}

	selectedPath, scopePath, code := c.resolveWSDocPath(target.workspace, opts.path, opts.format)
	if code != exitOK {
		return code
	}

	result, runtimeCode, message := newWSDocService().Open(context.Background(), appwsdoc.OpenRequest{
		Root:        target.root,
		WorkspaceID: target.workspaceID,
		Path:        selectedPath,
		ScopePath:   scopePath,
		SurfaceHint: opts.surfaceHint,
		Focus:       !opts.noFocus,
	})
	if runtimeCode != "" {
		return c.writeWSDocError(opts.format, target.workspaceID, runtimeCode, message, exitErrorIf(runtimeCode))
	}

	if opts.format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "ws.doc.open",
			WorkspaceID: target.workspaceID,
			Result: map[string]any{
				"path":               result.Path,
				"scope_path":         result.ScopePath,
				"cmux_workspace_id":  result.CMUXWorkspaceID,
				"docs_pane_ref":      result.DocsPaneRef,
				"viewer_surface_ref": result.ViewerSurfaceRef,
				"focused":            result.Focused,
			},
		})
		return exitOK
	}

	useColor := writerSupportsColor(c.Out)
	printResultSection(
		c.Out,
		useColor,
		styleSuccess(fmt.Sprintf("document opened: %s", relPathOrSelf(target.workspace, result.Path)), useColor),
		styleMuted(fmt.Sprintf("workspace: %s", target.workspaceID), useColor),
		styleMuted(fmt.Sprintf("scope: %s", result.ScopePath), useColor),
		styleMuted(fmt.Sprintf("docs_pane: %s", result.DocsPaneRef), useColor),
		styleMuted(fmt.Sprintf("viewer_surface: %s", result.ViewerSurfaceRef), useColor),
	)
	return exitOK
}

func parseWSDocOpenOptions(args []string) (wsDocOpenOptions, error) {
	opts := wsDocOpenOptions{format: "human"}
	positionals := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return wsDocOpenOptions{}, errHelpRequested
		case arg == "--id":
			if i+1 >= len(args) {
				return wsDocOpenOptions{}, fmt.Errorf("--id requires a value")
			}
			opts.target.workspaceID = strings.TrimSpace(args[i+1])
			i++
		case arg == "--current":
			opts.target.useCurrent = true
		case arg == "--select":
			opts.target.useSelect = true
		case arg == "--surface":
			if i+1 >= len(args) {
				return wsDocOpenOptions{}, fmt.Errorf("--surface requires a value")
			}
			opts.surfaceHint = strings.TrimSpace(args[i+1])
			i++
		case arg == "--no-focus":
			opts.noFocus = true
		case arg == "--format":
			if i+1 >= len(args) {
				return wsDocOpenOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--id="):
			opts.target.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--id="))
		case strings.HasPrefix(arg, "--current="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--current=")) != "" {
				return wsDocOpenOptions{}, fmt.Errorf("--current does not take a value")
			}
			opts.target.useCurrent = true
		case strings.HasPrefix(arg, "--select="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--select=")) != "" {
				return wsDocOpenOptions{}, fmt.Errorf("--select does not take a value")
			}
			opts.target.useSelect = true
		case strings.HasPrefix(arg, "--surface="):
			opts.surfaceHint = strings.TrimSpace(strings.TrimPrefix(arg, "--surface="))
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
		case strings.HasPrefix(arg, "-"):
			return wsDocOpenOptions{}, fmt.Errorf("unknown flag: %s", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	switch opts.format {
	case "human", "json":
	default:
		return wsDocOpenOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.format)
	}
	if opts.target.workspaceID != "" && opts.target.useCurrent {
		return wsDocOpenOptions{}, fmt.Errorf("--id and --current cannot be used together")
	}
	if opts.target.workspaceID != "" && opts.target.useSelect {
		return wsDocOpenOptions{}, fmt.Errorf("--id and --select cannot be used together")
	}
	if opts.target.useCurrent && opts.target.useSelect {
		return wsDocOpenOptions{}, fmt.Errorf("--current and --select cannot be used together")
	}
	if opts.target.workspaceID != "" {
		if err := validateWorkspaceID(opts.target.workspaceID); err != nil {
			return wsDocOpenOptions{}, fmt.Errorf("invalid workspace id: %v", err)
		}
	}
	if len(positionals) > 1 {
		return wsDocOpenOptions{}, fmt.Errorf("unexpected arguments: %q", strings.Join(positionals[1:], " "))
	}
	if len(positionals) == 1 {
		opts.path = positionals[0]
	}
	if opts.format == "json" && opts.target.useSelect {
		return wsDocOpenOptions{}, fmt.Errorf("ws doc open --select is not available in --format json mode")
	}
	return opts, nil
}

func (c *CLI) resolveWSDocTarget(opts wsDocTargetOptions, format string) (wsDocTarget, int) {
	wd, err := os.Getwd()
	if err != nil {
		return wsDocTarget{}, c.writeWSDocError(format, "", "internal_error", fmt.Sprintf("get working dir: %v", err), exitError)
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return wsDocTarget{}, c.writeWSDocError(format, "", "internal_error", fmt.Sprintf("resolve KRA_ROOT: %v", err), exitError)
	}

	if opts.workspaceID == "" && !opts.useCurrent && !opts.useSelect {
		if resolved, ok := detectWorkspaceFromCWD(root, wd); ok {
			workspacePath, found, err := resolveWorkspacePathByID(root, resolved.ID)
			if err != nil {
				return wsDocTarget{}, c.writeWSDocError(format, resolved.ID, "internal_error", fmt.Sprintf("resolve workspace path: %v", err), exitError)
			}
			if !found {
				return wsDocTarget{}, c.writeWSDocError(format, resolved.ID, "not_found", fmt.Sprintf("workspace not found: %s", resolved.ID), exitError)
			}
			return wsDocTarget{
				workspaceID: resolved.ID,
				scope:       resolved.Status,
				root:        root,
				workspace:   workspacePath,
			}, exitOK
		}
		return wsDocTarget{}, c.writeWSDocError(format, "", "invalid_argument", "ws doc open requires --id <id>, --current, --select, or current workspace context", exitUsage)
	}

	target := wsDocTarget{root: root}
	switch {
	case opts.useCurrent:
		resolved, ok := detectWorkspaceFromCWD(root, wd)
		if !ok {
			return wsDocTarget{}, c.writeWSDocError(format, "", "not_found", "ws doc open --current requires current path under workspaces/<id>/... or archive/<id>/...", exitError)
		}
		target.workspaceID = resolved.ID
		target.scope = resolved.Status
	case opts.useSelect:
		selectedID, err := c.selectWorkspaceIDByStatus(root, string(appws.ScopeActive), "doc.open")
		if err != nil {
			return wsDocTarget{}, c.writeWSDocError(format, "", "not_found", err.Error(), exitError)
		}
		target.workspaceID = selectedID
		target.scope = string(appws.ScopeActive)
	default:
		scope, ok, err := lookupWorkspaceStatusByID(context.Background(), root, opts.workspaceID)
		if err != nil {
			return wsDocTarget{}, c.writeWSDocError(format, opts.workspaceID, "internal_error", err.Error(), exitError)
		}
		if !ok {
			return wsDocTarget{}, c.writeWSDocError(format, opts.workspaceID, "not_found", fmt.Sprintf("workspace not found: %s", opts.workspaceID), exitError)
		}
		target.workspaceID = opts.workspaceID
		target.scope = scope
	}

	workspacePath, found, err := resolveWorkspacePathByID(root, target.workspaceID)
	if err != nil {
		return wsDocTarget{}, c.writeWSDocError(format, target.workspaceID, "internal_error", fmt.Sprintf("resolve workspace path: %v", err), exitError)
	}
	if !found {
		return wsDocTarget{}, c.writeWSDocError(format, target.workspaceID, "not_found", fmt.Sprintf("workspace not found: %s", target.workspaceID), exitError)
	}
	target.workspace = workspacePath
	return target, exitOK
}

func (c *CLI) resolveWSDocPath(workspacePath string, raw string, format string) (string, string, int) {
	scopePath, scopeRel, scopeInfo, err := resolveDocScopePath(workspacePath, raw)
	if err != nil {
		return "", "", c.writeWSDocError(format, "", "invalid_argument", err.Error(), exitUsage)
	}
	if scopeInfo.Mode().IsRegular() {
		if !isMarkdownPath(scopePath) {
			return "", "", c.writeWSDocError(format, "", "invalid_argument", fmt.Sprintf("path is not a Markdown file: %s", raw), exitUsage)
		}
		return scopePath, scopeRel, exitOK
	}

	candidates, err := listWorkspaceMarkdownCandidates(workspacePath, scopePath)
	if err != nil {
		return "", "", c.writeWSDocError(format, "", "internal_error", fmt.Sprintf("list markdown candidates: %v", err), exitError)
	}
	if len(candidates) == 0 {
		return "", "", c.writeWSDocError(format, "", "not_found", fmt.Sprintf("no Markdown files found under %s", scopeRel), exitError)
	}
	if len(candidates) == 1 {
		return filepath.Join(workspacePath, candidates[0]), scopeRel, exitOK
	}
	if format == "json" {
		return "", "", c.writeWSDocError(format, "", "non_interactive_selection_required", "multiple Markdown files matched; specify one file path explicitly in --format json mode", exitUsage)
	}

	selectorCandidates := make([]workspaceSelectorCandidate, 0, len(candidates))
	for _, rel := range candidates {
		selectorCandidates = append(selectorCandidates, workspaceSelectorCandidate{
			ID:          rel,
			Description: filepath.Dir(rel),
		})
	}
	title := fmt.Sprintf("Markdown: %s", scopeRel)
	selected, err := promptWSDocSelection(c, title, selectorCandidates)
	if err != nil {
		if err == errSelectorCanceled {
			fmt.Fprintln(c.Err, "aborted")
			return "", "", exitError
		}
		return "", "", c.writeWSDocError(format, "", "non_interactive_selection_required", err.Error(), exitUsage)
	}
	return filepath.Join(workspacePath, selected[0]), scopeRel, exitOK
}

func resolveDocScopePath(workspacePath string, raw string) (string, string, os.FileInfo, error) {
	baseResolved, err := filepath.EvalSymlinks(workspacePath)
	if err != nil {
		return "", "", nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	targetPath := strings.TrimSpace(raw)
	if targetPath == "" {
		targetPath = workspacePath
	} else if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(workspacePath, targetPath)
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil, fmt.Errorf("path not found: %s", strings.TrimSpace(raw))
		}
		return "", "", nil, fmt.Errorf("stat path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		return "", "", nil, fmt.Errorf("resolve path: %w", err)
	}
	rel, err := filepath.Rel(baseResolved, resolved)
	if err != nil {
		return "", "", nil, fmt.Errorf("resolve relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", nil, fmt.Errorf("path escapes workspace: %s", strings.TrimSpace(raw))
	}
	if rel == "repos" || strings.HasPrefix(rel, "repos"+string(filepath.Separator)) {
		return "", "", nil, fmt.Errorf("path under repos/ is not supported: %s", strings.TrimSpace(raw))
	}
	if rel == "." {
		return resolved, ".", info, nil
	}
	return resolved, filepath.ToSlash(rel), info, nil
}

func listWorkspaceMarkdownCandidates(workspacePath string, scopePath string) ([]string, error) {
	rootResolved, err := filepath.EvalSymlinks(workspacePath)
	if err != nil {
		return nil, err
	}
	candidates := make([]string, 0)
	walkErr := filepath.WalkDir(scopePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			resolvedDir, rerr := filepath.EvalSymlinks(path)
			if rerr == nil {
				rel, relErr := filepath.Rel(rootResolved, resolvedDir)
				if relErr == nil && (rel == "repos" || strings.HasPrefix(rel, "repos"+string(filepath.Separator))) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !isMarkdownPath(path) {
			return nil
		}
		resolved, rerr := filepath.EvalSymlinks(path)
		if rerr != nil {
			return rerr
		}
		rel, relErr := filepath.Rel(rootResolved, resolved)
		if relErr != nil {
			return relErr
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
		if rel == "repos" || strings.HasPrefix(rel, "repos"+string(filepath.Separator)) {
			return nil
		}
		candidates = append(candidates, filepath.ToSlash(rel))
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(candidates)
	return candidates, nil
}

func isMarkdownPath(path string) bool {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(path)))
	return ext == ".md" || ext == ".markdown"
}

func relPathOrSelf(workspacePath string, targetPath string) string {
	rel, err := filepath.Rel(workspacePath, targetPath)
	if err != nil {
		return targetPath
	}
	return filepath.ToSlash(rel)
}

func (c *CLI) writeWSDocError(format string, workspaceID string, code string, message string, exitCode int) int {
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "ws.doc.open",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitCode
	}
	fmt.Fprintln(c.Err, message)
	return exitCode
}
