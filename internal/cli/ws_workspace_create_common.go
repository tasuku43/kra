package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (c *CLI) createWorkspaceAtRoot(root string, id string, title string, sourceURL string, templateName string) (string, error) {
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

	if strings.TrimSpace(templateName) == "" {
		templateName = defaultWorkspaceTemplateName
	}
	tmpl, err := resolveWorkspaceTemplate(root, templateName)
	if err != nil {
		return "", err
	}
	violations, err := validateWorkspaceTemplate(tmpl)
	if err != nil {
		return "", err
	}
	if len(violations) > 0 {
		return "", fmt.Errorf("template validation failed:\n%s", renderWorkspaceTemplateViolations(violations))
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

	if err := copyWorkspaceTemplate(tmpl, wsPath); err != nil {
		cleanup()
		return "", fmt.Errorf("copy template %q: %w", tmpl.Name, err)
	}

	now := time.Now().Unix()
	meta := newWorkspaceMetaFileForCreate(id, title, sourceURL, now)
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		cleanup()
		return "", fmt.Errorf("write %s: %w", workspaceMetaFilename, err)
	}
	return wsPath, nil
}
