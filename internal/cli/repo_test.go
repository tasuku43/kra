package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/core/repostore"
	"github.com/tasuku43/kra/internal/repodiscovery"
	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_RepoAdd_AddsPoolAndRegistersRepo(t *testing.T) {
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
	env.EnsureRootLayout(t)
	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "helmfiles")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "add", repoSpec})
	if code != exitOK {
		t.Fatalf("repo add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "Added 1 / 1") {
		t.Fatalf("stdout missing result summary: %q", out.String())
	}

	spec, normErr := repospec.Normalize(repoSpec)
	if normErr != nil {
		t.Fatalf("Normalize(repoSpec): %v", normErr)
	}
	barePath := repostore.StorePath(env.RepoPoolPath(), spec)
	if fi, statErr := os.Stat(barePath); statErr != nil || !fi.IsDir() {
		t.Fatalf("bare repo missing: %s", barePath)
	}

	repoUID := fmt.Sprintf("%s/%s/%s", spec.Host, spec.Owner, spec.Repo)
	if strings.TrimSpace(repoUID) == "" {
		t.Fatalf("repo uid should not be empty")
	}
}

func TestCLI_RepoAdd_JSON_Success(t *testing.T) {
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
	env.EnsureRootLayout(t)
	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "json-add")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "add", "--format", "json", repoSpec})
	if code != exitOK {
		t.Fatalf("repo add --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "repo.add" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if got := int(resp.Result["added"].(float64)); got != 1 {
		t.Fatalf("result.added = %d, want 1", got)
	}
}

func TestCLI_RepoAdd_JSON_RequiresRepoSpec(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "add", "--format", "json"})
	if code != exitUsage {
		t.Fatalf("repo add --format json exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "repo.add" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_RepoAdd_JSON_PartialFailure(t *testing.T) {
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
	env.EnsureRootLayout(t)
	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "json-partial")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "add", "--format", "json", repoSpec, "not-a-repo-spec"})
	if code != exitError {
		t.Fatalf("repo add --format json exit code = %d, want %d", code, exitError)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "repo.add" || resp.Error.Code != "conflict" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if got := int(resp.Result["added"].(float64)); got != 1 {
		t.Fatalf("result.added = %d, want 1", got)
	}
}

func TestCLI_RepoAdd_RemoteMismatchFails(t *testing.T) {
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
	env.EnsureRootLayout(t)
	repoSpec1 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "helmfiles")
	repoSpec2 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "helmfiles")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "add", repoSpec1}); code != exitOK {
			t.Fatalf("repo add #1 exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "add", repoSpec2})
	if code != exitError {
		t.Fatalf("repo add #2 exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(out.String(), "remote_url mismatch") {
		t.Fatalf("stdout missing mismatch reason: %q", out.String())
	}
	if !strings.Contains(out.String(), "Added 0 / 1") {
		t.Fatalf("stdout missing result summary: %q", out.String())
	}
}

type fakeDiscoveryProvider struct {
	repos []repodiscovery.Repo
}

func (f *fakeDiscoveryProvider) Name() string {
	return "github"
}

func (f *fakeDiscoveryProvider) CheckAuth(ctx context.Context) error {
	return nil
}

func (f *fakeDiscoveryProvider) ListOrgRepos(ctx context.Context, org string) ([]repodiscovery.Repo, error) {
	return f.repos, nil
}

func TestCLI_RepoDiscover_ExcludesExistingAndAddsSelected(t *testing.T) {
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
	env.EnsureRootLayout(t)
	repoSpec1 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "existing")
	repoSpec2 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "newone")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "add", repoSpec1}); code != exitOK {
			t.Fatalf("repo add existing exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	spec1, _ := repospec.Normalize(repoSpec1)
	spec2, _ := repospec.Normalize(repoSpec2)

	provider := &fakeDiscoveryProvider{repos: []repodiscovery.Repo{
		{
			RepoUID:   fmt.Sprintf("%s/%s/%s", spec1.Host, spec1.Owner, spec1.Repo),
			RepoKey:   fmt.Sprintf("%s/%s", spec1.Owner, spec1.Repo),
			RemoteURL: repoSpec1,
		},
		{
			RepoUID:   fmt.Sprintf("%s/%s/%s", spec2.Host, spec2.Owner, spec2.Repo),
			RepoKey:   fmt.Sprintf("%s/%s", spec2.Owner, spec2.Repo),
			RemoteURL: repoSpec2,
		},
	}}
	origFactory := newRepoDiscoveryProvider
	newRepoDiscoveryProvider = func(name string) (repodiscovery.Provider, error) {
		return provider, nil
	}
	defer func() {
		newRepoDiscoveryProvider = origFactory
	}()
	origPrompt := promptRepoDiscoverSelection
	promptRepoDiscoverSelection = func(c *CLI, candidates []workspaceSelectorCandidate) ([]string, error) {
		return []string{"example-org/newone"}, nil
	}
	defer func() {
		promptRepoDiscoverSelection = origPrompt
	}()

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "discover", "--org", "example-org"})
	if code != exitOK {
		t.Fatalf("repo discover exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "Added 1 / 1") {
		t.Fatalf("stdout missing summary: %q", out.String())
	}
	if !strings.Contains(out.String(), "example-org/newone") {
		t.Fatalf("stdout missing added repo key: %q", out.String())
	}
	if strings.Contains(out.String(), "example-org/existing") {
		t.Fatalf("stdout should not include existing repo in selected result: %q", out.String())
	}
}

func TestParseRepoDiscoverOptions_DefaultProvider(t *testing.T) {
	opts, err := parseRepoDiscoverOptions([]string{"--org", "example-org"})
	if err != nil {
		t.Fatalf("parseRepoDiscoverOptions() error: %v", err)
	}
	if opts.Provider != "github" {
		t.Fatalf("provider = %q, want github", opts.Provider)
	}
}

func TestCLI_RepoRemove_RemovesSelectedRegisteredRepo(t *testing.T) {
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
	env.EnsureRootLayout(t)
	repoSpec1 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "remove-target")
	repoSpec2 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "keep-target")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "add", repoSpec1, repoSpec2}); code != exitOK {
			t.Fatalf("repo add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	origPrompt := promptRepoRemoveSelection
	promptRepoRemoveSelection = func(c *CLI, candidates []workspaceSelectorCandidate) ([]string, error) {
		return []string{"example-org/remove-target"}, nil
	}
	defer func() { promptRepoRemoveSelection = origPrompt }()

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	if code := c.Run([]string{"repo", "remove"}); code != exitOK {
		t.Fatalf("repo remove exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "Removed 1 / 1") {
		t.Fatalf("stdout missing remove summary: %q", out.String())
	}

	spec1, _ := repospec.Normalize(repoSpec1)
	spec2, _ := repospec.Normalize(repoSpec2)
	removedPath := repostore.StorePath(env.RepoPoolPath(), spec1)
	keptPath := repostore.StorePath(env.RepoPoolPath(), spec2)
	if fi, statErr := os.Stat(removedPath); statErr != nil || !fi.IsDir() {
		t.Fatalf("removed target bare repo should remain in pool: %s", removedPath)
	}
	if fi, statErr := os.Stat(keptPath); statErr != nil || !fi.IsDir() {
		t.Fatalf("kept bare repo missing: %s", keptPath)
	}
}

func TestCLI_RepoRemove_JSON_Success(t *testing.T) {
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
	env.EnsureRootLayout(t)
	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "remove-json")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "add", repoSpec}); code != exitOK {
			t.Fatalf("repo add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "remove", "--format", "json", "example-org/remove-json"})
	if code != exitOK {
		t.Fatalf("repo remove --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "repo.remove" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if got := int(resp.Result["removed"].(float64)); got != 1 {
		t.Fatalf("result.removed = %d, want 1", got)
	}
}

func TestCLI_RepoRemove_JSON_RequiresRepoKey(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "remove", "--format", "json"})
	if code != exitUsage {
		t.Fatalf("repo remove --format json exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "repo.remove" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_RepoRemove_FailsWhenRepoBoundToWorkspace(t *testing.T) {
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
	env.EnsureRootLayout(t)
	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "bound-repo")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "add", repoSpec}); code != exitOK {
			t.Fatalf("repo add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "TEST-200"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "TEST-200"))
		if code := c.Run([]string{"ws", "--act", "add-repo", "TEST-200"}); code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "remove", "example-org/bound-repo"})
	if code != exitError {
		t.Fatalf("repo remove exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "cannot remove repos that are still bound to workspaces") {
		t.Fatalf("stderr missing bound warning: %q", err.String())
	}
}

func TestCLI_RepoRemove_JSON_BlockedWhenRepoBound(t *testing.T) {
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
	env.EnsureRootLayout(t)
	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "bound-json")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "add", repoSpec}); code != exitOK {
			t.Fatalf("repo add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "TEST-201"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "TEST-201"))
		if code := c.Run([]string{"ws", "--act", "add-repo", "TEST-201"}); code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "remove", "--format", "json", "example-org/bound-json"})
	if code != exitError {
		t.Fatalf("repo remove --format json exit code = %d, want %d", code, exitError)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "repo.remove" || resp.Error.Code != "conflict" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_RepoGC_RemovesOrphanBareRepo(t *testing.T) {
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
	env.EnsureRootLayout(t)
	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "gc-target")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "add", repoSpec}); code != exitOK {
			t.Fatalf("repo add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	spec, normErr := repospec.Normalize(repoSpec)
	if normErr != nil {
		t.Fatalf("Normalize(repoSpec): %v", normErr)
	}
	barePath := repostore.StorePath(env.RepoPoolPath(), spec)
	if fi, statErr := os.Stat(barePath); statErr != nil || !fi.IsDir() {
		t.Fatalf("bare repo missing before gc: %s", barePath)
	}

	stdinR, stdinW, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("stdin pipe: %v", pipeErr)
	}
	defer func() { _ = stdinR.Close() }()
	_, _ = stdinW.WriteString("y\n")
	_ = stdinW.Close()

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = stdinR
	code := c.Run([]string{"repo", "gc", "example-org/gc-target"})
	if code != exitOK {
		t.Fatalf("repo gc exit code = %d, want %d (stderr=%q stdout=%q)", code, exitOK, err.String(), out.String())
	}
	if !strings.Contains(out.String(), "Removed 1 / 1") {
		t.Fatalf("stdout missing gc summary: %q", out.String())
	}
	if _, statErr := os.Stat(barePath); !os.IsNotExist(statErr) {
		t.Fatalf("bare repo still exists after gc: %s", barePath)
	}
}

func TestCLI_RepoGC_JSON_Success(t *testing.T) {
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
	env.EnsureRootLayout(t)
	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "gc-json-target")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "add", repoSpec}); code != exitOK {
			t.Fatalf("repo add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "gc", "--format", "json", "--yes", "example-org/gc-json-target"})
	if code != exitOK {
		t.Fatalf("repo gc --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "repo.gc" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if got := int(resp.Result["removed"].(float64)); got != 1 {
		t.Fatalf("result.removed = %d, want 1", got)
	}
}

func TestCLI_RepoGC_JSON_RequiresYes(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "gc", "--format", "json", "example-org/any"})
	if code != exitUsage {
		t.Fatalf("repo gc --format json exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "repo.gc" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_RepoGC_FailsWhenNoEligibleCandidates(t *testing.T) {
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
	env.EnsureRootLayout(t)
	initAndConfigureRootRepo(t, env.Root)
	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "gc-blocked")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "add", repoSpec}); code != exitOK {
			t.Fatalf("repo add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS-GC"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "WS-GC"))
		if code := c.Run([]string{"ws", "--act", "add-repo", "WS-GC"}); code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "gc", "example-org/gc-blocked"})
	if code != exitError {
		t.Fatalf("repo gc exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "selected repos are not eligible for gc") {
		t.Fatalf("stderr missing expected reason: %q", err.String())
	}
}

func TestCLI_RepoGC_JSON_BlockedWhenNotEligible(t *testing.T) {
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
	env.EnsureRootLayout(t)
	initAndConfigureRootRepo(t, env.Root)
	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "gc-json-blocked")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "add", repoSpec}); code != exitOK {
			t.Fatalf("repo add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS-GC-JSON"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "WS-GC-JSON"))
		if code := c.Run([]string{"ws", "--act", "add-repo", "WS-GC-JSON"}); code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"repo", "gc", "--format", "json", "--yes", "example-org/gc-json-blocked"})
	if code != exitError {
		t.Fatalf("repo gc --format json exit code = %d, want %d", code, exitError)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "repo.gc" || resp.Error.Code != "conflict" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_RepoGC_BlockedByArchiveMetadataReference(t *testing.T) {
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

	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "example-org", "gc-archive-ref")
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"repo", "add", repoSpec}); code != exitOK {
			t.Fatalf("repo add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
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
		c.In = strings.NewReader(addRepoSelectionInput("", "WS1/test"))
		if code := c.Run([]string{"ws", "--act", "add-repo", "WS1"}); code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "--act", "close", "WS1"}); code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"repo", "remove", "example-org/gc-archive-ref"})
		if code != exitError {
			t.Fatalf("repo remove exit code = %d, want %d", code, exitError)
		}
		if !strings.Contains(err.String(), "cannot remove repos that are still bound to workspaces") {
			t.Fatalf("stderr missing archive metadata block reason: %q", err.String())
		}
	}
}

func prepareRemoteRepoSpecWithName(t *testing.T, runGit func(dir string, args ...string), host string, owner string, repo string) string {
	t.Helper()

	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	runGit(src, "init", "-b", "main")
	runGit(src, "config", "user.email", "test@example.com")
	runGit(src, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(src, "add", ".")
	runGit(src, "commit", "-m", "init")

	remoteBare := filepath.Join(t.TempDir(), host, owner, repo+".git")
	if err := os.MkdirAll(filepath.Dir(remoteBare), 0o755); err != nil {
		t.Fatalf("mkdir remoteBare dir: %v", err)
	}
	runGit("", "clone", "--bare", src, remoteBare)
	return "file://" + remoteBare
}
