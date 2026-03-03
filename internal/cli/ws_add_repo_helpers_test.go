package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/core/repostore"
	"github.com/tasuku43/kra/internal/gitutil"
	"github.com/tasuku43/kra/internal/testutil"
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
	if err := upsertRootRepoRegistryEntries(env.Root, []rootRepoRegistryEntry{{
		RepoUID:   repoUID,
		RepoKey:   repoKey,
		RemoteURL: repoSpecInput,
	}}); err != nil {
		t.Fatalf("upsertRootRepoRegistryEntries() error: %v", err)
	}
	return repoUID, repoKey, alias
}

func addRepoSelectionInput(baseRef string, branch string) string {
	return fmt.Sprintf("1\n%s\n%s\n\n", strings.TrimSpace(baseRef), strings.TrimSpace(branch))
}

func TestListAddRepoPoolCandidates_UsesCurrentRootRegistryOnly(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	_, registeredKey, _ := seedRepoPoolAndState(t, env, prepareRemoteRepoSpec(t, func(dir string, args ...string) {
		runGit(t, dir, args...)
	}))
	unregisteredSpec := prepareRemoteRepoSpec(t, func(dir string, args ...string) {
		runGit(t, dir, args...)
	})
	if _, err := gitutil.EnsureBareRepoFetched(context.Background(), unregisteredSpec, repostore.StorePath(env.RepoPoolPath(), mustNormalizeRepoSpec(t, unregisteredSpec)), "main"); err != nil {
		t.Fatalf("EnsureBareRepoFetched(unregistered) error: %v", err)
	}

	rows, err := listAddRepoPoolCandidates(context.Background(), env.Root, env.RepoPoolPath(), "WS-NOT-EXIST", time.Now(), nil)
	if err != nil {
		t.Fatalf("listAddRepoPoolCandidates() error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(rows))
	}
	if rows[0].RepoKey != registeredKey {
		t.Fatalf("candidate repo_key = %q, want %q", rows[0].RepoKey, registeredKey)
	}
}

func mustNormalizeRepoSpec(t *testing.T, repoSpecInput string) repospec.Spec {
	t.Helper()
	spec, err := repospec.Normalize(repoSpecInput)
	if err != nil {
		t.Fatalf("Normalize(repoSpecInput) error: %v", err)
	}
	return spec
}
