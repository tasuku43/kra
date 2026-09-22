package repolock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestPath_IsRootLocalOperationLock(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "kra-root")
	want := filepath.Join(root, ".kra", "state", "operations", "repo-pool.lock")
	if got := Path(root); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestAcquire_SerializesAcrossCallers(t *testing.T) {
	root := t.TempDir()
	release, err := Acquire(context.Background(), root)
	if err != nil {
		t.Fatalf("Acquire() first err = %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	if _, err := Acquire(ctx, root); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire() second err = %v, want context deadline exceeded", err)
	}

	release()
	release()

	secondRelease, err := Acquire(context.Background(), root)
	if err != nil {
		t.Fatalf("Acquire() after release err = %v", err)
	}
	secondRelease()
}

func TestAcquire_RejectsEmptyRoot(t *testing.T) {
	if _, err := Acquire(context.Background(), " "); err == nil {
		t.Fatal("Acquire() err = nil, want error")
	}
}
