package appports

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tasuku43/kra/internal/app/wstask"
)

func TestWSTaskPortSave_ConcurrentCASAllowsOnlyOneWriter(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	path := filepath.Join(workspaceDir, workspaceMDFilename)
	const original = "# original\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	port := NewWSTaskPort()
	snapshot, err := port.Load(root, "WS1", "active")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, content := range []string{"# writer one\n", "# writer two\n"} {
		content := content
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- port.Save(snapshot, content)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	succeeded := 0
	conflicted := 0
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, wstask.ErrConflict):
			conflicted++
		default:
			t.Fatalf("Save() unexpected error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("Save() outcomes: succeeded=%d conflicted=%d, want 1/1", succeeded, conflicted)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("document mode = %o, want 640", got)
	}
}

func TestRootTaskPortSave_UsesRootOperationsDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, workspaceMDFilename)
	port := NewRootTaskPort()
	snapshot := wstask.DocumentSnapshot{Path: path, Exists: false, Source: wstask.SourceMD}
	if err := port.Save(snapshot, "# root\n"); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	locks, err := filepath.Glob(filepath.Join(root, ".kra", "state", "operations", "ws-task", "*.lock"))
	if err != nil {
		t.Fatalf("Glob() error: %v", err)
	}
	if len(locks) != 1 {
		t.Fatalf("lock files = %v, want one under root operations", locks)
	}
}
