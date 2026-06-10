package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/infra/statestore"
	"github.com/tasuku43/kra/internal/testutil"
)

func TestServeHandler_WorkspacesPageRendersActiveWorkspaceKanban(t *testing.T) {
	env := testutil.NewEnv(t)
	activePath := seedWorkspaceMeta(t, env.Root, "active", "PROJ-2314")
	archivedPath := seedWorkspaceMeta(t, env.Root, "archived", "OLD-1")
	updateServeTestMeta(t, activePath, func(meta *workspaceMetaFile) {
		meta.Workspace.Title = "Serve dashboard spec"
	})
	updateServeTestMeta(t, archivedPath, func(meta *workspaceMetaFile) {
		meta.Workspace.Title = "Archived workspace"
	})
	writeWorkspaceTasksFile(t, activePath, "## Tasks\n\n### TASK-001 Draft spec\nstatus: done\n\n### TASK-002 Build board\nstatus: doing\n\n### TASK-003 Review layout\nstatus: todo\n")
	writeWorkspaceTasksFile(t, archivedPath, "## Tasks\n\n### TASK-999 Archived task\nstatus: doing\n")

	req := httptest.NewRequest(http.MethodGet, "/workspaces/", nil)
	rec := httptest.NewRecorder()
	newServeHandler(env.Root).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"All Workspaces", "PROJ-2314", "Serve dashboard spec", `data-live-page="overview" data-theme="light"`, `data-theme-toggle`, `data-theme-label`, `kraServeTheme`, `data-live-sidebar`, `data-live-board`, `refreshServeData`, `setInterval(refreshServeData,5000)`, `<span class="workspace-link-title">Serve dashboard spec</span>`, `class="progress-mini"`, `1 / 3 done`, `style="width: 33%"`, "Todo", "Doing", "Done", "Build board", "Review layout"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "OLD-1") || strings.Contains(body, "Archived task") {
		t.Fatalf("archived workspace should not be rendered:\n%s", body)
	}
}

func TestServeHandler_WorkspaceDetailRendersBoardReadmeReposAndPR(t *testing.T) {
	ghCalls := 0
	restoreGH := stubServeGitHub(t, func(_ context.Context, _ ...string) (string, error) {
		ghCalls++
		return "[]", nil
	})
	defer restoreGH()

	env := testutil.NewEnv(t)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "PROJ-2314")
	updateServeTestMeta(t, wsPath, func(meta *workspaceMetaFile) {
		meta.Workspace.Title = "Serve dashboard spec"
		meta.Workspace.SourceURL = "https://github.com/tasuku43/kra/pull/128"
		meta.ReposRestore = []workspaceMetaRepoRestore{{
			RepoUID:   "github.com/tasuku43/kra",
			RepoKey:   "tasuku43/kra",
			RemoteURL: "git@github.com:tasuku43/kra.git",
			Alias:     "kra",
			Branch:    "feature/PROJ-2314/serve-dashboard",
			BaseRef:   "origin/main",
		}}
	})
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 Build board\nstatus: doing\n")
	readme := "# Serve dashboard\n\nRead-only **boards**.\n\n```mermaid\ngraph TD\n  A[Serve] --> B[Readme]\n```\n\n<script>alert('nope')</script>\n"
	if err := os.WriteFile(filepath.Join(wsPath, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(wsPath, "docs", "dev", "mock"), 0o755); err != nil {
		t.Fatalf("mkdir html dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsPath, "docs", "dev", "mock", "preview.html"), []byte("<!doctype html><main><h1>Mock Preview</h1></main>"), 0o644); err != nil {
		t.Fatalf("write preview.html: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(wsPath, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsPath, "scripts", "check.sh"), []byte("#!/usr/bin/env bash\nset -euo pipefail\nif true; then\n  exit 0\nfi\n"), 0o644); err != nil {
		t.Fatalf("write check.sh: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/workspaces/PROJ-2314/", nil)
	rec := httptest.NewRecorder()
	newServeHandler(env.Root).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"PROJ-2314: Serve dashboard spec",
		`data-live-page="detail" data-workspace-id="PROJ-2314" data-theme="light"`,
		`data-theme-toggle`,
		`data-live-detail-header`,
		`aria-label="workspace file tree"`,
		`data-open-view="workspace.md"`,
		`data-open-view="repos/"`,
		`data-open-view="README.md"`,
		`data-toggle-dir="docs"`,
		`data-file-tab="workspace.md"`,
		`data-file-panel="workspace.md" data-kind="workspace"`,
		`data-live-workspace-board`,
		`Board from workspace.md`,
		`data-live-detail-title`,
		"Build board",
		`0 / 1 done`,
		`Open files and workspace views as tabs.`,
		`data-file-panel="README.md" data-kind="markdown"`,
		`class="rendered-markdown"`,
		"<h1 id=\"serve-dashboard\">Serve dashboard</h1>",
		"<strong>boards</strong>",
		"language-mermaid",
		"mermaid.initialize",
		`data-file-panel="docs/dev/mock/preview.html" data-kind="html"`,
		`class="html-render"`,
		`data-html-preview-frame sandbox="allow-same-origin"`,
		"resizeHTMLPreviews(panel)",
		"Mock Preview",
		`data-file-panel="scripts/check.sh" data-kind="source"`,
		`language-shell`,
		`<span class="code-keyword">if</span> true; <span class="code-keyword">then</span>`,
		`data-file-panel="repos/" data-kind="repos"`,
		`data-repos-loaded="false"`,
		"repository context opened from repos/",
		"kra",
		"feature/PROJ-2314/serve-dashboard",
		"Loading pull requests...",
		`window.loadReposView=loadReposView`,
		`board.replaceChildren.apply(board,statuses.map(function(status){return statusColumn(workspace,status,'');}))`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if ghCalls != 0 {
		t.Fatalf("workspace detail initial render called gh %d times, want 0", ghCalls)
	}
	if strings.Contains(body, "<script>alert('nope')</script>") {
		t.Fatalf("README raw HTML should be escaped:\n%s", body)
	}
}

func TestServeHandler_WorkspaceReposAPILazilyLoadsPullRequests(t *testing.T) {
	ghCalls := 0
	restoreGH := stubServeGitHub(t, func(_ context.Context, args ...string) (string, error) {
		ghCalls++
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "--head feature/current"):
			return `[{"number":12,"title":"Branch only work","url":"https://github.com/tasuku43/kra/pull/12","state":"OPEN","isDraft":false,"headRefName":"feature/current"}]`, nil
		case strings.Contains(joined, "--search"):
			return `[{"number":9,"title":"PROJ-2314 follow-up","url":"https://github.com/tasuku43/kra/pull/9","state":"MERGED","isDraft":false,"headRefName":"feature/old"}]`, nil
		default:
			return "[]", nil
		}
	})
	defer restoreGH()

	env := testutil.NewEnv(t)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "PROJ-2314")
	updateServeTestMeta(t, wsPath, func(meta *workspaceMetaFile) {
		meta.Workspace.Title = "Serve dashboard spec"
		meta.ReposRestore = []workspaceMetaRepoRestore{{
			RepoUID:   "github.com/tasuku43/kra",
			RepoKey:   "tasuku43/kra",
			RemoteURL: "git@github.com:tasuku43/kra.git",
			Alias:     "kra",
			Branch:    "feature/current",
			BaseRef:   "origin/main",
		}}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/PROJ-2314/repos", nil)
	rec := httptest.NewRecorder()
	newServeHandler(env.Root).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`href="https://github.com/tasuku43/kra/pull/12"`,
		"Branch only work",
		"PROJ-2314 follow-up",
		"current branch",
		"title match",
		`pr-state pr-state-open`,
		`pr-state pr-state-merged`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Loading pull requests...") {
		t.Fatalf("repos API should return loaded PR state:\n%s", body)
	}
	if ghCalls != 2 {
		t.Fatalf("gh calls = %d, want 2", ghCalls)
	}
}

func TestServeHandler_WorkspacesAPIRendersLiveBoardJSON(t *testing.T) {
	ghCalls := 0
	restoreGH := stubServeGitHub(t, func(_ context.Context, _ ...string) (string, error) {
		ghCalls++
		return "[]", nil
	})
	defer restoreGH()

	env := testutil.NewEnv(t)
	activePath := seedWorkspaceMeta(t, env.Root, "active", "PROJ-2314")
	archivedPath := seedWorkspaceMeta(t, env.Root, "archived", "OLD-1")
	updateServeTestMeta(t, activePath, func(meta *workspaceMetaFile) {
		meta.Workspace.Title = "Serve dashboard spec"
		meta.ReposRestore = []workspaceMetaRepoRestore{{
			RepoUID:   "github.com/tasuku43/kra",
			RepoKey:   "tasuku43/kra",
			RemoteURL: "git@github.com:tasuku43/kra.git",
			Alias:     "kra",
			Branch:    "feature/PROJ-2314/serve-dashboard",
			BaseRef:   "origin/main",
		}}
	})
	updateServeTestMeta(t, archivedPath, func(meta *workspaceMetaFile) {
		meta.Workspace.Title = "Archived workspace"
	})
	writeWorkspaceTasksFile(t, activePath, "## Tasks\n\n### TASK-001 Draft spec\nstatus: done\n\n### TASK-002 Build board\nstatus: doing\n\n### TASK-003 Review layout\nstatus: todo\n")
	writeWorkspaceTasksFile(t, archivedPath, "## Tasks\n\n### TASK-999 Archived task\nstatus: doing\n")

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
	rec := httptest.NewRecorder()
	newServeHandler(env.Root).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var got serveAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, rec.Body.String())
	}
	if len(got.Workspaces) != 1 {
		t.Fatalf("workspaces len = %d, want 1: %#v", len(got.Workspaces), got.Workspaces)
	}
	ws := got.Workspaces[0]
	if ws.ID != "PROJ-2314" || ws.Title != "Serve dashboard spec" || ws.Href != "/workspaces/PROJ-2314/" {
		t.Fatalf("workspace identity = %#v", ws)
	}
	if ws.Progress.Done != 1 || ws.Progress.Total != 3 || ws.Progress.Percent != 33 {
		t.Fatalf("progress = %#v, want 1/3/33", ws.Progress)
	}
	if len(ws.Tasks["doing"]) != 1 || ws.Tasks["doing"][0].Title != "Build board" {
		t.Fatalf("doing tasks = %#v", ws.Tasks["doing"])
	}
	if len(ws.Tasks["todo"]) != 1 || ws.Tasks["todo"][0].ID != "TASK-003" {
		t.Fatalf("todo tasks = %#v", ws.Tasks["todo"])
	}
	if ghCalls != 0 {
		t.Fatalf("workspaces API called gh %d times, want 0", ghCalls)
	}
}

func TestServeHandler_WorkspaceAPIRendersOneWorkspace(t *testing.T) {
	env := testutil.NewEnv(t)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "PROJ-2314")
	updateServeTestMeta(t, wsPath, func(meta *workspaceMetaFile) {
		meta.Workspace.Title = "Serve dashboard spec"
	})
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 Build board\nstatus: doing\n")
	handler := newServeHandler(env.Root)

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/PROJ-2314", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got serveAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(got.Workspaces) != 1 || got.Workspaces[0].ID != "PROJ-2314" {
		t.Fatalf("workspaces = %#v", got.Workspaces)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/workspaces/MISSING", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestServeHandler_RedirectsAndNotFound(t *testing.T) {
	env := testutil.NewEnv(t)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")
	handler := newServeHandler(env.Root)

	for _, tc := range []struct {
		path     string
		wantCode int
		location string
	}{
		{path: "/", wantCode: http.StatusFound, location: "/workspaces/"},
		{path: "/workspaces/WS1", wantCode: http.StatusFound, location: "/workspaces/WS1/"},
		{path: "/workspaces/MISSING/", wantCode: http.StatusNotFound},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.wantCode {
			t.Fatalf("%s status = %d, want %d", tc.path, rec.Code, tc.wantCode)
		}
		if tc.location != "" && rec.Header().Get("Location") != tc.location {
			t.Fatalf("%s Location = %q, want %q", tc.path, rec.Header().Get("Location"), tc.location)
		}
	}
}

func TestServeReposForWorkspaceListsBranchPRBeforeTitleMatches(t *testing.T) {
	restoreGH := stubServeGitHub(t, func(_ context.Context, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "--head feature/current"):
			return `[{"number":12,"title":"Branch only work","url":"https://github.com/tasuku43/kra/pull/12","state":"OPEN","isDraft":false,"headRefName":"feature/current"}]`, nil
		case strings.Contains(joined, "--search"):
			return `[
				{"number":9,"title":"PROJ-2314 merged follow-up","url":"https://github.com/tasuku43/kra/pull/9","state":"MERGED","isDraft":false,"headRefName":"feature/old"},
				{"number":8,"title":"PROJ-2314 draft follow-up","url":"https://github.com/tasuku43/kra/pull/8","state":"OPEN","isDraft":true,"headRefName":"feature/draft"}
			]`, nil
		default:
			return "[]", nil
		}
	})
	defer restoreGH()

	repos := serveReposForWorkspaceWithPullRequests(wsListRow{
		ID: "PROJ-2314",
		Repos: []statestore.WorkspaceRepo{{
			RepoKey: "tasuku43/kra",
			Alias:   "kra",
			Branch:  "feature/current",
		}},
	})
	if len(repos) != 1 {
		t.Fatalf("repos len = %d, want 1", len(repos))
	}
	prs := repos[0].PullRequests
	if len(prs) != 3 {
		t.Fatalf("pull requests len = %d, want 3: %#v", len(prs), prs)
	}
	if prs[0].Number != 12 || !prs[0].BranchMatch || prs[0].State != "open" {
		t.Fatalf("first PR = %#v, want branch-matched open PR #12", prs[0])
	}
	if prs[1].Number != 9 || !prs[1].TitleMatch || prs[1].State != "merged" {
		t.Fatalf("second PR = %#v, want title-matched merged PR #9", prs[1])
	}
	if prs[2].Number != 8 || !prs[2].TitleMatch || prs[2].State != "draft" {
		t.Fatalf("third PR = %#v, want title-matched draft PR #8", prs[2])
	}

	html := string(serveReposView(serveWorkspace{Repos: repos}).HTML)
	for _, want := range []string{
		"Branch only work",
		"PROJ-2314 merged follow-up",
		"PROJ-2314 draft follow-up",
		"current branch",
		"title match",
		`pr-state pr-state-open`,
		`pr-state pr-state-merged`,
		`pr-state pr-state-draft`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("repo HTML missing %q:\n%s", want, html)
		}
	}
	if branchIndex, mergedIndex := strings.Index(html, "Branch only work"), strings.Index(html, "PROJ-2314 merged follow-up"); branchIndex < 0 || mergedIndex < 0 || branchIndex > mergedIndex {
		t.Fatalf("branch-matched PR should render before title matches:\n%s", html)
	}
}

func TestServePRInference_NormalizesGitHubRepoIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prRepo  string
		repoKey string
		repoUID string
		count   int
		want    bool
	}{
		{
			name:    "matches repo uid with github host prefix",
			prRepo:  "tasuku43/kra",
			repoUID: "github.com/tasuku43/kra",
			count:   2,
			want:    true,
		},
		{
			name:    "matches repo uid stored as https remote",
			prRepo:  "tasuku43/kra",
			repoUID: "https://github.com/tasuku43/kra.git",
			count:   2,
			want:    true,
		},
		{
			name:    "matches repo uid stored as ssh remote",
			prRepo:  "tasuku43/kra",
			repoUID: "git@github.com:tasuku43/kra.git",
			count:   2,
			want:    true,
		},
		{
			name:    "matches repo key case-insensitively",
			prRepo:  "tasuku43/kra",
			repoKey: "Tasuku43/KRA",
			count:   2,
			want:    true,
		},
		{
			name:    "does not match other repo in multi repo workspace",
			prRepo:  "tasuku43/kra",
			repoKey: "tasuku43/other",
			repoUID: "github.com/tasuku43/other",
			count:   2,
			want:    false,
		},
		{
			name:   "single repo workspace keeps source pr fallback",
			prRepo: "tasuku43/kra",
			count:  1,
			want:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourcePRMatchesRepo(tc.prRepo, tc.repoKey, tc.repoUID, tc.count); got != tc.want {
				t.Fatalf("sourcePRMatchesRepo() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestServeStyles_FixesBoardColumnHeightAndScrollsOverflow(t *testing.T) {
	styles := serveStyles + serveSmartViewStyles
	for _, want := range []string{
		"--board-column-height:360px",
		".status-column{height:var(--board-column-height)",
		"overflow-y:auto",
		"overscroll-behavior:contain",
		".readme{width:100%;max-width:none",
		".readme blockquote",
		".readme table",
		".mermaid{",
		"grid-template-columns:minmax(120px,2fr) minmax(120px,2fr) minmax(220px,6fr)",
		".repo-row a{overflow-wrap:anywhere}",
		".workspace-link-title{color:var(--muted)",
		".progress-track{height:10px",
		".progress-mini{width:100%",
		"body[data-theme=dark]",
		".theme-toggle{min-height:36px",
		"body[data-theme=dark] .theme-toggle-knob{transform:translateX(14px)}",
		"body:has(.tab-panel:target) .tab-panel.default:not(:target){display:none}",
		"body[data-live-page=detail] .app{grid-template-columns:300px minmax(0,1fr)}",
		".file-tree{min-width:0",
		".file-tabs{display:flex",
		".workspace-board-view,.repo-overview{padding:18px}",
		".html-preview{display:block",
		".html-render{display:block;width:100%;height:auto;min-height:0",
		".code-keyword{color:#93c5fd",
	} {
		if !strings.Contains(styles, want) {
			t.Fatalf("serve styles missing %q:\n%s", want, styles)
		}
	}
	if strings.Contains(styles, "body:has(.tab-panel:target) .tab-panel.default{display:none}") {
		t.Fatalf("README target panel should not be hidden by the default-panel rule:\n%s", styles)
	}
	for _, fixed := range []string{
		".html-preview{min-height:100%;display:grid",
		"grid-template-rows:auto minmax(320px,1fr) auto",
		".html-render{width:100%;height:100%;min-height:320px",
	} {
		if strings.Contains(styles, fixed) {
			t.Fatalf("HTML preview should size to rendered content, found fixed sizing %q:\n%s", fixed, styles)
		}
	}
}

func updateServeTestMeta(t *testing.T, wsPath string, update func(*workspaceMetaFile)) {
	t.Helper()
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		t.Fatalf("load workspace meta: %v", err)
	}
	update(&meta)
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}
}

func stubServeGitHub(t *testing.T, run serveGitHubRunner) func() {
	t.Helper()
	prev := runServeGitHubCommand
	runServeGitHubCommand = run
	return func() {
		runServeGitHubCommand = prev
	}
}
