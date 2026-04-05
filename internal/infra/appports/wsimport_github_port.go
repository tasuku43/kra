package appports

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/app/wsimport"
	"github.com/tasuku43/kra/internal/procexec"
)

type wsImportGitHubGHRunner func(ctx context.Context, args ...string) (string, error)

type WSImportGitHubPort struct {
	run wsImportGitHubGHRunner
}

type wsImportGitHubSearchItem struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	IsDraft    bool   `json:"isDraft"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
		Name          string `json:"name"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

type wsImportGitHubGraphQLResponse struct {
	Data struct {
		Search struct {
			Nodes []struct {
				Number      int    `json:"number"`
				Title       string `json:"title"`
				URL         string `json:"url"`
				IsDraft     bool   `json:"isDraft"`
				HeadRefName string `json:"headRefName"`
				Repository  struct {
					NameWithOwner string `json:"nameWithOwner"`
				} `json:"repository"`
			} `json:"nodes"`
		} `json:"search"`
	} `json:"data"`
}

const wsImportGitHubCommandTimeout = 2 * time.Minute

func NewWSImportGitHubPort() *WSImportGitHubPort {
	return NewWSImportGitHubPortWithRunner(runWSImportGitHubGHCommand)
}

func NewWSImportGitHubPortWithRunner(run func(ctx context.Context, args ...string) (string, error)) *WSImportGitHubPort {
	if run == nil {
		run = runWSImportGitHubGHCommand
	}
	return &WSImportGitHubPort{run: run}
}

func (p *WSImportGitHubPort) CheckAuth(ctx context.Context) error {
	if _, err := p.run(ctx, "auth", "status"); err != nil {
		return fmt.Errorf("github authentication required (run 'gh auth login'): %w", err)
	}
	return nil
}

func (p *WSImportGitHubPort) SearchIssues(ctx context.Context, scope wsimport.GitHubScope, state string, maxResults int) ([]wsimport.GitHubIssue, error) {
	args := []string{"search", "issues", "--limit", fmt.Sprintf("%d", maxResults), "--archived=false"}
	switch strings.TrimSpace(scope.Kind) {
	case "org":
		args = append(args, "--owner", strings.TrimSpace(scope.Value))
	case "repo":
		args = append(args, "--repo", strings.TrimSpace(scope.Value))
	default:
		return nil, fmt.Errorf("unsupported github scope kind: %s", scope.Kind)
	}
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "open":
		args = append(args, "--state", "open")
	case "closed":
		args = append(args, "--state", "closed")
	case "all":
	default:
		return nil, fmt.Errorf("unsupported github issue state: %s", state)
	}
	args = append(args, "--json", "number,title,url,repository")
	out, err := p.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("search github issues: %w", err)
	}
	items, err := decodeWSImportGitHubSearchItems(out)
	if err != nil {
		return nil, fmt.Errorf("decode github issues: %w", err)
	}
	result := make([]wsimport.GitHubIssue, 0, len(items))
	for _, item := range items {
		result = append(result, wsimport.GitHubIssue{
			Repository: wsImportGitHubRepoName(item),
			Number:     item.Number,
			Title:      item.Title,
			URL:        item.URL,
		})
	}
	return result, nil
}

func (p *WSImportGitHubPort) SearchReviewPullRequests(ctx context.Context, scope wsimport.GitHubScope, maxResults int) ([]wsimport.GitHubPullRequest, error) {
	searchQuery, err := buildWSImportGitHubReviewQuery(scope)
	if err != nil {
		return nil, err
	}
	out, err := p.run(
		ctx,
		"api", "graphql",
		"-f", "query="+wsImportGitHubReviewGraphQLQuery,
		"-F", "q="+searchQuery,
		"-F", fmt.Sprintf("limit=%d", maxResults),
	)
	if err != nil {
		return nil, fmt.Errorf("search github review pull requests: %w", err)
	}
	var resp wsImportGitHubGraphQLResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("decode github pull requests: %w", err)
	}
	result := make([]wsimport.GitHubPullRequest, 0, len(resp.Data.Search.Nodes))
	for _, item := range resp.Data.Search.Nodes {
		repo := strings.TrimSpace(item.Repository.NameWithOwner)
		if repo == "" || item.Number <= 0 {
			continue
		}
		result = append(result, wsimport.GitHubPullRequest{
			Repository: repo,
			Number:     item.Number,
			Title:      item.Title,
			URL:        item.URL,
			IsDraft:    item.IsDraft,
			HeadRef:    strings.TrimSpace(item.HeadRefName),
		})
	}
	return result, nil
}

func decodeWSImportGitHubSearchItems(raw string) ([]wsImportGitHubSearchItem, error) {
	var items []wsImportGitHubSearchItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	return items, nil
}

func buildWSImportGitHubReviewQuery(scope wsimport.GitHubScope) (string, error) {
	terms := []string{"is:pr", "state:open", "archived:false", "user-review-requested:@me"}
	switch strings.TrimSpace(scope.Kind) {
	case "org":
		terms = append(terms, "org:"+strings.TrimSpace(scope.Value))
	case "repo":
		terms = append(terms, "repo:"+strings.TrimSpace(scope.Value))
	default:
		return "", fmt.Errorf("unsupported github scope kind: %s", scope.Kind)
	}
	return strings.Join(terms, " "), nil
}

const wsImportGitHubReviewGraphQLQuery = `
query($q: String!, $limit: Int!) {
  search(query: $q, type: ISSUE, first: $limit) {
    nodes {
      ... on PullRequest {
        number
        title
        url
        isDraft
        headRefName
        repository {
          nameWithOwner
        }
      }
    }
  }
}
`

func wsImportGitHubRepoName(item wsImportGitHubSearchItem) string {
	if strings.TrimSpace(item.Repository.NameWithOwner) != "" {
		return strings.TrimSpace(item.Repository.NameWithOwner)
	}
	if strings.TrimSpace(item.Repository.Owner.Login) != "" && strings.TrimSpace(item.Repository.Name) != "" {
		return fmt.Sprintf("%s/%s", strings.TrimSpace(item.Repository.Owner.Login), strings.TrimSpace(item.Repository.Name))
	}
	return strings.TrimSpace(item.Repository.Name)
}

func runWSImportGitHubGHCommand(ctx context.Context, args ...string) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh not found in PATH: %w", err)
	}
	out, err := procexec.RunCombined(ctx, "", "gh", wsImportGitHubCommandTimeout, args...)
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("gh %s failed: %w (output=%s)", strings.Join(args, " "), err, s)
	}
	return s, nil
}
