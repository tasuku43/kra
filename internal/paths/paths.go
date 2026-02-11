package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const gionxHomeEnv = "GIONX_HOME"

// DefaultRepoPoolPath returns the default location for the bare repo pool.
func DefaultRepoPoolPath() (string, error) {
	home, err := GionxHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "repo-pool"), nil
}

// RegistryPath returns the global registry metadata path.
func RegistryPath() (string, error) {
	home, err := GionxHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "state", "root-registry.json"), nil
}

// CurrentContextPath returns the active context pointer file path.
func CurrentContextPath() (string, error) {
	home, err := GionxHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "state", "current-context"), nil
}

// ConfigPath returns the global config path.
func ConfigPath() (string, error) {
	home, err := GionxHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config.yaml"), nil
}

// GionxHomeDir resolves $GIONX_HOME or falls back to ~/.gionx.
func GionxHomeDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv(gionxHomeEnv)); v != "" {
		return cleanAbs(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for %s fallback: %w", gionxHomeEnv, err)
	}
	return filepath.Join(home, ".gionx"), nil
}

// ResolveExistingRoot resolves the current root.
//
// Resolution order:
//  1. current-context file (must look like a gionx root)
//  2. Walk up from startDir looking for a gionx root
func ResolveExistingRoot(startDir string) (string, error) {
	contextRoot, ok, err := ReadCurrentContext()
	if err != nil {
		return "", err
	}
	if ok {
		if !LooksLikeRoot(contextRoot) {
			return "", fmt.Errorf("current context does not look like a gionx root: %s", contextRoot)
		}
		return contextRoot, nil
	}

	return FindRoot(startDir)
}

// FindRoot walks up from startDir and returns the nearest directory that
// "looks like" a gionx root.
func FindRoot(startDir string) (string, error) {
	start, err := cleanAbs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve start dir: %w", err)
	}

	dir := start
	for {
		if LooksLikeRoot(dir) {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("GIONX_ROOT not found (walked up to filesystem root)")
		}
		dir = parent
	}
}

// LooksLikeRoot returns true when dir appears to be a gionx root directory.
//
// Current heuristic: both "workspaces/" and "archive/" exist and are directories.
func LooksLikeRoot(dir string) bool {
	if !isDir(filepath.Join(dir, "workspaces")) {
		return false
	}
	if !isDir(filepath.Join(dir, "archive")) {
		return false
	}
	return true
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

func cleanAbs(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// ReadCurrentContext loads the current context root path from current-context file.
func ReadCurrentContext() (string, bool, error) {
	contextPath, err := CurrentContextPath()
	if err != nil {
		return "", false, fmt.Errorf("resolve current-context path: %w", err)
	}
	b, err := os.ReadFile(contextPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read current-context file %s: %w", contextPath, err)
	}
	line := strings.TrimSpace(string(b))
	if line == "" {
		return "", false, fmt.Errorf("current-context file is empty: %s", contextPath)
	}
	root, err := cleanAbs(line)
	if err != nil {
		return "", false, fmt.Errorf("resolve current-context root in %s: %w", contextPath, err)
	}
	if !isDir(root) {
		return "", false, fmt.Errorf("current-context points to missing directory: %s (run 'gionx context use <root>' or remove %s)", root, contextPath)
	}
	return root, true, nil
}

// WriteCurrentContext updates the current-context file atomically.
func WriteCurrentContext(root string) error {
	root, err := cleanAbs(root)
	if err != nil {
		return fmt.Errorf("resolve context root: %w", err)
	}
	contextPath, err := CurrentContextPath()
	if err != nil {
		return fmt.Errorf("resolve current-context path: %w", err)
	}
	dir := filepath.Dir(contextPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create current-context dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".current-context-*.tmp")
	if err != nil {
		return fmt.Errorf("create current-context temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if _, err := tmp.WriteString(root + "\n"); err != nil {
		cleanup()
		return fmt.Errorf("write current-context temp file %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close current-context temp file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, contextPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace current-context file %s: %w", contextPath, err)
	}
	return nil
}
