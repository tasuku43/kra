package appports

import (
	"fmt"

	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/paths"
)

type RepoPort struct {
	EnsureDebugLogFn func(root string, tag string) error
	TouchRegistryFn  func(root string) error
}

func NewRepoPort(
	ensureDebugLogFn func(root string, tag string) error,
	touchRegistryFn func(root string) error,
) *RepoPort {
	return &RepoPort{
		EnsureDebugLogFn: ensureDebugLogFn,
		TouchRegistryFn:  touchRegistryFn,
	}
}

func (p *RepoPort) EnsureGitInPath() error {
	if err := gitutil.EnsureGitInPath(); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func (p *RepoPort) ResolveRoot(cwd string) (string, error) {
	root, err := paths.ResolveExistingRoot(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve GIONX_ROOT: %w", err)
	}
	return root, nil
}

func (p *RepoPort) EnsureDebugLog(root string, tag string) error {
	if p.EnsureDebugLogFn == nil {
		return nil
	}
	if err := p.EnsureDebugLogFn(root, tag); err != nil {
		return fmt.Errorf("enable debug logging: %w", err)
	}
	return nil
}

func (p *RepoPort) ResolveRepoPoolPath() (string, error) {
	repoPoolPath, err := paths.DefaultRepoPoolPath()
	if err != nil {
		return "", fmt.Errorf("resolve repo pool path: %w", err)
	}
	return repoPoolPath, nil
}

func (p *RepoPort) TouchRegistry(root string) error {
	if p.TouchRegistryFn != nil {
		if err := p.TouchRegistryFn(root); err != nil {
			return fmt.Errorf("update root registry: %w", err)
		}
		return nil
	}
	if err := TouchStateRegistry(root); err != nil {
		return fmt.Errorf("update root registry: %w", err)
	}
	return nil
}
