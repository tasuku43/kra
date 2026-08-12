package filelock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const retryInterval = 20 * time.Millisecond

var processLocks sync.Map

// Acquire obtains an exclusive process-wide and OS-level lock for path.
// The returned release function is idempotent.
func Acquire(ctx context.Context, path string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	processLockValue, _ := processLocks.LoadOrStore(path, newProcessLock())
	processLock := processLockValue.(chan struct{})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-processLock:
	}
	releaseProcessLock := func() { processLock <- struct{}{} }
	if err := ctx.Err(); err != nil {
		releaseProcessLock()
		return nil, err
	}

	lockFile, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		releaseProcessLock()
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	for {
		locked, lockErr := tryExclusiveFileLock(lockFile)
		if lockErr != nil {
			_ = lockFile.Close()
			releaseProcessLock()
			return nil, lockErr
		}
		if locked {
			var once sync.Once
			return func() {
				once.Do(func() {
					_ = unlockFile(lockFile)
					_ = lockFile.Close()
					releaseProcessLock()
				})
			}, nil
		}

		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = lockFile.Close()
			releaseProcessLock()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func newProcessLock() chan struct{} {
	lock := make(chan struct{}, 1)
	lock <- struct{}{}
	return lock
}
