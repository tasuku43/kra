package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tasuku43/kra/internal/core/workspacerisk"
	"github.com/tasuku43/kra/internal/infra/gitutil"
	"github.com/tasuku43/kra/internal/infra/statestore"
)

type gitRepoSnapshot struct {
	Status workspacerisk.RepoStatus

	Branch string

	Staged    int
	Unstaged  int
	Untracked int
	Files     []string
}

func inspectGitRepoSnapshot(ctx context.Context, dir string) gitRepoSnapshot {
	if _, err := os.Stat(dir); err != nil {
		return gitRepoSnapshot{Status: workspacerisk.RepoStatus{Error: err}}
	}

	gitDir, err := filepath.Abs(filepath.Join(dir, ".git"))
	if err != nil {
		return gitRepoSnapshot{Status: workspacerisk.RepoStatus{Error: err}}
	}
	out, err := gitutil.Run(ctx, dir, "--git-dir", gitDir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return gitRepoSnapshot{Status: workspacerisk.RepoStatus{Error: err}}
	}

	snapshot, parseErr := parseGitRepoSnapshot(out)
	if parseErr != nil {
		snapshot.Status.Error = parseErr
	}
	return snapshot
}

func parseGitRepoSnapshot(raw string) (gitRepoSnapshot, error) {
	snapshot := gitRepoSnapshot{}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}

		if strings.HasPrefix(line, "# ") {
			if err := parseBranchHeaderLine(line, &snapshot); err != nil {
				return gitRepoSnapshot{}, err
			}
			continue
		}

		short, x, y, untracked, include, err := parsePorcelainV2StatusLine(line)
		if err != nil {
			return gitRepoSnapshot{}, err
		}
		if !include {
			continue
		}
		snapshot.Status.Dirty = true
		snapshot.Files = append(snapshot.Files, short)
		if untracked {
			snapshot.Untracked++
			continue
		}
		if x != ' ' {
			snapshot.Staged++
		}
		if y != ' ' {
			snapshot.Unstaged++
		}
	}
	return snapshot, nil
}

func parseBranchHeaderLine(line string, snapshot *gitRepoSnapshot) error {
	line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
	parts := strings.SplitN(line, " ", 2)
	if len(parts) != 2 {
		return nil
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	switch key {
	case "branch.oid":
		snapshot.Status.HeadMissing = value == "(initial)" || value == "(unknown)"
	case "branch.head":
		if value == "(detached)" {
			snapshot.Status.Detached = true
			snapshot.Branch = ""
			return nil
		}
		if value == "(unknown)" {
			snapshot.Branch = ""
			return nil
		}
		snapshot.Branch = value
	case "branch.upstream":
		snapshot.Status.Upstream = value
	case "branch.ab":
		ahead, behind, err := parsePorcelainV2AheadBehind(value)
		if err != nil {
			return err
		}
		snapshot.Status.AheadCount = ahead
		snapshot.Status.BehindCount = behind
	}
	return nil
}

func parsePorcelainV2AheadBehind(raw string) (int, int, error) {
	parts := strings.Fields(raw)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected branch.ab format: %q", raw)
	}
	ahead, err := strconv.Atoi(strings.TrimPrefix(parts[0], "+"))
	if err != nil {
		return 0, 0, fmt.Errorf("parse ahead count: %w", err)
	}
	behind, err := strconv.Atoi(strings.TrimPrefix(parts[1], "-"))
	if err != nil {
		return 0, 0, fmt.Errorf("parse behind count: %w", err)
	}
	return ahead, behind, nil
}

func parsePorcelainV2StatusLine(line string) (short string, x byte, y byte, untracked bool, include bool, err error) {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "? "):
		path := strings.TrimSpace(strings.TrimPrefix(line, "? "))
		if path == "" {
			return "", ' ', ' ', false, false, nil
		}
		return "?? " + path, '?', '?', true, true, nil
	case strings.HasPrefix(line, "! "):
		return "", ' ', ' ', false, false, nil
	case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "), strings.HasPrefix(line, "u "):
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return "", ' ', ' ', false, false, fmt.Errorf("unexpected porcelain v2 status line: %q", line)
		}
		xy := strings.TrimSpace(fields[1])
		if len(xy) != 2 {
			return "", ' ', ' ', false, false, fmt.Errorf("unexpected porcelain v2 XY field: %q", line)
		}
		x = normalizePorcelainV2XYChar(xy[0])
		y = normalizePorcelainV2XYChar(xy[1])
		path := extractPorcelainV2Path(line, fields)
		if path == "" {
			return "", ' ', ' ', false, false, fmt.Errorf("missing porcelain v2 path: %q", line)
		}
		return fmt.Sprintf("%c%c %s", x, y, path), x, y, false, true, nil
	default:
		return "", ' ', ' ', false, false, nil
	}
}

func extractPorcelainV2Path(line string, fields []string) string {
	if len(fields) == 0 {
		return ""
	}

	if fields[0] == "2" {
		if strings.Contains(line, "\t") {
			leftRight := strings.SplitN(line, "\t", 2)
			if len(leftRight) == 2 {
				left := strings.Fields(strings.TrimSpace(leftRight[0]))
				newPath := ""
				if len(left) > 0 {
					newPath = strings.TrimSpace(left[len(left)-1])
				}
				oldPath := strings.TrimSpace(leftRight[1])
				if oldPath != "" && newPath != "" {
					return oldPath + " -> " + newPath
				}
			}
		}
		if len(fields) >= 11 {
			newPath := strings.TrimSpace(fields[9])
			oldPath := strings.TrimSpace(fields[10])
			if oldPath != "" && newPath != "" {
				return oldPath + " -> " + newPath
			}
		}
	}

	return strings.TrimSpace(fields[len(fields)-1])
}

func normalizePorcelainV2XYChar(v byte) byte {
	if v == '.' {
		return ' '
	}
	return v
}

func inspectWorkspaceRepoRisk(ctx context.Context, root string, workspaceID string, repos []statestore.WorkspaceRepo) (workspacerisk.WorkspaceRisk, []repoRiskItem) {
	states := make([]workspacerisk.RepoState, 0, len(repos))
	items := make([]repoRiskItem, 0, len(repos))
	for _, r := range repos {
		state := workspacerisk.RepoStateUnknown
		if !r.MissingAt.Valid {
			worktreePath := filepath.Join(root, "workspaces", workspaceID, "repos", r.Alias)
			snapshot := inspectGitRepoSnapshot(ctx, worktreePath)
			state = workspacerisk.ClassifyRepoStatus(snapshot.Status)
		}
		states = append(states, state)
		items = append(items, repoRiskItem{alias: r.Alias, state: state})
	}
	return workspacerisk.Aggregate(states), items
}
