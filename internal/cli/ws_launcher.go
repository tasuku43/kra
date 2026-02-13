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
	return c.runWSLauncherWithSelectMode(args, false)
}

func (c *CLI) runWSLauncherWithSelectMode(args []string, forceSelect bool) int {
	var archivedScope bool
	fixedAction := ""
	workspaceID := ""
parseFlags:
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "--archived":
			archivedScope = true
			args = args[1:]
		case "--act":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--act requires a value")
				c.printWSUsage(c.Err)
				return exitUsage
			}
			fixedAction = strings.TrimSpace(args[1])
			args = args[2:]
		case "--id":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--id requires a value")
				c.printWSUsage(c.Err)
				return exitUsage
			}
			workspaceID = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--act=") {
				fixedAction = strings.TrimSpace(strings.TrimPrefix(args[0], "--act="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--id=") {
				workspaceID = strings.TrimSpace(strings.TrimPrefix(args[0], "--id="))
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
	if workspaceID != "" {
		if err := validateWorkspaceID(workspaceID); err != nil {
			fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
			return exitUsage
		}
	}
	if fixedAction != "" {
		switch fixedAction {
		case "go", "add-repo", "remove-repo", "close":
			if archivedScope {
				fmt.Fprintf(c.Err, "--act %s cannot be used with --archived\n", fixedAction)
				c.printWSUsage(c.Err)
				return exitUsage
			}
		case "reopen", "purge", "unlock":
			archivedScope = true
		default:
			fmt.Fprintf(c.Err, "unsupported --act: %q\n", fixedAction)
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
	if fixedAction != "" && !forceSelect {
		return c.runWSFixedActionDirect(fixedAction, workspaceID, archivedScope, wd, root, actionArgs)
	}

	scope := appws.ScopeActive
	if archivedScope {
		scope = appws.ScopeArchived
	}

	adapter := &cliWSLauncherAdapter{cli: c, root: root}
	usecase := appws.NewService(adapter, adapter)
	result, err := usecase.Run(context.Background(), appws.LauncherRequest{
		ForceSelect: forceSelect,
		Scope:       scope,
		CurrentPath: wd,
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
			fmt.Fprintln(c.Err, "ws requires --id <id> or workspace context (use: kra ws select)")
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
	case "go":
		if target.Status == "archived" {
			return c.runWSGo([]string{"--ui", "--archived", target.ID})
		}
		return c.runWSGo([]string{"--ui", target.ID})
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

func (c *CLI) runWSFixedActionDirect(action string, workspaceID string, archivedScope bool, wd string, root string, actionArgs []string) int {
	switch action {
	case "go", "add-repo", "remove-repo", "close":
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

	if workspaceID == "" {
		if fromCWD, ok := detectWorkspaceFromCWD(root, wd); ok {
			workspaceID = fromCWD.ID
		}
	}

	opArgs := append([]string{}, actionArgs...)
	switch action {
	case "go", "add-repo", "remove-repo", "close":
		if workspaceID != "" && !runWSActionHasIDArg(opArgs) && !runWSActionHasPositional(opArgs) {
			opArgs = append([]string{"--id", workspaceID}, opArgs...)
		}
	case "reopen", "purge", "unlock":
		if workspaceID != "" && !runWSActionHasPositional(opArgs) {
			opArgs = append([]string{workspaceID}, opArgs...)
		}
	}

	switch action {
	case "go":
		if archivedScope {
			hasArchived := false
			for _, arg := range opArgs {
				if strings.TrimSpace(arg) == "--archived" {
					hasArchived = true
					break
				}
			}
			if !hasArchived {
				opArgs = append([]string{"--archived"}, opArgs...)
			}
		}
		return c.runWSGo(opArgs)
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

func (c *CLI) runWSSelect(args []string) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		c.printWSUsage(c.Out)
		return exitOK
	}
	hasMulti := false
	for _, arg := range args {
		if strings.TrimSpace(arg) == "--multi" {
			hasMulti = true
			break
		}
	}
	if hasMulti {
		return c.runWSSelectMulti(args)
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--id" || strings.HasPrefix(args[i], "--id=") {
			fmt.Fprintln(c.Err, "ws select does not support --id (always starts from workspace selection)")
			c.printWSUsage(c.Err)
			return exitUsage
		}
	}
	return c.runWSLauncherWithSelectMode(args, true)
}

func (c *CLI) runWSSelectMulti(args []string) int {
	archivedScope := false
	fixedAction := ""
	doCommit := false
	parseAct := func(next string) (string, bool) {
		v := strings.TrimSpace(next)
		if v == "" {
			fmt.Fprintln(c.Err, "--act requires a value")
			c.printWSUsage(c.Err)
			return "", false
		}
		return v, true
	}

	for len(args) > 0 {
		cur := strings.TrimSpace(args[0])
		switch cur {
		case "--multi":
			args = args[1:]
		case "--commit":
			doCommit = true
			args = args[1:]
		case "--archived":
			archivedScope = true
			args = args[1:]
		case "--act":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--act requires a value")
				c.printWSUsage(c.Err)
				return exitUsage
			}
			v, ok := parseAct(args[1])
			if !ok {
				return exitUsage
			}
			fixedAction = v
			args = args[2:]
		default:
			if strings.HasPrefix(cur, "--act=") {
				v, ok := parseAct(strings.TrimPrefix(cur, "--act="))
				if !ok {
					return exitUsage
				}
				fixedAction = v
				args = args[1:]
				continue
			}
			if cur == "--id" || strings.HasPrefix(cur, "--id=") {
				fmt.Fprintln(c.Err, "ws select does not support --id (always starts from workspace selection)")
				c.printWSUsage(c.Err)
				return exitUsage
			}
			fmt.Fprintf(c.Err, "unknown flag for ws select: %q\n", cur)
			c.printWSUsage(c.Err)
			return exitUsage
		}
	}

	if fixedAction == "" {
		fmt.Fprintln(c.Err, "--multi requires --act")
		c.printWSUsage(c.Err)
		return exitUsage
	}
	switch fixedAction {
	case "close":
		if archivedScope {
			fmt.Fprintln(c.Err, "--act close cannot be used with --archived in --multi mode")
			c.printWSUsage(c.Err)
			return exitUsage
		}
	case "reopen", "purge":
		archivedScope = true
	default:
		fmt.Fprintf(c.Err, "--act %s does not support --multi\n", fixedAction)
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
			fmt.Fprintf(c.Err, "run ws select --multi: %v\n", err)
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
		return ensureNoStagedChangesForReopen(ctx, root)
	case "purge":
		if err := ensureRootGitWorktree(ctx, root); err != nil {
			return err
		}
		return ensureNoStagedChanges(ctx, root)
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

func (c *CLI) promptLauncherAction(target workspaceContextSelection, fromContext bool) (string, error) {
	actions := make([]workspaceSelectorCandidate, 0, 3)
	switch target.Status {
	case "active":
		if fromContext {
			actions = append(actions,
				workspaceSelectorCandidate{ID: "add-repo", Description: "add repositories"},
				workspaceSelectorCandidate{ID: "remove-repo", Description: "remove repositories"},
				workspaceSelectorCandidate{ID: "close", Description: "archive this workspace"},
			)
		} else {
			actions = append(actions,
				workspaceSelectorCandidate{ID: "go", Description: "switch to workspace"},
				workspaceSelectorCandidate{ID: "add-repo", Description: "add repositories"},
				workspaceSelectorCandidate{ID: "remove-repo", Description: "remove repositories"},
				workspaceSelectorCandidate{ID: "close", Description: "archive this workspace"},
			)
		}
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
