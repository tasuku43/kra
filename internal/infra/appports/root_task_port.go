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
	return saveTaskDocumentCAS(snapshot, content, "root workspace.md changed during update")
}
