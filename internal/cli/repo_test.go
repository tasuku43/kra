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

	"github.com/tasuku43/gion-core/repospec"
	"github.com/tasuku43/gion-core/repostore"
	"github.com/tasuku43/gionx/internal/repodiscovery"
	"github.com/tasuku43/gionx/internal/statestore"
	"github.com/tasuku43/gionx/internal/testutil"
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
	repoSpec := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "chatwork", "helmfiles")

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

	db, openErr := statestore.Open(context.Background(), env.StateDBPath())
	if openErr != nil {
		t.Fatalf("Open(state db) error: %v", openErr)
	}
	defer func() { _ = db.Close() }()

	var count int
	repoUID := fmt.Sprintf("%s/%s/%s", spec.Host, spec.Owner, spec.Repo)
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(1) FROM repos WHERE repo_uid = ?", repoUID).Scan(&count); err != nil {
		t.Fatalf("query repos count: %v", err)
	}
	if count != 1 {
		t.Fatalf("repos count for %s = %d, want 1", repoUID, count)
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
	repoSpec1 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "chatwork", "helmfiles")
	repoSpec2 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "chatwork", "helmfiles")

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
	repoSpec1 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "chatwork", "existing")
	repoSpec2 := prepareRemoteRepoSpecWithName(t, runGit, "github.com", "chatwork", "newone")

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

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = strings.NewReader("1\n")
	code := c.Run([]string{"repo", "discover", "--org", "chatwork"})
	if code != exitOK {
		t.Fatalf("repo discover exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "Added 1 / 1") {
		t.Fatalf("stdout missing summary: %q", out.String())
	}
	if !strings.Contains(out.String(), "chatwork/newone") {
		t.Fatalf("stdout missing added repo key: %q", out.String())
	}
	if strings.Contains(out.String(), "chatwork/existing") {
		t.Fatalf("stdout should not include existing repo in selected result: %q", out.String())
	}
}

func TestParseRepoDiscoverOptions_DefaultProvider(t *testing.T) {
	opts, err := parseRepoDiscoverOptions([]string{"--org", "chatwork"})
	if err != nil {
		t.Fatalf("parseRepoDiscoverOptions() error: %v", err)
	}
	if opts.Provider != "github" {
		t.Fatalf("provider = %q, want github", opts.Provider)
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
