package repodiscovery

import (
	"context"
	"fmt"
	"strings"
)

type Repo struct {
	RepoUID   string
	RepoKey   string
	RemoteURL string
}

type Provider interface {
	Name() string
	CheckAuth(ctx context.Context) error
	ListOrgRepos(ctx context.Context, org string) ([]Repo, error)
}

func NewProvider(name string) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "github":
		return NewGitHubGHProvider(nil), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %q", name)
	}
}
