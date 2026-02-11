package jira

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_SearchIssuesByJQL_Success(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/rest/api/3/search/jql")
		}
		if got := r.URL.Query().Get("jql"); got != `assignee=currentUser()` {
			t.Fatalf("jql = %q, want %q", got, `assignee=currentUser()`)
		}
		_, _ = w.Write([]byte(`{"issues":[{"key":"PROJ-1","fields":{"summary":"First"}},{"key":"PROJ-2","fields":{"summary":"Second"}}]}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	client := NewClient()
	issues, err := client.SearchIssuesByJQL(context.Background(), `assignee=currentUser()`, 10)
	if err != nil {
		t.Fatalf("SearchIssuesByJQL() error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("issues len = %d, want 2", len(issues))
	}
	if issues[0].Key != "PROJ-1" || issues[0].Summary != "First" {
		t.Fatalf("issue[0] = %#v", issues[0])
	}
	if !strings.HasSuffix(issues[0].TicketURL, "/browse/PROJ-1") {
		t.Fatalf("ticket url = %q", issues[0].TicketURL)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("dev@example.com:token-123"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, wantAuth)
	}
}

func TestClient_ListScrumBoards_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/board" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/rest/agile/1.0/board")
		}
		if got := r.URL.Query().Get("type"); got != "scrum" {
			t.Fatalf("type query = %q, want scrum", got)
		}
		_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":10,"name":"Board A","type":"scrum","location":{"projectKey":"DEMO"}}]}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	client := NewClient()
	boards, err := client.ListScrumBoards(context.Background())
	if err != nil {
		t.Fatalf("ListScrumBoards() error: %v", err)
	}
	if len(boards) != 1 || boards[0].ID != 10 || boards[0].Name != "Board A" || boards[0].ProjectKey != "DEMO" {
		t.Fatalf("boards = %#v", boards)
	}
}

func TestClient_ListBoardSprintsActiveFuture_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/board/10/sprint" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/rest/agile/1.0/board/10/sprint")
		}
		if got := r.URL.Query().Get("state"); got != "active,future" {
			t.Fatalf("state query = %q", got)
		}
		_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":101,"name":"Sprint 101","state":"active","originBoardId":10}]}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	client := NewClient()
	sprints, err := client.ListBoardSprintsActiveFuture(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListBoardSprintsActiveFuture() error: %v", err)
	}
	if len(sprints) != 1 || sprints[0].ID != 101 || sprints[0].OriginBoardID != 10 {
		t.Fatalf("sprints = %#v", sprints)
	}
}

func TestClient_ListProjectOpenSprints_UsesJQL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/rest/api/3/search/jql")
		}
		jql := r.URL.Query().Get("jql")
		if !strings.Contains(jql, "project = DEMO") || !strings.Contains(jql, "openSprints()") || !strings.Contains(jql, "futureSprints()") {
			t.Fatalf("jql = %q", jql)
		}
		if got := r.URL.Query().Get("fields"); got != "*all" {
			t.Fatalf("fields query = %q, want %q", got, "*all")
		}
		_, _ = w.Write([]byte(`{"issues":[{"fields":{"sprint":{"id":16,"name":"DEMO スプリント 16","state":"active","originBoardId":98}}}]}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	client := NewClient()
	sprints, err := client.ListProjectOpenSprints(context.Background(), "DEMO", 50)
	if err != nil {
		t.Fatalf("ListProjectOpenSprints() error: %v", err)
	}
	if len(sprints) != 1 || sprints[0].ID != 16 {
		t.Fatalf("sprints = %#v", sprints)
	}
}

func TestClient_ListProjectOpenSprints_ParsesCustomFieldSprint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/rest/api/3/search/jql")
		}
		_, _ = w.Write([]byte(`{"issues":[{"fields":{"customfield_10020":[{"id":16,"name":"DEMO スプリント 16","state":"active","originBoardId":98}]}}]}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	client := NewClient()
	sprints, err := client.ListProjectOpenSprints(context.Background(), "DEMO", 50)
	if err != nil {
		t.Fatalf("ListProjectOpenSprints() error: %v", err)
	}
	if len(sprints) != 1 || sprints[0].ID != 16 || sprints[0].Name != "DEMO スプリント 16" {
		t.Fatalf("sprints = %#v", sprints)
	}
}

func TestClient_ListProjectOpenSprints_ParsesLegacySprintString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/rest/api/3/search/jql")
		}
		_, _ = w.Write([]byte(`{"issues":[{"fields":{"customfield_10020":["com.atlassian.greenhopper.service.sprint.Sprint@1f8[id=16,rapidViewId=98,state=ACTIVE,name=DEMO Sprint 16]"]}}]}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	client := NewClient()
	sprints, err := client.ListProjectOpenSprints(context.Background(), "DEMO", 50)
	if err != nil {
		t.Fatalf("ListProjectOpenSprints() error: %v", err)
	}
	if len(sprints) != 1 || sprints[0].ID != 16 || sprints[0].Name != "DEMO Sprint 16" || sprints[0].OriginBoardID != 98 || sprints[0].State != "active" {
		t.Fatalf("sprints = %#v", sprints)
	}
}

func TestClient_ListProjectOpenSprints_ParsesStringIDAndBoardID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/rest/api/3/search/jql")
		}
		_, _ = w.Write([]byte(`{"issues":[{"fields":{"customfield_10020":[{"id":"16","name":"DEMO Sprint 16","state":"ACTIVE","boardId":"98"}]}}]}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	client := NewClient()
	sprints, err := client.ListProjectOpenSprints(context.Background(), "DEMO", 50)
	if err != nil {
		t.Fatalf("ListProjectOpenSprints() error: %v", err)
	}
	if len(sprints) != 1 || sprints[0].ID != 16 || sprints[0].Name != "DEMO Sprint 16" || sprints[0].OriginBoardID != 98 || sprints[0].State != "active" {
		t.Fatalf("sprints = %#v", sprints)
	}
}

func TestClient_SearchIssuesByJQL_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	client := NewClient()
	_, err := client.SearchIssuesByJQL(context.Background(), `assignee=currentUser()`, 10)
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}
	if !strings.Contains(err.Error(), "jira authentication failed") {
		t.Fatalf("error = %v", err)
	}
}
