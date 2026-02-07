package repodiscovery

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/tasuku43/gion-core/repospec"
)

type ghRunner func(ctx context.Context, args ...string) (string, error)

type GitHubGHProvider struct {
	run ghRunner
}

func NewGitHubGHProvider(run ghRunner) *GitHubGHProvider {
	if run == nil {
		run = runGHCommand
	}
	return &GitHubGHProvider{run: run}
}

func (p *GitHubGHProvider) Name() string {
	return "github"
}

func (p *GitHubGHProvider) CheckAuth(ctx context.Context) error {
	if _, err := p.run(ctx, "auth", "status"); err != nil {
		return fmt.Errorf("github authentication required (run 'gh auth login'): %w", err)
	}
	return nil
}

func (p *GitHubGHProvider) ListOrgRepos(ctx context.Context, org string) ([]Repo, error) {
	org = strings.TrimSpace(org)
	if org == "" {
		return nil, fmt.Errorf("org is required")
	}

	endpoint := fmt.Sprintf("/orgs/%s/repos?per_page=100&type=all", org)
	out, err := p.run(ctx, "api", "--paginate", endpoint, "--jq", ".[] | [.full_name,.ssh_url,.clone_url] | @tsv")
	if err != nil {
		return nil, fmt.Errorf("list github repos for org %s: %w", org, err)
	}

	byRepoUID := map[string]Repo{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 1 {
			continue
		}
		fullName := strings.TrimSpace(parts[0])
		if fullName == "" || !strings.Contains(fullName, "/") {
			continue
		}
		sshURL := ""
		cloneURL := ""
		if len(parts) >= 2 {
			sshURL = strings.TrimSpace(parts[1])
		}
		if len(parts) >= 3 {
			cloneURL = strings.TrimSpace(parts[2])
		}

		remoteURL := sshURL
		if remoteURL == "" {
			remoteURL = cloneURL
		}
		if remoteURL == "" {
			remoteURL = fmt.Sprintf("git@github.com:%s.git", fullName)
		}

		spec, err := repospec.Normalize(remoteURL)
		if err != nil {
			return nil, fmt.Errorf("normalize discovered repo %q: %w", remoteURL, err)
		}
		repoUID := fmt.Sprintf("%s/%s/%s", spec.Host, spec.Owner, spec.Repo)
		repoKey := fmt.Sprintf("%s/%s", spec.Owner, spec.Repo)
		byRepoUID[repoUID] = Repo{
			RepoUID:   repoUID,
			RepoKey:   repoKey,
			RemoteURL: remoteURL,
		}
	}

	repos := make([]Repo, 0, len(byRepoUID))
	for _, r := range byRepoUID {
		repos = append(repos, r)
	}
	slices.SortFunc(repos, func(a, b Repo) int {
		return strings.Compare(a.RepoKey, b.RepoKey)
	})
	return repos, nil
}

func runGHCommand(ctx context.Context, args ...string) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh not found in PATH: %w", err)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("gh %s failed: %w (output=%s)", strings.Join(args, " "), err, s)
	}
	return s, nil
}
