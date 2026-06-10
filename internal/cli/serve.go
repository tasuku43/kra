package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

const defaultServeAddr = "127.0.0.1:8765"
const serveGitHubCommandTimeout = 8 * time.Second

type serveGitHubRunner func(ctx context.Context, args ...string) (string, error)

var runServeGitHubCommand serveGitHubRunner = runServeGHCommand

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
	Name         string
	RepoKey      string
	Branch       string
	PullRequests []servePullRequest
	Missing      bool
}

type servePullRequest struct {
	Number      int
	Title       string
	URL         string
	State       string
	HeadRef     string
	BranchMatch bool
	TitleMatch  bool
	SourceMatch bool
}

type serveReadme struct {
	Name    string
	Content string
	HTML    template.HTML
	Exists  bool
}

type serveFileTreeNode struct {
	Name     string
	Path     string
	Type     string
	Children []serveFileTreeNode
}

type serveFileView struct {
	ID         string
	Path       string
	Name       string
	Kind       string
	Language   string
	Badge      string
	Meta       string
	HTML       template.HTML
	SourceHTML template.HTML
}

type servePageData struct {
	Root       string
	Workspace  serveWorkspace
	Workspaces []serveWorkspace
	Readme     serveReadme
	FileTree   []serveFileTreeNode
	FileViews  []serveFileView
}

type serveAPIResponse struct {
	Workspaces []serveAPIWorkspace `json:"workspaces"`
}

type serveAPIWorkspace struct {
	ID       string                    `json:"id"`
	Title    string                    `json:"title"`
	Href     string                    `json:"href"`
	Progress serveProgress             `json:"progress"`
	Tasks    map[string][]serveAPITask `json:"tasks"`
	Counts   map[string]int            `json:"counts"`
}

type serveAPITask struct {
	ID    string `json:"id"`
	Title string `json:"title"`
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
	case r.URL.Path == "/api/workspaces" || r.URL.Path == "/api/workspaces/":
		renderServeWorkspacesAPI(w, root)
		return
	case strings.HasPrefix(r.URL.Path, "/api/workspaces/"):
		renderServeWorkspaceAPI(w, root, strings.TrimPrefix(r.URL.Path, "/api/workspaces/"))
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

func renderServeWorkspacesAPI(w http.ResponseWriter, root string) {
	workspaces, err := loadServeWorkspaces(root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeServeJSON(w, serveAPIResponse{Workspaces: serveAPIWorkspaces(workspaces)})
}

func renderServeWorkspaceAPI(w http.ResponseWriter, root string, workspacePath string) {
	workspacePath = strings.TrimSuffix(workspacePath, "/")
	if strings.TrimSpace(workspacePath) == "" || strings.Contains(workspacePath, "/") {
		http.NotFound(w, nil)
		return
	}
	id, err := url.PathUnescape(workspacePath)
	if err != nil || validateWorkspaceID(id) != nil {
		http.NotFound(w, nil)
		return
	}
	workspaces, err := loadServeWorkspaces(root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, ws := range workspaces {
		if ws.ID == id {
			writeServeJSON(w, serveAPIResponse{Workspaces: serveAPIWorkspaces([]serveWorkspace{ws})})
			return
		}
	}
	http.Error(w, "workspace not found", http.StatusNotFound)
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
	fileTree, fileViews := loadServeWorkspaceFiles(root, workspaceID, selected)
	writeServeHTML(w, serveDetailTemplate, servePageData{
		Root:       root,
		Workspace:  selected,
		Workspaces: workspaces,
		Readme:     readme,
		FileTree:   fileTree,
		FileViews:  fileViews,
	})
}

func writeServeJSON(w http.ResponseWriter, data serveAPIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(data)
}

func serveAPIWorkspaces(workspaces []serveWorkspace) []serveAPIWorkspace {
	out := make([]serveAPIWorkspace, 0, len(workspaces))
	for _, ws := range workspaces {
		tasks := make(map[string][]serveAPITask, len(serveStatusIDs()))
		counts := make(map[string]int, len(serveStatusIDs()))
		for _, status := range serveStatusIDs() {
			items := ws.Tasks.ItemsByStatus(wstask.Status(status))
			counts[status] = len(items)
			apiItems := make([]serveAPITask, 0, len(items))
			for _, item := range items {
				apiItems = append(apiItems, serveAPITask{
					ID:    item.ID,
					Title: item.Title,
				})
			}
			tasks[status] = apiItems
		}
		out = append(out, serveAPIWorkspace{
			ID:       ws.ID,
			Title:    ws.Title,
			Href:     serveWorkspaceHref(ws.ID),
			Progress: serveTaskProgress(ws.Tasks),
			Tasks:    tasks,
			Counts:   counts,
		})
	}
	return out
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
		repoKey := normalizeGitHubRepoKey(firstNonEmpty(strings.TrimSpace(repo.RepoKey), strings.TrimSpace(repo.RepoUID)))
		pullRequests, _ := lookupServeGitHubPullRequests(context.Background(), repoKey, row.ID, branch)
		if hasPR && sourcePRMatchesRepo(prRepoKey, repo.RepoKey, repo.RepoUID, len(row.Repos)) {
			pullRequests = mergeServePullRequests(pullRequests, servePullRequest{
				Number:      parseGitHubPullRequestNumber(prURL),
				Title:       prTitle,
				URL:         prURL,
				State:       "unknown",
				SourceMatch: true,
			})
		}
		sortServePullRequests(pullRequests)
		repos = append(repos, serveRepo{
			Name:         name,
			RepoKey:      repoKey,
			Branch:       branch,
			PullRequests: pullRequests,
			Missing:      repo.MissingAt.Valid,
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

func parseGitHubPullRequestNumber(link string) int {
	_, label, _, ok := parseGitHubPullRequestSource(link)
	if !ok {
		return 0
	}
	label = strings.TrimPrefix(strings.TrimSpace(label), "#")
	n, err := strconv.Atoi(label)
	if err != nil {
		return 0
	}
	return n
}

func sourcePRMatchesRepo(prRepoKey string, repoKey string, repoUID string, repoCount int) bool {
	normalizedPRRepoKey := normalizeGitHubRepoKey(prRepoKey)
	if normalizedPRRepoKey == "" {
		return false
	}
	if repoCount == 1 {
		return true
	}
	candidates := []string{
		repoKey,
		repoUID,
	}
	for _, candidate := range candidates {
		if normalizeGitHubRepoKey(candidate) == normalizedPRRepoKey {
			return true
		}
	}
	return false
}

func normalizeGitHubRepoKey(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimSuffix(trimmed, ".git")
	if strings.HasPrefix(trimmed, "git@github.com:") {
		return normalizeOwnerRepoKey(strings.TrimPrefix(trimmed, "git@github.com:"))
	}
	if u, err := url.Parse(trimmed); err == nil && strings.EqualFold(u.Host, "github.com") {
		return normalizeOwnerRepoKey(strings.Trim(u.Path, "/"))
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "github.com/") {
		return normalizeOwnerRepoKey(trimmed[len("github.com/"):])
	}
	return normalizeOwnerRepoKey(trimmed)
}

func normalizeOwnerRepoKey(value string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(value), "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
	if owner == "" || repo == "" {
		return ""
	}
	return strings.ToLower(owner + "/" + repo)
}

type serveGitHubPullRequestJSON struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	State       string `json:"state"`
	IsDraft     bool   `json:"isDraft"`
	HeadRefName string `json:"headRefName"`
}

func lookupServeGitHubPullRequests(ctx context.Context, repoKey string, workspaceID string, branch string) ([]servePullRequest, error) {
	repoKey = normalizeGitHubRepoKey(repoKey)
	if repoKey == "" || runServeGitHubCommand == nil {
		return nil, nil
	}
	var out []servePullRequest
	if shouldLookupServeBranchPR(branch) {
		items, err := runServeGitHubPRList(ctx, repoKey, "--head", strings.TrimSpace(branch), "--limit", "20")
		if err == nil {
			for _, item := range items {
				out = mergeServePullRequests(out, servePullRequestFromGitHub(item, true, titleMatchesWorkspaceID(item.Title, workspaceID)))
			}
		}
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID != "" {
		items, err := runServeGitHubPRList(ctx, repoKey, "--search", fmt.Sprintf("%q in:title", workspaceID), "--limit", "50")
		if err == nil {
			for _, item := range items {
				out = mergeServePullRequests(out, servePullRequestFromGitHub(item, strings.EqualFold(strings.TrimSpace(item.HeadRefName), strings.TrimSpace(branch)), true))
			}
		}
	}
	sortServePullRequests(out)
	return out, nil
}

func shouldLookupServeBranchPR(branch string) bool {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return false
	}
	switch strings.ToLower(branch) {
	case "unknown", "head", "(detached)":
		return false
	default:
		return true
	}
}

func runServeGitHubPRList(ctx context.Context, repoKey string, filters ...string) ([]serveGitHubPullRequestJSON, error) {
	args := []string{"pr", "list", "--repo", repoKey, "--state", "all"}
	args = append(args, filters...)
	args = append(args, "--json", "number,title,url,state,isDraft,headRefName")
	out, err := runServeGitHubCommand(ctx, args...)
	if err != nil {
		return nil, err
	}
	var items []serveGitHubPullRequestJSON
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return nil, err
	}
	return items, nil
}

func runServeGHCommand(ctx context.Context, args ...string) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(ctx, serveGitHubCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), err
	}
	return strings.TrimSpace(string(out)), nil
}

func servePullRequestFromGitHub(item serveGitHubPullRequestJSON, branchMatch bool, titleMatch bool) servePullRequest {
	return servePullRequest{
		Number:      item.Number,
		Title:       strings.TrimSpace(item.Title),
		URL:         strings.TrimSpace(item.URL),
		State:       servePullRequestState(item.State, item.IsDraft),
		HeadRef:     strings.TrimSpace(item.HeadRefName),
		BranchMatch: branchMatch,
		TitleMatch:  titleMatch,
	}
}

func servePullRequestState(state string, isDraft bool) string {
	normalized := strings.ToLower(strings.TrimSpace(state))
	if isDraft && normalized == "open" {
		return "draft"
	}
	switch normalized {
	case "open", "merged", "closed", "draft":
		return normalized
	default:
		return "unknown"
	}
}

func titleMatchesWorkspaceID(title string, workspaceID string) bool {
	title = strings.ToLower(strings.TrimSpace(title))
	workspaceID = strings.ToLower(strings.TrimSpace(workspaceID))
	return title != "" && workspaceID != "" && strings.Contains(title, workspaceID)
}

func mergeServePullRequests(items []servePullRequest, next servePullRequest) []servePullRequest {
	next.URL = strings.TrimSpace(next.URL)
	next.Title = strings.TrimSpace(next.Title)
	next.State = servePullRequestState(next.State, false)
	if next.Title == "" && next.Number > 0 {
		next.Title = fmt.Sprintf("#%d", next.Number)
	}
	if next.URL == "" && next.Number <= 0 {
		return items
	}
	for i := range items {
		if servePullRequestSame(items[i], next) {
			items[i].BranchMatch = items[i].BranchMatch || next.BranchMatch
			items[i].TitleMatch = items[i].TitleMatch || next.TitleMatch
			items[i].SourceMatch = items[i].SourceMatch || next.SourceMatch
			if items[i].State == "unknown" && next.State != "unknown" {
				items[i].State = next.State
			}
			if strings.TrimSpace(items[i].HeadRef) == "" {
				items[i].HeadRef = next.HeadRef
			}
			if strings.TrimSpace(items[i].Title) == "" || strings.HasPrefix(items[i].Title, "#") {
				items[i].Title = next.Title
			}
			return items
		}
	}
	return append(items, next)
}

func servePullRequestSame(a servePullRequest, b servePullRequest) bool {
	if a.URL != "" && b.URL != "" {
		return strings.EqualFold(a.URL, b.URL)
	}
	return a.Number > 0 && b.Number > 0 && a.Number == b.Number
}

func sortServePullRequests(items []servePullRequest) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].BranchMatch != items[j].BranchMatch {
			return items[i].BranchMatch
		}
		if items[i].SourceMatch != items[j].SourceMatch {
			return items[i].SourceMatch
		}
		if items[i].TitleMatch != items[j].TitleMatch {
			return items[i].TitleMatch
		}
		return items[i].Number > items[j].Number
	})
}

func servePullRequestMeta(pr servePullRequest) string {
	parts := make([]string, 0, 4)
	if pr.Number > 0 {
		parts = append(parts, fmt.Sprintf("#%d", pr.Number))
	}
	if strings.TrimSpace(pr.HeadRef) != "" {
		parts = append(parts, pr.HeadRef)
	}
	switch {
	case pr.BranchMatch:
		parts = append(parts, "current branch")
	case pr.SourceMatch:
		parts = append(parts, "workspace source")
	case pr.TitleMatch:
		parts = append(parts, "title match")
	}
	return strings.Join(parts, " · ")
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

func loadServeWorkspaceFiles(root string, workspaceID string, workspace serveWorkspace) ([]serveFileTreeNode, []serveFileView) {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	workspaceView := serveWorkspaceBoardView(workspace)
	if b, err := os.ReadFile(filepath.Join(wsPath, workspaceDocumentFilename)); err == nil {
		workspaceView.SourceHTML = template.HTML(template.HTMLEscapeString(string(b)))
	}
	viewsByPath := map[string]serveFileView{
		workspaceDocumentFilename: workspaceView,
		"repos/":                  serveReposView(workspace),
	}
	tree := []serveFileTreeNode{
		{Name: workspaceDocumentFilename, Path: workspaceDocumentFilename, Type: "workspace"},
		{Name: "repos", Path: "repos/", Type: "special"},
	}

	_ = filepath.WalkDir(wsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == wsPath {
			return nil
		}
		rel, err := filepath.Rel(wsPath, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", "repos":
				return filepath.SkipDir
			}
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if name == ".DS_Store" || name == ".kra.meta.json" || rel == workspaceDocumentFilename {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		insertServeTreeFile(&tree, strings.Split(rel, "/"), rel, "")
		viewsByPath[rel] = serveFileViewForContent(rel, string(content))
		return nil
	})

	sortServeTree(tree)
	views := make([]serveFileView, 0, len(viewsByPath))
	views = append(views, viewsByPath[workspaceDocumentFilename])
	if readme, ok := viewsByPath["README.md"]; ok {
		views = append(views, readme)
	}
	views = append(views, viewsByPath["repos/"])
	paths := make([]string, 0, len(viewsByPath))
	for path := range viewsByPath {
		if path == workspaceDocumentFilename || path == "README.md" || path == "repos/" {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		views = append(views, viewsByPath[path])
	}
	return tree, views
}

func insertServeTreeFile(nodes *[]serveFileTreeNode, parts []string, rel string, prefix string) {
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		*nodes = append(*nodes, serveFileTreeNode{Name: parts[0], Path: rel, Type: "file"})
		return
	}
	dirName := parts[0]
	dirPath := dirName
	if prefix != "" {
		dirPath = prefix + "/" + dirName
	}
	for i := range *nodes {
		if (*nodes)[i].Type == "dir" && (*nodes)[i].Name == dirName {
			insertServeTreeFile(&(*nodes)[i].Children, parts[1:], rel, dirPath)
			return
		}
	}
	*nodes = append(*nodes, serveFileTreeNode{Name: dirName, Path: dirPath, Type: "dir"})
	insertServeTreeFile(&(*nodes)[len(*nodes)-1].Children, parts[1:], rel, dirPath)
}

func sortServeTree(nodes []serveFileTreeNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		priority := func(node serveFileTreeNode) int {
			switch node.Type {
			case "workspace":
				return 0
			case "special":
				return 1
			case "dir":
				return 2
			default:
				return 3
			}
		}
		if priority(nodes[i]) != priority(nodes[j]) {
			return priority(nodes[i]) < priority(nodes[j])
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})
	for i := range nodes {
		sortServeTree(nodes[i].Children)
	}
}

func serveWorkspaceBoardView(workspace serveWorkspace) serveFileView {
	return serveFileView{
		ID:    serveViewID(workspaceDocumentFilename),
		Path:  workspaceDocumentFilename,
		Name:  workspaceDocumentFilename,
		Kind:  "workspace",
		Badge: "workspace",
		Meta:  "board view",
	}
}

func serveReposView(workspace serveWorkspace) serveFileView {
	var b strings.Builder
	b.WriteString(`<section class="repo-overview"><div class="view-intro"><div><h2>Repositories</h2><p class="meta">repository context opened from repos/</p></div><span class="badge">`)
	b.WriteString(template.HTMLEscapeString(fmt.Sprintf("%d linked", len(workspace.Repos))))
	b.WriteString(`</span></div><div class="repo-grid">`)
	if len(workspace.Repos) == 0 {
		b.WriteString(`<p class="empty">No managed repositories.</p>`)
	}
	for _, repo := range workspace.Repos {
		b.WriteString(`<article class="repo-card large"><strong>`)
		b.WriteString(template.HTMLEscapeString(repo.Name))
		if repo.Missing {
			b.WriteString(` <span class="small">(missing)</span>`)
		}
		b.WriteString(`</strong><span class="repo-branch">branch `)
		b.WriteString(template.HTMLEscapeString(repo.Branch))
		b.WriteString(`</span><div class="repo-pr-list">`)
		if len(repo.PullRequests) == 0 {
			b.WriteString(`<span class="empty">No matching pull requests.</span>`)
		} else {
			for _, pr := range repo.PullRequests {
				b.WriteString(`<a class="repo-pr" href="`)
				b.WriteString(template.HTMLEscapeString(pr.URL))
				b.WriteString(`" target="_blank" rel="noreferrer"><span class="repo-pr-main"><span class="repo-pr-title">`)
				b.WriteString(template.HTMLEscapeString(firstNonEmpty(pr.Title, fmt.Sprintf("#%d", pr.Number))))
				b.WriteString(`</span><span class="repo-pr-meta">`)
				meta := servePullRequestMeta(pr)
				b.WriteString(template.HTMLEscapeString(meta))
				b.WriteString(`</span></span><span class="pr-state pr-state-`)
				b.WriteString(template.HTMLEscapeString(pr.State))
				b.WriteString(`">`)
				b.WriteString(template.HTMLEscapeString(pr.State))
				b.WriteString(`</span></a>`)
			}
		}
		b.WriteString(`</div></article>`)
	}
	b.WriteString(`</div></section>`)
	return serveFileView{
		ID:    serveViewID("repos/"),
		Path:  "repos/",
		Name:  "repos/",
		Kind:  "repos",
		Badge: "special view",
		Meta:  "repository overview",
		HTML:  template.HTML(b.String()),
	}
}

func serveFileViewForContent(rel string, content string) serveFileView {
	ext := strings.ToLower(filepath.Ext(rel))
	view := serveFileView{
		ID:         serveViewID(rel),
		Path:       rel,
		Name:       rel,
		SourceHTML: template.HTML(template.HTMLEscapeString(content)),
	}
	switch ext {
	case ".md", ".markdown":
		view.Kind = "markdown"
		view.Badge = "markdown"
		view.Meta = "rendered markdown"
		view.HTML = renderServeMarkdown(content)
	case ".html", ".htm":
		view.Kind = "html"
		view.Badge = "html"
		view.Meta = "rendered preview"
	case ".go":
		view.Kind = "source"
		view.Language = "go"
		view.Badge = "go"
		view.Meta = "source"
		view.HTML = renderServeCodeHTML(content, "go")
	case ".sh", ".bash", ".zsh":
		view.Kind = "source"
		view.Language = "shell"
		view.Badge = "shell"
		view.Meta = "source"
		view.HTML = renderServeCodeHTML(content, "shell")
	case ".js", ".mjs", ".cjs":
		view.Kind = "source"
		view.Language = "javascript"
		view.Badge = "javascript"
		view.Meta = "source"
		view.HTML = renderServeCodeHTML(content, "javascript")
	default:
		view.Kind = "source"
		view.Badge = "source"
		view.Meta = "source"
		view.HTML = renderServeCodeHTML(content, "")
	}
	return view
}

func renderServeCodeHTML(content string, language string) template.HTML {
	escaped := template.HTMLEscapeString(content)
	switch language {
	case "go":
		escaped = serveHighlightWords(escaped, []string{"package", "import", "func", "return", "if", "else", "for", "range", "var", "const", "type", "struct", "interface", "defer", "go"}, "code-keyword")
	case "shell":
		escaped = serveHighlightWords(escaped, []string{"if", "then", "else", "fi", "for", "do", "done", "case", "esac", "set", "exit", "export", "local", "function"}, "code-keyword")
	case "javascript":
		escaped = serveHighlightWords(escaped, []string{"const", "let", "var", "function", "return", "if", "else", "for", "await", "async", "import", "from"}, "code-keyword")
	}
	return template.HTML(escaped)
}

func serveHighlightWords(input string, words []string, className string) string {
	for _, word := range words {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
		input = re.ReplaceAllString(input, `<span class="`+className+`">$0</span>`)
	}
	return input
}

func serveViewID(path string) string {
	re := regexp.MustCompile(`[^A-Za-z0-9_-]+`)
	id := strings.Trim(re.ReplaceAllString(path, "-"), "-")
	if id == "" {
		return "view"
	}
	return "view-" + id
}

func writeServeHTML(w http.ResponseWriter, tmpl string, data servePageData) {
	t := template.Must(template.New("serve").Funcs(template.FuncMap{
		"workspaceHref": serveWorkspaceHref,
		"listStatuses": func() []string {
			return serveStatusIDs()
		},
		"itemsByStatus": func(overview wstask.Overview, status string) []wstask.Item {
			return overview.ItemsByStatus(wstask.Status(status))
		},
		"progress":    serveTaskProgress,
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

func serveStatusIDs() []string {
	return []string{
		string(wstask.StatusTodo),
		string(wstask.StatusDoing),
		string(wstask.StatusBlocked),
		string(wstask.StatusDone),
	}
}

type serveProgress struct {
	Done    int `json:"done"`
	Total   int `json:"total"`
	Percent int `json:"percent"`
}

func serveTaskProgress(overview wstask.Overview) serveProgress {
	total := overview.Counts.Total
	done := overview.Counts.Done
	if total < 0 {
		total = 0
	}
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	percent := 0
	if total > 0 {
		percent = done * 100 / total
	}
	return serveProgress{
		Done:    done,
		Total:   total,
		Percent: percent,
	}
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
:root{color-scheme:light;--bg:#f4f7fb;--sidebar:rgba(255,255,255,.88);--surface:#fff;--surface-2:#f5f8fb;--ink:#18212f;--muted:#647085;--line:#d7e0eb;--accent:#2364aa;--accent-2:#1b8a73;--brand-bg:#e9f2fc;--brand-line:#b7cee8;--nav-hover:#e8f2fb;--lane-head:#fff;--lane-shadow:0 1px 1px rgba(19,34,54,.05);--card:#fff;--card-line:#d8e0ea;--card-hover:0 0 0 3px rgba(35,100,170,.12);--progress-bg:#dce6ef;--progress-line:#c6d3df;--button-ink:#fff;--ok:#17855f;--warn:#b46b07;--danger:#c64052;--shadow:0 18px 44px rgba(19,34,54,.12);--board-column-height:360px;--readme-code:#0f172a;--readme-quote:#f0f9ff;--readme-quote-line:#0ea5a3;--readme-table:#f8fbff}body[data-theme=dark]{color-scheme:dark;--bg:#15181d;--sidebar:#101318;--surface:#1d2229;--surface-2:#161b22;--ink:#eff4f8;--muted:#a2adba;--line:#323b46;--accent:#75b8ff;--accent-2:#55c7aa;--brand-bg:#172b34;--brand-line:#31545d;--nav-hover:#202a34;--lane-head:#20262e;--lane-shadow:0 18px 38px rgba(0,0,0,.2);--card:#232a33;--card-line:#3a4552;--card-hover:0 0 0 3px rgba(117,184,255,.14);--progress-bg:#2b3540;--progress-line:#3a4653;--button-ink:#102033;--readme-code:#090d13;--readme-quote:#16252d;--readme-quote-line:#55c7aa;--readme-table:#202830}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font-family:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;font-size:14px;letter-spacing:0}button,a{font:inherit}a{color:var(--accent);text-decoration:none;font-weight:800}a:hover{text-decoration:underline}.app{min-height:100vh;display:grid;grid-template-columns:260px minmax(0,1fr);background:var(--bg)}.sidebar{height:100vh;position:sticky;top:0;padding:22px 18px;border-right:1px solid var(--line);background:var(--sidebar);backdrop-filter:blur(18px)}.brand{display:flex;align-items:center;gap:11px;margin-bottom:24px}.brand-mark{width:38px;height:38px;display:grid;place-items:center;border:1px solid var(--brand-line);border-radius:8px;background:var(--brand-bg);color:var(--accent);font-weight:900}.brand strong{display:block;font-size:16px}.meta,.small,.side-label-link,.workspace-title,.workspace-link-title{color:var(--muted)}.meta,.small,.side-label-link{font-size:12px}.side-label-link{display:block;margin:18px 8px 8px;font-weight:900;text-transform:uppercase}.workspace-link{width:100%;min-height:58px;display:flex;align-items:flex-start;gap:10px;padding:10px;border-radius:8px;color:var(--ink)}.workspace-link.active,.workspace-link:hover{background:var(--nav-hover);text-decoration:none}.workspace-link-text{min-width:0;display:grid;gap:2px;flex:1}.workspace-link-id{font-weight:900}.workspace-link-title{color:var(--muted);font-size:12px;font-weight:600;line-height:1.35;overflow-wrap:anywhere}.progress-mini{width:100%;height:5px;margin-top:5px;border-radius:999px;background:var(--progress-bg);overflow:hidden}.progress-mini-fill,.progress-fill{display:block;height:100%;border-radius:999px;background:linear-gradient(90deg,var(--accent-2),var(--accent))}.dot{width:9px;height:9px;margin-top:5px;border-radius:50%;background:var(--ok);flex:0 0 auto}main{min-width:0;padding:24px 28px 42px}.topbar{display:flex;justify-content:space-between;align-items:flex-start;gap:18px;margin-bottom:18px}.topbar-actions{display:flex;align-items:center;gap:10px;flex-wrap:wrap;justify-content:flex-end}h1,h2,h3,p{margin:0}h1{font-size:28px;line-height:1.15;letter-spacing:0}.btn{min-height:36px;display:inline-flex;align-items:center;padding:0 12px;border:1px solid var(--accent);border-radius:8px;background:var(--accent);color:var(--button-ink);cursor:pointer;font-weight:800}.btn:hover{text-decoration:none}.theme-toggle{min-height:36px;display:inline-flex;align-items:center;gap:8px;padding:0 10px;border:1px solid var(--line);border-radius:8px;background:var(--surface);color:var(--ink);cursor:pointer;font-weight:800}.theme-toggle:hover{border-color:var(--accent)}.theme-toggle-track{width:32px;height:18px;padding:2px;border-radius:999px;background:var(--progress-bg)}.theme-toggle-knob{display:block;width:14px;height:14px;border-radius:999px;background:var(--surface);box-shadow:0 1px 3px rgba(16,24,39,.24);transition:transform .18s ease}body[data-theme=dark] .theme-toggle-knob{transform:translateX(14px)}.board{display:grid;gap:14px}.swimlane,.detail{border:1px solid var(--line);border-radius:8px;background:var(--surface);box-shadow:var(--lane-shadow);overflow:hidden}.detail{box-shadow:var(--shadow)}.swimlane-head,.detail-header{display:flex;justify-content:space-between;align-items:flex-start;gap:14px;padding:14px 16px;border-bottom:1px solid var(--line);background:var(--lane-head)}.workspace-id{display:block;color:var(--ink);font-size:17px;font-weight:900}.workspace-title{margin-top:3px;line-height:1.4}.progress{min-width:220px;display:grid;gap:7px}.progress-meta{display:flex;justify-content:space-between;gap:10px;color:var(--muted);font-size:12px;font-weight:900;text-transform:uppercase}.progress-track{height:10px;border:1px solid var(--progress-line);border-radius:999px;background:var(--progress-bg);overflow:hidden}.status-grid{display:grid;grid-template-columns:repeat(4,minmax(190px,1fr));gap:10px;padding:12px;overflow-x:auto}.detail-board{padding:14px 18px 4px}.detail-board .status-grid{padding:0}.status-column{height:var(--board-column-height);border:1px solid var(--line);border-radius:8px;background:var(--surface-2);padding:10px;overflow-y:auto;overscroll-behavior:contain}.column-title{display:flex;justify-content:space-between;gap:8px;margin-bottom:9px;color:var(--muted);font-size:12px;font-weight:900;text-transform:uppercase}.task-card{display:block;margin-bottom:8px;padding:10px;border:1px solid var(--card-line);border-radius:8px;background:var(--card);color:var(--ink);line-height:1.4;font-weight:700}.task-card:hover{border-color:var(--accent);box-shadow:var(--card-hover);text-decoration:none}.task-card .small{display:block;margin-top:5px;font-weight:500}.tabs{display:flex;gap:8px;padding:12px 18px 0}.tab{min-height:36px;padding:0 12px;display:inline-flex;align-items:center;border:1px solid var(--line);border-radius:8px 8px 0 0;background:var(--surface);color:var(--muted);font-weight:900}.tab.active{color:var(--accent);border-color:var(--accent);background:var(--brand-bg)}.tab-panel{display:none;padding:18px}.tab-panel:target{display:block}.tab-panel.default{display:block}body:has(.tab-panel:target) .tab-panel.default:not(:target){display:none}.readme{width:100%;max-width:none;border:1px solid var(--line);border-radius:8px;background:var(--surface);padding:22px;line-height:1.7;overflow-wrap:anywhere}.readme>*:first-child{margin-top:0}.readme>*:last-child{margin-bottom:0}.readme h1,.readme h2,.readme h3{margin:1.45em 0 .55em;color:var(--ink);line-height:1.24}.readme h1{padding-bottom:.35em;border-bottom:1px solid var(--line);font-size:30px}.readme h2{padding-bottom:.28em;border-bottom:1px solid var(--line);font-size:23px}.readme h3{font-size:18px}.readme p,.readme ul,.readme ol,.readme blockquote,.readme table,.readme pre{margin:0 0 1em}.readme ul,.readme ol{padding-left:1.45em}.readme li+li{margin-top:.25em}.readme code{border:1px solid var(--line);border-radius:6px;background:var(--brand-bg);color:var(--accent-2);padding:.12em .34em;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.92em}.readme pre{overflow:auto;border-radius:8px;background:var(--readme-code);color:#e5eefc;padding:16px}.readme pre code{border:0;background:transparent;color:inherit;padding:0}.readme blockquote{border-left:5px solid var(--readme-quote-line);border-radius:0 8px 8px 0;background:var(--readme-quote);color:var(--muted);padding:12px 14px}.readme table{width:100%;border-collapse:collapse;display:block;overflow:auto}.readme th,.readme td{border:1px solid var(--line);padding:8px 10px}.readme th{background:var(--readme-table);font-weight:900}.readme tr:nth-child(2n) td{background:var(--surface-2)}.readme hr{height:1px;border:0;background:var(--line);margin:24px 0}.readme img{max-width:100%;border-radius:8px}.readme input[type=checkbox]{margin-right:.45em}.mermaid{margin:0 0 1em;padding:16px;border:1px solid var(--line);border-radius:8px;background:var(--surface-2);overflow:auto}.empty{color:var(--muted)}.repo-table{display:grid;gap:8px}.repo-row{display:grid;grid-template-columns:minmax(120px,2fr) minmax(120px,2fr) minmax(220px,6fr);gap:12px;align-items:center;min-height:58px;padding:10px 12px;border:1px solid var(--line);border-radius:8px;background:var(--surface)}.repo-row a{overflow-wrap:anywhere}.repo-row.header{min-height:36px;background:transparent;border-color:transparent;color:var(--muted);font-size:12px;font-weight:900;text-transform:uppercase}@media(max-width:1100px){.app{grid-template-columns:1fr}.sidebar{position:static;height:auto;border-right:0;border-bottom:1px solid var(--line)}.status-grid{grid-template-columns:repeat(4,minmax(220px,1fr))}}@media(max-width:720px){main{padding:18px 14px 32px}.topbar,.detail-header,.swimlane-head{flex-direction:column;align-items:stretch}.topbar-actions{justify-content:flex-start}.progress{min-width:0}.readme{padding:16px}.readme h1{font-size:24px}.repo-row{grid-template-columns:1fr}}
`

const serveSmartViewStyles = `
body[data-live-page=detail] .app{grid-template-columns:300px minmax(0,1fr)}.back-link{min-height:32px;display:inline-flex;align-items:center;margin-bottom:12px;color:var(--muted);font-size:12px;font-weight:900}.back-link:before{content:"<";margin-right:6px}.file-tree{min-width:0;border-top:1px solid var(--line);padding-top:10px}.tree-row{width:100%;min-height:30px;display:flex;align-items:center;gap:7px;padding:0 8px;border:0;border-radius:6px;background:transparent;color:var(--ink);cursor:pointer;font-size:13px;font-weight:700;text-align:left}.tree-row.active,.tree-row:hover{background:var(--nav-hover);color:var(--accent);text-decoration:none}.tree-children{margin-left:16px}.tree-children.hidden{display:none}.folder-chevron{width:7px;height:7px;border-right:2px solid currentColor;border-bottom:2px solid currentColor;transform:rotate(45deg) translateY(-1px)}.tree-row.folder.collapsed .folder-chevron{transform:rotate(-45deg)}.folder-icon,.special-folder-icon{width:13px;height:10px;border:1px solid var(--progress-line);border-top:4px solid var(--progress-line);border-radius:2px;background:var(--surface);flex:0 0 auto}.special-folder-icon{border-color:var(--accent);border-top-color:var(--accent);background:var(--brand-bg)}.file-dot{width:9px;height:11px;border:1px solid var(--progress-line);border-radius:2px;background:var(--surface);flex:0 0 auto}.open-marker{margin-left:auto;color:var(--muted);font-size:11px;font-weight:900;text-transform:uppercase}.file-detail .detail-header{display:grid;grid-template-columns:minmax(0,1fr) auto minmax(220px,360px);align-items:start}.header-meta{display:flex;gap:8px;flex-wrap:wrap;justify-content:center}.header-meta span{min-height:26px;display:inline-flex;align-items:center;padding:0 9px;border-radius:999px;background:var(--surface-2);color:var(--muted);font-size:12px;font-weight:900;text-transform:uppercase}.layout-note{color:var(--muted);font-size:12px;line-height:1.45;text-align:right}.file-viewer{min-width:0;display:grid;grid-template-rows:auto minmax(0,1fr);background:var(--surface)}.file-tabs{display:flex;gap:4px;padding:8px 10px 0;border-bottom:1px solid var(--line);background:var(--surface-2);overflow-x:auto}.file-tab{min-height:34px;display:inline-flex;align-items:center;gap:8px;padding:0 10px;border:1px solid var(--line);border-bottom:0;border-radius:8px 8px 0 0;background:var(--brand-bg);color:var(--muted);cursor:pointer;font-size:13px;font-weight:800;white-space:nowrap}.file-tab.active{background:var(--surface);color:var(--ink)}.tab-close{width:8px;height:8px;position:relative;flex:0 0 auto}.tab-close:before,.tab-close:after{content:"";position:absolute;top:3px;left:0;width:8px;height:2px;border-radius:999px;background:var(--muted)}.tab-close:before{transform:rotate(45deg)}.tab-close:after{transform:rotate(-45deg)}.file-panel[hidden],.file-tab[hidden]{display:none}.viewer-head{display:flex;justify-content:space-between;align-items:flex-start;gap:12px;padding:14px 16px;border-bottom:1px solid var(--line)}.viewer-head h2{font-size:14px;line-height:1.25}.badge{min-height:26px;display:inline-flex;align-items:center;padding:0 9px;border-radius:999px;background:var(--surface-2);color:var(--muted);font-size:12px;font-weight:900;white-space:nowrap}.viewer-body{min-height:0;overflow:auto}.rendered-markdown{max-width:920px;padding:26px 30px 42px;line-height:1.7}.rendered-markdown h1,.rendered-markdown h2,.rendered-markdown h3{margin:1.35em 0 .55em}.rendered-markdown h1{margin-top:0;padding-bottom:.35em;border-bottom:1px solid var(--line);font-size:30px}.rendered-markdown h2{padding-bottom:.28em;border-bottom:1px solid var(--line);font-size:23px}.rendered-markdown pre,.code-source,.source-details pre{margin:0;overflow:auto;background:var(--readme-code);color:#e5eefc;padding:16px;font:13px/1.65 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}.code-keyword{color:#93c5fd;font-weight:900}.code-source{min-height:420px}.html-preview{display:block;background:var(--surface-2)}.preview-toolbar{min-height:38px;display:flex;align-items:center;gap:7px;padding:0 12px;border-bottom:1px solid var(--line);background:var(--surface)}.preview-toolbar span{width:10px;height:10px;border-radius:999px;background:var(--progress-line)}.preview-toolbar strong{margin-left:6px;color:var(--muted);font-size:12px}.html-render{display:block;width:100%;height:auto;min-height:0;border:0;background:#fff}.source-details{border-top:1px solid var(--line);background:var(--surface)}.source-details summary{min-height:38px;display:flex;align-items:center;padding:0 14px;color:var(--muted);cursor:pointer;font-weight:900}.workspace-board-view,.repo-overview{padding:18px}.view-intro{display:flex;justify-content:space-between;align-items:flex-start;gap:16px;margin-bottom:14px}.view-intro h2{font-size:18px}.secondary-btn{min-height:32px;display:inline-flex;align-items:center;padding:0 10px;border:1px solid var(--line);border-radius:8px;background:var(--surface);color:var(--muted);font-weight:900}.repo-grid{display:grid;grid-template-columns:repeat(2,minmax(260px,1fr));gap:12px}.repo-card{display:grid;gap:5px;margin-top:10px;padding:10px;border:1px solid var(--line);border-radius:8px;background:var(--surface)}.repo-card.large{min-height:108px}.repo-card span{color:var(--muted);font-size:12px;overflow-wrap:anywhere}.repo-branch{font-weight:700}.repo-pr-list{display:grid;gap:6px;margin-top:7px}.repo-pr{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px;align-items:center;padding:8px;border:1px solid var(--line);border-radius:8px;background:var(--surface-2);color:var(--ink);font-weight:700}.repo-pr:hover{border-color:var(--accent);text-decoration:none}.repo-pr-main{min-width:0;display:grid;gap:3px}.repo-pr-title{color:var(--ink);font-size:13px}.repo-pr-meta{color:var(--muted);font-size:11px;font-weight:700}.pr-state{min-height:22px;display:inline-flex;align-items:center;padding:0 8px;border-radius:999px;background:var(--brand-bg);color:var(--accent);font-size:11px;font-weight:900;text-transform:uppercase}.pr-state-merged{background:#dcfce7;color:#166534}.pr-state-closed{background:#fee2e2;color:#991b1b}.pr-state-draft{background:#f3e8ff;color:#6b21a8}.pr-state-unknown{background:var(--surface);color:var(--muted);border:1px solid var(--line)}@media(max-width:1100px){body[data-live-page=detail] .app{grid-template-columns:1fr}}@media(max-width:720px){.file-detail .detail-header{display:flex;flex-direction:column;align-items:stretch}.layout-note{text-align:left}.rendered-markdown h1{font-size:24px}.repo-grid{grid-template-columns:1fr}.repo-pr{grid-template-columns:1fr}}
`

const serveLiveScript = `<script>
(function(){
  var statuses=[
    {id:'todo',title:'Todo'},
    {id:'doing',title:'Doing'},
    {id:'blocked',title:'Blocked'},
    {id:'done',title:'Done'}
  ];
  function storedTheme(){
    try{return localStorage.getItem('kraServeTheme')||'';}catch(e){return '';}
  }
  function saveTheme(theme){
    try{localStorage.setItem('kraServeTheme',theme);}catch(e){}
  }
  function setTheme(theme){
    theme=theme==='dark'?'dark':'light';
    document.body.setAttribute('data-theme',theme);
    var button=document.querySelector('[data-theme-toggle]');
    var label=document.querySelector('[data-theme-label]');
    if(button){button.setAttribute('aria-pressed',String(theme==='dark'));}
    if(label){label.textContent=theme==='dark'?'Dark':'Light';}
    saveTheme(theme);
  }
  function initTheme(){
    var preferred=storedTheme();
    if(!preferred&&window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches){preferred='dark';}
    setTheme(preferred||document.body.getAttribute('data-theme')||'light');
    var button=document.querySelector('[data-theme-toggle]');
    if(button){button.addEventListener('click',function(){setTheme(document.body.getAttribute('data-theme')==='dark'?'light':'dark');});}
  }
  function clampPercent(value){value=Number(value)||0;return Math.max(0,Math.min(100,value));}
  function node(tag,className,text){
    var el=document.createElement(tag);
    if(className){el.className=className;}
    if(text!==undefined){el.textContent=text;}
    return el;
  }
  function progressView(progress,mini){
    progress=progress||{done:0,total:0,percent:0};
    var outer=node('span',mini?'progress-mini':'progress');
    outer.setAttribute('aria-label','task progress');
    if(!mini){
      var meta=node('div','progress-meta');
      meta.append(node('span','', 'Progress'),node('span','', String(progress.done||0)+' / '+String(progress.total||0)+' done'));
      outer.append(meta);
    }
    var track=node('span',mini?'':'progress-track');
    if(mini){track=outer;}else{outer.append(track);}
    var fill=node('span',mini?'progress-mini-fill':'progress-fill');
    fill.style.width=String(clampPercent(progress.percent))+'%';
    track.append(fill);
    return outer;
  }
  function taskCard(task,href){
    var card=node(href?'a':'div','task-card');
    if(href){card.href=href;}
    card.append(document.createTextNode(task.title||task.id||'Untitled task'));
    card.append(node('span','small',task.id||''));
    return card;
  }
  function statusColumn(workspace,status,href){
    var section=node('section','status-column');
    var items=(workspace.tasks&&workspace.tasks[status.id])||[];
    var head=node('div','column-title');
    head.append(node('span','',status.title),node('span','',String(items.length)));
    section.append(head);
    if(items.length===0){
      section.append(node('span','small','No cards'));
      return section;
    }
    items.forEach(function(item){section.append(taskCard(item,href));});
    return section;
  }
  function statusGrid(workspace,href){
    var grid=node('div','status-grid');
    statuses.forEach(function(status){grid.append(statusColumn(workspace,status,href));});
    return grid;
  }
  function sidebarLink(workspace,selectedID){
    var link=node('a','workspace-link'+(workspace.id===selectedID?' active':''));
    link.href=workspace.href||('/workspaces/'+encodeURIComponent(workspace.id)+'/');
    link.append(node('span','dot'));
    var text=node('span','workspace-link-text');
    text.append(node('span','workspace-link-id',workspace.id));
    text.append(node('span','workspace-link-title',workspace.title||workspace.id));
    text.append(progressView(workspace.progress,true));
    link.append(text);
    return link;
  }
  function renderSidebar(workspaces,selectedID){
    var sidebar=document.querySelector('[data-live-sidebar]');
    if(!sidebar){return;}
    sidebar.replaceChildren.apply(sidebar,workspaces.map(function(workspace){return sidebarLink(workspace,selectedID);}));
  }
  function swimlane(workspace){
    var article=node('article','swimlane');
    var head=node('header','swimlane-head');
    var titleWrap=node('div');
    var idLink=node('a','workspace-id',workspace.id);
    idLink.href=workspace.href||('/workspaces/'+encodeURIComponent(workspace.id)+'/');
    titleWrap.append(idLink,node('p','workspace-title',workspace.title||workspace.id));
    head.append(titleWrap,progressView(workspace.progress,false));
    article.append(head,statusGrid(workspace,workspace.href));
    return article;
  }
  function renderOverview(workspaces){
    var board=document.querySelector('[data-live-board]');
    if(!board){return;}
    if(workspaces.length===0){
      board.replaceChildren(node('p','empty','No open workspaces.'));
      return;
    }
    board.replaceChildren.apply(board,workspaces.map(swimlane));
  }
  function renderDetail(workspace){
    if(!workspace){return;}
    var title=document.querySelector('[data-live-detail-title]');
    if(title){title.textContent=workspace.id+': '+(workspace.title||workspace.id);}
    var header=document.querySelector('[data-live-detail-header]');
    if(header){
      var titleWrap=node('div');
      titleWrap.append(node('strong','workspace-id',workspace.id),node('p','workspace-title',workspace.title||workspace.id));
      var meta=node('div','header-meta');
      meta.append(node('span','',String((workspace.progress&&workspace.progress.done)||0)+' / '+String((workspace.progress&&workspace.progress.total)||0)+' done'));
      var note=node('p','layout-note','Open files and workspace views as tabs.');
      header.replaceChildren(titleWrap,meta,note);
    }
    var board=document.querySelector('[data-live-workspace-board]');
    if(board){board.replaceChildren(statusGrid(workspace,''));}
  }
  async function refreshServeData(){
    try{
      var res=await fetch('/api/workspaces?ts='+Date.now(),{cache:'no-store'});
      if(!res.ok){return;}
      var data=await res.json();
      var workspaces=Array.isArray(data.workspaces)?data.workspaces:[];
      var selectedID=document.body.getAttribute('data-workspace-id')||'';
      renderSidebar(workspaces,selectedID);
      if(document.body.getAttribute('data-live-page')==='overview'){
        renderOverview(workspaces);
      }else if(selectedID){
        renderDetail(workspaces.find(function(workspace){return workspace.id===selectedID;}));
      }
    }catch(e){}
  }
  window.refreshServeData=refreshServeData;
  initTheme();
})();
</script>`

const serveOverviewTemplate = `<!doctype html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>kra serve - All Workspaces</title>
  <style>` + serveStyles + serveSmartViewStyles + `</style>
</head>
<body data-live-page="overview" data-theme="light">
<div class="app">
  <aside class="sidebar">
    <div class="brand"><div class="brand-mark">kra</div><div><strong>Workspace Boards</strong><span class="small">open workspaces only</span></div></div>
    <a class="side-label-link" href="/workspaces/">Open workspaces</a>
    <nav data-live-sidebar>{{range .Workspaces}}{{$progress := progress .Tasks}}<a class="workspace-link" href="{{workspaceHref .ID}}"><span class="dot"></span><span class="workspace-link-text"><span class="workspace-link-id">{{.ID}}</span><span class="workspace-link-title">{{.Title}}</span><span class="progress-mini" aria-label="task progress"><span class="progress-mini-fill" style="width: {{$progress.Percent}}%"></span></span></span></a>{{end}}</nav>
  </aside>
  <main>
    <header class="topbar"><div><h1>All Workspaces</h1><p class="meta">{{.Root}} ・ read-only ・ generated from workspace.md</p></div><div class="topbar-actions"><button class="theme-toggle" type="button" data-theme-toggle aria-pressed="false"><span class="theme-toggle-track"><span class="theme-toggle-knob"></span></span><span data-theme-label>Light</span></button><a class="btn" href="/workspaces/">Refresh</a></div></header>
    <section class="board" aria-label="workspace kanban" data-live-board>
      {{range .Workspaces}}{{template "workspaceBoard" .}}{{else}}<p class="empty">No open workspaces.</p>{{end}}
    </section>
  </main>
</div>
` + serveLiveScript + `
</body>
</html>
{{define "workspaceBoard"}}
<article class="swimlane">
  <header class="swimlane-head">
    <div><a class="workspace-id" href="{{workspaceHref .ID}}">{{.ID}}</a><p class="workspace-title">{{.Title}}</p></div>
  {{$progress := progress .Tasks}}<div class="progress" aria-label="task progress"><div class="progress-meta"><span>Progress</span><span>{{$progress.Done}} / {{$progress.Total}} done</span></div><div class="progress-track"><div class="progress-fill" style="width: {{$progress.Percent}}%"></div></div></div>
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
  <style>` + serveStyles + serveSmartViewStyles + `</style>
</head>
<body data-live-page="detail" data-workspace-id="{{.Workspace.ID}}" data-theme="light">
<div class="app">
  <aside class="sidebar">
    <div class="brand"><div class="brand-mark">kra</div><div><strong>{{.Workspace.ID}}</strong><span class="small">workspace root</span></div></div>
    <a class="back-link" href="/workspaces/">All workspaces</a>
    <nav class="file-tree" aria-label="workspace file tree">{{range .FileTree}}{{template "fileTreeNode" .}}{{end}}</nav>
  </aside>
  <main>
    <header class="topbar"><div><h1 data-live-detail-title>{{.Workspace.ID}}: {{.Workspace.Title}}</h1><p class="meta">workspace files and smart views</p></div><div class="topbar-actions"><button class="theme-toggle" type="button" data-theme-toggle aria-pressed="false"><span class="theme-toggle-track"><span class="theme-toggle-knob"></span></span><span data-theme-label>Light</span></button><a class="btn" href="/workspaces/">All Workspaces</a></div></header>
    <section class="detail file-detail">
      <div class="detail-header" data-live-detail-header><div><strong class="workspace-id">{{.Workspace.ID}}</strong><p class="workspace-title">{{.Workspace.Title}}</p></div>{{$progress := progress .Workspace.Tasks}}<div class="header-meta"><span>{{$progress.Done}} / {{$progress.Total}} done</span><span>{{.Workspace.SourceURL}}</span></div><p class="layout-note">Open files and workspace views as tabs.</p></div>
      <div class="file-viewer" aria-label="selected file">
        <div class="file-tabs" role="tablist" aria-label="open files">
          {{range .FileViews}}<button type="button" class="file-tab {{if eq .Path "workspace.md"}}is-open active{{end}}" data-file-tab="{{.Path}}" data-view-id="{{.ID}}" {{if ne .Path "workspace.md"}}hidden{{end}}><span>{{.Path}}</span><span class="tab-close" aria-hidden="true" data-close-view="{{.Path}}"></span></button>{{end}}
        </div>
        <div class="file-panels">
          {{range .FileViews}}<section class="file-panel {{if eq .Path "workspace.md"}}active{{end}}" id="{{.ID}}" data-file-panel="{{.Path}}" data-kind="{{.Kind}}" {{if ne .Path "workspace.md"}}hidden{{end}}>
            <header class="viewer-head"><div><h2>{{.Name}}</h2><p class="meta">{{.Meta}}</p></div><span class="badge">{{.Badge}}</span></header>
            <div class="viewer-body">
              {{if eq .Kind "workspace"}}
                <section class="workspace-board-view" aria-label="workspace.md board view"><div class="view-intro"><div><h2>Board from workspace.md</h2><p class="meta">source-backed task state rendered as a workspace board</p></div><a class="secondary-btn" href="#{{.ID}}-source">View source</a></div><div class="status-grid" data-live-workspace-board>{{range $status := listStatuses}}{{template "detailStatusColumn" dict "Workspace" $.Workspace "Status" $status}}{{end}}</div><details class="source-details" id="{{.ID}}-source"><summary>Source</summary><pre><code>{{.SourceHTML}}</code></pre></details></section>
              {{else if eq .Kind "repos"}}
                {{.HTML}}
              {{else if eq .Kind "markdown"}}
                <article class="rendered-markdown">{{.HTML}}</article>
              {{else if eq .Kind "html"}}
                <section class="html-preview"><div class="preview-toolbar"><span></span><span></span><span></span><strong>{{.Name}}</strong></div><iframe class="html-render" data-html-preview-frame sandbox="allow-same-origin" srcdoc="{{.SourceHTML}}"></iframe><details class="source-details"><summary>Source</summary><pre><code>{{.SourceHTML}}</code></pre></details></section>
              {{else}}
                <pre class="code-source language-{{.Language}}"><code>{{.HTML}}</code></pre>
              {{end}}
            </div>
          </section>{{end}}
        </div>
      </div>
    </section>
  </main>
</div>
` + serveLiveScript + `
<script>
function activateFileView(path){
  var tab=document.querySelector('[data-file-tab="'+CSS.escape(path)+'"]');
  var panel=document.querySelector('[data-file-panel="'+CSS.escape(path)+'"]');
  if(!tab||!panel){return;}
  tab.hidden=false;
  tab.classList.add('is-open');
  document.querySelectorAll('[data-file-tab]').forEach(function(item){item.classList.toggle('active',item===tab);});
  document.querySelectorAll('[data-file-panel]').forEach(function(item){item.hidden=item!==panel;item.classList.toggle('active',item===panel);});
  document.querySelectorAll('[data-open-view]').forEach(function(item){item.classList.toggle('active',item.getAttribute('data-open-view')===path);});
  resizeHTMLPreviews(panel);
}
function resizeHTMLPreview(frame){
  try{
    var doc=frame.contentDocument||(frame.contentWindow&&frame.contentWindow.document);
    if(!doc||!doc.documentElement||!doc.body){return;}
    frame.style.height='0px';
    var height=Math.max(doc.body.scrollHeight,doc.body.offsetHeight,doc.documentElement.scrollHeight,doc.documentElement.offsetHeight);
    frame.style.height=String(Math.max(1,Math.ceil(height)))+'px';
  }catch(e){}
}
function resizeHTMLPreviews(root){
  (root||document).querySelectorAll('[data-html-preview-frame]').forEach(function(frame){
    requestAnimationFrame(function(){resizeHTMLPreview(frame);});
  });
}
document.querySelectorAll('[data-html-preview-frame]').forEach(function(frame){
  frame.addEventListener('load',function(){resizeHTMLPreview(frame);});
});
window.addEventListener('resize',function(){resizeHTMLPreviews(document);});
document.querySelectorAll('[data-open-view]').forEach(function(item){item.addEventListener('click',function(){activateFileView(item.getAttribute('data-open-view'));});});
document.querySelectorAll('[data-toggle-dir]').forEach(function(item){item.addEventListener('click',function(){var children=item.parentElement&&item.parentElement.querySelector(':scope > .tree-children');if(!children){return;}var collapsed=children.classList.toggle('hidden');item.classList.toggle('collapsed',collapsed);item.setAttribute('aria-expanded',String(!collapsed));});});
document.querySelectorAll('[data-file-tab]').forEach(function(item){item.addEventListener('click',function(event){if(event.target&&event.target.hasAttribute('data-close-view')){return;}activateFileView(item.getAttribute('data-file-tab'));});});
document.querySelectorAll('[data-close-view]').forEach(function(item){item.addEventListener('click',function(event){event.stopPropagation();var path=item.getAttribute('data-close-view');if(path==='workspace.md'){return;}var current=document.querySelector('[data-file-panel="'+CSS.escape(path)+'"]');var wasActive=current&&current.classList.contains('active');var tab=document.querySelector('[data-file-tab="'+CSS.escape(path)+'"]');if(tab){tab.hidden=true;tab.classList.remove('is-open','active');}if(current){current.hidden=true;current.classList.remove('active');}if(wasActive||!document.querySelector('[data-file-panel].active')){activateFileView('workspace.md');}});});
activateFileView('workspace.md');
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
{{end}}
{{define "fileTreeNode"}}
  {{if eq .Type "dir"}}
    <div class="tree-dir"><button type="button" class="tree-row folder" data-toggle-dir="{{.Path}}" aria-expanded="true"><span class="folder-chevron" aria-hidden="true"></span><span class="folder-icon" aria-hidden="true"></span><span>{{.Name}}</span></button><div class="tree-children">{{range .Children}}{{template "fileTreeNode" .}}{{end}}</div></div>
  {{else if eq .Type "special"}}
    <button type="button" class="tree-row special" data-open-view="{{.Path}}"><span class="special-folder-icon" aria-hidden="true"></span><span>{{.Name}}/</span><span class="open-marker">view</span></button>
  {{else}}
    <button type="button" class="tree-row file {{if eq .Path "workspace.md"}}active{{end}}" data-open-view="{{.Path}}"><span class="file-dot" aria-hidden="true"></span><span>{{.Name}}</span></button>
  {{end}}
{{end}}`
