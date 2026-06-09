package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

const defaultServeAddr = "127.0.0.1:8765"

type serveOptions struct {
	addr string
}

type serveWorkspace struct {
	ID        string
	Title     string
	SourceURL string
	Tasks     wstask.Overview
	Repos     []serveRepo
}

type serveRepo struct {
	Name    string
	Branch  string
	PRLabel string
	PRURL   string
	Missing bool
}

type serveReadme struct {
	Name    string
	Content string
	HTML    template.HTML
	Exists  bool
}

type servePageData struct {
	Root       string
	Workspace  serveWorkspace
	Workspaces []serveWorkspace
	Readme     serveReadme
}

func (c *CLI) runServe(args []string) int {
	opts, err := parseServeOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printServeUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printServeUsage(c.Err)
		return exitUsage
	}

	root, code := c.resolveRootForRootCommand("human", "serve")
	if code != exitOK {
		return code
	}

	addr := strings.TrimSpace(opts.addr)
	fmt.Fprintf(c.Out, "Serving kra workspace boards at http://%s/workspaces/\n", displayServeAddr(addr))
	server := &http.Server{
		Addr:    addr,
		Handler: newServeHandler(root),
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(c.Err, "serve: %v\n", err)
		return exitError
	}
	return exitOK
}

func parseServeOptions(args []string) (serveOptions, error) {
	opts := serveOptions{addr: defaultServeAddr}
	rest := append([]string{}, args...)
	for len(rest) > 0 {
		arg := strings.TrimSpace(rest[0])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return serveOptions{}, errHelpRequested
		case strings.HasPrefix(arg, "--addr="):
			opts.addr = strings.TrimSpace(strings.TrimPrefix(arg, "--addr="))
			rest = rest[1:]
		case arg == "--addr":
			if len(rest) < 2 {
				return serveOptions{}, fmt.Errorf("--addr requires a value")
			}
			opts.addr = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			return serveOptions{}, fmt.Errorf("unknown flag for serve: %q", arg)
		}
	}
	if strings.TrimSpace(opts.addr) == "" {
		return serveOptions{}, fmt.Errorf("--addr must not be empty")
	}
	return opts, nil
}

func displayServeAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

func newServeHandler(root string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveHTTP(root, w, r)
	})
	return mux
}

func serveHTTP(root string, w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/":
		http.Redirect(w, r, "/workspaces/", http.StatusFound)
		return
	case r.URL.Path == "/workspaces":
		http.Redirect(w, r, "/workspaces/", http.StatusFound)
		return
	case r.URL.Path == "/workspaces/":
		renderServeWorkspaces(w, r, root)
		return
	case strings.HasPrefix(r.URL.Path, "/workspaces/"):
		renderServeWorkspaceDetailRoute(w, r, root)
		return
	default:
		http.NotFound(w, r)
		return
	}
}

func renderServeWorkspaceDetailRoute(w http.ResponseWriter, r *http.Request, root string) {
	rest := strings.TrimPrefix(r.URL.Path, "/workspaces/")
	if strings.TrimSpace(rest) == "" {
		renderServeWorkspaces(w, r, root)
		return
	}
	if strings.Count(rest, "/") > 1 {
		http.NotFound(w, r)
		return
	}
	if !strings.HasSuffix(rest, "/") {
		http.Redirect(w, r, "/workspaces/"+rest+"/", http.StatusFound)
		return
	}
	id, err := url.PathUnescape(strings.TrimSuffix(rest, "/"))
	if err != nil || validateWorkspaceID(id) != nil {
		http.NotFound(w, r)
		return
	}
	renderServeWorkspaceDetail(w, r, root, id)
}

func renderServeWorkspaces(w http.ResponseWriter, _ *http.Request, root string) {
	workspaces, err := loadServeWorkspaces(root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeServeHTML(w, serveOverviewTemplate, servePageData{
		Root:       root,
		Workspaces: workspaces,
	})
}

func renderServeWorkspaceDetail(w http.ResponseWriter, _ *http.Request, root string, workspaceID string) {
	workspaces, err := loadServeWorkspaces(root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var selected serveWorkspace
	found := false
	for _, ws := range workspaces {
		if ws.ID == workspaceID {
			selected = ws
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	readme := loadServeReadme(root, workspaceID)
	writeServeHTML(w, serveDetailTemplate, servePageData{
		Root:       root,
		Workspace:  selected,
		Workspaces: workspaces,
		Readme:     readme,
	})
}

func loadServeWorkspaces(root string) ([]serveWorkspace, error) {
	ctx := context.Background()
	rowsResult, err := listRowsFromFilesystemResult(ctx, root, "active", true)
	if err != nil {
		return nil, fmt.Errorf("list active workspaces: %w", err)
	}
	if err := hydrateWSListTaskOverviews(root, "active", rowsResult.Rows); err != nil {
		return nil, fmt.Errorf("load workspace tasks: %w", err)
	}
	out := make([]serveWorkspace, 0, len(rowsResult.Rows))
	for _, row := range rowsResult.Rows {
		out = append(out, serveWorkspace{
			ID:        row.ID,
			Title:     firstNonEmpty(strings.TrimSpace(row.Title), row.ID),
			SourceURL: strings.TrimSpace(row.SourceURL),
			Tasks:     row.Tasks,
			Repos:     serveReposForWorkspace(row),
		})
	}
	return out, nil
}

func serveReposForWorkspace(row wsListRow) []serveRepo {
	repos := make([]serveRepo, 0, len(row.Repos))
	prRepoKey, prLabel, prURL, hasPR := parseGitHubPullRequestSource(row.SourceURL)
	prTitle := firstNonEmpty(strings.TrimSpace(row.Title), prLabel)
	for _, repo := range row.Repos {
		name := firstNonEmpty(strings.TrimSpace(repo.Alias), strings.TrimSpace(repo.RepoKey), strings.TrimSpace(repo.RepoUID))
		branch := firstNonEmpty(strings.TrimSpace(repo.Branch), "unknown")
		prLabelForRepo := "none"
		prURLForRepo := ""
		if hasPR && sourcePRMatchesRepo(prRepoKey, repo.RepoKey, repo.RepoUID, len(row.Repos)) {
			prLabelForRepo = prTitle
			prURLForRepo = prURL
		}
		repos = append(repos, serveRepo{
			Name:    name,
			Branch:  branch,
			PRLabel: prLabelForRepo,
			PRURL:   prURLForRepo,
			Missing: repo.MissingAt.Valid,
		})
	}
	return repos
}

func parseGitHubPullRequestSource(sourceURL string) (repoKey string, label string, link string, ok bool) {
	trimmed := strings.TrimSpace(sourceURL)
	if trimmed == "" {
		return "", "", "", false
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", "", "", false
	}
	if !strings.EqualFold(u.Host, "github.com") {
		return "", "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pull" || strings.TrimSpace(parts[3]) == "" {
		return "", "", "", false
	}
	repoKey = parts[0] + "/" + parts[1]
	return repoKey, "#" + parts[3], trimmed, true
}

func sourcePRMatchesRepo(prRepoKey string, repoKey string, repoUID string, repoCount int) bool {
	if strings.TrimSpace(prRepoKey) == "" {
		return false
	}
	if repoCount == 1 {
		return true
	}
	candidates := []string{
		strings.TrimSpace(repoKey),
		strings.TrimPrefix(strings.TrimSpace(repoUID), "github.com/"),
	}
	for _, candidate := range candidates {
		if strings.EqualFold(candidate, prRepoKey) {
			return true
		}
	}
	return false
}

func loadServeReadme(root string, workspaceID string) serveReadme {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	candidates := []string{"README.md", workspaceDocumentFilename}
	for _, name := range candidates {
		b, err := os.ReadFile(filepath.Join(wsPath, name))
		if err == nil {
			content := string(b)
			return serveReadme{Name: name, Content: content, HTML: renderServeMarkdown(content), Exists: true}
		}
	}
	return serveReadme{Name: "README.md", Content: "", Exists: false}
}

func renderServeMarkdown(markdown string) template.HTML {
	var b bytes.Buffer
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	if err := md.Convert([]byte(markdown), &b); err != nil {
		return template.HTML(template.HTMLEscapeString(markdown))
	}
	return template.HTML(b.String())
}

func writeServeHTML(w http.ResponseWriter, tmpl string, data servePageData) {
	t := template.Must(template.New("serve").Funcs(template.FuncMap{
		"workspaceHref": serveWorkspaceHref,
		"listStatuses": func() []string {
			return []string{
				string(wstask.StatusTodo),
				string(wstask.StatusDoing),
				string(wstask.StatusBlocked),
				string(wstask.StatusDone),
			}
		},
		"itemsByStatus": func(overview wstask.Overview, status string) []wstask.Item {
			return overview.ItemsByStatus(wstask.Status(status))
		},
		"statusTitle": serveStatusTitle,
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict requires key/value pairs")
			}
			out := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict key must be string")
				}
				out[key] = values[i+1]
			}
			return out, nil
		},
		"safeID": func(id string) string {
			return url.PathEscape(id)
		},
	}).Parse(tmpl))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = t.Execute(w, data)
}

func serveWorkspaceHref(id string) string {
	return "/workspaces/" + url.PathEscape(id) + "/"
}

func serveStatusTitle(status string) string {
	switch wstask.Status(status) {
	case wstask.StatusTodo:
		return "Todo"
	case wstask.StatusDoing:
		return "Doing"
	case wstask.StatusBlocked:
		return "Blocked"
	case wstask.StatusDone:
		return "Done"
	default:
		return status
	}
}

const serveStyles = `
:root{color-scheme:light;--bg:#f6f8fb;--panel:#fff;--panel-soft:#f1f5f9;--ink:#172033;--muted:#687386;--line:#d8e0ea;--accent:#2563eb;--accent-soft:#dbeafe;--warn:#b66b00;--warn-soft:#fff2d6;--danger:#c2413d;--danger-soft:#fee2e2;--shadow:0 14px 34px rgba(20,31,54,.12);--board-column-height:360px;--readme-code:#0f172a;--readme-quote:#f0f9ff;--readme-quote-line:#0ea5a3;--readme-table:#f8fbff}*{box-sizing:border-box}body{margin:0;background:linear-gradient(180deg,#edf4ff 0,rgba(237,244,255,0) 300px),var(--bg);color:var(--ink);font-family:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;font-size:14px;letter-spacing:0}a{color:var(--accent);text-decoration:none;font-weight:800}a:hover{text-decoration:underline}.app{min-height:100vh;display:grid;grid-template-columns:260px minmax(0,1fr)}.sidebar{height:100vh;position:sticky;top:0;padding:22px 18px;border-right:1px solid var(--line);background:rgba(255,255,255,.86);backdrop-filter:blur(18px)}.brand{display:flex;align-items:center;gap:11px;margin-bottom:24px}.brand-mark{width:38px;height:38px;display:grid;place-items:center;border:1px solid #bdd2f7;border-radius:8px;background:#eff6ff;color:var(--accent);font-weight:900}.brand strong{display:block;font-size:16px}.meta,.small,.side-label-link{color:var(--muted);font-size:12px}.side-label-link{display:block;margin:18px 8px 8px;font-weight:900;text-transform:uppercase}.workspace-link{width:100%;min-height:42px;display:flex;align-items:center;gap:10px;padding:9px 10px;border-radius:8px;color:var(--ink)}.workspace-link.active,.workspace-link:hover{background:var(--accent-soft);color:#1447a6;text-decoration:none}.dot{width:9px;height:9px;border-radius:50%;background:#16835f;flex:0 0 auto}main{min-width:0;padding:24px 28px 42px}.topbar{display:flex;justify-content:space-between;align-items:flex-start;gap:18px;margin-bottom:18px}h1,h2,h3,p{margin:0}h1{font-size:28px;line-height:1.15;letter-spacing:0}.btn{min-height:36px;display:inline-flex;align-items:center;padding:0 12px;border:1px solid var(--accent);border-radius:8px;background:var(--accent);color:#fff;cursor:pointer;font-weight:800}.btn:hover{text-decoration:none}.board{display:grid;gap:14px}.swimlane,.detail{border:1px solid var(--line);border-radius:8px;background:rgba(255,255,255,.94);box-shadow:0 1px 1px rgba(20,31,54,.04);overflow:hidden}.detail{border-color:#bcd3ff;box-shadow:var(--shadow)}.swimlane-head,.detail-header{display:flex;justify-content:space-between;align-items:flex-start;gap:14px;padding:14px 16px;border-bottom:1px solid var(--line);background:#fff}.workspace-id{display:block;color:var(--ink);font-size:17px;font-weight:900}.workspace-title{margin-top:3px;color:var(--muted);line-height:1.4}.status-grid{display:grid;grid-template-columns:repeat(4,minmax(190px,1fr));gap:10px;padding:12px;overflow-x:auto}.detail-board{padding:14px 18px 4px}.detail-board .status-grid{padding:0}.status-column{height:var(--board-column-height);border:1px solid var(--line);border-radius:8px;background:var(--panel-soft);padding:10px;overflow-y:auto;overscroll-behavior:contain}.column-title{display:flex;justify-content:space-between;gap:8px;margin-bottom:9px;color:var(--muted);font-size:12px;font-weight:900;text-transform:uppercase}.task-card{display:block;margin-bottom:8px;padding:10px;border:1px solid #d9e1ec;border-radius:8px;background:#fff;color:var(--ink);line-height:1.4;font-weight:700}.task-card .small{display:block;margin-top:5px;font-weight:500}.tabs{display:flex;gap:8px;padding:12px 18px 0}.tab{min-height:36px;padding:0 12px;display:inline-flex;align-items:center;border:1px solid var(--line);border-radius:8px 8px 0 0;background:#fff;color:var(--muted);font-weight:900}.tab.active{color:#1447a6;border-color:#8ab4ff;background:#eff6ff}.tab-panel{display:none;padding:18px}.tab-panel:target{display:block}.tab-panel.default{display:block}body:has(.tab-panel:target) .tab-panel.default{display:none}.readme{width:100%;max-width:none;border:1px solid var(--line);border-radius:8px;background:#fff;padding:22px;line-height:1.7;overflow-wrap:anywhere}.readme>*:first-child{margin-top:0}.readme>*:last-child{margin-bottom:0}.readme h1,.readme h2,.readme h3{margin:1.45em 0 .55em;color:#10213d;line-height:1.24}.readme h1{padding-bottom:.35em;border-bottom:1px solid var(--line);font-size:30px}.readme h2{padding-bottom:.28em;border-bottom:1px solid #e6edf6;font-size:23px}.readme h3{font-size:18px}.readme p,.readme ul,.readme ol,.readme blockquote,.readme table,.readme pre{margin:0 0 1em}.readme ul,.readme ol{padding-left:1.45em}.readme li+li{margin-top:.25em}.readme code{border:1px solid #dce5f0;border-radius:6px;background:#edf6f3;color:#164e42;padding:.12em .34em;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.92em}.readme pre{overflow:auto;border-radius:8px;background:var(--readme-code);color:#e5eefc;padding:16px}.readme pre code{border:0;background:transparent;color:inherit;padding:0}.readme blockquote{border-left:5px solid var(--readme-quote-line);border-radius:0 8px 8px 0;background:var(--readme-quote);color:#31546b;padding:12px 14px}.readme table{width:100%;border-collapse:collapse;display:block;overflow:auto}.readme th,.readme td{border:1px solid #d8e0ea;padding:8px 10px}.readme th{background:var(--readme-table);font-weight:900}.readme tr:nth-child(2n) td{background:#fbfdff}.readme hr{height:1px;border:0;background:var(--line);margin:24px 0}.readme img{max-width:100%;border-radius:8px}.readme input[type=checkbox]{margin-right:.45em}.mermaid{margin:0 0 1em;padding:16px;border:1px solid #cfe7e3;border-radius:8px;background:linear-gradient(180deg,#f5fffd,#ffffff);overflow:auto}.empty{color:var(--muted)}.repo-table{display:grid;gap:8px}.repo-row{display:grid;grid-template-columns:minmax(120px,2fr) minmax(120px,2fr) minmax(220px,6fr);gap:12px;align-items:center;min-height:58px;padding:10px 12px;border:1px solid var(--line);border-radius:8px;background:#fff}.repo-row a{overflow-wrap:anywhere}.repo-row.header{min-height:36px;background:transparent;border-color:transparent;color:var(--muted);font-size:12px;font-weight:900;text-transform:uppercase}@media(max-width:1100px){.app{grid-template-columns:1fr}.sidebar{position:static;height:auto;border-right:0;border-bottom:1px solid var(--line)}.status-grid{grid-template-columns:repeat(4,minmax(220px,1fr))}}@media(max-width:720px){main{padding:18px 14px 32px}.topbar,.detail-header,.swimlane-head{flex-direction:column;align-items:stretch}.readme{padding:16px}.readme h1{font-size:24px}.repo-row{grid-template-columns:1fr}}
`

const serveOverviewTemplate = `<!doctype html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>kra serve - All Workspaces</title>
  <style>` + serveStyles + `</style>
</head>
<body>
<div class="app">
  <aside class="sidebar">
    <div class="brand"><div class="brand-mark">kra</div><div><strong>Workspace Boards</strong><span class="small">open workspaces only</span></div></div>
    <a class="side-label-link" href="/workspaces/">Open workspaces</a>
    <nav>{{range .Workspaces}}<a class="workspace-link" href="{{workspaceHref .ID}}"><span class="dot"></span>{{.ID}}</a>{{end}}</nav>
  </aside>
  <main>
    <header class="topbar"><div><h1>All Workspaces</h1><p class="meta">{{.Root}} ・ read-only ・ generated from workspace.md</p></div><a class="btn" href="/workspaces/">Refresh</a></header>
    <section class="board" aria-label="workspace kanban">
      {{range .Workspaces}}{{template "workspaceBoard" .}}{{else}}<p class="empty">No open workspaces.</p>{{end}}
    </section>
  </main>
</div>
</body>
</html>
{{define "workspaceBoard"}}
<article class="swimlane">
  <header class="swimlane-head">
    <div><a class="workspace-id" href="{{workspaceHref .ID}}">{{.ID}}</a><p class="workspace-title">{{.Title}}</p></div>
  </header>
  <div class="status-grid">{{range $status := listStatuses}}{{template "statusColumn" dict "Workspace" $ "Status" $status}}{{end}}</div>
</article>
{{end}}
{{define "statusColumn"}}
<section class="status-column">
  {{$items := itemsByStatus .Workspace.Tasks .Status}}
  <div class="column-title"><span>{{statusTitle .Status}}</span><span>{{len $items}}</span></div>
  {{range $items}}<a class="task-card" href="{{workspaceHref $.Workspace.ID}}">{{.Title}}<span class="small">{{.ID}}</span></a>{{else}}<span class="small">No cards</span>{{end}}
</section>
{{end}}`

const serveDetailTemplate = `<!doctype html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>kra serve - {{.Workspace.ID}}</title>
  <style>` + serveStyles + `</style>
</head>
<body>
<div class="app">
  <aside class="sidebar">
    <div class="brand"><div class="brand-mark">kra</div><div><strong>Workspace Boards</strong><span class="small">open workspaces only</span></div></div>
    <a class="side-label-link" href="/workspaces/">Open workspaces</a>
    <nav>{{range .Workspaces}}<a class="workspace-link {{if eq .ID $.Workspace.ID}}active{{end}}" href="{{workspaceHref .ID}}"><span class="dot"></span>{{.ID}}</a>{{end}}</nav>
  </aside>
  <main>
    <header class="topbar"><div><h1>{{.Workspace.ID}}: {{.Workspace.Title}}</h1><p class="meta">read-only workspace board with README and repositories</p></div><a class="btn" href="/workspaces/">All Workspaces</a></header>
    <section class="detail">
      <div class="detail-header"><div><strong class="workspace-id">{{.Workspace.ID}}</strong><p class="workspace-title">{{.Workspace.Title}}</p></div></div>
      <section class="detail-board" aria-label="workspace board"><div class="status-grid">{{range $status := listStatuses}}{{template "detailStatusColumn" dict "Workspace" $.Workspace "Status" $status}}{{end}}</div></section>
      <div class="tabs"><a class="tab active" href="#readme">README</a><a class="tab" href="#repositories">Repositories</a></div>
      <div class="tab-panel default" id="readme">{{if .Readme.Exists}}<article class="readme">{{.Readme.HTML}}</article>{{else}}<div class="readme empty">README.md or workspace.md was not found.</div>{{end}}</div>
      <div class="tab-panel" id="repositories">
        <div class="repo-table">
          <div class="repo-row header"><span>Repository</span><span>Branch</span><span>Pull request</span></div>
          {{range .Workspace.Repos}}<div class="repo-row"><strong>{{.Name}}{{if .Missing}} <span class="small">(missing)</span>{{end}}</strong><span>{{.Branch}}</span><span>{{if .PRURL}}<a href="{{.PRURL}}" target="_blank" rel="noreferrer">{{.PRLabel}}</a>{{else}}{{.PRLabel}}{{end}}</span></div>{{else}}<div class="repo-row"><span class="empty">No managed repositories.</span><span></span><span></span></div>{{end}}
        </div>
      </div>
    </section>
  </main>
</div>
<script>
function updateTabs(){var hash=window.location.hash||'#readme';document.querySelectorAll('.tab').forEach(function(tab){tab.classList.toggle('active',tab.getAttribute('href')===hash);});}
window.addEventListener('hashchange',updateTabs);updateTabs();
</script>
<script type="module">
import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs';
mermaid.initialize({startOnLoad:false,theme:'base',themeVariables:{primaryColor:'#eff6ff',primaryBorderColor:'#2563eb',primaryTextColor:'#172033',lineColor:'#0ea5a3',tertiaryColor:'#f0f9ff'}});
document.querySelectorAll('pre code.language-mermaid').forEach(function(code){
  var diagram=document.createElement('div');
  diagram.className='mermaid';
  diagram.textContent=code.textContent;
  code.closest('pre').replaceWith(diagram);
});
mermaid.run({querySelector:'.mermaid'});
</script>
</body>
</html>
{{define "detailStatusColumn"}}
<section class="status-column">
  {{$items := itemsByStatus .Workspace.Tasks .Status}}
  <div class="column-title"><span>{{statusTitle .Status}}</span><span>{{len $items}}</span></div>
  {{range $items}}<div class="task-card">{{.Title}}<span class="small">{{.ID}}</span></div>{{else}}<span class="small">No cards</span>{{end}}
</section>
{{end}}`
