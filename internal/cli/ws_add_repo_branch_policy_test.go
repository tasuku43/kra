package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_WS_AddRepo_JSON_DefaultBranchUsesWorkspaceBranchTemplate(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	repoSpec := createTestRemoteRepoSpec(t)
	_, repoKey, _ := seedRepoPoolAndState(t, env, repoSpec)

	writeRootConfigYAML(t, env.Root, "workspace:\n  branch:\n    template: \"feature/{{workspace_id}}/{{repo_name}}\"\n")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{
			"ws", "--act", "add-repo",
			"--format", "json",
			"--id", "WS1",
			"--repo", repoKey,
			"--yes",
		})
		if code != exitOK {
			t.Fatalf("ws add-repo json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		resp := decodeJSONResponse(t, out.String())
		if !resp.OK || resp.Action != "add-repo" {
			t.Fatalf("unexpected json response: %+v", resp)
		}
	}

	branch := readWorkspaceMetaBranch(t, env.Root, "WS1", repoKey)
	if branch != "feature/WS1/"+repoKey[strings.LastIndex(repoKey, "/")+1:] {
		t.Fatalf("branch=%q, want template-rendered value", branch)
	}
}

func TestCLI_WS_AddRepo_JSON_BranchFlagOverridesWorkspaceBranchTemplate(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	repoSpec := createTestRemoteRepoSpec(t)
	_, repoKey, _ := seedRepoPoolAndState(t, env, repoSpec)

	writeRootConfigYAML(t, env.Root, "workspace:\n  branch:\n    template: \"feature/{{workspace_id}}/{{repo_name}}\"\n")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{
			"ws", "--act", "add-repo",
			"--format", "json",
			"--id", "WS1",
			"--repo", repoKey,
			"--branch", "custom/branch-1",
			"--yes",
		})
		if code != exitOK {
			t.Fatalf("ws add-repo json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		resp := decodeJSONResponse(t, out.String())
		if !resp.OK || resp.Action != "add-repo" {
			t.Fatalf("unexpected json response: %+v", resp)
		}
	}

	branch := readWorkspaceMetaBranch(t, env.Root, "WS1", repoKey)
	if branch != "custom/branch-1" {
		t.Fatalf("branch=%q, want=%q", branch, "custom/branch-1")
	}
}

func writeRootConfigYAML(t *testing.T, root string, content string) {
	t.Helper()
	path := filepath.Join(root, ".kra", "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
}

func readWorkspaceMetaBranch(t *testing.T, root string, workspaceID string, repoKey string) string {
	t.Helper()
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		t.Fatalf("load %s: %v", workspaceMetaFilename, err)
	}
	for _, r := range meta.ReposRestore {
		if r.RepoKey == repoKey {
			return r.Branch
		}
	}
	t.Fatalf("repo_key not found in repos_restore: %s", repoKey)
	return ""
}
