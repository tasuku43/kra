package repodiscovery

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewProvider_DefaultsToGitHub(t *testing.T) {
	p, err := NewProvider("")
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}
	if p.Name() != "github" {
		t.Fatalf("provider name = %q, want %q", p.Name(), "github")
	}
}

func TestNewProvider_Unsupported(t *testing.T) {
	_, err := NewProvider("gitlab")
	if err == nil {
		t.Fatalf("NewProvider() error = nil, want error")
	}
}

func TestGitHubGHProvider_CheckAuthError(t *testing.T) {
	p := NewGitHubGHProvider(func(ctx context.Context, args ...string) (string, error) {
		return "", errors.New("no auth")
	})
	if err := p.CheckAuth(context.Background()); err == nil {
		t.Fatalf("CheckAuth() error = nil, want error")
	}
}

func TestGitHubGHProvider_ListOrgRepos(t *testing.T) {
	p := NewGitHubGHProvider(func(ctx context.Context, args ...string) (string, error) {
		if len(args) < 4 {
			t.Fatalf("unexpected args: %v", args)
		}
		if args[0] != "api" || args[1] != "--paginate" {
			t.Fatalf("unexpected args head: %v", args)
		}
		return strings.Join([]string{
			"example-org/helmfiles\tgit@github.com:example-org/helmfiles.git\thttps://github.com/example-org/helmfiles.git",
			"example-org/infra\t\thttps://github.com/example-org/infra.git",
			"example-org/helmfiles\tgit@github.com:example-org/helmfiles.git\thttps://github.com/example-org/helmfiles.git",
		}, "\n"), nil
	})

	repos, err := p.ListOrgRepos(context.Background(), "example-org")
	if err != nil {
		t.Fatalf("ListOrgRepos() error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("len(repos) = %d, want 2", len(repos))
	}
	if repos[0].RepoKey != "example-org/helmfiles" {
		t.Fatalf("repos[0].RepoKey = %q", repos[0].RepoKey)
	}
	if repos[1].RepoKey != "example-org/infra" {
		t.Fatalf("repos[1].RepoKey = %q", repos[1].RepoKey)
	}
}
