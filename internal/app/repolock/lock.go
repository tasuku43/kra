package repolock

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tasuku43/kra/internal/filelock"
)

const lockFileName = "repo-pool.lock"

// Path returns the root-local lock shared by repo-pool and worktree operations.
// The lock lives under KRA_ROOT so host and isolated processes see the same
// lock file when they share a mounted root.
func Path(root string) string {
	return filepath.Join(root, ".kra", "state", "operations", lockFileName)
}

// Acquire serializes repo-pool, worktree, and root-repo-registry mutations for
// one KRA_ROOT. The returned release function is idempotent.
func Acquire(ctx context.Context, root string) (func(), error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("acquire repo pool lock: root is empty")
	}
	release, err := filelock.Acquire(ctx, Path(root))
	if err != nil {
		return nil, fmt.Errorf("acquire repo pool lock for %s: %w", root, err)
	}
	return release, nil
}
