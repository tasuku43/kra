package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/gion-core/repospec"
	"github.com/tasuku43/gion-core/repostore"
	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/statestore"
	"github.com/tasuku43/gionx/internal/testutil"
)

func seedRepoPoolAndState(t *testing.T, env testutil.Env, repoSpecInput string) (repoUID string, repoKey string, alias string) {
	t.Helper()
	ctx := context.Background()

	spec, err := repospec.Normalize(repoSpecInput)
	if err != nil {
		t.Fatalf("Normalize(repoSpec): %v", err)
	}
	repoKey = fmt.Sprintf("%s/%s", spec.Owner, spec.Repo)
	repoUID = fmt.Sprintf("%s/%s", spec.Host, repoKey)
	alias = spec.Repo

	defaultBranch, err := gitutil.DefaultBranchFromRemote(ctx, repoSpecInput)
	if err != nil {
		t.Fatalf("DefaultBranchFromRemote() error: %v", err)
	}
	barePath := repostore.StorePath(env.RepoPoolPath(), spec)
	if _, err := gitutil.EnsureBareRepoFetched(ctx, repoSpecInput, barePath, defaultBranch); err != nil {
		t.Fatalf("EnsureBareRepoFetched() error: %v", err)
	}

	db, err := statestore.Open(ctx, env.StateDBPath())
	if err != nil {
		t.Fatalf("Open(state db) error: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := statestore.EnsureSettings(ctx, db, env.Root, env.RepoPoolPath()); err != nil {
		t.Fatalf("EnsureSettings error: %v", err)
	}
	now := time.Now().Unix()
	if err := statestore.EnsureRepo(ctx, db, statestore.EnsureRepoInput{
		RepoUID:   repoUID,
		RepoKey:   repoKey,
		RemoteURL: strings.TrimSpace(repoSpecInput),
		Now:       now,
	}); err != nil {
		t.Fatalf("EnsureRepo error: %v", err)
	}
	return repoUID, repoKey, alias
}

func addRepoSelectionInput(baseRef string, branch string) string {
	return fmt.Sprintf("1\n%s\n%s\n\n", strings.TrimSpace(baseRef), strings.TrimSpace(branch))
}
