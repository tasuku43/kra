package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/core/repostore"
	"github.com/tasuku43/kra/internal/infra/gitutil"
	"github.com/tasuku43/kra/internal/infra/paths"
)

type addRepoPoolCandidate struct {
	RepoUID   string
	RepoKey   string
	RemoteURL string
	Alias     string
	BarePath  string
}

type addRepoPlanItem struct {
	Candidate addRepoPoolCandidate

	BaseRefInput   string
	DefaultBaseRef string
	BaseRefUsed    string
	Branch         string
	WorktreePath   string

	LocalBranchExists  bool
	RemoteBranchExists bool

	FetchDecision string
}

type addRepoInputProgress struct {
	RepoKey string
	BaseRef string
	Branch  string
}

type addRepoAppliedItem struct {
	Plan addRepoPlanItem

	CreatedLocalBranch bool
	CreatedWorktree    bool
	AddedBinding       bool
}

const (
	addRepoFetchTTL = 5 * time.Minute
)

type addRepoFetchOptions struct {
	Refresh bool
	NoFetch bool
}

type addRepoFetchDecision struct {
	ShouldFetch bool
	Reason      string
}

type addRepoFetchProgressStatus string

const (
	addRepoFetchProgressQueued  addRepoFetchProgressStatus = "queued"
	addRepoFetchProgressRunning addRepoFetchProgressStatus = "running"
	addRepoFetchProgressSkipped addRepoFetchProgressStatus = "skipped"
	addRepoFetchProgressDone    addRepoFetchProgressStatus = "done"
	addRepoFetchProgressFailed  addRepoFetchProgressStatus = "failed"
)

type addRepoFetchProgressRow struct {
	RepoKey string
	Status  addRepoFetchProgressStatus
	Reason  string
}

type addRepoFetchProgressType string

const (
	addRepoFetchProgressStart addRepoFetchProgressType = "start"
	addRepoFetchProgressDoneT addRepoFetchProgressType = "done"
)

type addRepoFetchProgressEvent struct {
	RepoKey string
	Type    addRepoFetchProgressType
	Success bool
	Reason  string
}

func (c *CLI) runWSAddRepo(args []string) int {
	idFromFlag := ""
	outputFormat := "human"
	forceApply := false
	refreshFetch := false
	noFetch := false
	repoKeysFromFlag := make([]string, 0, 4)
	branchFromFlag := ""
	baseRefFromFlag := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSAddRepoUsage(c.Out)
			return exitOK
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printWSAddRepoUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--repo":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--repo requires a value")
				c.printWSAddRepoUsage(c.Err)
				return exitUsage
			}
			repoKeysFromFlag = append(repoKeysFromFlag, strings.TrimSpace(args[1]))
			args = args[2:]
		case "--branch":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--branch requires a value")
				c.printWSAddRepoUsage(c.Err)
				return exitUsage
			}
			branchFromFlag = strings.TrimSpace(args[1])
			args = args[2:]
		case "--base-ref":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--base-ref requires a value")
				c.printWSAddRepoUsage(c.Err)
				return exitUsage
			}
			baseRefFromFlag = strings.TrimSpace(args[1])
			args = args[2:]
		case "--yes":
			forceApply = true
			args = args[1:]
		case "--refresh":
			refreshFetch = true
			args = args[1:]
		case "--no-fetch":
			noFetch = true
			args = args[1:]
		case "--id":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--id requires a value")
				c.printWSAddRepoUsage(c.Err)
				return exitUsage
			}
			idFromFlag = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--id=") {
				idFromFlag = strings.TrimSpace(strings.TrimPrefix(args[0], "--id="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--repo=") {
				repoKeysFromFlag = append(repoKeysFromFlag, strings.TrimSpace(strings.TrimPrefix(args[0], "--repo=")))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--branch=") {
				branchFromFlag = strings.TrimSpace(strings.TrimPrefix(args[0], "--branch="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--base-ref=") {
				baseRefFromFlag = strings.TrimSpace(strings.TrimPrefix(args[0], "--base-ref="))
				args = args[1:]
				continue
			}
			if args[0] == "--refresh" {
				refreshFetch = true
				args = args[1:]
				continue
			}
			if args[0] == "--no-fetch" {
				noFetch = true
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for ws add-repo: %q\n", args[0])
			c.printWSAddRepoUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printWSAddRepoUsage(c.Err)
		return exitUsage
	}
	if refreshFetch && noFetch {
		fmt.Fprintln(c.Err, "--refresh and --no-fetch cannot be used together")
		c.printWSAddRepoUsage(c.Err)
		return exitUsage
	}
	if outputFormat == "human" && (len(repoKeysFromFlag) > 0 || branchFromFlag != "" || baseRefFromFlag != "" || forceApply) {
		fmt.Fprintln(c.Err, "--repo/--branch/--base-ref/--yes are only supported with --format json")
		c.printWSAddRepoUsage(c.Err)
		return exitUsage
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws add-repo: %q\n", strings.Join(args[1:], " "))
		c.printWSAddRepoUsage(c.Err)
		return exitUsage
	}

	if err := gitutil.EnsureGitInPath(); err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve KRA_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "ws-add-repo"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	cfg, err := c.loadMergedConfig(root)
	if err != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "add-repo",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("load config: %v", err),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "load config: %v\n", err)
		return exitError
	}
	branchTemplate := cfg.Workspace.Branch.Template

	ctx := context.Background()
	repoPoolPath, err := paths.DefaultRepoPoolPath()
	if err != nil {
		fmt.Fprintf(c.Err, "resolve repo pool path: %v\n", err)
		return exitError
	}

	workspaceID := ""
	var resolveErr error
	resolveArgs := append([]string{}, args...)
	if idFromFlag != "" {
		if len(resolveArgs) > 0 {
			fmt.Fprintln(c.Err, "--id and positional <workspace-id> cannot be used together")
			c.printWSAddRepoUsage(c.Err)
			return exitUsage
		}
		resolveArgs = []string{idFromFlag}
	}
	workspaceID, resolveErr = resolveWorkspaceIDForAddRepo(root, wd, resolveArgs)
	if resolveErr != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "add-repo",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: resolveErr.Error(),
				},
			})
			return exitUsage
		}
		fmt.Fprintf(c.Err, "%v\n", resolveErr)
		c.printWSAddRepoUsage(c.Err)
		return exitUsage
	}
	if outputFormat == "json" {
		return c.runWSAddRepoJSON(workspaceID, root, repoPoolPath, repoKeysFromFlag, baseRefFromFlag, branchFromFlag, branchTemplate, forceApply, addRepoFetchOptions{
			Refresh: refreshFetch,
			NoFetch: noFetch,
		})
	}
	c.debugf("run ws add-repo workspace=%s cwd=%s", workspaceID, wd)

	existsFS, activeFS, fsErr := workspaceActiveOnFilesystem(root, workspaceID)
	if fsErr != nil {
		fmt.Fprintf(c.Err, "load workspace: %v\n", fsErr)
		return exitError
	}
	if !existsFS {
		fmt.Fprintf(c.Err, "workspace not found: %s\n", workspaceID)
		return exitError
	}
	if !activeFS {
		fmt.Fprintf(c.Err, "workspace is not active (status=archived): %s\n", workspaceID)
		return exitError
	}

	releaseLock, err := acquireWorkspaceAddRepoLock(root, workspaceID)
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	defer releaseLock()

	candidates, err := listAddRepoPoolCandidates(ctx, root, repoPoolPath, workspaceID, time.Now(), c.debugf)
	if err != nil {
		fmt.Fprintf(c.Err, "list repo pool candidates: %v\n", err)
		return exitError
	}
	if len(candidates) == 0 {
		fmt.Fprintln(c.Err, "no repos available in pool for this workspace")
		return exitError
	}

	selected, err := c.promptAddRepoPoolSelection(candidates)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
		fmt.Fprintf(c.Err, "select repos from pool: %v\n", err)
		return exitError
	}
	if len(selected) == 0 {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}

	useColorOut := writerSupportsColor(c.Out)
	useColorErr := writerSupportsColor(c.Err)
	fetchOpts := addRepoFetchOptions{
		Refresh: refreshFetch,
		NoFetch: noFetch,
	}
	progress := make([]addRepoInputProgress, len(selected))
	for i, cand := range selected {
		progress[i] = addRepoInputProgress{RepoKey: cand.RepoKey}
	}
	c.debugf("add-repo inputs render stage=initial active_index=%d prev_lines=%d prompt_closed=%t show_pending_branch=%t keep_base_ref_open=%t", 0, 0, false, true, true)
	renderedInputLines := renderAddRepoInputsProgress(c.Err, workspaceID, progress, 0, useColorErr, 0, true, true)

	plan := make([]addRepoPlanItem, 0, len(selected))
	prefetchRows := make([]addRepoFetchProgressRow, 0, len(selected))
	prefetchEvents := make(chan addRepoFetchProgressEvent, len(selected)*2+1)
	prefetched := make(map[string]bool, len(selected))
	prefetchFailed := make(map[string]string, len(selected))
	var prefetchMu sync.Mutex
	var prefetchWG sync.WaitGroup
	for i, cand := range selected {
		if i > 0 {
			c.debugf("add-repo inputs render stage=next-repo active_index=%d prev_lines=%d prompt_closed=%t show_pending_branch=%t keep_base_ref_open=%t", i, renderedInputLines, false, true, true)
			renderedInputLines = renderAddRepoInputsProgress(c.Err, workspaceID, progress, i, useColorErr, renderedInputLines, true, true)
		}
		defaultBaseRef, err := detectDefaultBaseRefFromBare(ctx, cand.BarePath)
		if err != nil {
			fmt.Fprintf(c.Err, "detect default base_ref for %s: %v\n", cand.RepoKey, err)
			return exitError
		}
		baseRefInput, baseRefEdited, err := c.promptAddRepoEditableInput(addRepoInputDetailPromptPrefix(useColorErr), "base_ref", defaultBaseRef, useColorErr)
		if err != nil {
			fmt.Fprintf(c.Err, "read base_ref: %v\n", err)
			return exitError
		}
		c.debugf("add-repo base_ref input repo=%s raw=%q edited=%t", cand.RepoKey, baseRefInput, baseRefEdited)
		baseRefUsed, err := resolveBaseRefInput(baseRefInput, defaultBaseRef)
		if err != nil {
			fmt.Fprintf(c.Err, "invalid base_ref (must be origin/<branch>): %q\n", baseRefInput)
			return exitError
		}
		baseRefRecord := ""
		if baseRefEdited {
			baseRefRecord = baseRefUsed
		}
		progress[i].BaseRef = baseRefUsed
		c.debugf("add-repo inputs render stage=after-base-ref active_index=%d prev_lines=%d prompt_closed=%t show_pending_branch=%t keep_base_ref_open=%t", i, renderedInputLines, true, false, true)
		renderedInputLines = renderAddRepoInputsProgress(c.Err, workspaceID, progress, i, useColorErr, renderedInputLines, false, true)

		branchDisplayDefault, err := renderAddRepoDefaultBranch(branchTemplate, workspaceID, cand.RepoKey)
		if err != nil {
			fmt.Fprintf(c.Err, "invalid workspace.branch.template: %v\n", err)
			return exitError
		}
		branchInput, _, err := c.promptAddRepoEditableInput(addRepoInputDetailPromptPrefix(useColorErr), "branch", branchDisplayDefault, useColorErr)
		if err != nil {
			fmt.Fprintf(c.Err, "read branch: %v\n", err)
			return exitError
		}
		c.debugf("add-repo branch input repo=%s raw=%q", cand.RepoKey, branchInput)
		branch := resolveBranchInput(branchInput, branchDisplayDefault)
		if err := gitutil.CheckRefFormat(ctx, "refs/heads/"+branch); err != nil {
			fmt.Fprintf(c.Err, "invalid branch name for %s: %v\n", cand.RepoKey, err)
			return exitError
		}
		progress[i].Branch = branch
		c.debugf("add-repo inputs render stage=after-branch active_index=%d prev_lines=%d prompt_closed=%t show_pending_branch=%t keep_base_ref_open=%t", i, renderedInputLines, true, false, false)
		renderedInputLines = renderAddRepoInputsProgress(c.Err, workspaceID, progress, i, useColorErr, renderedInputLines, false, false)

		plan = append(plan, addRepoPlanItem{
			Candidate:      cand,
			BaseRefInput:   baseRefRecord,
			DefaultBaseRef: defaultBaseRef,
			Branch:         branch,
		})
		planItem := plan[len(plan)-1]
		decision, err := evaluateAddRepoFetchDecision(ctx, planItem, fetchOpts)
		if err != nil {
			fmt.Fprintf(c.Err, "decide fetch for %s: %v\n", cand.RepoKey, err)
			return exitError
		}
		prefetchRows = append(prefetchRows, addRepoFetchProgressRow{
			RepoKey: cand.RepoKey,
			Status:  addRepoFetchProgressQueued,
			Reason:  decision.Reason,
		})
		if !decision.ShouldFetch {
			prefetchEvents <- addRepoFetchProgressEvent{
				RepoKey: cand.RepoKey,
				Type:    addRepoFetchProgressDoneT,
				Success: true,
				Reason:  decision.Reason,
			}
			continue
		}

		prefetchWG.Add(1)
		go func(item addRepoPlanItem) {
			defer prefetchWG.Done()
			prefetchEvents <- addRepoFetchProgressEvent{
				RepoKey: item.Candidate.RepoKey,
				Type:    addRepoFetchProgressStart,
			}
			err := runAddRepoFetchWithPolicy(ctx, item.Candidate.BarePath)
			if err != nil {
				prefetchMu.Lock()
				prefetched[item.Candidate.RepoKey] = false
				prefetchFailed[item.Candidate.RepoKey] = err.Error()
				prefetchMu.Unlock()
				prefetchEvents <- addRepoFetchProgressEvent{
					RepoKey: item.Candidate.RepoKey,
					Type:    addRepoFetchProgressDoneT,
					Success: false,
					Reason:  err.Error(),
				}
				return
			}
			prefetchMu.Lock()
			prefetched[item.Candidate.RepoKey] = true
			delete(prefetchFailed, item.Candidate.RepoKey)
			prefetchMu.Unlock()
			prefetchEvents <- addRepoFetchProgressEvent{
				RepoKey: item.Candidate.RepoKey,
				Type:    addRepoFetchProgressDoneT,
				Success: true,
				Reason:  "prefetch complete",
			}
		}(planItem)
	}
	prefetchDone := make(chan struct{})
	go func() {
		printAddRepoFetchProgress(c.Err, useColorErr, prefetchRows, prefetchEvents)
		close(prefetchDone)
	}()
	prefetchWG.Wait()
	close(prefetchEvents)
	<-prefetchDone

	if err := ensureAddRepoPlanFetchPhaseB(ctx, plan, fetchOpts, prefetched, prefetchFailed); err != nil {
		fmt.Fprintf(c.Err, "fetch selected repos: %v\n", err)
		return exitError
	}

	if err := preflightAddRepoPlan(ctx, root, workspaceID, plan); err != nil {
		fmt.Fprintf(c.Err, "preflight add-repo: %v\n", err)
		return exitError
	}

	printAddRepoPlan(c.Out, workspaceID, plan, useColorOut)
	line, err := c.promptLine(renderAddRepoApplyPrompt(useColorErr))
	if err != nil {
		fmt.Fprintf(c.Err, "read confirmation: %v\n", err)
		return exitError
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
	default:
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}

	if err := ensureAddRepoPlanFetchPhaseB(ctx, plan, fetchOpts, prefetched, prefetchFailed); err != nil {
		fmt.Fprintf(c.Err, "fetch selected repos before apply: %v\n", err)
		return exitError
	}

	applied, err := applyAddRepoPlanAllOrNothing(ctx, plan, c.debugf)
	if err != nil {
		fmt.Fprintf(c.Err, "apply add-repo: %v\n", err)
		return exitError
	}
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	nowUnix := time.Now().Unix()
	if err := upsertWorkspaceMetaReposRestore(wsPath, buildWorkspaceMetaReposRestore(applied), nowUnix); err != nil {
		rollbackAddRepoApplied(ctx, applied, c.debugf)
		fmt.Fprintf(c.Err, "update %s: %v\n", workspaceMetaFilename, err)
		return exitError
	}

	printAddRepoResult(c.Out, applied, useColorOut)
	c.debugf("ws add-repo completed workspace=%s added=%d", workspaceID, len(applied))
	return exitOK
}

func (c *CLI) runWSAddRepoJSON(workspaceID string, root string, repoPoolPath string, repoKeys []string, baseRefInput string, branchInput string, branchTemplate string, yes bool, fetchOpts addRepoFetchOptions) int {
	ctx := context.Background()
	if len(repoKeys) == 0 {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "add-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "invalid_argument",
				Message: "--repo is required in --format json mode",
			},
		})
		return exitUsage
	}
	if !yes {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "add-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "invalid_argument",
				Message: "--yes is required in --format json mode",
			},
		})
		return exitUsage
	}

	existsFS, activeFS, fsErr := workspaceActiveOnFilesystem(root, workspaceID)
	if fsErr != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "add-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("load workspace: %v", fsErr),
			},
		})
		return exitError
	}
	if !existsFS {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "add-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "workspace_not_found",
				Message: fmt.Sprintf("workspace not found: %s", workspaceID),
			},
		})
		return exitError
	}
	if !activeFS {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "add-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "conflict",
				Message: fmt.Sprintf("workspace is not active (status=archived): %s", workspaceID),
			},
		})
		return exitError
	}

	releaseLock, err := acquireWorkspaceAddRepoLock(root, workspaceID)
	if err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "add-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "conflict",
				Message: err.Error(),
			},
		})
		return exitError
	}
	defer releaseLock()

	candidates, err := listAddRepoPoolCandidates(ctx, root, repoPoolPath, workspaceID, time.Now(), c.debugf)
	if err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "add-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("list repo pool candidates: %v", err),
			},
		})
		return exitError
	}
	byRepoKey := make(map[string]addRepoPoolCandidate, len(candidates))
	for _, cand := range candidates {
		byRepoKey[cand.RepoKey] = cand
	}
	plan := make([]addRepoPlanItem, 0, len(repoKeys))
	seen := map[string]bool{}
	for _, repoKey := range repoKeys {
		repoKey = strings.TrimSpace(repoKey)
		if repoKey == "" || seen[repoKey] {
			continue
		}
		seen[repoKey] = true
		cand, ok := byRepoKey[repoKey]
		if !ok {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "add-repo",
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: fmt.Sprintf("repo not available in pool for workspace: %s", repoKey),
				},
			})
			return exitUsage
		}
		defaultBaseRef, err := detectDefaultBaseRefFromBare(ctx, cand.BarePath)
		if err != nil {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "add-repo",
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("detect default base_ref for %s: %v", cand.RepoKey, err),
				},
			})
			return exitError
		}
		baseRefUsed, err := resolveBaseRefInput(baseRefInput, defaultBaseRef)
		if err != nil {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "add-repo",
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: fmt.Sprintf("invalid base_ref (must be origin/<branch>): %q", baseRefInput),
				},
			})
			return exitUsage
		}
		defaultBranch, err := renderAddRepoDefaultBranch(branchTemplate, workspaceID, cand.RepoKey)
		if err != nil {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "add-repo",
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: fmt.Sprintf("invalid workspace.branch.template: %v", err),
				},
			})
			return exitUsage
		}
		branch := resolveBranchInput(branchInput, defaultBranch)
		if err := gitutil.CheckRefFormat(ctx, "refs/heads/"+branch); err != nil {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "add-repo",
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: fmt.Sprintf("invalid branch name for %s: %v", cand.RepoKey, err),
				},
			})
			return exitUsage
		}
		plan = append(plan, addRepoPlanItem{
			Candidate:      cand,
			BaseRefInput:   baseRefInput,
			DefaultBaseRef: defaultBaseRef,
			BaseRefUsed:    baseRefUsed,
			Branch:         branch,
		})
	}

	if err := ensureAddRepoPlanFetchPhaseB(ctx, plan, fetchOpts, nil, nil); err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "add-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("fetch selected repos: %v", err),
			},
		})
		return exitError
	}
	if err := preflightAddRepoPlan(ctx, root, workspaceID, plan); err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "add-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "conflict",
				Message: fmt.Sprintf("preflight add-repo: %v", err),
			},
		})
		return exitError
	}
	applied, err := applyAddRepoPlanAllOrNothing(ctx, plan, c.debugf)
	if err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "add-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("apply add-repo: %v", err),
			},
		})
		return exitError
	}
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	nowUnix := time.Now().Unix()
	if err := upsertWorkspaceMetaReposRestore(wsPath, buildWorkspaceMetaReposRestore(applied), nowUnix); err != nil {
		rollbackAddRepoApplied(ctx, applied, c.debugf)
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "add-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("update %s: %v", workspaceMetaFilename, err),
			},
		})
		return exitError
	}
	repos := make([]string, 0, len(applied))
	for _, it := range applied {
		repos = append(repos, it.Plan.Candidate.RepoKey)
	}
	_ = writeCLIJSON(c.Out, cliJSONResponse{
		OK:          true,
		Action:      "add-repo",
		WorkspaceID: workspaceID,
		Result: map[string]any{
			"added": len(applied),
			"repos": repos,
		},
	})
	return exitOK
}

func workspaceActiveOnFilesystem(root string, workspaceID string) (exists bool, active bool, err error) {
	fi, err := os.Stat(filepath.Join(root, "workspaces", workspaceID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, false, nil
		}
		return false, false, err
	}
	if !fi.IsDir() {
		return false, false, nil
	}
	afi, err := os.Stat(filepath.Join(root, "archive", workspaceID))
	if err == nil && afi.IsDir() {
		return true, false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, false, err
	}
	return true, true, nil
}

func resolveWorkspaceIDForAddRepo(root string, cwd string, args []string) (string, error) {
	if len(args) == 1 {
		workspaceID := strings.TrimSpace(args[0])
		if err := validateWorkspaceID(workspaceID); err != nil {
			return "", fmt.Errorf("invalid workspace id: %w", err)
		}
		return workspaceID, nil
	}

	wsRoot := filepath.Join(root, "workspaces")
	rel, err := filepath.Rel(wsRoot, cwd)
	if err != nil {
		return "", fmt.Errorf("workspace id is required")
	}
	rel = filepath.Clean(rel)
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("workspace id is required when not inside KRA_ROOT/workspaces/<id>")
	}
	first := strings.Split(rel, string(filepath.Separator))[0]
	if err := validateWorkspaceID(first); err != nil {
		return "", fmt.Errorf("workspace id is required when not inside KRA_ROOT/workspaces/<id>")
	}
	return first, nil
}

func acquireWorkspaceAddRepoLock(root string, workspaceID string) (func(), error) {
	lockDir := filepath.Join(root, ".kra", "locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}
	lockPath := filepath.Join(lockDir, fmt.Sprintf("ws-add-repo-%s.lock", workspaceID))

	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if writeErr := writeWorkspaceAddRepoLockMetadata(f); writeErr != nil {
				_ = f.Close()
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("write workspace lock metadata: %w", writeErr)
			}
			released := false
			return func() {
				if released {
					return
				}
				released = true
				_ = f.Close()
				_ = os.Remove(lockPath)
			}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire workspace lock: %w", err)
		}

		stale, inspectErr := isWorkspaceAddRepoLockStale(lockPath)
		if inspectErr != nil {
			return nil, fmt.Errorf("inspect workspace lock: %w", inspectErr)
		}
		if !stale {
			return nil, fmt.Errorf("workspace is locked by another add-repo operation: %s", workspaceID)
		}
		if rmErr := os.Remove(lockPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale workspace lock: %w", rmErr)
		}
	}

	return nil, fmt.Errorf("workspace is locked by another add-repo operation: %s", workspaceID)
}

func writeWorkspaceAddRepoLockMetadata(f *os.File) error {
	meta := fmt.Sprintf("pid=%d\nstarted_at=%s\nruntime=%s/%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano), runtime.GOOS, runtime.GOARCH)
	if _, err := f.WriteString(meta); err != nil {
		return err
	}
	return f.Sync()
}

func isWorkspaceAddRepoLockStale(lockPath string) (bool, error) {
	raw, err := os.ReadFile(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	pid, ok := parseWorkspaceAddRepoLockPID(string(raw))
	if !ok || pid <= 0 {
		// Legacy or malformed lock file; treat as stale and recover automatically.
		return true, nil
	}
	return !isProcessAlive(pid), nil
}

func parseWorkspaceAddRepoLockPID(content string) (int, bool) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "pid=") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "pid="))
		if raw == "" {
			return 0, false
		}
		pid, err := strconv.Atoi(raw)
		if err != nil {
			return 0, false
		}
		return pid, true
	}
	return 0, false
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		var errno syscall.Errno
		if errors.As(err, &errno) && errno == syscall.EPERM {
			return true
		}
		return false
	}
	return true
}

func listAddRepoPoolCandidates(ctx context.Context, root string, repoPoolPath string, workspaceID string, now time.Time, debugf func(string, ...any)) ([]addRepoPoolCandidate, error) {
	_ = now
	baseCandidates, err := scanRepoPoolCandidatesFromFilesystem(ctx, repoPoolPath, debugf)
	if err != nil {
		return nil, err
	}

	bound, err := listWorkspaceReposForClose(ctx, root, workspaceID)
	if err != nil && debugf != nil {
		debugf("ws add-repo: bound repo lookup fallback err=%v", err)
	}
	boundRepoUID := make(map[string]bool, len(bound))
	for _, r := range bound {
		boundRepoUID[r.RepoUID] = true
	}

	out := make([]addRepoPoolCandidate, 0, len(baseCandidates))
	for _, it := range baseCandidates {
		if boundRepoUID[it.RepoUID] {
			continue
		}
		spec, err := repospec.Normalize(it.RemoteURL)
		if err != nil {
			if debugf != nil {
				debugf("skip repo candidate: normalize failed repo_uid=%s err=%v", it.RepoUID, err)
			}
			continue
		}
		barePath := repostore.StorePath(repoPoolPath, spec)
		if fi, err := os.Stat(barePath); err != nil || !fi.IsDir() {
			if debugf != nil {
				debugf("skip repo candidate: bare not found repo_uid=%s path=%s", it.RepoUID, barePath)
			}
			continue
		}
		out = append(out, addRepoPoolCandidate{
			RepoUID:   it.RepoUID,
			RepoKey:   it.RepoKey,
			RemoteURL: it.RemoteURL,
			Alias:     deriveAliasFromRepoKey(it.RepoKey),
			BarePath:  barePath,
		})
	}
	return out, nil
}

func scanRepoPoolCandidatesFromFilesystem(ctx context.Context, repoPoolPath string, debugf func(string, ...any)) ([]addRepoPoolCandidate, error) {
	items := make([]addRepoPoolCandidate, 0, 16)
	err := filepath.WalkDir(repoPoolPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || !strings.HasSuffix(path, ".git") {
			return nil
		}
		remoteURL, err := gitutil.RunBare(ctx, path, "config", "--get", "remote.origin.url")
		if err != nil {
			if debugf != nil {
				debugf("skip bare repo without origin url path=%s err=%v", path, err)
			}
			return nil
		}
		if missingPath, missing := missingLocalFileRemotePath(strings.TrimSpace(remoteURL)); missing {
			if debugf != nil {
				debugf("skip bare repo with missing local file remote path=%s remote_path=%s", path, missingPath)
			}
			return nil
		}
		spec, err := repospec.Normalize(strings.TrimSpace(remoteURL))
		if err != nil {
			if debugf != nil {
				debugf("skip bare repo normalize failed path=%s err=%v", path, err)
			}
			return nil
		}
		repoKey := fmt.Sprintf("%s/%s", spec.Owner, spec.Repo)
		items = append(items, addRepoPoolCandidate{
			RepoUID:   fmt.Sprintf("%s/%s", spec.Host, repoKey),
			RepoKey:   repoKey,
			RemoteURL: strings.TrimSpace(remoteURL),
			Alias:     spec.Repo,
			BarePath:  path,
		})
		return filepath.SkipDir
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return items, nil
}

func missingLocalFileRemotePath(remote string) (string, bool) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", false
	}
	u, err := url.Parse(remote)
	if err != nil || u.Scheme != "file" {
		return "", false
	}
	p := strings.TrimSpace(u.Path)
	if p == "" {
		return "", false
	}
	if _, err := os.Stat(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return p, true
		}
	}
	return "", false
}

func deriveAliasFromRepoKey(repoKey string) string {
	parts := strings.Split(strings.TrimSpace(repoKey), "/")
	if len(parts) == 0 {
		return strings.TrimSpace(repoKey)
	}
	return parts[len(parts)-1]
}

func (c *CLI) promptAddRepoPoolSelection(candidates []addRepoPoolCandidate) ([]addRepoPoolCandidate, error) {
	if len(candidates) == 0 {
		return nil, errSelectorCanceled
	}

	inFile, ok := c.In.(*os.File)
	if ok && isatty.IsTerminal(inFile.Fd()) {
		selectorCandidates := make([]workspaceSelectorCandidate, 0, len(candidates))
		candidateByID := make(map[string]addRepoPoolCandidate, len(candidates))
		for _, it := range candidates {
			selectorCandidates = append(selectorCandidates, workspaceSelectorCandidate{
				ID:    it.RepoKey,
				Title: "",
			})
			candidateByID[it.RepoKey] = it
		}
		selectedIDs, err := c.promptWorkspaceSelectorWithOptions("active", "add", "Repos(pool):", "repo", selectorCandidates)
		if err != nil {
			return nil, err
		}
		selected := make([]addRepoPoolCandidate, 0, len(selectedIDs))
		for _, id := range selectedIDs {
			cand, exists := candidateByID[id]
			if !exists {
				continue
			}
			selected = append(selected, cand)
		}
		if len(selected) == 0 {
			return nil, errSelectorCanceled
		}
		return selected, nil
	}

	return c.promptAddRepoPoolSelectionFallback(candidates)
}

func (c *CLI) promptAddRepoPoolSelectionFallback(candidates []addRepoPoolCandidate) ([]addRepoPoolCandidate, error) {
	if len(candidates) == 0 {
		return nil, errSelectorCanceled
	}

	useColorErr := writerSupportsColor(c.Err)
	filter := ""
	for {
		visible := filterAddRepoPoolCandidates(candidates, filter)
		fmt.Fprintln(c.Err, styleBold("Repos(pool):", useColorErr))
		fmt.Fprintln(c.Err)
		if len(visible) == 0 {
			fmt.Fprintf(c.Err, "%s(none)\n", uiIndent)
		} else {
			for i, it := range visible {
				fmt.Fprintf(c.Err, "%s[%d] %s\n", uiIndent, i+1, it.RepoKey)
			}
		}
		fmt.Fprintln(c.Err)
		fmt.Fprintf(c.Err, "%s%s %s\n", uiIndent, styleMuted("filter:", useColorErr), filter)

		line, err := c.promptLine(fmt.Sprintf("%sselect repos (comma numbers, /filter, empty=cancel): ", uiIndent))
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return nil, errSelectorCanceled
		}
		if strings.HasPrefix(line, "/") {
			filter = strings.TrimSpace(strings.TrimPrefix(line, "/"))
			continue
		}

		indices, err := parseMultiSelectIndices(line, len(visible))
		if err != nil {
			fmt.Fprintf(c.Err, "%sinvalid selection: %v\n", uiIndent, err)
			continue
		}
		selected := make([]addRepoPoolCandidate, 0, len(indices))
		for _, idx := range indices {
			selected = append(selected, visible[idx])
		}
		return selected, nil
	}
}

func filterAddRepoPoolCandidates(candidates []addRepoPoolCandidate, filter string) []addRepoPoolCandidate {
	if strings.TrimSpace(filter) == "" {
		return slices.Clone(candidates)
	}
	out := make([]addRepoPoolCandidate, 0, len(candidates))
	for _, it := range candidates {
		if fuzzyFilterMatch(it.RepoKey, filter) {
			out = append(out, it)
		}
	}
	return out
}

func parseMultiSelectIndices(input string, max int) ([]int, error) {
	if max <= 0 {
		return nil, fmt.Errorf("no selectable items")
	}
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty selection")
	}

	seen := map[int]bool{}
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return nil, fmt.Errorf("not a number: %q", f)
		}
		if v < 1 || v > max {
			return nil, fmt.Errorf("out of range: %d (1-%d)", v, max)
		}
		idx := v - 1
		if seen[idx] {
			continue
		}
		seen[idx] = true
		out = append(out, idx)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty selection")
	}
	return out, nil
}

func detectDefaultBaseRefFromBare(ctx context.Context, barePath string) (string, error) {
	if ref, err := gitutil.RunBare(ctx, barePath, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		ref = strings.TrimSpace(ref)
		const prefix = "refs/remotes/origin/"
		if strings.HasPrefix(ref, prefix) {
			branch := strings.TrimSpace(strings.TrimPrefix(ref, prefix))
			if branch != "" {
				return "origin/" + branch, nil
			}
		}
	}
	okMain, err := gitutil.ShowRefExistsBare(ctx, barePath, "refs/remotes/origin/main")
	if err != nil {
		return "", err
	}
	if okMain {
		return "origin/main", nil
	}
	okMaster, err := gitutil.ShowRefExistsBare(ctx, barePath, "refs/remotes/origin/master")
	if err != nil {
		return "", err
	}
	if okMaster {
		return "origin/master", nil
	}
	return "", fmt.Errorf("failed to detect default base_ref in bare repo")
}

func evaluateAddRepoFetchDecision(ctx context.Context, p addRepoPlanItem, opts addRepoFetchOptions) (addRepoFetchDecision, error) {
	if opts.NoFetch {
		return addRepoFetchDecision{
			ShouldFetch: false,
			Reason:      "skipped (--no-fetch)",
		}, nil
	}
	if opts.Refresh {
		return addRepoFetchDecision{
			ShouldFetch: true,
			Reason:      "required (--refresh)",
		}, nil
	}
	baseRef := strings.TrimSpace(p.BaseRefUsed)
	if baseRef == "" {
		baseRef = strings.TrimSpace(p.BaseRefInput)
	}
	if baseRef == "" {
		baseRef = strings.TrimSpace(p.DefaultBaseRef)
	}
	if baseRef == "" {
		return addRepoFetchDecision{}, fmt.Errorf("base_ref is required")
	}
	okBaseRef, err := gitutil.ShowRefExistsBare(ctx, p.Candidate.BarePath, "refs/remotes/"+baseRef)
	if err != nil {
		return addRepoFetchDecision{}, err
	}
	if !okBaseRef {
		return addRepoFetchDecision{
			ShouldFetch: true,
			Reason:      fmt.Sprintf("required (base_ref missing: %s)", baseRef),
		}, nil
	}
	okBranchRef, err := gitutil.ShowRefExistsBare(ctx, p.Candidate.BarePath, "refs/remotes/origin/"+p.Branch)
	if err != nil {
		return addRepoFetchDecision{}, err
	}
	_ = okBranchRef
	fetchHeadPath := filepath.Join(p.Candidate.BarePath, "FETCH_HEAD")
	info, err := os.Stat(fetchHeadPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return addRepoFetchDecision{
				ShouldFetch: true,
				Reason:      "required (stale, age=unknown)",
			}, nil
		}
		return addRepoFetchDecision{}, err
	}
	age := time.Since(info.ModTime())
	if age <= addRepoFetchTTL {
		return addRepoFetchDecision{
			ShouldFetch: false,
			Reason:      fmt.Sprintf("skipped (fresh, age=%s <= 5m)", age.Round(time.Second)),
		}, nil
	}
	return addRepoFetchDecision{
		ShouldFetch: true,
		Reason:      fmt.Sprintf("required (stale, age=%s)", age.Round(time.Second)),
	}, nil
}

func runAddRepoFetchWithPolicy(ctx context.Context, barePath string) error {
	if _, err := gitutil.RunBare(ctx, barePath, "fetch", "origin", "--prune", "--no-tags"); err != nil {
		if !isRetryableFetchError(err) {
			return err
		}
		if _, retryErr := gitutil.RunBare(ctx, barePath, "fetch", "origin", "--prune", "--no-tags"); retryErr != nil {
			return retryErr
		}
	}
	return nil
}

func isRetryableFetchError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	retryable := []string{
		"timeout",
		"timed out",
		"connection reset",
		"connection refused",
		"temporary",
		"network is unreachable",
		"tls handshake timeout",
	}
	for _, token := range retryable {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}

func ensureAddRepoPlanFetchPhaseB(ctx context.Context, plan []addRepoPlanItem, opts addRepoFetchOptions, prefetched map[string]bool, prefetchFailed map[string]string) error {
	for i := range plan {
		decision, err := evaluateAddRepoFetchDecision(ctx, plan[i], opts)
		if err != nil {
			return fmt.Errorf("%s: %w", plan[i].Candidate.RepoKey, err)
		}
		plan[i].FetchDecision = ""
		if !decision.ShouldFetch {
			continue
		}
		if prefetched != nil && prefetched[plan[i].Candidate.RepoKey] {
			continue
		}
		if prefetchFailed != nil {
			if reason, ok := prefetchFailed[plan[i].Candidate.RepoKey]; ok && strings.TrimSpace(reason) != "" {
				plan[i].FetchDecision = fmt.Sprintf("required (prefetch failed: %s)", reason)
			}
		}
		if err := runAddRepoFetchWithPolicy(ctx, plan[i].Candidate.BarePath); err != nil {
			return fmt.Errorf("%s: %w", plan[i].Candidate.RepoKey, err)
		}
	}
	return nil
}

func printAddRepoFetchProgress(out io.Writer, useColor bool, rows []addRepoFetchProgressRow, events <-chan addRepoFetchProgressEvent) {
	if len(rows) == 0 {
		for range events {
		}
		return
	}
	rowByRepo := make(map[string]*addRepoFetchProgressRow, len(rows))
	current := make([]addRepoFetchProgressRow, 0, len(rows))
	for i := range rows {
		current = append(current, rows[i])
		rowByRepo[rows[i].RepoKey] = &current[len(current)-1]
	}
	file, tty := out.(*os.File)
	if !tty || !writerIsTTY(file) {
		fmt.Fprintln(out)
		fmt.Fprintln(out, renderProgressTitle(useColor))
		for ev := range events {
			row := rowByRepo[ev.RepoKey]
			if row == nil {
				r := addRepoFetchProgressRow{RepoKey: ev.RepoKey, Status: addRepoFetchProgressQueued}
				current = append(current, r)
				row = &current[len(current)-1]
				rowByRepo[ev.RepoKey] = row
			}
			applyAddRepoFetchEvent(row, ev)
			status := string(row.Status)
			label := status
			if useColor {
				switch row.Status {
				case addRepoFetchProgressRunning:
					label = styleInfo(status, useColor)
				case addRepoFetchProgressDone:
					label = styleSuccess(status, useColor)
				case addRepoFetchProgressFailed:
					label = styleError(status, useColor)
				case addRepoFetchProgressSkipped:
					label = styleMuted(status, useColor)
				default:
					label = styleMuted(status, useColor)
				}
			}
			fmt.Fprintf(out, "%s- %s: %s\n", uiIndent, row.RepoKey, label)
		}
		return
	}

	spinnerFrames := []string{"-", "\\", "|", "/"}
	spinnerIndex := 0
	printed := 0
	render := func() {
		lines := renderAddRepoFetchProgressLines(useColor, current, spinnerFrames[spinnerIndex])
		if printed > 0 {
			fmt.Fprintf(out, "\x1b[%dA", printed)
		}
		for _, line := range lines {
			fmt.Fprintf(out, "\x1b[2K%s\n", line)
		}
		printed = len(lines)
	}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	open := true
	render()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				open = false
				render()
				return
			}
			row := rowByRepo[ev.RepoKey]
			if row == nil {
				current = append(current, addRepoFetchProgressRow{RepoKey: ev.RepoKey, Status: addRepoFetchProgressQueued})
				row = &current[len(current)-1]
				rowByRepo[ev.RepoKey] = row
			}
			applyAddRepoFetchEvent(row, ev)
			spinnerIndex = (spinnerIndex + 1) % len(spinnerFrames)
			render()
		case <-ticker.C:
			if !open || !hasAddRepoFetchRunning(current) {
				continue
			}
			spinnerIndex = (spinnerIndex + 1) % len(spinnerFrames)
			render()
		}
	}
}

func applyAddRepoFetchEvent(row *addRepoFetchProgressRow, ev addRepoFetchProgressEvent) {
	switch ev.Type {
	case addRepoFetchProgressStart:
		row.Status = addRepoFetchProgressRunning
		row.Reason = ""
	case addRepoFetchProgressDoneT:
		row.Reason = strings.TrimSpace(ev.Reason)
		if !ev.Success {
			row.Status = addRepoFetchProgressFailed
			return
		}
		if strings.HasPrefix(strings.ToLower(row.Reason), "skipped") {
			row.Status = addRepoFetchProgressSkipped
			return
		}
		row.Status = addRepoFetchProgressDone
	}
}

func hasAddRepoFetchRunning(rows []addRepoFetchProgressRow) bool {
	for _, row := range rows {
		if row.Status == addRepoFetchProgressRunning || row.Status == addRepoFetchProgressQueued {
			return true
		}
	}
	return false
}

func renderAddRepoFetchProgressLines(useColor bool, rows []addRepoFetchProgressRow, spinner string) []string {
	lines := make([]string, 0, len(rows)+2)
	lines = append(lines, "")
	lines = append(lines, renderProgressTitle(useColor))
	for _, row := range rows {
		prefix := "·"
		switch row.Status {
		case addRepoFetchProgressQueued:
			prefix = "·"
		case addRepoFetchProgressRunning:
			prefix = spinner
		case addRepoFetchProgressSkipped:
			prefix = "-"
		case addRepoFetchProgressDone:
			prefix = "✔"
		case addRepoFetchProgressFailed:
			prefix = "!"
		}
		status := string(row.Status)
		if useColor {
			switch row.Status {
			case addRepoFetchProgressRunning:
				prefix = styleInfo(prefix, useColor)
				status = styleInfo(status, useColor)
			case addRepoFetchProgressDone:
				prefix = styleSuccess(prefix, useColor)
				status = styleSuccess(status, useColor)
			case addRepoFetchProgressFailed:
				prefix = styleError(prefix, useColor)
				status = styleError(status, useColor)
			case addRepoFetchProgressSkipped:
				prefix = styleMuted(prefix, useColor)
				status = styleMuted(status, useColor)
			default:
				prefix = styleMuted(prefix, useColor)
				status = styleMuted(status, useColor)
			}
		}
		line := fmt.Sprintf("%s%s %s (%s)", uiIndent, prefix, row.RepoKey, status)
		if strings.TrimSpace(row.Reason) != "" {
			line = fmt.Sprintf("%s: %s", line, row.Reason)
		}
		lines = append(lines, line)
	}
	return lines
}

func preflightAddRepoPlan(ctx context.Context, root string, workspaceID string, plan []addRepoPlanItem) error {
	if len(plan) == 0 {
		return fmt.Errorf("no repo selected")
	}

	existingRepos, err := listWorkspaceReposForClose(ctx, root, workspaceID)
	if err != nil {
		return fmt.Errorf("list workspace repos: %w", err)
	}
	aliasTaken := make(map[string]bool, len(existingRepos)+len(plan))
	for _, r := range existingRepos {
		aliasTaken[r.Alias] = true
	}

	for i := range plan {
		p := &plan[i]
		if p.Candidate.Alias == "" {
			return fmt.Errorf("alias is required: %s", p.Candidate.RepoKey)
		}
		if aliasTaken[p.Candidate.Alias] {
			return fmt.Errorf("repo alias conflict: %s", p.Candidate.Alias)
		}
		aliasTaken[p.Candidate.Alias] = true

		p.WorktreePath = filepath.Join(root, "workspaces", workspaceID, "repos", p.Candidate.Alias)
		if _, err := os.Stat(p.WorktreePath); err == nil {
			return fmt.Errorf("worktree path already exists: %s", p.WorktreePath)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat worktree path: %w", err)
		}

		p.BaseRefUsed = strings.TrimSpace(p.BaseRefInput)
		if p.BaseRefUsed == "" {
			p.BaseRefUsed = strings.TrimSpace(p.DefaultBaseRef)
		}
		if p.BaseRefUsed == "" {
			return fmt.Errorf("base_ref is required for %s", p.Candidate.RepoKey)
		}
		if !strings.HasPrefix(p.BaseRefUsed, "origin/") {
			return fmt.Errorf("invalid base_ref (must be origin/<branch>): %q", p.BaseRefUsed)
		}
		baseRef := "refs/remotes/" + p.BaseRefUsed
		ok, err := gitutil.ShowRefExistsBare(ctx, p.Candidate.BarePath, baseRef)
		if err != nil {
			return fmt.Errorf("check base_ref %s: %w", p.BaseRefUsed, err)
		}
		if !ok {
			return fmt.Errorf("base_ref not found: %s", p.BaseRefUsed)
		}

		localRef := "refs/heads/" + p.Branch
		p.LocalBranchExists, err = gitutil.ShowRefExistsBare(ctx, p.Candidate.BarePath, localRef)
		if err != nil {
			return fmt.Errorf("check local branch for %s: %w", p.Candidate.RepoKey, err)
		}

		remoteRef := "refs/remotes/origin/" + p.Branch
		p.RemoteBranchExists, err = gitutil.ShowRefExistsBare(ctx, p.Candidate.BarePath, remoteRef)
		if err != nil {
			return fmt.Errorf("check remote branch for %s: %w", p.Candidate.RepoKey, err)
		}

		if p.LocalBranchExists {
			inUse, err := isBranchCheckedOutInBare(ctx, p.Candidate.BarePath, p.Branch)
			if err != nil {
				return fmt.Errorf("check branch checkout status for %s: %w", p.Candidate.RepoKey, err)
			}
			if inUse {
				return fmt.Errorf("branch is already checked out by another worktree: %s", p.Branch)
			}
		}
	}
	return nil
}

func isBranchCheckedOutInBare(ctx context.Context, barePath string, branch string) (bool, error) {
	out, err := gitutil.RunBare(ctx, barePath, "worktree", "list", "--porcelain")
	if err != nil {
		return false, err
	}
	target := "refs/heads/" + strings.TrimSpace(branch)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "branch ") {
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(line, "branch ")) == target {
			return true, nil
		}
	}
	return false, nil
}

func printAddRepoPlan(out io.Writer, workspaceID string, plan []addRepoPlanItem, useColor bool) {
	bullet := styleMuted("•", useColor)
	reposLabel := styleAccent("repos", useColor)
	connectorMuted := func(connector string) string {
		return styleMuted(connector, useColor)
	}

	body := []string{
		fmt.Sprintf("%s%s add %d repos (worktrees) to workspace %s", uiIndent, bullet, len(plan), workspaceID),
	}
	if len(plan) > 0 {
		body = append(body, fmt.Sprintf("%s%s %s:", uiIndent, bullet, reposLabel))
	}
	for i, p := range plan {
		connector := "├─ "
		if i == len(plan)-1 {
			connector = "└─ "
		}
		body = append(body, fmt.Sprintf("%s%s%s", uiIndent+uiIndent, connectorMuted(connector), p.Candidate.RepoKey))
		if strings.TrimSpace(p.FetchDecision) != "" {
			stem := "│  "
			if i == len(plan)-1 {
				stem = "   "
			}
			body = append(body, fmt.Sprintf("%s%s%s fetch: %s", uiIndent+uiIndent, connectorMuted(stem), connectorMuted("└─"), p.FetchDecision))
		}
	}
	fmt.Fprintln(out)
	printSection(out, styleBold("Plan:", useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func renderAddRepoApplyPrompt(useColor bool) string {
	bullet := styleMuted("•", useColor)
	guide := styleMuted("[Enter=yes / n=no]", useColor)
	return fmt.Sprintf("%s%s apply this plan? %s: ", uiIndent, bullet, guide)
}

func renderAddRepoInputsProgress(out io.Writer, workspaceID string, rows []addRepoInputProgress, activeIndex int, useColor bool, prevLines int, showPendingBranch bool, keepBaseRefOpen bool) int {
	lines := buildAddRepoInputsLinesWithPendingOption(workspaceID, rows, activeIndex, useColor, showPendingBranch, keepBaseRefOpen)
	oldRows := prevLines
	if writerIsTTY(out) && oldRows > 0 {
		moveUp := oldRows
		fmt.Fprintf(out, "\x1b[%dA", moveUp)
	}
	if writerIsTTY(out) {
		renderRows := len(lines)
		if oldRows > renderRows {
			renderRows = oldRows
		}
		for i := 0; i < renderRows; i++ {
			line := ""
			if i < len(lines) {
				line = lines[i]
			}
			// Ensure each redraw starts from column 1; inline prompt may leave cursor mid-line.
			fmt.Fprintf(out, "\r\x1b[2K%s\n", line)
		}
		if oldRows > len(lines) {
			fmt.Fprintf(out, "\x1b[%dA", oldRows-len(lines))
		}
		return len(lines)
	}
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	return len(lines)
}

func buildAddRepoInputsLines(workspaceID string, rows []addRepoInputProgress, activeIndex int, useColor bool) []string {
	return buildAddRepoInputsLinesWithPendingOption(workspaceID, rows, activeIndex, useColor, true, true)
}

func buildAddRepoInputsLinesWithPendingOption(workspaceID string, rows []addRepoInputProgress, activeIndex int, useColor bool, showPendingBranch bool, keepBaseRefOpen bool) []string {
	bullet := styleMuted("•", useColor)
	labelWorkspace := styleAccent("workspace", useColor)
	labelRepos := styleAccent("repos", useColor)
	labelBaseRef := styleAccent("base_ref", useColor)
	labelBranch := styleAccent("branch", useColor)

	body := []string{
		fmt.Sprintf("%s%s %s: %s", uiIndent, bullet, labelWorkspace, workspaceID),
		fmt.Sprintf("%s%s %s:", uiIndent, bullet, labelRepos),
	}
	if len(rows) == 0 {
		return renderSectionAtoms(newSectionAtom(styleBold("Inputs:", useColor), body, sectionRenderOptions{
			blankAfterHeading: false,
			trailingBlank:     false,
		}))
	}
	displayCount := activeIndex + 1
	if displayCount < 1 {
		displayCount = 1
	}
	if displayCount > len(rows) {
		displayCount = len(rows)
	}

	for i := 0; i < displayCount; i++ {
		row := rows[i]
		repoConnector := "├─ "
		if i == displayCount-1 {
			repoConnector = "└─ "
		}
		body = append(body, fmt.Sprintf("%s%s%s", uiIndent+uiIndent, styleMuted(repoConnector, useColor), row.RepoKey))

		hasBase := strings.TrimSpace(row.BaseRef) != ""
		hasBranch := strings.TrimSpace(row.Branch) != ""
		if !hasBase && !hasBranch {
			continue
		}
		branchPending := showPendingBranch && i == activeIndex && hasBase && !hasBranch
		baseRefOpen := keepBaseRefOpen && i == activeIndex && hasBase && !hasBranch
		stem := "│  "
		if i == displayCount-1 {
			stem = "   "
		}
		if hasBase {
			connector := "└─ "
			if hasBranch || baseRefOpen || branchPending {
				connector = "├─ "
			}
			body = append(body, fmt.Sprintf("%s%s%s%s: %s", uiIndent+uiIndent, styleMuted(stem, useColor), styleMuted(connector, useColor), labelBaseRef, row.BaseRef))
		}
		if hasBranch {
			body = append(body, fmt.Sprintf("%s%s%s%s: %s", uiIndent+uiIndent, styleMuted(stem, useColor), styleMuted("└─ ", useColor), labelBranch, row.Branch))
		} else if branchPending {
			body = append(body, fmt.Sprintf("%s%s%s%s: %s", uiIndent+uiIndent, styleMuted(stem, useColor), styleMuted("└─ ", useColor), labelBranch, workspaceID))
		}
	}
	return renderSectionAtoms(newSectionAtom(styleBold("Inputs:", useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     false,
	}))
}

func renderAddRepoInputPrompt(prefix string, label string, defaultValue string, useColor bool) string {
	return fmt.Sprintf("%s%s: %s", prefix, styleAccent(label, useColor), defaultValue)
}

func (c *CLI) promptAddRepoEditableInput(prefix string, label string, initialValue string, useColor bool) (string, bool, error) {
	prompt := fmt.Sprintf("%s%s: ", prefix, styleAccent(label, useColor))
	initial := strings.TrimSpace(initialValue)

	inFile, ok := c.In.(*os.File)
	if !ok || !isatty.IsTerminal(inFile.Fd()) {
		line, err := c.promptLine(prompt + initial)
		if err != nil {
			return "", false, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return initial, false, nil
		}
		return line, true, nil
	}

	value, edited, err := runInlineTextInputWithInitial(inFile, c.Err, prompt, initial)
	if err != nil {
		if errors.Is(err, errInputCanceled) {
			return "", false, fmt.Errorf("interrupted")
		}
		return "", false, err
	}
	return value, edited, nil
}

func addRepoInputDetailPromptPrefix(useColor bool) string {
	// Match finalized detail line indentation: uiIndent*2 + stem("   ") + "└─ ".
	return uiIndent + uiIndent + "   " + styleMuted("└─ ", useColor)
}

func resolveBaseRefInput(rawInput string, defaultBaseRef string) (string, error) {
	v := strings.TrimSpace(rawInput)
	if v == "" {
		return strings.TrimSpace(defaultBaseRef), nil
	}
	if strings.HasPrefix(v, "origin/") {
		if len(v) <= len("origin/") {
			return "", fmt.Errorf("invalid base_ref")
		}
		return v, nil
	}
	if strings.HasPrefix(v, "/") {
		branch := strings.TrimSpace(strings.TrimPrefix(v, "/"))
		if branch == "" {
			return "", fmt.Errorf("invalid base_ref")
		}
		return "origin/" + branch, nil
	}
	return "origin/" + v, nil
}

func resolveBranchInput(rawInput string, defaultPrefix string) string {
	v := strings.TrimSpace(rawInput)
	if v == "" {
		return strings.TrimSpace(defaultPrefix)
	}
	return v
}

var addRepoBranchTemplatePlaceholderPattern = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)

func renderAddRepoDefaultBranch(template string, workspaceID string, repoKey string) (string, error) {
	trimmedTemplate := strings.TrimSpace(template)
	if trimmedTemplate == "" {
		return workspaceID, nil
	}

	allowed := map[string]string{
		"workspace_id": workspaceID,
		"repo_key":     repoKey,
		"repo_name":    addRepoBranchTemplateRepoName(repoKey),
	}
	rendered := addRepoBranchTemplatePlaceholderPattern.ReplaceAllStringFunc(trimmedTemplate, func(s string) string {
		m := addRepoBranchTemplatePlaceholderPattern.FindStringSubmatch(s)
		if len(m) < 2 {
			return s
		}
		key := strings.TrimSpace(m[1])
		if v, ok := allowed[key]; ok {
			return v
		}
		return "{{invalid:" + key + "}}"
	})
	if strings.Contains(rendered, "{{invalid:") {
		start := strings.Index(rendered, "{{invalid:")
		end := strings.Index(rendered[start:], "}}")
		token := "unknown"
		if start >= 0 && end > 0 {
			token = strings.TrimSuffix(strings.TrimPrefix(rendered[start:start+end+2], "{{invalid:"), "}}")
		}
		return "", fmt.Errorf("unsupported placeholder %q (allowed: workspace_id, repo_key, repo_name)", token)
	}
	if strings.Contains(rendered, "{{") || strings.Contains(rendered, "}}") {
		return "", fmt.Errorf("unresolved placeholder syntax")
	}
	return strings.TrimSpace(rendered), nil
}

func addRepoBranchTemplateRepoName(repoKey string) string {
	trimmed := strings.TrimSpace(repoKey)
	if trimmed == "" {
		return ""
	}
	lastSlash := strings.LastIndex(trimmed, "/")
	if lastSlash < 0 || lastSlash == len(trimmed)-1 {
		return trimmed
	}
	return trimmed[lastSlash+1:]
}

func applyAddRepoPlanAllOrNothing(ctx context.Context, plan []addRepoPlanItem, debugf func(string, ...any)) ([]addRepoAppliedItem, error) {
	applied := make([]addRepoAppliedItem, 0, len(plan))

	for _, p := range plan {
		current := addRepoAppliedItem{Plan: p}

		if !p.LocalBranchExists {
			if p.RemoteBranchExists {
				if _, err := gitutil.RunBare(ctx, p.Candidate.BarePath, "branch", "--track", p.Branch, "origin/"+p.Branch); err != nil {
					rollbackAddRepoApplied(ctx, applied, debugf)
					return nil, fmt.Errorf("create tracking branch for %s: %w", p.Candidate.RepoKey, err)
				}
			} else {
				if _, err := gitutil.RunBare(ctx, p.Candidate.BarePath, "branch", p.Branch, p.BaseRefUsed); err != nil {
					rollbackAddRepoApplied(ctx, applied, debugf)
					return nil, fmt.Errorf("create branch from base_ref for %s: %w", p.Candidate.RepoKey, err)
				}
			}
			current.CreatedLocalBranch = true
		}

		if _, err := gitutil.RunBare(ctx, p.Candidate.BarePath, "worktree", "add", p.WorktreePath, p.Branch); err != nil {
			rollbackAddRepoApplied(ctx, append(applied, current), debugf)
			return nil, fmt.Errorf("create worktree for %s: %w", p.Candidate.RepoKey, err)
		}
		current.CreatedWorktree = true

		applied = append(applied, current)
	}
	return applied, nil
}

func rollbackAddRepoApplied(ctx context.Context, applied []addRepoAppliedItem, debugf func(string, ...any)) {
	for i := len(applied) - 1; i >= 0; i-- {
		it := applied[i]
		if it.CreatedWorktree {
			_, _ = gitutil.RunBare(ctx, it.Plan.Candidate.BarePath, "worktree", "remove", "--force", it.Plan.WorktreePath)
			_ = os.RemoveAll(it.Plan.WorktreePath)
		}
		if it.CreatedLocalBranch {
			_, _ = gitutil.RunBare(ctx, it.Plan.Candidate.BarePath, "branch", "-D", it.Plan.Branch)
		}
	}
	if debugf != nil {
		debugf("ws add-repo rolled back applied_count=%d", len(applied))
	}
}

func printAddRepoResult(out io.Writer, applied []addRepoAppliedItem, useColor bool) {
	bullet := styleMuted("•", useColor)
	body := []string{
		fmt.Sprintf("%s%s Added %d / %d", uiIndent, bullet, len(applied), len(applied)),
	}
	for _, it := range applied {
		check := styleSuccess("✔", useColor)
		body = append(body, fmt.Sprintf("%s%s %s %s", uiIndent, bullet, check, it.Plan.Candidate.RepoKey))
	}
	printSection(out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func buildWorkspaceMetaReposRestore(applied []addRepoAppliedItem) []workspaceMetaRepoRestore {
	repos := make([]workspaceMetaRepoRestore, 0, len(applied))
	for _, it := range applied {
		repos = append(repos, workspaceMetaRepoRestore{
			RepoUID:   it.Plan.Candidate.RepoUID,
			RepoKey:   it.Plan.Candidate.RepoKey,
			RemoteURL: it.Plan.Candidate.RemoteURL,
			Alias:     it.Plan.Candidate.Alias,
			Branch:    it.Plan.Branch,
			BaseRef:   it.Plan.BaseRefUsed,
		})
	}
	return repos
}

func localDayKey(t time.Time) int {
	year, month, day := t.Date()
	return year*10000 + int(month)*100 + day
}
