package cli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/config"
	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_RepoPreset_AddShowListRemove_And_WSAddRepoPreset(t *testing.T) {
	testutil.RequireCommand(t, "git")

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		if dir != "" {
			cmd.Dir = dir
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}

	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	repoSpec1 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "preset-a")
	repoSpec2 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "preset-b")
	_, repoKey1, _ := seedRepoPoolAndState(t, env, repoSpec1)
	_, repoKey2, _ := seedRepoPoolAndState(t, env, repoSpec2)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")

	origPrompt := promptRepoPresetSelection
	promptRepoPresetSelection = func(c *CLI, candidates []workspaceSelectorCandidate) ([]string, error) {
		return []string{repoKey2, repoKey1}, nil
	}
	defer func() { promptRepoPresetSelection = origPrompt }()

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "preset", "add", "backend"}); code != exitOK {
			t.Fatalf("repo preset add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		if !strings.Contains(out.String(), "backend") || !strings.Contains(out.String(), "repos: 2") {
			t.Fatalf("stdout missing add result summary: %q", out.String())
		}
	}

	rootConfigPath := filepath.Join(env.Root, ".kra", "config.yaml")
	cfg, err := config.LoadFile(rootConfigPath)
	if err != nil {
		t.Fatalf("LoadFile(root config) error: %v", err)
	}
	gotRepos := cfg.Workspace.RepoPresets["backend"].Repos
	if len(gotRepos) != 2 || gotRepos[0] != repoKey2 || gotRepos[1] != repoKey1 {
		t.Fatalf("backend repos order = %v, want [%s %s]", gotRepos, repoKey2, repoKey1)
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "preset", "show", "backend"}); code != exitOK {
			t.Fatalf("repo preset show exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		for _, want := range []string{"Preset:", repoKey2, repoKey1} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("stdout missing %q: %q", want, out.String())
			}
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "preset", "list"}); code != exitOK {
			t.Fatalf("repo preset list exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		if !strings.Contains(out.String(), "backend") {
			t.Fatalf("stdout missing preset name: %q", out.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "add-repo", "--format", "json", "--id", "WS1", "--preset", "backend", "--yes"}); code != exitOK {
			t.Fatalf("ws add-repo --preset exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		resp := decodeJSONResponse(t, out.String())
		if !resp.OK || resp.Action != "add-repo" {
			t.Fatalf("unexpected json response: %+v", resp)
		}
		if got := int(resp.Result["added"].(float64)); got != 2 {
			t.Fatalf("result.added = %d, want 2", got)
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "preset", "rm", "backend"}); code != exitOK {
			t.Fatalf("repo preset rm exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "preset", "list"}); code != exitOK {
			t.Fatalf("repo preset list exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		if !strings.Contains(out.String(), "(none)") {
			t.Fatalf("stdout should show empty list marker: %q", out.String())
		}
	}
}

func TestCLI_RepoPreset_Add_ExistingNonTTYRequiresYes(t *testing.T) {
	testutil.RequireCommand(t, "git")

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		if dir != "" {
			cmd.Dir = dir
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}

	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "preset-overwrite")
	_, repoKey, _ := seedRepoPoolAndState(t, env, repoSpec)

	origPrompt := promptRepoPresetSelection
	promptRepoPresetSelection = func(c *CLI, candidates []workspaceSelectorCandidate) ([]string, error) {
		return []string{repoKey}, nil
	}
	defer func() { promptRepoPresetSelection = origPrompt }()

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("")
		if code := c.Run([]string{"repo", "preset", "add", "backend"}); code != exitOK {
			t.Fatalf("initial repo preset add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("")
		if code := c.Run([]string{"repo", "preset", "add", "backend"}); code != exitUsage {
			t.Fatalf("repo preset add overwrite(no --yes) exit code = %d, want %d", code, exitUsage)
		}
		if !strings.Contains(err.String(), "--yes is required") {
			t.Fatalf("stderr missing overwrite hint: %q", err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("")
		if code := c.Run([]string{"repo", "preset", "add", "backend", "--yes"}); code != exitOK {
			t.Fatalf("repo preset add overwrite(--yes) exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
}
