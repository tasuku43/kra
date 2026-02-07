package repodiscovery

import (
	"context"
	"errors"
	"fmt"
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

func TestGitHubGHProvider_ListOrgRepos_FallbackToUserEndpointOn404(t *testing.T) {
	callCount := 0
	p := NewGitHubGHProvider(func(ctx context.Context, args ...string) (string, error) {
		callCount++
		if callCount == 1 {
			if got := args[2]; !strings.HasPrefix(got, "/orgs/tasuku43/repos") {
				t.Fatalf("1st endpoint = %q", got)
			}
			return "", fmt.Errorf("gh: Not Found (HTTP 404)")
		}
		if got := args[2]; !strings.HasPrefix(got, "/users/tasuku43/repos") {
			t.Fatalf("2nd endpoint = %q", got)
		}
		return "tasuku43/gionx\tgit@github.com:tasuku43/gionx.git\thttps://github.com/tasuku43/gionx.git", nil
	})

	repos, err := p.ListOrgRepos(context.Background(), "tasuku43")
	if err != nil {
		t.Fatalf("ListOrgRepos() error: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("callCount = %d, want 2", callCount)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
	if repos[0].RepoKey != "tasuku43/gionx" {
		t.Fatalf("RepoKey = %q", repos[0].RepoKey)
	}
}
