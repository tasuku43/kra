package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/app/wsimport"
	"github.com/tasuku43/kra/internal/config"
	"github.com/tasuku43/kra/internal/paths"
	"github.com/tasuku43/kra/internal/testutil"
)

func TestGitHubImportAttachFailureRemovesWorkspaceAndReportsMessage(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	in := wsimport.GitHubWorkspaceInput{ID: "owner-repo-pr-1", Repository: "owner/repo", Number: 1, Branch: "bad branch"}
	plan, inputs := buildWSImportGitHubPlan("review", wsimport.GitHubScope{}, "open", 10, env.Root, []wsimport.GitHubWorkspaceInput{in})
	// A malformed registry fails before any network access.
	path := rootRepoRegistryPath(env.Root)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	c := New(&bytes.Buffer{}, &bytes.Buffer{})
	c.applyWSImportGitHubPlan(context.Background(), env.Root, config.Config{}, &plan, inputs)
	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", in.ID)); !os.IsNotExist(err) {
		t.Fatalf("workspace remains: %v", err)
	}
	b, err := json.Marshal(plan.Items)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary.Failed != 1 || !strings.Contains(string(b), `"message":`) {
		t.Fatalf("missing failure message: %s", b)
	}
}

func TestWorkspaceReposIgnoreNonGitDirectoriesAndStatusDoesNotEscape(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	base := filepath.Join(env.Root, "workspaces", "ws", "repos")
	for _, name := range []string{".claude", "ordinary"} {
		if err := os.MkdirAll(filepath.Join(base, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	repos, err := listWorkspaceReposFromFilesystem(context.Background(), env.Root, "active", "ws", workspaceMetaFile{})
	if err != nil || len(repos) != 0 {
		t.Fatalf("repos=%+v err=%v", repos, err)
	}
	if snap := inspectGitRepoSnapshot(context.Background(), filepath.Join(base, "ordinary")); snap.Status.Error == nil {
		t.Fatal("status escaped into root repository")
	}
}

func TestGitHubImportUsesExistingPoolRemote(t *testing.T) {
	for _, registered := range []bool{true, false} {
		t.Run(fmt.Sprint(registered), func(t *testing.T) {
			env := testutil.NewEnv(t)
			initAndConfigureRootRepo(t, env.Root)
			bare := filepath.Join(paths.RepoPoolPath(env.Root), "github.com", "owner", "repo.git")
			if err := os.MkdirAll(filepath.Dir(bare), 0755); err != nil {
				t.Fatal(err)
			}
			runGit(t, env.Root, "init", "--bare", bare)
			remote := "https://github.com/owner/repo.git"
			runGit(t, env.Root, "--git-dir", bare, "remote", "add", "origin", remote)
			if registered {
				if err := upsertRootRepoRegistryEntries(env.Root, []rootRepoRegistryEntry{{RepoUID: "github.com/owner/repo", RepoKey: "owner/repo", RemoteURL: remote}}); err != nil {
					t.Fatal(err)
				}
			}
			c := New(&bytes.Buffer{}, &bytes.Buffer{})
			err := c.autoAttachGitHubRepoToWorkspace(context.Background(), env.Root, config.Config{}, "missing", "owner/repo", "bad branch")
			if err == nil || strings.Contains(err.Error(), "remote_url mismatch") {
				t.Fatalf("expected later failure, got %v", err)
			}
		})
	}
}

func TestWSCloseDryRunIgnoresNonRepoDirectories(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")
	dir := filepath.Join(env.Root, "workspaces", "WS1", "repos", ".claude")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".cc-writes"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	c := New(&out, &stderr)
	code := c.Run([]string{"ws", "close", "--dry-run", "--format", "json", "--id", "WS1"})
	if code != exitOK || strings.Contains(out.String(), `"alias":".claude"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, out.String(), stderr.String())
	}
}
