package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	for _, want := range []string{"All Workspaces", "PROJ-2314", "Serve dashboard spec", `data-live-page="overview"`, `data-live-sidebar`, `data-live-board`, `refreshServeData`, `<span class="workspace-link-title">Serve dashboard spec</span>`, `class="progress-mini"`, `1 / 3 done`, `style="width: 33%"`, "Todo", "Doing", "Done", "Build board", "Review layout"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "OLD-1") || strings.Contains(body, "Archived task") {
		t.Fatalf("archived workspace should not be rendered:\n%s", body)
	}
}

func TestServeHandler_WorkspaceDetailRendersBoardReadmeReposAndPR(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/workspaces/PROJ-2314/", nil)
	rec := httptest.NewRecorder()
	newServeHandler(env.Root).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"PROJ-2314: Serve dashboard spec",
		`data-live-page="detail" data-workspace-id="PROJ-2314"`,
		`data-live-detail-header`,
		`data-live-detail-board`,
		`data-live-detail-title`,
		"Build board",
		`0 / 1 done`,
		`style="width: 0%"`,
		"<h1 id=\"serve-dashboard\">Serve dashboard</h1>",
		"<strong>boards</strong>",
		"language-mermaid",
		"mermaid.initialize",
		"Repositories",
		"kra",
		"feature/PROJ-2314/serve-dashboard",
		`<a href="https://github.com/tasuku43/kra/pull/128" target="_blank" rel="noreferrer">Serve dashboard spec</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<script>alert('nope')</script>") {
		t.Fatalf("README raw HTML should be escaped:\n%s", body)
	}
}

func TestServeHandler_WorkspacesAPIRendersLiveBoardJSON(t *testing.T) {
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

func TestServeStyles_FixesBoardColumnHeightAndScrollsOverflow(t *testing.T) {
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
		"body:has(.tab-panel:target) .tab-panel.default:not(:target){display:none}",
	} {
		if !strings.Contains(serveStyles, want) {
			t.Fatalf("serveStyles missing %q:\n%s", want, serveStyles)
		}
	}
	if strings.Contains(serveStyles, "body:has(.tab-panel:target) .tab-panel.default{display:none}") {
		t.Fatalf("README target panel should not be hidden by the default-panel rule:\n%s", serveStyles)
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
