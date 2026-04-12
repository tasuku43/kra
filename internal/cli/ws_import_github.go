package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/app/wsimport"
	"github.com/tasuku43/kra/internal/config"
	"github.com/tasuku43/kra/internal/infra/appports"
	"github.com/tasuku43/kra/internal/infra/gitutil"
	"github.com/tasuku43/kra/internal/infra/paths"
)

const (
	wsImportGitHubDefaultLimit = 30
	wsImportGitHubMinLimit     = 1
	wsImportGitHubMaxLimit     = 200
)

var newWSImportGitHubPort = func() wsimport.GitHubImportPort {
	return appports.NewWSImportGitHubPort()
}

type wsImportGitHubOpts struct {
	scopeKind    string
	scopeValue   string
	state        string
	limit        int
	apply        bool
	noPrompt     bool
	outputFormat string
}

type wsImportGitHubPlan struct {
	Source  wsImportGitHubSource  `json:"source"`
	Filters wsImportGitHubFilters `json:"filters"`
	Summary wsImportGitHubSummary `json:"summary"`
	Items   []wsImportGitHubItem  `json:"items"`
}

type wsImportGitHubSource struct {
	Type string `json:"type"`
	Mode string `json:"mode"`
}

type wsImportGitHubFilters struct {
	Scope    wsImportGitHubScope `json:"scope"`
	State    string              `json:"state,omitempty"`
	Reviewer string              `json:"reviewer,omitempty"`
	Limit    int                 `json:"limit"`
}

type wsImportGitHubScope struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type wsImportGitHubSummary struct {
	Candidates int `json:"candidates"`
	ToCreate   int `json:"to_create"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
}

type wsImportGitHubItem struct {
	Repo        string `json:"repo"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	WorkspaceID string `json:"workspace_id"`
	Action      string `json:"action"`
	Reason      string `json:"reason,omitempty"`
	Message     string `json:"message,omitempty"`
}

func (c *CLI) runWSImportGitHub(args []string) int {
	if len(args) == 0 {
		c.printWSImportGitHubUsage(c.Err)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		c.printWSImportGitHubUsage(c.Out)
		return exitOK
	case "issue":
		return c.runWSImportGitHubIssue(args[1:])
	case "review":
		return c.runWSImportGitHubReview(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"ws", "import", "github"}, args[0]), " "))
		c.printWSImportGitHubUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runWSImportGitHubIssue(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSImportGitHubIssueUsage(c.Out)
			return exitOK
		}
	}
	opts, err := parseWSImportGitHubOpts(args, true)
	if err != nil {
		return c.writeWSImportGitHubUsageError("ws.import.github.issue", err.Error(), c.printWSImportGitHubIssueUsage, wsImportGitHubWantsJSON(args))
	}
	return c.runWSImportGitHubIssueWithOpts(opts)
}

func (c *CLI) runWSImportGitHubReview(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSImportGitHubReviewUsage(c.Out)
			return exitOK
		}
	}
	opts, err := parseWSImportGitHubOpts(args, false)
	if err != nil {
		return c.writeWSImportGitHubUsageError("ws.import.github.review", err.Error(), c.printWSImportGitHubReviewUsage, wsImportGitHubWantsJSON(args))
	}
	return c.runWSImportGitHubReviewWithOpts(opts)
}

func (c *CLI) runWSImportGitHubIssueWithOpts(opts wsImportGitHubOpts) int {
	const action = "ws.import.github.issue"
	outputJSON := opts.outputFormat == "json"

	root, cfg, runtimeErrCode, runtimeErrMessage, usageExit := c.prepareWSImportGitHubRuntime("ws-import-github-issue")
	if runtimeErrMessage != "" {
		return c.writeWSImportGitHubRuntimeError(action, outputJSON, runtimeErrCode, runtimeErrMessage, usageExit)
	}
	resolved, err := applyWSImportGitHubIssueConfigDefaults(opts, cfg)
	if err != nil {
		return c.writeWSImportGitHubUsageError(action, err.Error(), c.printWSImportGitHubIssueUsage, outputJSON)
	}

	plan, createInputs, err := c.prepareWSImportGitHubIssuePlan(context.Background(), root, cfg, resolved)
	if err != nil {
		return c.writeWSImportGitHubRuntimeError(action, outputJSON, "internal_error", err.Error(), exitError)
	}
	if outputJSON {
		return c.writeWSImportGitHubJSONResult(action, plan, false, plan.Summary.Failed == 0, "conflict", "interactive issue selection is required")
	}
	if resolved.noPrompt {
		return c.writeWSImportGitHubUsageError(action, "--no-prompt is not supported for github issue import", c.printWSImportGitHubIssueUsage, false)
	}
	if len(createInputs) == 0 {
		c.printWSImportGitHubPlanHuman(plan)
		if plan.Summary.Failed > 0 {
			return exitError
		}
		return exitOK
	}
	selected, err := c.selectWSImportGitHubIssueInputs(createInputs)
	if err != nil {
		return c.writeWSImportGitHubRuntimeError(action, false, "internal_error", fmt.Sprintf("select github issues: %v", err), exitError)
	}
	if len(selected) == 0 {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}
	confirm, err := c.promptLine(renderWSImportGitHubIssueApplyPrompt(len(selected), writerSupportsColor(c.Err)))
	if err != nil {
		return c.writeWSImportGitHubRuntimeError(action, false, "internal_error", fmt.Sprintf("read confirmation: %v", err), exitError)
	}
	confirm = strings.ToLower(strings.TrimSpace(confirm))
	if confirm != "" && confirm != "y" && confirm != "yes" {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}
	c.applyWSImportGitHubPlan(context.Background(), root, cfg, &plan, selected)
	c.printWSImportGitHubResultHuman(plan)
	if plan.Summary.Failed > 0 {
		return exitError
	}
	return exitOK
}

func (c *CLI) runWSImportGitHubReviewWithOpts(opts wsImportGitHubOpts) int {
	const action = "ws.import.github.review"
	outputJSON := opts.outputFormat == "json"

	root, cfg, runtimeErrCode, runtimeErrMessage, usageExit := c.prepareWSImportGitHubRuntime("ws-import-github-review")
	if runtimeErrMessage != "" {
		return c.writeWSImportGitHubRuntimeError(action, outputJSON, runtimeErrCode, runtimeErrMessage, usageExit)
	}
	resolved, err := applyWSImportGitHubReviewConfigDefaults(opts, cfg)
	if err != nil {
		return c.writeWSImportGitHubUsageError(action, err.Error(), c.printWSImportGitHubReviewUsage, outputJSON)
	}

	plan, createInputs, err := c.prepareWSImportGitHubReviewPlan(context.Background(), root, cfg, resolved)
	if err != nil {
		return c.writeWSImportGitHubRuntimeError(action, outputJSON, "internal_error", err.Error(), exitError)
	}
	if len(plan.Items) == 0 {
		if outputJSON {
			return c.writeWSImportGitHubJSONResult(action, plan, false, true, "", "")
		}
		c.printWSImportGitHubPlanHuman(plan)
		return exitOK
	}

	shouldApply := false
	interactivePromptFlow := !resolved.noPrompt && !resolved.apply
	if resolved.noPrompt {
		shouldApply = resolved.apply
	} else if resolved.apply {
		shouldApply = true
	} else {
		if outputJSON {
			confirm, err := c.promptLine(renderWSImportJiraApplyPrompt(writerSupportsColor(c.Err)))
			if err != nil {
				return c.writeWSImportGitHubRuntimeError(action, true, "internal_error", fmt.Sprintf("read apply confirmation: %v", err), exitError)
			}
			confirm = strings.ToLower(strings.TrimSpace(confirm))
			shouldApply = confirm == "" || confirm == "y" || confirm == "yes"
		} else {
			c.printWSImportGitHubPlanHuman(plan)
			confirm, err := c.promptWSImportJiraApplyOnOut()
			if err != nil {
				return c.writeWSImportGitHubRuntimeError(action, false, "internal_error", fmt.Sprintf("read apply confirmation: %v", err), exitError)
			}
			confirm = strings.ToLower(strings.TrimSpace(confirm))
			shouldApply = confirm == "" || confirm == "y" || confirm == "yes"
		}
		if !shouldApply {
			if outputJSON {
				return c.writeWSImportGitHubJSONResult(action, plan, false, plan.Summary.Failed == 0, "conflict", "one or more import items failed in plan")
			}
			if plan.Summary.Failed > 0 {
				return exitError
			}
			return exitOK
		}
	}
	if shouldApply {
		c.applyWSImportGitHubPlan(context.Background(), root, cfg, &plan, createInputs)
	}
	if outputJSON {
		return c.writeWSImportGitHubJSONResult(action, plan, shouldApply, plan.Summary.Failed == 0, "conflict", "import completed with failures")
	}
	if shouldApply {
		c.printWSImportGitHubResultHuman(plan)
		if plan.Summary.Failed > 0 {
			return exitError
		}
		return exitOK
	}
	if !interactivePromptFlow {
		c.printWSImportGitHubPlanHuman(plan)
	}
	if plan.Summary.Failed > 0 {
		return exitError
	}
	return exitOK
}

func (c *CLI) prepareWSImportGitHubIssuePlan(ctx context.Context, root string, _ config.Config, opts wsImportGitHubOpts) (wsImportGitHubPlan, []wsimport.GitHubWorkspaceInput, error) {
	svc := wsimport.NewService(nil, newWSImportGitHubPort())
	scope := wsimport.GitHubScope{Kind: opts.scopeKind, Value: opts.scopeValue}
	inputs, err := svc.ResolveGitHubIssueWorkspaceInputs(ctx, scope, opts.state, opts.limit)
	if err != nil {
		return wsImportGitHubPlan{}, nil, fmt.Errorf("resolve github issues: %w", err)
	}
	plan, createInputs := buildWSImportGitHubPlan("issue", scope, opts.state, opts.limit, root, inputs)
	return plan, createInputs, nil
}

func (c *CLI) prepareWSImportGitHubReviewPlan(ctx context.Context, root string, _ config.Config, opts wsImportGitHubOpts) (wsImportGitHubPlan, []wsimport.GitHubWorkspaceInput, error) {
	svc := wsimport.NewService(nil, newWSImportGitHubPort())
	scope := wsimport.GitHubScope{Kind: opts.scopeKind, Value: opts.scopeValue}
	inputs, err := svc.ResolveGitHubReviewWorkspaceInputs(ctx, scope, opts.limit)
	if err != nil {
		return wsImportGitHubPlan{}, nil, fmt.Errorf("resolve github review requests: %w", err)
	}
	plan, createInputs := buildWSImportGitHubPlan("review", scope, config.GitHubStateOpen, opts.limit, root, inputs)
	return plan, createInputs, nil
}

func (c *CLI) applyWSImportGitHubPlan(ctx context.Context, root string, cfg config.Config, plan *wsImportGitHubPlan, createInputs []wsimport.GitHubWorkspaceInput) {
	createdCount := 0
	for _, in := range createInputs {
		if _, err := c.createWorkspaceAtRoot(root, in.ID, in.Title, in.SourceURL, defaultWorkspaceTemplateName); err != nil {
			markWSImportGitHubCreateItemAsFailed(plan, in, classifyWSImportGitHubCreateFailureReason(err), err.Error())
			plan.Summary.Failed++
			continue
		}
		if err := c.autoAttachGitHubRepoToWorkspace(ctx, root, cfg, in.ID, in.Repository, in.Branch); err != nil {
			markWSImportGitHubCreateItemAsFailed(plan, in, "create_failed", err.Error())
			plan.Summary.Failed++
			continue
		}
		createdCount++
	}
	plan.Summary.ToCreate = createdCount
}

func parseWSImportGitHubOpts(args []string, supportsState bool) (wsImportGitHubOpts, error) {
	opts := wsImportGitHubOpts{limit: wsImportGitHubDefaultLimit, outputFormat: "human", state: config.GitHubStateOpen}
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		switch rest[0] {
		case "-h", "--help", "help":
			return wsImportGitHubOpts{}, fmt.Errorf("help requested")
		case "--org":
			if len(rest) < 2 {
				return wsImportGitHubOpts{}, fmt.Errorf("--org requires a value")
			}
			if opts.scopeKind != "" {
				return wsImportGitHubOpts{}, fmt.Errorf("--org and --repo cannot be combined")
			}
			opts.scopeKind, opts.scopeValue = "org", strings.TrimSpace(rest[1])
			rest = rest[2:]
		case "--repo":
			if len(rest) < 2 {
				return wsImportGitHubOpts{}, fmt.Errorf("--repo requires a value")
			}
			if opts.scopeKind != "" {
				return wsImportGitHubOpts{}, fmt.Errorf("--org and --repo cannot be combined")
			}
			opts.scopeKind, opts.scopeValue = "repo", strings.TrimSpace(rest[1])
			rest = rest[2:]
		case "--state":
			if !supportsState {
				return wsImportGitHubOpts{}, fmt.Errorf("unknown flag for ws import github review: %q", rest[0])
			}
			if len(rest) < 2 {
				return wsImportGitHubOpts{}, fmt.Errorf("--state requires a value")
			}
			opts.state = strings.ToLower(strings.TrimSpace(rest[1]))
			rest = rest[2:]
		case "--limit":
			if len(rest) < 2 {
				return wsImportGitHubOpts{}, fmt.Errorf("--limit requires a value")
			}
			n, err := strconv.Atoi(strings.TrimSpace(rest[1]))
			if err != nil {
				return wsImportGitHubOpts{}, fmt.Errorf("invalid --limit: %q", rest[1])
			}
			opts.limit = n
			rest = rest[2:]
		case "--apply":
			opts.apply = true
			rest = rest[1:]
		case "--no-prompt":
			opts.noPrompt = true
			rest = rest[1:]
		case "--json":
			opts.outputFormat = "json"
			rest = rest[1:]
		case "--format":
			if len(rest) < 2 {
				return wsImportGitHubOpts{}, fmt.Errorf("--format requires a value")
			}
			opts.outputFormat = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			if strings.HasPrefix(rest[0], "--format=") {
				opts.outputFormat = strings.TrimSpace(strings.TrimPrefix(rest[0], "--format="))
				rest = rest[1:]
				continue
			}
			if strings.HasPrefix(rest[0], "--state=") && supportsState {
				opts.state = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(rest[0], "--state=")))
				rest = rest[1:]
				continue
			}
			return wsImportGitHubOpts{}, fmt.Errorf("unknown flag: %q", rest[0])
		}
	}
	if len(rest) > 0 {
		return wsImportGitHubOpts{}, fmt.Errorf("unexpected args: %q", strings.Join(rest, " "))
	}
	if opts.limit < wsImportGitHubMinLimit || opts.limit > wsImportGitHubMaxLimit {
		return wsImportGitHubOpts{}, fmt.Errorf("--limit must be in range %d..%d", wsImportGitHubMinLimit, wsImportGitHubMaxLimit)
	}
	if supportsState {
		switch opts.state {
		case config.GitHubStateOpen, config.GitHubStateClosed, config.GitHubStateAll:
		default:
			return wsImportGitHubOpts{}, fmt.Errorf("--state must be one of: open, closed, all")
		}
	}
	switch opts.outputFormat {
	case "human", "json":
	default:
		return wsImportGitHubOpts{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.outputFormat)
	}
	return opts, nil
}

func wsImportGitHubWantsJSON(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--json":
			return true
		case strings.HasPrefix(arg, "--format="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "--format=")) == "json"
		case arg == "--format" && i+1 < len(args):
			return strings.TrimSpace(args[i+1]) == "json"
		}
	}
	return false
}

func applyWSImportGitHubIssueConfigDefaults(opts wsImportGitHubOpts, cfg config.Config) (wsImportGitHubOpts, error) {
	out := opts
	if out.scopeKind == "" {
		switch {
		case cfg.Integration.GitHub.Defaults.Issue.Org != "" && cfg.Integration.GitHub.Defaults.Issue.Repo != "":
			return wsImportGitHubOpts{}, fmt.Errorf("integration.github.defaults.issue.org and integration.github.defaults.issue.repo cannot be combined")
		case cfg.Integration.GitHub.Defaults.Issue.Org != "":
			out.scopeKind, out.scopeValue = "org", cfg.Integration.GitHub.Defaults.Issue.Org
		case cfg.Integration.GitHub.Defaults.Issue.Repo != "":
			out.scopeKind, out.scopeValue = "repo", cfg.Integration.GitHub.Defaults.Issue.Repo
		}
	}
	if out.scopeKind == "" || strings.TrimSpace(out.scopeValue) == "" {
		return wsImportGitHubOpts{}, fmt.Errorf("github issue import requires --org or --repo")
	}
	if strings.TrimSpace(out.state) == "" {
		out.state = firstNonEmpty(cfg.Integration.GitHub.Defaults.Issue.State, config.GitHubStateOpen)
	}
	return out, nil
}

func applyWSImportGitHubReviewConfigDefaults(opts wsImportGitHubOpts, cfg config.Config) (wsImportGitHubOpts, error) {
	out := opts
	if out.scopeKind == "" {
		switch {
		case cfg.Integration.GitHub.Defaults.Review.Org != "" && cfg.Integration.GitHub.Defaults.Review.Repo != "":
			return wsImportGitHubOpts{}, fmt.Errorf("integration.github.defaults.review.org and integration.github.defaults.review.repo cannot be combined")
		case cfg.Integration.GitHub.Defaults.Review.Org != "":
			out.scopeKind, out.scopeValue = "org", cfg.Integration.GitHub.Defaults.Review.Org
		case cfg.Integration.GitHub.Defaults.Review.Repo != "":
			out.scopeKind, out.scopeValue = "repo", cfg.Integration.GitHub.Defaults.Review.Repo
		}
	}
	if out.scopeKind == "" || strings.TrimSpace(out.scopeValue) == "" {
		return wsImportGitHubOpts{}, fmt.Errorf("github review import requires --org or --repo")
	}
	out.state = config.GitHubStateOpen
	return out, nil
}

func buildWSImportGitHubPlan(mode string, scope wsimport.GitHubScope, state string, limit int, root string, inputs []wsimport.GitHubWorkspaceInput) (wsImportGitHubPlan, []wsimport.GitHubWorkspaceInput) {
	plan := wsImportGitHubPlan{
		Source: wsImportGitHubSource{Type: "github", Mode: mode},
		Filters: wsImportGitHubFilters{
			Scope: wsImportGitHubScope{Kind: scope.Kind, Value: scope.Value},
			State: state,
			Limit: limit,
		},
		Summary: wsImportGitHubSummary{Candidates: len(inputs)},
		Items:   make([]wsImportGitHubItem, 0, len(inputs)),
	}
	if mode == "review" {
		plan.Filters.Reviewer = "currentUser()"
	}
	createInputs := make([]wsimport.GitHubWorkspaceInput, 0, len(inputs))
	for _, in := range inputs {
		item := wsImportGitHubItem{
			Repo:        in.Repository,
			Number:      in.Number,
			Title:       in.Title,
			WorkspaceID: in.ID,
			Action:      "create",
		}
		if err := validateWorkspaceID(in.ID); err != nil {
			item.Action = "fail"
			item.Reason = "invalid_workspace_id"
			item.Message = err.Error()
			plan.Summary.Failed++
			plan.Items = append(plan.Items, item)
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "workspaces", in.ID)); err == nil {
			item.Action = "skip"
			item.Reason = "already_active"
			plan.Summary.Skipped++
			plan.Items = append(plan.Items, item)
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "archive", in.ID)); err == nil {
			item.Action = "skip"
			item.Reason = "archived_exists"
			plan.Summary.Skipped++
			plan.Items = append(plan.Items, item)
			continue
		}
		plan.Summary.ToCreate++
		plan.Items = append(plan.Items, item)
		createInputs = append(createInputs, in)
	}
	slices.SortFunc(plan.Items, func(a, b wsImportGitHubItem) int {
		if a.Repo != b.Repo {
			return strings.Compare(a.Repo, b.Repo)
		}
		return a.Number - b.Number
	})
	return plan, createInputs
}

func (c *CLI) selectWSImportGitHubIssueInputs(inputs []wsimport.GitHubWorkspaceInput) ([]wsimport.GitHubWorkspaceInput, error) {
	candidates := make([]workspaceSelectorCandidate, 0, len(inputs))
	byID := make(map[string]wsimport.GitHubWorkspaceInput, len(inputs))
	for _, in := range inputs {
		id := in.ID
		label := fmt.Sprintf("%s#%d", in.Repository, in.Number)
		candidates = append(candidates, workspaceSelectorCandidate{
			ID:          id,
			Title:       label,
			Description: formatWorkspaceTitle(in.Title),
		})
		byID[id] = in
	}
	selectedIDs, err := c.promptWorkspaceSelectorWithOptions("active", "create", "Issues:", "issue", candidates)
	if err != nil {
		return nil, err
	}
	selected := make([]wsimport.GitHubWorkspaceInput, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		if in, ok := byID[id]; ok {
			selected = append(selected, in)
		}
	}
	return selected, nil
}

func renderWSImportGitHubIssueApplyPrompt(count int, useColor bool) string {
	bullet := styleMuted("•", useColor)
	guide := styleMuted("[Enter=yes / n=no]", useColor)
	return fmt.Sprintf("%s%s create %d workspaces? %s: ", uiIndent, bullet, count, guide)
}

func (c *CLI) autoAttachGitHubRepoToWorkspace(ctx context.Context, root string, cfg config.Config, workspaceID string, repoKey string, branch string) error {
	if err := gitutil.EnsureGitInPath(); err != nil {
		return err
	}
	repoPoolPath, err := paths.DefaultRepoPoolPath()
	if err != nil {
		return fmt.Errorf("resolve repo pool path: %w", err)
	}
	remoteURL := fmt.Sprintf("git@github.com:%s.git", strings.TrimSpace(repoKey))
	req := repoPoolAddRequest{RepoSpecInput: remoteURL}
	outcomes := applyRepoPoolAdds(ctx, repoPoolPath, []repoPoolAddRequest{req}, 1, c.debugf, nil)
	if len(outcomes) != 1 || !outcomes[0].Success {
		reason := "unknown repo pool error"
		if len(outcomes) == 1 && strings.TrimSpace(outcomes[0].Reason) != "" {
			reason = outcomes[0].Reason
		}
		return fmt.Errorf("add repo to pool: %s", reason)
	}
	if err := syncRootRepoRegistryFromPoolAddRequests(root, []repoPoolAddRequest{req}, outcomes); err != nil {
		return fmt.Errorf("update root repo registry: %w", err)
	}
	releaseLock, err := acquireWorkspaceAddRepoLock(root, workspaceID)
	if err != nil {
		return err
	}
	defer releaseLock()
	candidates, err := listAddRepoPoolCandidates(ctx, root, repoPoolPath, workspaceID, time.Now(), c.debugf)
	if err != nil {
		return fmt.Errorf("list repo pool candidates: %w", err)
	}
	var cand addRepoPoolCandidate
	found := false
	for _, it := range candidates {
		if it.RepoKey == repoKey {
			cand = it
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("repo not available in pool for workspace: %s", repoKey)
	}
	defaultBaseRef, err := detectDefaultBaseRefFromBare(ctx, cand.BarePath)
	if err != nil {
		return fmt.Errorf("detect default base_ref for %s: %w", cand.RepoKey, err)
	}
	baseRefUsed, err := resolveBaseRefInput("", defaultBaseRef)
	if err != nil {
		return fmt.Errorf("resolve base_ref for %s: %w", cand.RepoKey, err)
	}
	if strings.TrimSpace(branch) == "" {
		branch, err = renderAddRepoDefaultBranch(cfg.Workspace.Branch.Template, workspaceID, cand.RepoKey)
		if err != nil {
			return fmt.Errorf("invalid workspace.branch.template: %w", err)
		}
	}
	if err := gitutil.CheckRefFormat(ctx, "refs/heads/"+branch); err != nil {
		return fmt.Errorf("invalid branch name for %s: %w", cand.RepoKey, err)
	}
	plan := []addRepoPlanItem{{
		Candidate:      cand,
		DefaultBaseRef: defaultBaseRef,
		BaseRefUsed:    baseRefUsed,
		Branch:         branch,
	}}
	if err := ensureAddRepoPlanFetchPhaseB(ctx, plan, addRepoFetchOptions{}, nil, nil); err != nil {
		return fmt.Errorf("fetch selected repos: %w", err)
	}
	if err := preflightAddRepoPlan(ctx, root, workspaceID, plan); err != nil {
		return fmt.Errorf("preflight add-repo: %w", err)
	}
	applied, err := applyAddRepoPlanAllOrNothing(ctx, plan, c.debugf)
	if err != nil {
		return fmt.Errorf("apply add-repo: %w", err)
	}
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if err := upsertWorkspaceMetaReposRestore(wsPath, buildWorkspaceMetaReposRestore(applied), time.Now().Unix()); err != nil {
		rollbackAddRepoApplied(ctx, applied, c.debugf)
		return fmt.Errorf("update %s: %w", workspaceMetaFilename, err)
	}
	return nil
}

func (c *CLI) prepareWSImportGitHubRuntime(debugTag string) (string, config.Config, string, string, int) {
	wd, err := os.Getwd()
	if err != nil {
		return "", config.Config{}, "internal_error", fmt.Sprintf("get working dir: %v", err), exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return "", config.Config{}, "not_found", fmt.Sprintf("resolve KRA_ROOT: %v", err), exitError
	}
	if err := c.ensureDebugLog(root, debugTag); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	cfg, err := c.loadMergedConfig(root)
	if err != nil {
		return "", config.Config{}, "internal_error", fmt.Sprintf("load config: %v", err), exitError
	}
	return root, cfg, "", "", exitOK
}

func (c *CLI) writeWSImportGitHubUsageError(action string, message string, printUsage func(w io.Writer), outputJSON bool) int {
	if outputJSON {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: action,
			Error:  &cliJSONError{Code: "invalid_argument", Message: message},
		})
		return exitUsage
	}
	fmt.Fprintln(c.Err, message)
	switch action {
	case "ws.import.github.issue":
		c.printWSImportGitHubIssueUsage(c.Err)
	case "ws.import.github.review":
		c.printWSImportGitHubReviewUsage(c.Err)
	default:
		c.printWSImportGitHubUsage(c.Err)
	}
	return exitUsage
}

func (c *CLI) writeWSImportGitHubRuntimeError(action string, outputJSON bool, code string, message string, exitCode int) int {
	if outputJSON {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: action,
			Error:  &cliJSONError{Code: code, Message: message},
		})
		return exitCode
	}
	fmt.Fprintln(c.Err, message)
	return exitCode
}

func (c *CLI) writeWSImportGitHubJSONResult(action string, plan wsImportGitHubPlan, applied bool, ok bool, code string, message string) int {
	resp := cliJSONResponse{
		OK:     ok,
		Action: action,
		Result: map[string]any{
			"source":  plan.Source,
			"filters": plan.Filters,
			"summary": plan.Summary,
			"items":   plan.Items,
			"applied": applied,
		},
	}
	if !ok {
		resp.Error = &cliJSONError{Code: code, Message: message}
	}
	_ = writeCLIJSON(c.Out, resp)
	if ok {
		return exitOK
	}
	return exitError
}

func (c *CLI) printWSImportGitHubPlanHuman(plan wsImportGitHubPlan) {
	useColor := writerSupportsColor(c.Out)
	bullet := styleMuted("•", useColor)
	connectorMuted := func(connector string) string { return styleMuted(connector, useColor) }
	styleLabel := func(label string) string { return styleAccent(label, useColor) }
	sourceLine := fmt.Sprintf("%s github mode=%s scope=%s:%s", styleLabel("source:"), plan.Source.Mode, plan.Filters.Scope.Kind, plan.Filters.Scope.Value)
	filterLine := fmt.Sprintf("%s%s %s limit=%d", uiIndent, bullet, styleLabel("filters:"), plan.Filters.Limit)
	if plan.Filters.State != "" {
		filterLine = fmt.Sprintf("%s state=%s", filterLine, plan.Filters.State)
	}
	if plan.Filters.Reviewer != "" {
		filterLine = fmt.Sprintf("%s reviewer=%s", filterLine, plan.Filters.Reviewer)
	}
	body := []string{
		fmt.Sprintf("%s%s %s", uiIndent, bullet, sourceLine),
		filterLine,
		fmt.Sprintf("%s%s %s (%d)", uiIndent, bullet, styleInfo("to create", useColor), plan.Summary.ToCreate),
	}
	body = append(body, renderWSImportGitHubPlanItems(plan.Items, "create", connectorMuted)...)
	if plan.Summary.Skipped > 0 {
		body = append(body, fmt.Sprintf("%s%s %s (%d)", uiIndent, bullet, styleWarn("skipped", useColor), plan.Summary.Skipped))
	}
	if plan.Summary.Failed > 0 {
		body = append(body, fmt.Sprintf("%s%s %s (%d)", uiIndent, bullet, styleError("failed", useColor), plan.Summary.Failed))
		body = append(body, renderWSImportGitHubPlanItems(plan.Items, "fail", connectorMuted)...)
	}
	for _, line := range renderSectionAtoms(newSectionAtom(styleBold("Plan:", useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})) {
		fmt.Fprintln(c.Out, line)
	}
}

func (c *CLI) printWSImportGitHubResultHuman(plan wsImportGitHubPlan) {
	useColor := writerSupportsColor(c.Out)
	body := []string{
		fmt.Sprintf("%s%s %s %s %s", uiIndent, styleMuted("•", useColor), styleInfo(fmt.Sprintf("create=%d", plan.Summary.ToCreate), useColor), styleWarn(fmt.Sprintf("skipped=%d", plan.Summary.Skipped), useColor), styleError(fmt.Sprintf("failed=%d", plan.Summary.Failed), useColor)),
	}
	resultLine := styleSuccess("import completed", useColor)
	if plan.Summary.Failed > 0 {
		resultLine = styleWarn("import completed with failures", useColor)
	}
	body = append(body, fmt.Sprintf("%s%s %s", uiIndent, styleMuted("•", useColor), resultLine))
	printSection(c.Out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func renderWSImportGitHubPlanItems(items []wsImportGitHubItem, action string, connectorMuted func(string) string) []string {
	group := make([]wsImportGitHubItem, 0, len(items))
	for _, it := range items {
		if it.Action == action {
			group = append(group, it)
		}
	}
	if len(group) == 0 {
		return nil
	}
	lines := make([]string, 0, len(group))
	for i, it := range group {
		connector := "├─ "
		if i == len(group)-1 {
			connector = "└─ "
		}
		lines = append(lines, fmt.Sprintf("%s%s%s", uiIndent+uiIndent, connectorMuted(connector), renderWSImportGitHubPlanItemLabel(it)))
	}
	return lines
}

func renderWSImportGitHubPlanItemLabel(it wsImportGitHubItem) string {
	base := fmt.Sprintf("%s#%d: %s", strings.TrimSpace(it.Repo), it.Number, formatWorkspaceTitle(it.Title))
	switch it.Action {
	case "skip":
		if strings.TrimSpace(it.Reason) == "" || strings.EqualFold(strings.TrimSpace(it.Reason), "already_active") {
			return base
		}
		return fmt.Sprintf("%s (%s)", base, strings.TrimSpace(it.Reason))
	case "fail":
		if strings.TrimSpace(it.Message) == "" {
			return fmt.Sprintf("%s (%s)", base, strings.TrimSpace(it.Reason))
		}
		return fmt.Sprintf("%s (%s: %s)", base, strings.TrimSpace(it.Reason), strings.TrimSpace(it.Message))
	default:
		return base
	}
}

func classifyWSImportGitHubCreateFailureReason(err error) string {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "permission denied"):
		return "permission_denied"
	case strings.Contains(msg, "not found"), strings.Contains(msg, "no such file or directory"):
		return "not_found"
	default:
		return "create_failed"
	}
}

func markWSImportGitHubCreateItemAsFailed(plan *wsImportGitHubPlan, in wsimport.GitHubWorkspaceInput, reason string, message string) {
	for i := range plan.Items {
		if plan.Items[i].WorkspaceID != in.ID {
			continue
		}
		if plan.Items[i].Action != "create" {
			continue
		}
		plan.Items[i].Action = "fail"
		plan.Items[i].Reason = reason
		plan.Items[i].Message = message
		return
	}
	plan.Items = append(plan.Items, wsImportGitHubItem{
		Repo:        in.Repository,
		Number:      in.Number,
		Title:       in.Title,
		WorkspaceID: in.ID,
		Action:      "fail",
		Reason:      reason,
		Message:     message,
	})
}
