package appports

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tasuku43/kra/internal/app/wstask"
)

type WSTaskPort struct{}

func NewWSTaskPort() *WSTaskPort {
	return &WSTaskPort{}
}

func (p *WSTaskPort) Load(root string, workspaceID string, scope string) (wstask.DocumentSnapshot, error) {
	workspacePath, err := wsTaskWorkspacePath(root, workspaceID, scope)
	if err != nil {
		return wstask.DocumentSnapshot{}, err
	}
	taskPath := filepath.Join(workspacePath, "tasks.md")
	content, err := os.ReadFile(taskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return wstask.DocumentSnapshot{
				Path:   taskPath,
				Exists: false,
			}, nil
		}
		return wstask.DocumentSnapshot{}, fmt.Errorf("read workspace task document: %w", err)
	}
	return wstask.DocumentSnapshot{
		Path:    taskPath,
		Exists:  true,
		Content: string(content),
	}, nil
}

func (p *WSTaskPort) Save(path string, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write workspace task document: %w", err)
	}
	return nil
}

func wsTaskWorkspacePath(root string, workspaceID string, scope string) (string, error) {
	base := filepath.Join(root, "workspaces", workspaceID)
	if scope == "archived" {
		base = filepath.Join(root, "archive", workspaceID)
	}
	fi, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", wstask.ErrWorkspaceNotFound, workspaceID)
		}
		return "", fmt.Errorf("stat workspace path: %w", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("%w: %s", wstask.ErrWorkspaceNotFound, workspaceID)
	}
	return base, nil
}
