package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
)

func (c *CLI) runWSCreate(args []string) int {
	var noPrompt bool
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSCreateUsage(c.Out)
			return exitOK
		case "--no-prompt":
			noPrompt = true
			args = args[1:]
		default:
			fmt.Fprintf(c.Err, "unknown flag for ws create: %q\n", args[0])
			c.printWSCreateUsage(c.Err)
			return exitUsage
		}
	}

	if len(args) == 0 {
		c.printWSCreateUsage(c.Err)
		return exitUsage
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws create: %q\n", strings.Join(args[1:], " "))
		c.printWSCreateUsage(c.Err)
		return exitUsage
	}

	id := args[0]
	if err := validateWorkspaceID(id); err != nil {
		fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
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
	if err := c.ensureDebugLog(root, "ws-create"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws create id=%s noPrompt=%t", id, noPrompt)

	ctx := context.Background()
	dbPath, err := paths.StateDBPathForRoot(root)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve state db path: %v\n", err)
		return exitError
	}
	repoPoolPath, err := paths.DefaultRepoPoolPath()
	if err != nil {
		fmt.Fprintf(c.Err, "resolve repo pool path: %v\n", err)
		return exitError
	}

	db, err := statestore.Open(ctx, dbPath)
	if err != nil {
		fmt.Fprintf(c.Err, "open state store: %v\n", err)
		return exitError
	}
	defer func() { _ = db.Close() }()

	if err := statestore.EnsureSettings(ctx, db, root, repoPoolPath); err != nil {
		fmt.Fprintf(c.Err, "initialize settings: %v\n", err)
		return exitError
	}
	if err := c.touchStateRegistry(root, dbPath); err != nil {
		fmt.Fprintf(c.Err, "update state registry: %v\n", err)
		return exitError
	}

	if status, ok, err := statestore.LookupWorkspaceStatus(ctx, db, id); err != nil {
		fmt.Fprintf(c.Err, "load workspace: %v\n", err)
		return exitError
	} else if ok {
		switch status {
		case "active":
			fmt.Fprintf(c.Err, "workspace already exists and is active: %s\n", id)
			return exitError
		case "archived":
			fmt.Fprintf(c.Err, "workspace already exists and is archived: %s\nrun: gionx ws reopen %s\n", id, id)
			return exitError
		default:
			fmt.Fprintf(c.Err, "workspace already exists with unknown status %q: %s\n", status, id)
			return exitError
		}
	}

	description := ""
	if !noPrompt {
		d, err := c.promptLine("description: ")
		if err != nil {
			fmt.Fprintf(c.Err, "read description: %v\n", err)
			return exitError
		}
		description = d
	}

	wsPath := filepath.Join(root, "workspaces", id)
	if _, err := os.Stat(wsPath); err == nil {
		fmt.Fprintf(c.Err, "workspace directory already exists: %s\n", wsPath)
		return exitError
	} else if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(c.Err, "stat workspace dir: %v\n", err)
		return exitError
	}

	createdDir := false
	if err := os.Mkdir(wsPath, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			fmt.Fprintf(c.Err, "workspace directory already exists: %s\n", wsPath)
			return exitError
		}
		fmt.Fprintf(c.Err, "create workspace dir: %v\n", err)
		return exitError
	}
	createdDir = true
	cleanup := func() {
		if createdDir {
			_ = os.RemoveAll(wsPath)
		}
	}

	if err := os.Mkdir(filepath.Join(wsPath, "notes"), 0o755); err != nil {
		cleanup()
		fmt.Fprintf(c.Err, "create notes/: %v\n", err)
		return exitError
	}
	if err := os.Mkdir(filepath.Join(wsPath, "artifacts"), 0o755); err != nil {
		cleanup()
		fmt.Fprintf(c.Err, "create artifacts/: %v\n", err)
		return exitError
	}
	if err := os.WriteFile(filepath.Join(wsPath, "AGENTS.md"), []byte(defaultWorkspaceAgentsContent(id, description)), 0o644); err != nil {
		cleanup()
		fmt.Fprintf(c.Err, "write AGENTS.md: %v\n", err)
		return exitError
	}

	now := time.Now().Unix()
	if _, err := statestore.CreateWorkspace(ctx, db, statestore.CreateWorkspaceInput{
		ID:          id,
		Description: description,
		SourceURL:   "",
		Now:         now,
	}); err != nil {
		cleanup()
		var existsErr *statestore.WorkspaceAlreadyExistsError
		if errors.As(err, &existsErr) {
			switch existsErr.Status {
			case "active":
				fmt.Fprintf(c.Err, "workspace already exists and is active: %s\n", id)
				return exitError
			case "archived":
				fmt.Fprintf(c.Err, "workspace already exists and is archived: %s\nrun: gionx ws reopen %s\n", id, id)
				return exitError
			default:
				fmt.Fprintf(c.Err, "workspace already exists with unknown status %q: %s\n", existsErr.Status, id)
				return exitError
			}
		}
		fmt.Fprintf(c.Err, "create workspace in state store: %v\n", err)
		return exitError
	}

	fmt.Fprintf(c.Out, "created: %s\n", wsPath)
	c.debugf("ws create completed id=%s path=%s", id, wsPath)
	return exitOK
}

func (c *CLI) promptLine(prompt string) (string, error) {
	if prompt != "" {
		fmt.Fprint(c.Err, prompt)
	}
	if c.inReader == nil {
		c.inReader = bufio.NewReader(c.In)
	}
	line, err := c.inReader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func validateWorkspaceID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.ContainsAny(id, "/\\") {
		return fmt.Errorf("must not contain path separators")
	}
	return nil
}

func defaultWorkspaceAgentsContent(id string, description string) string {
	desc := strings.TrimSpace(description)
	if desc == "" {
		desc = "(empty)"
	}
	return fmt.Sprintf(`# workspace AGENTS guide

## Workspace

- ID: %s
- Description: %s

## Directory map

- notes/: investigation notes, decisions, TODOs, links
- artifacts/: files and evidence (screenshots, logs, dumps, PoCs)
- repos/: git worktrees (NOT Git-tracked; added via gionx ws add-repo)

Notes vs artifacts:
- notes/: write what you learned and decided
- artifacts/: store evidence files you may need later

## Closing

When you are done, run:
  gionx ws close %s
`, id, desc, id)
}
