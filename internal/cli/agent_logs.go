//go:build experimental

package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/infra/paths"
)

type agentLogsOptions struct {
	workspaceID string
	tail        int
	follow      bool
}

func (c *CLI) runAgentLogs(args []string) int {
	opts, err := parseAgentLogsOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printAgentLogsUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printAgentLogsUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve KRA_ROOT: %v\n", err)
		return exitError
	}

	records, err := loadAgentActivities(root)
	if err != nil {
		fmt.Fprintf(c.Err, "load agent activities: %v\n", err)
		return exitError
	}
	record, ok := findAgentActivityByWorkspace(records, opts.workspaceID)
	if !ok {
		fmt.Fprintf(c.Err, "agent activity not found for workspace: %s\n", opts.workspaceID)
		return exitError
	}
	if strings.TrimSpace(record.LogPath) == "" {
		fmt.Fprintf(c.Err, "log_path is empty for workspace: %s\n", opts.workspaceID)
		return exitError
	}

	logPath := record.LogPath
	if !isAbsPath(logPath) {
		logPath = pathJoin(root, logPath)
	}
	if err := printTailLines(c.Out, logPath, opts.tail); err != nil {
		fmt.Fprintf(c.Err, "read logs: %v\n", err)
		return exitError
	}
	if opts.follow {
		if err := followLogFile(c.Out, logPath); err != nil {
			fmt.Fprintf(c.Err, "follow logs: %v\n", err)
			return exitError
		}
	}
	return exitOK
}

func parseAgentLogsOptions(args []string) (agentLogsOptions, error) {
	opts := agentLogsOptions{tail: 100}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := rest[0]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return agentLogsOptions{}, errHelpRequested
		case strings.HasPrefix(arg, "--workspace="):
			opts.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--workspace="))
			rest = rest[1:]
		case arg == "--workspace":
			if len(rest) < 2 {
				return agentLogsOptions{}, fmt.Errorf("--workspace requires a value")
			}
			opts.workspaceID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--tail="):
			n, convErr := parsePositiveInt(strings.TrimSpace(strings.TrimPrefix(arg, "--tail=")))
			if convErr != nil {
				return agentLogsOptions{}, fmt.Errorf("--tail must be a positive integer")
			}
			opts.tail = n
			rest = rest[1:]
		case arg == "--tail":
			if len(rest) < 2 {
				return agentLogsOptions{}, fmt.Errorf("--tail requires a value")
			}
			n, convErr := parsePositiveInt(strings.TrimSpace(rest[1]))
			if convErr != nil {
				return agentLogsOptions{}, fmt.Errorf("--tail must be a positive integer")
			}
			opts.tail = n
			rest = rest[2:]
		case arg == "--follow":
			opts.follow = true
			rest = rest[1:]
		default:
			return agentLogsOptions{}, fmt.Errorf("unknown flag for agent logs: %q", arg)
		}
	}
	if len(rest) > 0 {
		return agentLogsOptions{}, fmt.Errorf("unexpected args for agent logs: %q", strings.Join(rest, " "))
	}
	if opts.workspaceID == "" {
		return agentLogsOptions{}, fmt.Errorf("--workspace is required")
	}
	return opts, nil
}

func findAgentActivityByWorkspace(records []agentActivityRecord, workspaceID string) (agentActivityRecord, bool) {
	for _, r := range records {
		if r.WorkspaceID == workspaceID {
			return r, true
		}
	}
	return agentActivityRecord{}, false
}

func printTailLines(out io.Writer, path string, tail int) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start := 0
	if tail < len(lines) {
		start = len(lines) - tail
	}
	for _, ln := range lines[start:] {
		fmt.Fprintln(out, ln)
	}
	return nil
}

func followLogFile(out io.Writer, path string) error {
	offset := int64(0)
	for {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			_ = f.Close()
			return err
		}
		reader := bufio.NewScanner(f)
		for reader.Scan() {
			fmt.Fprintln(out, reader.Text())
		}
		if err := reader.Err(); err != nil {
			_ = f.Close()
			return err
		}
		next, err := f.Seek(0, io.SeekCurrent)
		_ = f.Close()
		if err != nil {
			return err
		}
		offset = next
		time.Sleep(300 * time.Millisecond)
	}
}

func parsePositiveInt(raw string) (int, error) {
	if raw == "" {
		return 0, fmt.Errorf("empty")
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid")
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return 0, fmt.Errorf("invalid")
	}
	return n, nil
}

func isAbsPath(path string) bool {
	return strings.HasPrefix(path, "/")
}

func pathJoin(root, p string) string {
	return strings.TrimRight(root, "/") + "/" + strings.TrimLeft(p, "/")
}
