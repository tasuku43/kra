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
		if len(args) >= 2 && args[0] == "search" && args[1] == "prs" {
			return `[{"number":456,"title":"Add retry budget","url":"https://github.com/my-org/api/pull/456","isDraft":true,"repository":{"nameWithOwner":"my-org/api"}}]`, nil
		}
		if len(args) >= 2 && args[0] == "pr" && args[1] == "view" {
			return `{"headRefName":"feature/retry-budget"}`, nil
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
	viewArgs := strings.Join(calls[1], " ")
	if !strings.Contains(searchArgs, "--review-requested @me") || !strings.Contains(searchArgs, "--repo my-org/api") {
		t.Fatalf("search args = %q, want review-requested repo search", searchArgs)
	}
	if !strings.Contains(viewArgs, "pr view 456 --repo my-org/api --json headRefName") {
		t.Fatalf("view args = %q, want headRefName lookup", viewArgs)
	}
}
