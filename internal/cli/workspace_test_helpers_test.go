package cli

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/infra/appports"
	"github.com/tasuku43/kra/internal/infra/paths"
	"github.com/tasuku43/kra/internal/testutil"
)

var remoteRepoFixture struct {
	once sync.Once
	path string
	err  error
}

func initAndConfigureRootRepo(t *testing.T, root string) {
	t.Helper()
	testutil.RequireCommand(t, "git")

	if err := ensureInitLayout(root); err != nil {
		t.Fatalf("ensureInitLayout(%q): %v", root, err)
	}
	port := appports.NewInitPort(nil, nil)
	if err := port.TouchRegistry(root); err != nil {
		t.Fatalf("TouchRegistry(%q): %v", root, err)
	}
	if err := port.SetContextName(root, "test"); err != nil {
		t.Fatalf("SetContextName(%q): %v", root, err)
	}
	if err := paths.WriteCurrentContext(root); err != nil {
		t.Fatalf("WriteCurrentContext(%q): %v", root, err)
	}

	cmd := exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.email failed: %v (output=%s)", err, strings.TrimSpace(string(out)))
	}

	cmd = exec.Command("git", "config", "user.name", "test")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.name failed: %v (output=%s)", err, strings.TrimSpace(string(out)))
	}
}

func prepareRemoteRepoSpec(t *testing.T, runGit func(dir string, args ...string)) string {
	t.Helper()

	templateBare := prepareRemoteRepoTemplate(t, runGit)
	remoteBare := filepath.Join(t.TempDir(), "github.com", "o", "r.git")
	if err := os.MkdirAll(filepath.Dir(remoteBare), 0o755); err != nil {
		t.Fatalf("mkdir remoteBare dir: %v", err)
	}
	if err := copyDir(templateBare, remoteBare); err != nil {
		t.Fatalf("copy bare repo fixture: %v", err)
	}
	return "file://" + remoteBare
}

func prepareRemoteRepoTemplate(t *testing.T, runGit func(dir string, args ...string)) string {
	t.Helper()

	remoteRepoFixture.once.Do(func() {
		base, err := os.MkdirTemp("", "kra-remote-repo-fixture-*")
		if err != nil {
			remoteRepoFixture.err = err
			return
		}
		src := filepath.Join(base, "src")
		if err := os.MkdirAll(src, 0o755); err != nil {
			remoteRepoFixture.err = err
			return
		}
		runGit(src, "init", "-b", "main")
		runGit(src, "config", "user.email", "test@example.com")
		runGit(src, "config", "user.name", "test")
		if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hello\n"), 0o644); err != nil {
			remoteRepoFixture.err = err
			return
		}
		runGit(src, "add", ".")
		runGit(src, "commit", "-m", "init")

		remoteBare := filepath.Join(base, "github.com", "o", "r.git")
		if err := os.MkdirAll(filepath.Dir(remoteBare), 0o755); err != nil {
			remoteRepoFixture.err = err
			return
		}
		runGit("", "clone", "--bare", src, remoteBare)
		remoteRepoFixture.path = remoteBare
	})
	if remoteRepoFixture.err != nil {
		t.Fatalf("prepare remote repo template: %v", remoteRepoFixture.err)
	}
	return remoteRepoFixture.path
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()

		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return out.Close()
	})
}

func seedWorkspaceMeta(t *testing.T, root string, scope string, workspaceID string) string {
	t.Helper()

	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if scope == "archived" {
		wsPath = filepath.Join(root, "archive", workspaceID)
	}
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace path: %v", err)
	}

	now := time.Now().Unix()
	meta := newWorkspaceMetaFileForCreate(workspaceID, workspaceID, "", now)
	meta.Workspace.UpdatedAt = now
	if scope == "archived" {
		meta.Workspace.Status = "archived"
	}
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}
	return wsPath
}
