package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// StateDBPathForRoot returns the per-root location for the SQLite state store,
// following XDG conventions.
func StateDBPathForRoot(root string) (string, error) {
	cleanRoot, err := cleanAbs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}

	xdgDataHome, err := XDGDataHome()
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(cleanRoot))
	rootHash := hex.EncodeToString(sum[:16])
	return filepath.Join(xdgDataHome, "gionx", "roots", rootHash, "state.db"), nil
}

// DefaultRepoPoolPath returns the default location for the bare repo pool,
// following XDG conventions.
func DefaultRepoPoolPath() (string, error) {
	xdgCacheHome, err := XDGCacheHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(xdgCacheHome, "gionx", "repo-pool"), nil
}

// RegistryPath returns the global registry metadata path.
func RegistryPath() (string, error) {
	xdgDataHome, err := XDGDataHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(xdgDataHome, "gionx", "registry.json"), nil
}

// XDGDataHome resolves $XDG_DATA_HOME or falls back to ~/.local/share.
func XDGDataHome() (string, error) {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return cleanAbs(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for XDG_DATA_HOME fallback: %w", err)
	}
	return filepath.Join(home, ".local", "share"), nil
}

// XDGCacheHome resolves $XDG_CACHE_HOME or falls back to ~/.cache.
func XDGCacheHome() (string, error) {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return cleanAbs(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for XDG_CACHE_HOME fallback: %w", err)
	}
	return filepath.Join(home, ".cache"), nil
}

// ResolveExistingRoot resolves the current GIONX_ROOT.
//
// Resolution order:
//  1. $GIONX_ROOT (must look like a gionx root)
//  2. Walk up from startDir looking for a gionx root
func ResolveExistingRoot(startDir string) (string, error) {
	if envRoot := os.Getenv("GIONX_ROOT"); envRoot != "" {
		root, err := cleanAbs(envRoot)
		if err != nil {
			return "", fmt.Errorf("resolve $GIONX_ROOT: %w", err)
		}
		if !LooksLikeRoot(root) {
			return "", fmt.Errorf("$GIONX_ROOT does not look like a gionx root: %s", root)
		}
		return root, nil
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
	if path == "" {
		return "", errors.New("empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
