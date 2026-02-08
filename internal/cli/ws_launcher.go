package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
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
	var archivedScope bool
	var forceSelect bool
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "--archived":
			archivedScope = true
			args = args[1:]
		case "--select":
			forceSelect = true
			args = args[1:]
		default:
			fmt.Fprintf(c.Err, "unknown flag for ws: %q\n", args[0])
			c.printWSUsage(c.Err)
			return exitUsage
		}
	}
	if len(args) > 0 {
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
		fmt.Fprintf(c.Err, "resolve GIONX_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "ws-launcher"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}

	target, fromContext := workspaceContextSelection{}, false
	if !forceSelect {
		if current, ok := detectWorkspaceFromCWD(root, wd); ok {
			target = current
			fromContext = true
		}
	}

	if !fromContext {
		scope := "active"
		if archivedScope {
			scope = "archived"
		}
		id, err := c.selectWorkspaceIDByStatus(root, scope, "select")
		if err != nil {
			switch {
			case err == errNoActiveWorkspaces:
				fmt.Fprintln(c.Err, "no active workspaces available")
			case err == errNoArchivedWorkspaces:
				fmt.Fprintln(c.Err, "no archived workspaces available")
			case err == errSelectorCanceled:
				fmt.Fprintln(c.Err, "aborted")
			default:
				fmt.Fprintf(c.Err, "select workspace: %v\n", err)
			}
			return exitError
		}
		target = workspaceContextSelection{ID: id, Status: scope}
	}

	action, err := c.promptLauncherAction(target, fromContext)
	if err != nil {
		if err == errSelectorCanceled {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
		fmt.Fprintf(c.Err, "select action: %v\n", err)
		return exitError
	}
	c.debugf("ws launcher selected workspace=%s status=%s action=%s fromContext=%t", target.ID, target.Status, action, fromContext)

	switch action {
	case "go":
		if target.Status == "archived" {
			return c.runWSGo([]string{"--ui", "--archived", target.ID})
		}
		return c.runWSGo([]string{"--ui", target.ID})
	case "add-repo":
		return c.runWSAddRepo([]string{target.ID})
	case "close":
		return c.runWSClose([]string{target.ID})
	case "reopen":
		return c.runWSReopen([]string{target.ID})
	case "purge":
		return c.runWSPurge([]string{target.ID})
	default:
		return exitError
	}
}

func (c *CLI) selectWorkspaceIDByStatus(root string, status string, action string) (string, error) {
	ctx := context.Background()
	dbPath, err := paths.StateDBPathForRoot(root)
	if err != nil {
		return "", fmt.Errorf("resolve state db path: %w", err)
	}
	repoPoolPath, err := paths.DefaultRepoPoolPath()
	if err != nil {
		return "", fmt.Errorf("resolve repo pool path: %w", err)
	}
	db, err := statestore.Open(ctx, dbPath)
	if err != nil {
		return "", fmt.Errorf("open state store: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := statestore.EnsureSettings(ctx, db, root, repoPoolPath); err != nil {
		return "", fmt.Errorf("initialize settings: %w", err)
	}

	candidates, err := listWorkspaceCandidatesByStatus(ctx, db, root, status)
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		if status == "archived" {
			return "", errNoArchivedWorkspaces
		}
		return "", errNoActiveWorkspaces
	}
	ids, err := c.promptWorkspaceSelectorSingle(status, action, candidates)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

func (c *CLI) promptLauncherAction(target workspaceContextSelection, fromContext bool) (string, error) {
	actions := make([]workspaceSelectorCandidate, 0, 3)
	switch target.Status {
	case "active":
		if fromContext {
			actions = append(actions,
				workspaceSelectorCandidate{ID: "add-repo"},
				workspaceSelectorCandidate{ID: "close"},
			)
		} else {
			actions = append(actions,
				workspaceSelectorCandidate{ID: "go"},
				workspaceSelectorCandidate{ID: "add-repo"},
				workspaceSelectorCandidate{ID: "close"},
			)
		}
	case "archived":
		actions = append(actions,
			workspaceSelectorCandidate{ID: "reopen"},
			workspaceSelectorCandidate{ID: "purge"},
		)
	default:
		return "", fmt.Errorf("unsupported workspace status: %s", target.Status)
	}

	title := fmt.Sprintf("Action:\n%sworkspace: %s", uiIndent, target.ID)
	ids, err := c.promptWorkspaceSelectorWithOptionsAndMode(target.Status, "run", title, "action", actions, true)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}
