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
			"chatwork/helmfiles\tgit@github.com:chatwork/helmfiles.git\thttps://github.com/chatwork/helmfiles.git",
			"chatwork/infra\t\thttps://github.com/chatwork/infra.git",
			"chatwork/helmfiles\tgit@github.com:chatwork/helmfiles.git\thttps://github.com/chatwork/helmfiles.git",
		}, "\n"), nil
	})

	repos, err := p.ListOrgRepos(context.Background(), "chatwork")
	if err != nil {
		t.Fatalf("ListOrgRepos() error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("len(repos) = %d, want 2", len(repos))
	}
	if repos[0].RepoKey != "chatwork/helmfiles" {
		t.Fatalf("repos[0].RepoKey = %q", repos[0].RepoKey)
	}
	if repos[1].RepoKey != "chatwork/infra" {
		t.Fatalf("repos[1].RepoKey = %q", repos[1].RepoKey)
	}
}
