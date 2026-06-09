package cli

import (
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
	for _, want := range []string{"All Workspaces", "PROJ-2314", "Serve dashboard spec", `<span class="workspace-link-title">Serve dashboard spec</span>`, `class="progress-mini"`, `1 / 3 done`, `style="width: 33%"`, "Todo", "Doing", "Done", "Build board", "Review layout"} {
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
