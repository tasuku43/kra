package cli

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/tasuku43/gionx/internal/infra/gitutil"
)

// toGitTopLevelPath converts a path relative to GIONX_ROOT into a path
// relative to the enclosing git toplevel (the path domain of `git diff --name-only`).
func toGitTopLevelPath(ctx context.Context, root string, rootRelativePath string) (string, error) {
	topRaw, err := gitutil.Run(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	top := filepath.Clean(strings.TrimSpace(topRaw))
	root = filepath.Clean(root)
	if resolved, evalErr := filepath.EvalSymlinks(top); evalErr == nil {
		top = filepath.Clean(resolved)
	}
	if resolved, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
		root = filepath.Clean(resolved)
	}

	relRoot := "."
	if r, relErr := filepath.Rel(top, root); relErr == nil {
		r = filepath.Clean(r)
		if r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			relRoot = "."
		} else {
			relRoot = r
		}
	}

	p := filepath.Clean(filepath.FromSlash(rootRelativePath))
	if relRoot == "." {
		return p, nil
	}
	return filepath.Clean(filepath.Join(relRoot, p)), nil
}
