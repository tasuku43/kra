package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

func commitKRAMetaChange(ctx context.Context, root string, message string, allowlist []string) (string, error) {
	if err := ensureRootGitWorktree(ctx, root); err != nil {
		return "", err
	}
	pathspecs := normalizeCommitPathspecs(allowlist)
	if len(pathspecs) == 0 {
		return "", nil
	}

	allowed, err := allowedGitPathMatchers(ctx, root, pathspecs)
	if err != nil {
		return "", err
	}
	for _, p := range pathspecs {
		if _, err := runRootGit(ctx, root, "add", "-A", "--", p); err != nil {
			if strings.Contains(err.Error(), "did not match any files") || strings.Contains(err.Error(), "did not match any file") {
				continue
			}
			resetKRAMetaStaging(ctx, root, pathspecs)
			return "", err
		}
	}

	staged, err := gitDiffCachedNameOnlyPaths(ctx, root, pathspecs...)
	if err != nil {
		resetKRAMetaStaging(ctx, root, pathspecs)
		return "", err
	}
	stagedInScope := 0
	for _, p := range staged {
		if allowedPathInCommitScope(p, allowed) {
			stagedInScope++
			continue
		}
		resetKRAMetaStaging(ctx, root, pathspecs)
		return "", fmt.Errorf("unexpected staged path outside allowlist: %s", p)
	}
	if stagedInScope == 0 {
		resetKRAMetaStaging(ctx, root, pathspecs)
		return "", nil
	}

	args := []string{"commit", "--only", "-m", message, "--"}
	args = append(args, pathspecs...)
	if _, err := runRootGit(ctx, root, args...); err != nil {
		resetKRAMetaStaging(ctx, root, pathspecs)
		return "", err
	}
	resetKRAMetaStaging(ctx, root, pathspecs)

	sha, err := runRootGit(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

func normalizeCommitPathspecs(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, p := range paths {
		p = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(p))))
		if p == "" || p == "." || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

type commitPathMatcher struct {
	exact  string
	prefix string
}

func allowedGitPathMatchers(ctx context.Context, root string, pathspecs []string) ([]commitPathMatcher, error) {
	out := make([]commitPathMatcher, 0, len(pathspecs))
	for _, p := range pathspecs {
		gitPath, err := toGitTopLevelPath(ctx, root, p)
		if err != nil {
			return nil, err
		}
		gitPath = filepath.Clean(gitPath)
		out = append(out, commitPathMatcher{
			exact:  gitPath,
			prefix: gitPath + string(filepath.Separator),
		})
	}
	return out, nil
}

func allowedPathInCommitScope(path string, allowed []commitPathMatcher) bool {
	path = filepath.Clean(filepath.FromSlash(path))
	for _, a := range allowed {
		if path == a.exact || strings.HasPrefix(path, a.prefix) {
			return true
		}
	}
	return false
}

func resetKRAMetaStaging(ctx context.Context, root string, pathspecs []string) {
	for _, p := range pathspecs {
		if strings.TrimSpace(p) == "" {
			continue
		}
		_, _ = runRootGit(ctx, root, "reset", "-q", "--", p)
	}
}

func workspaceMetaCommitPathspec(root string, wsPath string) (string, error) {
	rel, err := filepath.Rel(root, filepath.Join(wsPath, workspaceMetaFilename))
	if err != nil {
		return "", err
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace metadata path is outside KRA_ROOT: %s", wsPath)
	}
	return filepath.ToSlash(rel), nil
}
