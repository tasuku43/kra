package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireWorkspaceAddRepoLock_CreateAndRelease(t *testing.T) {
	root := t.TempDir()
	release, err := acquireWorkspaceAddRepoLock(root, "WS-LOCK-001")
	if err != nil {
		t.Fatalf("acquireWorkspaceAddRepoLock() error: %v", err)
	}

	lockPath := filepath.Join(root, ".gionx", "locks", "ws-add-repo-WS-LOCK-001.lock")
	raw, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	if !strings.Contains(string(raw), "pid=") {
		t.Fatalf("lock file must include pid metadata: %q", string(raw))
	}

	release()
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock file should be removed after release, stat err=%v", err)
	}
}

func TestAcquireWorkspaceAddRepoLock_StaleLegacyLockRecovered(t *testing.T) {
	root := t.TempDir()
	lockDir := filepath.Join(root, ".gionx", "locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(lockDir): %v", err)
	}
	lockPath := filepath.Join(lockDir, "ws-add-repo-WS-LOCK-002.lock")
	if err := os.WriteFile(lockPath, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile(lockPath): %v", err)
	}

	release, err := acquireWorkspaceAddRepoLock(root, "WS-LOCK-002")
	if err != nil {
		t.Fatalf("acquireWorkspaceAddRepoLock() should recover stale lock: %v", err)
	}
	release()
}

func TestAcquireWorkspaceAddRepoLock_ActivePIDLockBlocked(t *testing.T) {
	root := t.TempDir()
	lockDir := filepath.Join(root, ".gionx", "locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(lockDir): %v", err)
	}
	lockPath := filepath.Join(lockDir, "ws-add-repo-WS-LOCK-003.lock")
	content := fmt.Sprintf("pid=%d\n", os.Getpid())
	if err := os.WriteFile(lockPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(lockPath): %v", err)
	}

	_, err := acquireWorkspaceAddRepoLock(root, "WS-LOCK-003")
	if err == nil {
		t.Fatalf("acquireWorkspaceAddRepoLock() should fail for active pid lock")
	}
	if !strings.Contains(err.Error(), "workspace is locked by another add-repo operation") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseWorkspaceAddRepoLockPID(t *testing.T) {
	pid, ok := parseWorkspaceAddRepoLockPID("pid=12345\nstarted_at=...\n")
	if !ok || pid != 12345 {
		t.Fatalf("parseWorkspaceAddRepoLockPID() = (%d, %t), want (12345, true)", pid, ok)
	}
}
