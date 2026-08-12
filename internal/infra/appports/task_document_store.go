package appports

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/filelock"
)

const taskDocumentLockTimeout = 10 * time.Second

func saveTaskDocumentCAS(snapshot wstask.DocumentSnapshot, content string, conflictMessage string) error {
	root := taskDocumentRoot(snapshot.Path)
	operationsDir := filepath.Join(root, ".kra", "state", "operations", "ws-task")
	pathHash := sha256.Sum256([]byte(filepath.Clean(snapshot.Path)))
	lockPath := filepath.Join(operationsDir, fmt.Sprintf("%x.lock", pathHash))
	ctx, cancel := context.WithTimeout(context.Background(), taskDocumentLockTimeout)
	defer cancel()
	release, err := filelock.Acquire(ctx, lockPath)
	if err != nil {
		return fmt.Errorf("lock workspace document: %w", err)
	}
	defer release()

	mode := os.FileMode(0o644)
	current, err := os.ReadFile(snapshot.Path)
	switch {
	case err == nil:
		if !snapshot.Exists || string(current) != snapshot.Content {
			return fmt.Errorf("%w: %s", wstask.ErrConflict, conflictMessage)
		}
		if info, statErr := os.Stat(snapshot.Path); statErr == nil {
			mode = info.Mode().Perm()
		}
	case errors.Is(err, os.ErrNotExist):
		if snapshot.Exists {
			return fmt.Errorf("%w: %s", wstask.ErrConflict, conflictMessage)
		}
	default:
		return fmt.Errorf("read workspace document before write: %w", err)
	}

	if err := os.MkdirAll(operationsDir, 0o755); err != nil {
		return fmt.Errorf("create workspace document temp dir: %w", err)
	}
	tmpFile, err := os.CreateTemp(operationsDir, "workspace-document.tmp-*")
	if err != nil {
		return fmt.Errorf("create workspace document temp: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmpFile.Chmod(mode); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("chmod workspace document temp: %w", err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write workspace document temp: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close workspace document temp: %w", err)
	}
	if err := os.Rename(tmpPath, snapshot.Path); err != nil {
		return fmt.Errorf("replace workspace document: %w", err)
	}
	return nil
}

func taskDocumentRoot(path string) string {
	documentDir := filepath.Dir(filepath.Clean(path))
	scopeDir := filepath.Dir(documentDir)
	switch filepath.Base(scopeDir) {
	case "workspaces", "archive":
		return filepath.Dir(scopeDir)
	default:
		return documentDir
	}
}
