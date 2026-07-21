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
		if args[0] == "config" {
			return "https", nil
		}
		if len(args) < 4 {
			t.Fatalf("unexpected args: %v", args)
		}
		if args[0] != "api" || args[1] != "--paginate" {
			t.Fatalf("unexpected args head: %v", args)
		}
		if got := args[len(args)-1]; got != ".[] | [.full_name,.clone_url] | @tsv" {
			t.Fatalf("jq expression = %q, want HTTPS clone URL field only", got)
		}
		return strings.Join([]string{
			"example-org/helmfiles\thttps://github.com/example-org/helmfiles.git",
			"example-org/infra\thttps://github.com/example-org/infra.git",
			"example-org/helmfiles\thttps://github.com/example-org/helmfiles.git",
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
	if repos[0].RemoteURL != "https://github.com/example-org/helmfiles.git" {
		t.Fatalf("repos[0].RemoteURL = %q, want HTTPS clone URL", repos[0].RemoteURL)
	}
	if repos[1].RepoKey != "example-org/infra" {
		t.Fatalf("repos[1].RepoKey = %q", repos[1].RepoKey)
	}
}

func TestGitHubGHProvider_ListOrgRepos_FallbackToUserEndpointOn404(t *testing.T) {
	apiCallCount := 0
	p := NewGitHubGHProvider(func(ctx context.Context, args ...string) (string, error) {
		if args[0] == "config" {
			return "https", nil
		}
		apiCallCount++
		if apiCallCount == 1 {
			if got := args[2]; !strings.HasPrefix(got, "/orgs/tasuku43/repos") {
				t.Fatalf("1st endpoint = %q", got)
			}
			return "", fmt.Errorf("gh: Not Found (HTTP 404)")
		}
		if got := args[2]; !strings.HasPrefix(got, "/users/tasuku43/repos") {
			t.Fatalf("2nd endpoint = %q", got)
		}
		return "tasuku43/kra\thttps://github.com/tasuku43/kra.git", nil
	})

	repos, err := p.ListOrgRepos(context.Background(), "tasuku43")
	if err != nil {
		t.Fatalf("ListOrgRepos() error: %v", err)
	}
	if apiCallCount != 2 {
		t.Fatalf("apiCallCount = %d, want 2", apiCallCount)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
	if repos[0].RepoKey != "tasuku43/kra" {
		t.Fatalf("RepoKey = %q", repos[0].RepoKey)
	}
	if repos[0].RemoteURL != "https://github.com/tasuku43/kra.git" {
		t.Fatalf("RemoteURL = %q, want HTTPS clone URL", repos[0].RemoteURL)
	}
}

func TestGitHubGHProvider_ListOrgRepos_DerivesHTTPSURLWhenCloneURLIsMissing(t *testing.T) {
	p := NewGitHubGHProvider(func(ctx context.Context, args ...string) (string, error) {
		if args[0] == "config" {
			return "https", nil
		}
		return "example-org/private-repo", nil
	})

	repos, err := p.ListOrgRepos(context.Background(), "example-org")
	if err != nil {
		t.Fatalf("ListOrgRepos() error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
	if repos[0].RemoteURL != "https://github.com/example-org/private-repo.git" {
		t.Fatalf("RemoteURL = %q, want derived HTTPS clone URL", repos[0].RemoteURL)
	}
}

func TestGitHubGHProvider_ListOrgRepos_UsesSSHWhenConfigured(t *testing.T) {
	p := NewGitHubGHProvider(func(ctx context.Context, args ...string) (string, error) {
		if args[0] == "config" {
			if got := strings.Join(args, " "); got != "config get git_protocol --host github.com" {
				t.Fatalf("config args = %q", got)
			}
			return "ssh", nil
		}
		if got := args[len(args)-1]; got != ".[] | [.full_name,.ssh_url] | @tsv" {
			t.Fatalf("jq expression = %q, want SSH clone URL field", got)
		}
		return "example-org/private-repo\tgit@github.com:example-org/private-repo.git", nil
	})

	repos, err := p.ListOrgRepos(context.Background(), "example-org")
	if err != nil {
		t.Fatalf("ListOrgRepos() error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
	if repos[0].RemoteURL != "git@github.com:example-org/private-repo.git" {
		t.Fatalf("RemoteURL = %q, want configured SSH clone URL", repos[0].RemoteURL)
	}
}

func TestGitHubGHProvider_ListOrgRepos_FallsBackToHTTPSWhenProtocolConfigFails(t *testing.T) {
	p := NewGitHubGHProvider(func(ctx context.Context, args ...string) (string, error) {
		if args[0] == "config" {
			return "", errors.New("config unavailable")
		}
		if got := args[len(args)-1]; got != ".[] | [.full_name,.clone_url] | @tsv" {
			t.Fatalf("jq expression = %q, want HTTPS fallback", got)
		}
		return "example-org/private-repo\thttps://github.com/example-org/private-repo.git", nil
	})

	repos, err := p.ListOrgRepos(context.Background(), "example-org")
	if err != nil {
		t.Fatalf("ListOrgRepos() error: %v", err)
	}
	if len(repos) != 1 || repos[0].RemoteURL != "https://github.com/example-org/private-repo.git" {
		t.Fatalf("repos = %+v, want HTTPS fallback", repos)
	}
}
