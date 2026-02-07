package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const debugLogDir = ".gionx/logs"

type debugSession struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func (c *CLI) ensureDebugLog(root string, scope string) error {
	if !c.Debug {
		return nil
	}

	c.debugSession.mu.Lock()
	defer c.debugSession.mu.Unlock()

	if c.debugSession.file != nil {
		c.writeDebugLocked("scope=%s", scope)
		return nil
	}

	dir := filepath.Join(root, debugLogDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create debug log dir: %w", err)
	}

	name := fmt.Sprintf(
		"%s-%d-%s.log",
		time.Now().UTC().Format("20060102-150405"),
		os.Getpid(),
		sanitizeDebugScope(scope),
	)
	path := filepath.Join(dir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open debug log file: %w", err)
	}

	c.debugSession.file = f
	c.debugSession.path = path
	fmt.Fprintf(c.Err, "[debug] log: %s\n", path)
	c.writeDebugLocked("debug started scope=%s root=%s", scope, root)
	return nil
}

func (c *CLI) closeDebugLog() {
	c.debugSession.mu.Lock()
	defer c.debugSession.mu.Unlock()

	if c.debugSession.file == nil {
		return
	}

	c.writeDebugLocked("debug ended")
	_ = c.debugSession.file.Close()
	c.debugSession.file = nil
	c.debugSession.path = ""
}

func (c *CLI) debugf(format string, args ...any) {
	if !c.Debug {
		return
	}

	c.debugSession.mu.Lock()
	defer c.debugSession.mu.Unlock()
	c.writeDebugLocked(format, args...)
}

func (c *CLI) writeDebugLocked(format string, args ...any) {
	if c.debugSession.file == nil {
		return
	}

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(c.debugSession.file, "%s %s\n", ts, msg)
}

func sanitizeDebugScope(scope string) string {
	if strings.TrimSpace(scope) == "" {
		return "command"
	}

	var b strings.Builder
	b.Grow(len(scope))
	for _, r := range scope {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "command"
	}
	return out
}
