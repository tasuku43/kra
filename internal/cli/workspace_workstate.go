package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tasuku43/kra/internal/infra/gitutil"
	"github.com/tasuku43/kra/internal/infra/statestore"
)

type workspaceWorkState string

const (
	workspaceWorkStateTodo       workspaceWorkState = "todo"
	workspaceWorkStateInProgress workspaceWorkState = "in-progress"
)

const (
	workspaceBaselineDirName = "workspace-baselines"
)

type workspaceBaselineRepo struct {
	BaselineHead string `json:"baseline_head,omitempty"`
}

type workspaceBaseline struct {
	Version   int                              `json:"version"`
	CreatedAt int64                            `json:"created_at"`
	Repos     map[string]workspaceBaselineRepo `json:"repos,omitempty"`
	FS        map[string]string                `json:"fs,omitempty"`
}

func workspaceBaselinePath(root string, workspaceID string) string {
	return filepath.Join(root, ".kra", "state", workspaceBaselineDirName, workspaceID+".json")
}

func normalizeWorkspaceBaseline(baseline workspaceBaseline) workspaceBaseline {
	if baseline.Version <= 0 {
		baseline.Version = 1
	}
	if baseline.Repos == nil {
		baseline.Repos = map[string]workspaceBaselineRepo{}
	}
	if baseline.FS == nil {
		baseline.FS = map[string]string{}
	}
	return baseline
}

func loadLegacyWorkspaceBaseline(root string, workspaceID string) (workspaceBaseline, error) {
	path := workspaceBaselinePath(root, workspaceID)
	b, err := os.ReadFile(path)
	if err != nil {
		return workspaceBaseline{}, err
	}
	var baseline workspaceBaseline
	if err := json.Unmarshal(b, &baseline); err != nil {
		return workspaceBaseline{}, err
	}
	return normalizeWorkspaceBaseline(baseline), nil
}

func loadWorkspaceBaseline(root string, workspaceID string) (workspaceBaseline, error) {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	meta, metaErr := loadWorkspaceMetaFile(wsPath)
	if metaErr == nil && meta.Baseline != nil {
		baseline := normalizeWorkspaceBaseline(*meta.Baseline)
		return baseline, nil
	}
	legacyBaseline, legacyErr := loadLegacyWorkspaceBaseline(root, workspaceID)
	if legacyErr == nil {
		return legacyBaseline, nil
	}
	if metaErr != nil {
		return workspaceBaseline{}, metaErr
	}
	if os.IsNotExist(legacyErr) {
		return workspaceBaseline{}, os.ErrNotExist
	}
	return workspaceBaseline{}, legacyErr
}

func saveWorkspaceBaseline(root string, workspaceID string, baseline workspaceBaseline) error {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		return err
	}
	normalized := normalizeWorkspaceBaseline(baseline)
	meta.Baseline = &normalized
	return writeWorkspaceMetaFile(wsPath, meta)
}

func createOrRefreshWorkspaceBaseline(ctx context.Context, root string, workspaceID string, now int64) error {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	meta, _ := loadWorkspaceMetaFile(wsPath)
	repos, err := listWorkspaceReposFromFilesystem(ctx, root, "active", workspaceID, meta)
	if err != nil {
		return err
	}
	fsHashes, err := collectWorkspaceNonRepoFileHashes(wsPath)
	if err != nil {
		return err
	}
	repoBaseline := map[string]workspaceBaselineRepo{}
	for _, repo := range repos {
		alias := strings.TrimSpace(repo.Alias)
		if alias == "" || repo.MissingAt.Valid {
			continue
		}
		worktreePath := filepath.Join(wsPath, "repos", alias)
		head, err := gitutil.Run(ctx, worktreePath, "rev-parse", "HEAD")
		if err != nil {
			continue
		}
		trimmedHead := strings.TrimSpace(head)
		if trimmedHead == "" {
			continue
		}
		repoBaseline[alias] = workspaceBaselineRepo{BaselineHead: trimmedHead}
	}
	baseline := workspaceBaseline{
		Version:   1,
		CreatedAt: now,
		Repos:     repoBaseline,
		FS:        fsHashes,
	}
	if err := saveWorkspaceBaseline(root, workspaceID, baseline); err != nil {
		return err
	}
	return nil
}

func ensureWorkspaceBaselineExists(ctx context.Context, root string, workspaceID string, now int64) error {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), workspaceMetaFilename) {
			return nil
		}
		return err
	}
	if meta.Baseline != nil {
		return nil
	}
	if _, err := os.Stat(workspaceBaselinePath(root, workspaceID)); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return createOrRefreshWorkspaceBaseline(ctx, root, workspaceID, now)
}

func removeWorkspaceBaseline(root string, workspaceID string) error {
	if err := os.Remove(workspaceBaselinePath(root, workspaceID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func resolveWorkspaceWorkState(
	ctx context.Context,
	root string,
	scope string,
	workspaceID string,
	repos []statestore.WorkspaceRepo,
) workspaceWorkState {
	if scope != "active" {
		return workspaceWorkStateTodo
	}

	state, err := deriveWorkspaceWorkStateFromBaseline(ctx, root, workspaceID, repos)
	if err != nil {
		// Baseline is rebuildable data; fail closed for work-state and continue.
		state = workspaceWorkStateInProgress
	}
	return state
}

func deriveWorkspaceWorkStateFromBaseline(ctx context.Context, root string, workspaceID string, repos []statestore.WorkspaceRepo) (workspaceWorkState, error) {
	baseline, err := loadWorkspaceBaseline(root, workspaceID)
	if err != nil {
		if os.IsNotExist(err) {
			return deriveWorkspaceWorkStateFromLegacyRisk(ctx, root, workspaceID, repos), nil
		}
		return workspaceWorkStateInProgress, err
	}
	changed, err := hasRepoSignalsChanged(ctx, root, workspaceID, repos, baseline)
	if err != nil {
		return workspaceWorkStateInProgress, err
	}
	if changed {
		return workspaceWorkStateInProgress, nil
	}
	fsChanged, err := hasWorkspaceFSChanged(root, workspaceID, baseline)
	if err != nil {
		return workspaceWorkStateInProgress, err
	}
	if fsChanged {
		return workspaceWorkStateInProgress, nil
	}
	return workspaceWorkStateTodo, nil
}

func deriveWorkspaceWorkStateFromLegacyRisk(ctx context.Context, root string, workspaceID string, repos []statestore.WorkspaceRepo) workspaceWorkState {
	risk, _ := inspectWorkspaceRepoRisk(ctx, root, workspaceID, repos)
	switch risk {
	case "clean":
		return workspaceWorkStateTodo
	default:
		return workspaceWorkStateInProgress
	}
}

func hasRepoSignalsChanged(ctx context.Context, root string, workspaceID string, repos []statestore.WorkspaceRepo, baseline workspaceBaseline) (bool, error) {
	currentAliases := map[string]struct{}{}
	for _, repo := range repos {
		alias := strings.TrimSpace(repo.Alias)
		if alias == "" {
			continue
		}
		currentAliases[alias] = struct{}{}
		if repo.MissingAt.Valid {
			return true, nil
		}

		worktreePath := filepath.Join(root, "workspaces", workspaceID, "repos", alias)
		snapshot := inspectGitRepoSnapshot(ctx, worktreePath)
		if snapshot.Status.Error != nil {
			return true, nil
		}
		if snapshot.Status.Dirty {
			return true, nil
		}
		currentHeadOut, err := gitutil.Run(ctx, worktreePath, "rev-parse", "HEAD")
		if err != nil {
			return true, nil
		}
		currentHead := strings.TrimSpace(currentHeadOut)
		base, ok := baseline.Repos[alias]
		if !ok || strings.TrimSpace(base.BaselineHead) == "" {
			return true, nil
		}
		if currentHead == strings.TrimSpace(base.BaselineHead) {
			continue
		}
		countOut, err := gitutil.Run(ctx, worktreePath, "rev-list", "--count", fmt.Sprintf("%s..%s", strings.TrimSpace(base.BaselineHead), currentHead))
		if err != nil {
			return true, nil
		}
		count, convErr := strconv.Atoi(strings.TrimSpace(countOut))
		if convErr != nil {
			return true, nil
		}
		if count > 0 {
			return true, nil
		}
		// Rewritten history where head changed but count is 0 should still be treated as progress.
		return true, nil
	}
	for alias := range baseline.Repos {
		if _, ok := currentAliases[alias]; !ok {
			return true, nil
		}
	}
	return false, nil
}

func hasWorkspaceFSChanged(root string, workspaceID string, baseline workspaceBaseline) (bool, error) {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	current, err := collectWorkspaceNonRepoFileHashes(wsPath)
	if err != nil {
		return false, err
	}
	if len(current) != len(baseline.FS) {
		return true, nil
	}
	for path, hash := range baseline.FS {
		if current[path] != hash {
			return true, nil
		}
	}
	return false, nil
}

func collectWorkspaceNonRepoFileHashes(wsPath string) (map[string]string, error) {
	out := map[string]string{}
	reposDir := filepath.Join(wsPath, "repos")
	err := filepath.WalkDir(wsPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == wsPath {
			return nil
		}
		if path == reposDir || strings.HasPrefix(path, reposDir+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(wsPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." || rel == workspaceMetaFilename {
			return nil
		}
		sum, err := hashWorkspaceFile(path, d)
		if err != nil {
			return err
		}
		out[rel] = sum
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func hashWorkspaceFile(path string, d fs.DirEntry) (string, error) {
	if d.Type()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256([]byte("symlink:" + target))
		return "sha256:" + hex.EncodeToString(sum[:]), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func workStateSortPriority(state workspaceWorkState) int {
	switch state {
	case workspaceWorkStateInProgress:
		return 0
	default:
		return 1
	}
}

func selectorMarkerForWorkState(state workspaceWorkState, selected bool) string {
	_ = state
	if selected {
		return "●"
	}
	return "○"
}

func wsListMarkerForWorkState(state workspaceWorkState) string {
	_ = state
	return "•"
}

func normalizeWorkspaceWorkState(state workspaceWorkState) workspaceWorkState {
	if state == workspaceWorkStateInProgress {
		return workspaceWorkStateInProgress
	}
	return workspaceWorkStateTodo
}
