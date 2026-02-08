package cli

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
)

type repoAppAdapter struct {
	cli *CLI
}

func (a *repoAppAdapter) EnsureGitInPath() error {
	if err := gitutil.EnsureGitInPath(); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func (a *repoAppAdapter) ResolveRoot(cwd string) (string, error) {
	root, err := paths.ResolveExistingRoot(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve GIONX_ROOT: %w", err)
	}
	return root, nil
}

func (a *repoAppAdapter) EnsureDebugLog(root string, tag string) error {
	if err := a.cli.ensureDebugLog(root, tag); err != nil {
		return fmt.Errorf("enable debug logging: %w", err)
	}
	return nil
}

func (a *repoAppAdapter) ResolveStateDBPath(root string) (string, error) {
	dbPath, err := paths.StateDBPathForRoot(root)
	if err != nil {
		return "", fmt.Errorf("resolve state db path: %w", err)
	}
	return dbPath, nil
}

func (a *repoAppAdapter) ResolveRepoPoolPath() (string, error) {
	repoPoolPath, err := paths.DefaultRepoPoolPath()
	if err != nil {
		return "", fmt.Errorf("resolve repo pool path: %w", err)
	}
	return repoPoolPath, nil
}

func (a *repoAppAdapter) OpenState(ctx context.Context, dbPath string) (*sql.DB, error) {
	db, err := statestore.Open(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("open state store: %w", err)
	}
	return db, nil
}

func (a *repoAppAdapter) EnsureSettings(ctx context.Context, db *sql.DB, root string, repoPoolPath string) error {
	if err := statestore.EnsureSettings(ctx, db, root, repoPoolPath); err != nil {
		return fmt.Errorf("initialize settings: %w", err)
	}
	return nil
}

func (a *repoAppAdapter) TouchRegistry(root string) error {
	if err := a.cli.touchStateRegistry(root); err != nil {
		return fmt.Errorf("update root registry: %w", err)
	}
	return nil
}
