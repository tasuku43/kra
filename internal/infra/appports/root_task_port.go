package appports

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tasuku43/kra/internal/app/wstask"
)

type RootTaskPort struct{}

func NewRootTaskPort() *RootTaskPort {
	return &RootTaskPort{}
}

func (p *RootTaskPort) Load(root string, _ string, _ string) (wstask.DocumentSnapshot, error) {
	taskPath := filepath.Join(root, "workspace.md")
	content, err := os.ReadFile(taskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return wstask.DocumentSnapshot{
				Path:   taskPath,
				Exists: false,
				Source: wstask.SourceMD,
			}, nil
		}
		return wstask.DocumentSnapshot{}, fmt.Errorf("read root workspace.md: %w", err)
	}
	return wstask.DocumentSnapshot{
		Path:    taskPath,
		Exists:  true,
		Content: string(content),
		Source:  wstask.SourceMD,
	}, nil
}

func (p *RootTaskPort) Save(snapshot wstask.DocumentSnapshot, content string) error {
	switch current, err := os.ReadFile(snapshot.Path); {
	case err == nil:
		if string(current) != snapshot.Content {
			return fmt.Errorf("%w: root workspace.md changed during update", wstask.ErrConflict)
		}
	case os.IsNotExist(err):
		if snapshot.Exists {
			return fmt.Errorf("%w: root workspace.md changed during update", wstask.ErrConflict)
		}
	default:
		return fmt.Errorf("read root workspace.md before write: %w", err)
	}
	if err := os.WriteFile(snapshot.Path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write root workspace.md: %w", err)
	}
	return nil
}
