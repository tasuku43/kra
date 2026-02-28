package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	appws "github.com/tasuku43/kra/internal/app/ws"
	"github.com/tasuku43/kra/internal/infra/paths"
)

type workspaceContextSelection struct {
	ID     string
	Status string
}

func detectWorkspaceFromCWD(root string, cwd string) (workspaceContextSelection, bool) {
	if root == "" || cwd == "" {
		return workspaceContextSelection{}, false
	}
	cleanRoot := filepath.Clean(root)
	cleanCWD := filepath.Clean(cwd)

	try := func(base string, status string) (workspaceContextSelection, bool) {
		rel, err := filepath.Rel(base, cleanCWD)
		if err != nil {
			return workspaceContextSelection{}, false
		}
		rel = filepath.Clean(rel)
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return workspaceContextSelection{}, false
		}
		first := strings.Split(rel, string(filepath.Separator))[0]
		if err := validateWorkspaceID(first); err != nil {
			return workspaceContextSelection{}, false
		}
		return workspaceContextSelection{ID: first, Status: status}, true
	}

	if out, ok := try(filepath.Join(cleanRoot, "workspaces"), "active"); ok {
		return out, true
	}
	if out, ok := try(filepath.Join(cleanRoot, "archive"), "archived"); ok {
		return out, true
	}
	return workspaceContextSelection{}, false
}

func (c *CLI) runWSLauncher(args []string) int {
	return c.runWSLauncherWithSelectModeAndAction(args, false, "")
}

func (c *CLI) runWSActionSubcommand(action string, args []string) int {
	return c.runWSLauncherWithSelectModeAndAction(args, false, strings.TrimSpace(action))
}

func (c *CLI) runWSLauncherWithSelectMode(args []string, forceSelect bool) int {
	return c.runWSLauncherWithSelectModeAndAction(args, forceSelect, "")
}

func (c *CLI) runWSLauncherWithSelectModeAndAction(args []string, forceSelect bool, explicitAction string) int {
	var archivedScope bool
	fixedAction := strings.TrimSpace(explicitAction)
	workspaceID := ""
	useCurrent := false
	selectMode := forceSelect
parseFlags:
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "--archived":
			archivedScope = true
			args = args[1:]
		case "--select":
			selectMode = true
			args = args[1:]
		case "--current":
			useCurrent = true
			args = args[1:]
		case "--id":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--id requires a value")
				c.printWSUsage(c.Err)
				return exitUsage
			}
			workspaceID = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--id=") {
				workspaceID = strings.TrimSpace(strings.TrimPrefix(args[0], "--id="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--select=") {
				v := strings.TrimSpace(strings.TrimPrefix(args[0], "--select="))
				if v != "" {
					fmt.Fprintln(c.Err, "--select does not take a value")
					c.printWSUsage(c.Err)
					return exitUsage
				}
				selectMode = true
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--current=") {
				v := strings.TrimSpace(strings.TrimPrefix(args[0], "--current="))
				if v != "" {
					fmt.Fprintln(c.Err, "--current does not take a value")
					c.printWSUsage(c.Err)
					return exitUsage
				}
				useCurrent = true
				args = args[1:]
				continue
			}
			if fixedAction != "" {
				break parseFlags
			}
			fmt.Fprintf(c.Err, "unknown flag for ws: %q\n", args[0])
			c.printWSUsage(c.Err)
			return exitUsage
		}
	}
	if fixedAction == "" && len(args) > 0 {
		action := strings.TrimSpace(args[0])
		switch action {
		case "open", "add-repo", "remove-repo", "close", "reopen", "purge", "unlock":
			fixedAction = action
			args = args[1:]
		default:
			if !strings.HasPrefix(action, "-") {
				fmt.Fprintf(c.Err, "unsupported action: %q\n", action)
				c.printWSUsage(c.Err)
				return exitUsage
			}
		}
	}
	if workspaceID != "" {
		if err := validateWorkspaceID(workspaceID); err != nil {
			fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
			return exitUsage
		}
	}
	if fixedAction != "" {
		switch fixedAction {
		case "open", "add-repo", "remove-repo", "close":
			if archivedScope {
				fmt.Fprintf(c.Err, "action %s cannot be used with --archived\n", fixedAction)
				c.printWSUsage(c.Err)
				return exitUsage
			}
		case "reopen", "purge", "unlock":
			archivedScope = true
		default:
			fmt.Fprintf(c.Err, "unsupported action: %q\n", fixedAction)
			c.printWSUsage(c.Err)
			return exitUsage
		}
	}
	actionArgs := append([]string{}, args...)
	if fixedAction == "" && len(actionArgs) > 0 {
		fmt.Fprintf(c.Err, "unexpected args for ws: %q\n", strings.Join(args, " "))
		c.printWSUsage(c.Err)
		return exitUsage
	}
	if workspaceID != "" && useCurrent {
		fmt.Fprintln(c.Err, "--id and --current cannot be used together")
		c.printWSUsage(c.Err)
		return exitUsage
	}
	if selectMode && workspaceID != "" {
		fmt.Fprintln(c.Err, "--select and --id cannot be used together")
		c.printWSUsage(c.Err)
		return exitUsage
	}
	if selectMode && useCurrent {
		fmt.Fprintln(c.Err, "--select and --current cannot be used together")
		c.printWSUsage(c.Err)
		return exitUsage
	}
	if useCurrent && archivedScope {
		fmt.Fprintln(c.Err, "--archived cannot be used with --current")
		c.printWSUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve KRA_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "ws-launcher"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	if fixedAction == "" && !selectMode && workspaceID == "" && !useCurrent {
		fmt.Fprintln(c.Err, "ws requires one of --id <id>, --current, or --select")
		c.printWSUsage(c.Err)
		return exitUsage
	}

	currentSelection := workspaceContextSelection{}
	currentResolved := false
	if useCurrent {
		resolved, ok := detectWorkspaceFromCWD(root, wd)
		if !ok {
			fmt.Fprintln(c.Err, "ws --current requires current path under workspaces/<id>/... or archive/<id>/...")
			return exitError
		}
		currentSelection = resolved
		currentResolved = true
	}

	if fixedAction != "" && !selectMode {
		if currentResolved {
			workspaceID = currentSelection.ID
			if currentSelection.Status == "archived" {
				archivedScope = true
			}
		}
		if workspaceID == "" && !runWSActionHasHelp(actionArgs) && !runWSActionHasIDArg(actionArgs) && !runWSActionHasPositional(actionArgs) {
			fmt.Fprintln(c.Err, "ws action requires one of --id <id>, --current, --select, or explicit action target")
			c.printWSUsage(c.Err)
			return exitUsage
		}
		return c.runWSFixedActionDirect(fixedAction, workspaceID, archivedScope, actionArgs)
	}

	scope := appws.ScopeActive
	if archivedScope {
		scope = appws.ScopeArchived
	}

	adapter := &cliWSLauncherAdapter{cli: c, root: root}
	usecase := appws.NewService(adapter, adapter)
	result, err := usecase.Run(context.Background(), appws.LauncherRequest{
		ForceSelect: selectMode,
		Scope:       scope,
		CurrentPath: func() string {
			if useCurrent {
				return wd
			}
			return ""
		}(),
		WorkspaceID: workspaceID,
		FixedAction: appws.Action(fixedAction),
	})
	if err != nil {
		if err == errSelectorCanceled {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
		switch {
		case errors.Is(err, appws.ErrWorkspaceNotSelected):
			fmt.Fprintln(c.Err, "ws target is not resolved (use one of: --id <id>, --current, --select)")
		case errors.Is(err, appws.ErrWorkspaceNotFound):
			fmt.Fprintf(c.Err, "workspace not found: %s\n", workspaceID)
		case errors.Is(err, appws.ErrActionNotAllowed):
			fmt.Fprintf(c.Err, "action %q is not allowed for selected scope\n", fixedAction)
			return exitError
		case err == errNoActiveWorkspaces:
			fmt.Fprintln(c.Err, "no active workspaces available")
		case err == errNoArchivedWorkspaces:
			fmt.Fprintln(c.Err, "no archived workspaces available")
		default:
			fmt.Fprintf(c.Err, "run ws launcher: %v\n", err)
		}
		return exitError
	}
	target := workspaceContextSelection{
		ID:     result.Workspace.ID,
		Status: string(result.Workspace.Status),
	}
	action := string(result.Action)
	c.debugf("ws launcher selected workspace=%s status=%s action=%s", target.ID, target.Status, action)

	switch action {
	case "open":
		return c.runWSOpen([]string{"--id", target.ID})
	case "add-repo":
		return c.runWSAddRepo([]string{target.ID})
	case "remove-repo":
		return c.runWSRemoveRepo([]string{target.ID})
	case "close":
		return c.runWSClose([]string{target.ID})
	case "reopen":
		return c.runWSReopen([]string{target.ID})
	case "unlock":
		return c.runWSUnlock([]string{target.ID})
	case "purge":
		return c.runWSPurge([]string{target.ID})
	default:
		return exitError
	}
}

func runWSActionHasIDArg(actionArgs []string) bool {
	for i := 0; i < len(actionArgs); i++ {
		arg := strings.TrimSpace(actionArgs[i])
		if arg == "--id" {
			return true
		}
		if strings.HasPrefix(arg, "--id=") {
			return true
		}
	}
	return false
}

func runWSActionHasPositional(actionArgs []string) bool {
	for _, arg := range actionArgs {
		if !strings.HasPrefix(strings.TrimSpace(arg), "-") {
			return true
		}
	}
	return false
}

func runWSActionHasHelp(actionArgs []string) bool {
	for _, arg := range actionArgs {
		v := strings.TrimSpace(arg)
		if v == "-h" || v == "--help" || v == "help" {
			return true
		}
	}
	return false
}

func (c *CLI) runWSFixedActionDirect(action string, workspaceID string, archivedScope bool, actionArgs []string) int {
	switch action {
	case "open", "add-repo", "remove-repo", "close":
		if archivedScope {
			c.printWSUsage(c.Err)
			return exitUsage
		}
	case "reopen", "purge", "unlock":
		archivedScope = true
	default:
		c.printWSUsage(c.Err)
		return exitUsage
	}

	opArgs := append([]string{}, actionArgs...)
	switch action {
	case "open", "add-repo", "remove-repo", "close":
		if workspaceID != "" && !runWSActionHasIDArg(opArgs) && !runWSActionHasPositional(opArgs) {
			opArgs = append([]string{"--id", workspaceID}, opArgs...)
		}
	case "reopen", "purge", "unlock":
		if workspaceID != "" && !runWSActionHasPositional(opArgs) {
			opArgs = append([]string{workspaceID}, opArgs...)
		}
	}

	switch action {
	case "open":
		return c.runWSOpen(opArgs)
	case "add-repo":
		return c.runWSAddRepo(opArgs)
	case "remove-repo":
		return c.runWSRemoveRepo(opArgs)
	case "close":
		return c.runWSClose(opArgs)
	case "reopen":
		return c.runWSReopen(opArgs)
	case "unlock":
		return c.runWSUnlock(opArgs)
	case "purge":
		return c.runWSPurge(opArgs)
	default:
		return exitUsage
	}
}

func (c *CLI) runWSSelectMulti(args []string) int {
	archivedScope := false
	fixedAction := ""
	doCommit := true
	commitModeExplicit := ""
	parseAction := func(next string) (string, bool) {
		v := strings.TrimSpace(next)
		if v == "" {
			fmt.Fprintln(c.Err, "action is required")
			c.printWSUsage(c.Err)
			return "", false
		}
		return v, true
	}

	for len(args) > 0 {
		cur := strings.TrimSpace(args[0])
		switch cur {
		case "--select":
			args = args[1:]
		case "--multi":
			args = args[1:]
		case "--commit":
			if commitModeExplicit == "no-commit" {
				fmt.Fprintln(c.Err, "--commit and --no-commit cannot be used together")
				c.printWSUsage(c.Err)
				return exitUsage
			}
			doCommit = true
			commitModeExplicit = "commit"
			args = args[1:]
		case "--no-commit":
			if commitModeExplicit == "commit" {
				fmt.Fprintln(c.Err, "--commit and --no-commit cannot be used together")
				c.printWSUsage(c.Err)
				return exitUsage
			}
			doCommit = false
			commitModeExplicit = "no-commit"
			args = args[1:]
		case "--archived":
			archivedScope = true
			args = args[1:]
		default:
			if cur == "--id" || strings.HasPrefix(cur, "--id=") {
				fmt.Fprintln(c.Err, "--select mode does not support --id (always starts from workspace selection)")
				c.printWSUsage(c.Err)
				return exitUsage
			}
			if strings.HasPrefix(cur, "-") {
				fmt.Fprintf(c.Err, "unknown flag for ws --select --multi: %q\n", cur)
				c.printWSUsage(c.Err)
				return exitUsage
			}
			if fixedAction != "" {
				fmt.Fprintf(c.Err, "unexpected args for ws --select --multi: %q\n", strings.Join(args, " "))
				c.printWSUsage(c.Err)
				return exitUsage
			}
			v, ok := parseAction(cur)
			if !ok {
				return exitUsage
			}
			fixedAction = v
			args = args[1:]
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(c.Err, "unexpected args for ws --select --multi: %q\n", strings.Join(args, " "))
		c.printWSUsage(c.Err)
		return exitUsage
	}

	if fixedAction == "" {
		fmt.Fprintln(c.Err, "--multi requires action")
		c.printWSUsage(c.Err)
		return exitUsage
	}
	switch fixedAction {
	case "close":
		if archivedScope {
			fmt.Fprintln(c.Err, "action close cannot be used with --archived in --multi mode")
			c.printWSUsage(c.Err)
			return exitUsage
		}
	case "reopen", "purge":
		archivedScope = true
	default:
		fmt.Fprintf(c.Err, "action %s does not support --multi\n", fixedAction)
		c.printWSUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve KRA_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "ws-select-multi"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}

	status := "active"
	if archivedScope {
		status = "archived"
	}
	ids, err := c.selectWorkspaceIDsByStatus(root, status, fixedAction)
	if err != nil {
		switch err {
		case errNoActiveWorkspaces:
			fmt.Fprintln(c.Err, "no active workspaces available")
		case errNoArchivedWorkspaces:
			fmt.Fprintln(c.Err, "no archived workspaces available")
		case errSelectorCanceled:
			fmt.Fprintln(c.Err, "aborted")
		default:
			fmt.Fprintf(c.Err, "run ws --select --multi: %v\n", err)
		}
		return exitError
	}
	if err := preflightWSSelectMultiAction(context.Background(), fixedAction, root, doCommit); err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}

	success := make([]string, 0, len(ids))
	failed := make([]string, 0, len(ids))
	for _, id := range ids {
		code := c.runWSSelectMultiActionByID(fixedAction, id, doCommit)
		if code == exitOK {
			success = append(success, id)
			continue
		}
		failed = append(failed, id)
	}
	if len(failed) > 0 {
		return exitError
	}
	return exitOK
}

func (c *CLI) runWSSelectMultiActionByID(action string, workspaceID string, doCommit bool) int {
	switch action {
	case "close":
		args := []string{"--id", workspaceID}
		if doCommit {
			args = append([]string{"--commit"}, args...)
		}
		return c.runWSClose(args)
	case "reopen":
		args := []string{workspaceID}
		if doCommit {
			args = append([]string{"--commit"}, args...)
		}
		return c.runWSReopen(args)
	case "purge":
		args := []string{workspaceID}
		if doCommit {
			args = append([]string{"--commit"}, args...)
		}
		return c.runWSPurge(args)
	default:
		return exitUsage
	}
}

func preflightWSSelectMultiAction(ctx context.Context, action string, root string, doCommit bool) error {
	if !doCommit {
		return nil
	}
	switch action {
	case "reopen":
		if err := ensureRootGitWorktree(ctx, root); err != nil {
			return err
		}
		return nil
	case "purge":
		if err := ensureRootGitWorktree(ctx, root); err != nil {
			return err
		}
		return nil
	default:
		return nil
	}
}

type cliWSLauncherAdapter struct {
	cli  *CLI
	root string
}

func (a *cliWSLauncherAdapter) SelectWorkspace(_ context.Context, scope appws.Scope, action string, _ bool) (string, error) {
	return a.cli.selectWorkspaceIDByStatus(a.root, string(scope), action)
}

func (a *cliWSLauncherAdapter) SelectAction(_ context.Context, workspace appws.WorkspaceRef, fromContext bool) (appws.Action, error) {
	action, err := a.cli.promptLauncherAction(workspaceContextSelection{
		ID:     workspace.ID,
		Status: string(workspace.Status),
	}, fromContext)
	if err != nil {
		return "", err
	}
	return appws.Action(action), nil
}

func (a *cliWSLauncherAdapter) ResolveFromPath(_ context.Context, path string) (appws.WorkspaceRef, bool, error) {
	out, ok := detectWorkspaceFromCWD(a.root, path)
	if !ok {
		return appws.WorkspaceRef{}, false, nil
	}
	return appws.WorkspaceRef{
		ID:     out.ID,
		Status: appws.Scope(out.Status),
	}, true, nil
}

func (a *cliWSLauncherAdapter) ResolveByID(ctx context.Context, id string) (appws.WorkspaceRef, bool, error) {
	status, ok, err := lookupWorkspaceStatusByID(ctx, a.root, id)
	if err != nil {
		return appws.WorkspaceRef{}, false, err
	}
	if !ok {
		return appws.WorkspaceRef{}, false, nil
	}
	return appws.WorkspaceRef{ID: id, Status: appws.Scope(status)}, true, nil
}

func (c *CLI) selectWorkspaceIDByStatus(root string, status string, action string) (string, error) {
	ctx := context.Background()
	c.debugf("ws launcher load candidates status=%s action=%s", status, action)
	start := time.Now()
	candidates, err := listWorkspaceCandidatesByStatus(ctx, root, status)
	elapsedMs := time.Since(start).Milliseconds()
	if err != nil {
		c.debugf("ws launcher load candidates failed status=%s action=%s elapsed_ms=%d err=%v", status, action, elapsedMs, err)
		return "", err
	}
	c.debugf("ws launcher load candidates done status=%s action=%s count=%d elapsed_ms=%d", status, action, len(candidates), elapsedMs)
	if len(candidates) == 0 {
		if status == "archived" {
			return "", errNoArchivedWorkspaces
		}
		return "", errNoActiveWorkspaces
	}
	selectStart := time.Now()
	ids, err := c.promptWorkspaceSelectorSingle(status, action, candidates)
	selectElapsedMs := time.Since(selectStart).Milliseconds()
	if err != nil {
		c.debugf("ws launcher prompt selector failed status=%s action=%s elapsed_ms=%d err=%v", status, action, selectElapsedMs, err)
		return "", err
	}
	c.debugf("ws launcher prompt selector done status=%s action=%s elapsed_ms=%d selected=%v", status, action, selectElapsedMs, ids)
	return ids[0], nil
}

func (c *CLI) selectWorkspaceIDsByStatus(root string, status string, action string) ([]string, error) {
	ctx := context.Background()
	c.debugf("ws launcher load candidates status=%s action=%s multi=true", status, action)
	start := time.Now()
	candidates, err := listWorkspaceCandidatesByStatus(ctx, root, status)
	elapsedMs := time.Since(start).Milliseconds()
	if err != nil {
		c.debugf("ws launcher load candidates failed status=%s action=%s elapsed_ms=%d err=%v", status, action, elapsedMs, err)
		return nil, err
	}
	c.debugf("ws launcher load candidates done status=%s action=%s count=%d elapsed_ms=%d", status, action, len(candidates), elapsedMs)
	if len(candidates) == 0 {
		if status == "archived" {
			return nil, errNoArchivedWorkspaces
		}
		return nil, errNoActiveWorkspaces
	}
	selectStart := time.Now()
	ids, err := c.promptWorkspaceSelector(status, action, candidates)
	selectElapsedMs := time.Since(selectStart).Milliseconds()
	if err != nil {
		c.debugf("ws launcher prompt selector failed status=%s action=%s multi=true elapsed_ms=%d err=%v", status, action, selectElapsedMs, err)
		return nil, err
	}
	c.debugf("ws launcher prompt selector done status=%s action=%s multi=true elapsed_ms=%d selected=%v", status, action, selectElapsedMs, ids)
	return ids, nil
}

func lookupWorkspaceStatusByID(ctx context.Context, root string, workspaceID string) (string, bool, error) {
	_ = ctx
	activePath := filepath.Join(root, "workspaces", workspaceID)
	if fi, err := os.Stat(activePath); err == nil {
		if fi.IsDir() {
			return "active", true, nil
		}
	} else if err != nil && !os.IsNotExist(err) {
		return "", false, fmt.Errorf("stat active workspace path: %w", err)
	}

	archivedPath := filepath.Join(root, "archive", workspaceID)
	if fi, err := os.Stat(archivedPath); err == nil {
		if fi.IsDir() {
			return "archived", true, nil
		}
	} else if err != nil && !os.IsNotExist(err) {
		return "", false, fmt.Errorf("stat archived workspace path: %w", err)
	}
	return "", false, nil
}

func (c *CLI) promptLauncherAction(target workspaceContextSelection, _ bool) (string, error) {
	actions := make([]workspaceSelectorCandidate, 0, 3)
	switch target.Status {
	case "active":
		actions = append(actions,
			workspaceSelectorCandidate{ID: "open", Description: "open workspace runtime"},
			workspaceSelectorCandidate{ID: "add-repo", Description: "add repositories"},
			workspaceSelectorCandidate{ID: "remove-repo", Description: "remove repositories"},
			workspaceSelectorCandidate{ID: "close", Description: "archive this workspace"},
		)
	case "archived":
		actions = append(actions,
			workspaceSelectorCandidate{ID: "reopen", Description: "restore workspace"},
			workspaceSelectorCandidate{ID: "unlock", Description: "disable purge guard"},
			workspaceSelectorCandidate{ID: "purge", Description: "delete permanently"},
		)
	default:
		return "", fmt.Errorf("unsupported workspace status: %s", target.Status)
	}

	useColor := writerSupportsColor(c.Err)
	title := renderActionSelectorTitle(target.ID, useColor)
	ids, err := c.promptWorkspaceSelectorWithOptionsAndMode(target.Status, "run", title, "action", actions, true)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

func renderActionSelectorTitle(workspaceID string, useColor bool) string {
	label := "workspace"
	if useColor {
		label = styleAccent(label, useColor)
	}
	return fmt.Sprintf("Action:\n%s%s: %s", uiIndent, label, workspaceID)
}
