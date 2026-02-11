package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (c *CLI) createWorkspaceAtRoot(root string, id string, title string, sourceURL string) (string, error) {
	wsPath := filepath.Join(root, "workspaces", id)
	archivedPath := filepath.Join(root, "archive", id)

	if _, err := os.Stat(wsPath); err == nil {
		return "", fmt.Errorf("workspace directory already exists: %s", wsPath)
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat workspace dir: %w", err)
	}
	if _, err := os.Stat(archivedPath); err == nil {
		return "", fmt.Errorf("workspace already exists and is archived: %s\nrun: gionx ws --act reopen %s", id, id)
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat archived workspace dir: %w", err)
	}

	createdDir := false
	if err := os.Mkdir(wsPath, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("workspace directory already exists: %s", wsPath)
		}
		return "", fmt.Errorf("create workspace dir: %w", err)
	}
	createdDir = true
	cleanup := func() {
		if createdDir {
			_ = os.RemoveAll(wsPath)
		}
	}

	if err := os.Mkdir(filepath.Join(wsPath, "notes"), 0o755); err != nil {
		cleanup()
		return "", fmt.Errorf("create notes/: %w", err)
	}
	if err := os.Mkdir(filepath.Join(wsPath, "artifacts"), 0o755); err != nil {
		cleanup()
		return "", fmt.Errorf("create artifacts/: %w", err)
	}
	if err := os.WriteFile(filepath.Join(wsPath, "AGENTS.md"), []byte(defaultWorkspaceAgentsContent(id, title)), 0o644); err != nil {
		cleanup()
		return "", fmt.Errorf("write AGENTS.md: %w", err)
	}

	now := time.Now().Unix()
	meta := newWorkspaceMetaFileForCreate(id, title, sourceURL, now)
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		cleanup()
		return "", fmt.Errorf("write %s: %w", workspaceMetaFilename, err)
	}
	return wsPath, nil
}
