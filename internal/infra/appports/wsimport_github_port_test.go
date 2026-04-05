package appports

import (
	"context"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/app/wsimport"
)

func TestWSImportGitHubPort_SearchIssues_UsesScopeAndState(t *testing.T) {
	var calls [][]string
	port := NewWSImportGitHubPortWithRunner(func(ctx context.Context, args ...string) (string, error) {
		calls = append(calls, append([]string{}, args...))
		return `[{"number":101,"title":"Fix retry","url":"https://github.com/my-org/api/issues/101","repository":{"nameWithOwner":"my-org/api"}}]`, nil
	})

	items, err := port.SearchIssues(context.Background(), wsimport.GitHubScope{Kind: "org", Value: "my-org"}, "closed", 20)
	if err != nil {
		t.Fatalf("SearchIssues() error = %v", err)
	}
	if len(items) != 1 || items[0].Repository != "my-org/api" || items[0].Number != 101 {
		t.Fatalf("SearchIssues() items = %+v, want parsed issue", items)
	}
	got := strings.Join(calls[0], " ")
	if !strings.Contains(got, "search issues") || !strings.Contains(got, "--owner my-org") || !strings.Contains(got, "--state closed") {
		t.Fatalf("gh args = %q, want owner + state search", got)
	}
}

func TestWSImportGitHubPort_SearchReviewPullRequests_FetchesHeadRef(t *testing.T) {
	var calls [][]string
	port := NewWSImportGitHubPortWithRunner(func(ctx context.Context, args ...string) (string, error) {
		calls = append(calls, append([]string{}, args...))
		if len(args) >= 2 && args[0] == "api" && args[1] == "graphql" {
			return `{"data":{"search":{"nodes":[{"number":456,"title":"Add retry budget","url":"https://github.com/my-org/api/pull/456","isDraft":true,"headRefName":"feature/retry-budget","repository":{"nameWithOwner":"my-org/api"}}]}}}`, nil
		}
		return ``, nil
	})

	items, err := port.SearchReviewPullRequests(context.Background(), wsimport.GitHubScope{Kind: "repo", Value: "my-org/api"}, 10)
	if err != nil {
		t.Fatalf("SearchReviewPullRequests() error = %v", err)
	}
	if len(items) != 1 || items[0].HeadRef != "feature/retry-budget" || !items[0].IsDraft {
		t.Fatalf("SearchReviewPullRequests() items = %+v, want parsed pr with head ref", items)
	}
	searchArgs := strings.Join(calls[0], " ")
	if len(calls) != 1 {
		t.Fatalf("calls len = %d, want 1", len(calls))
	}
	if !strings.Contains(searchArgs, "api graphql") || !strings.Contains(searchArgs, "user-review-requested:@me") || !strings.Contains(searchArgs, "repo:my-org/api") {
		t.Fatalf("search args = %q, want graphql user-review-requested repo search", searchArgs)
	}
}
