package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/tasuku43/gionx/internal/app/initcmd"
	"github.com/tasuku43/gionx/internal/infra/appports"
	"github.com/tasuku43/gionx/internal/infra/paths"
)

const (
	rootAgentsFilename = "AGENTS.md"
	gitignoreFilename  = ".gitignore"
)

func (c *CLI) runInit(args []string) int {
	rootFromFlag := ""
	for len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printInitUsage(c.Out)
			return exitOK
		case "--root":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--root requires a value")
				c.printInitUsage(c.Err)
				return exitUsage
			}
			rootFromFlag = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--root=") {
				rootFromFlag = strings.TrimSpace(strings.TrimPrefix(args[0], "--root="))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unexpected args for init: %q\n", strings.Join(args, " "))
			c.printInitUsage(c.Err)
			return exitUsage
		}
	}

	root, err := c.resolveInitRoot(rootFromFlag)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve GIONX_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "init"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run init args=%q", args)

	ctx := context.Background()
	svc := initcmd.NewService(appports.NewInitPort(ensureInitLayout, c.touchStateRegistry))
	result, err := svc.Run(ctx, initcmd.Request{Root: root})
	if err != nil {
		switch {
		case strings.HasPrefix(err.Error(), "init layout:"):
			fmt.Fprintf(c.Err, "%v\n", err)
		case strings.HasPrefix(err.Error(), "update root registry:"):
			fmt.Fprintf(c.Err, "%v\n", err)
		default:
			fmt.Fprintf(c.Err, "run init usecase: %v\n", err)
		}
		return exitError
	}
	if err := paths.WriteCurrentContext(result.Root); err != nil {
		fmt.Fprintf(c.Err, "update current context: %v\n", err)
		return exitError
	}

	useColorOut := writerSupportsColor(c.Out)
	printResultSection(c.Out, useColorOut, styleSuccess(fmt.Sprintf("Initialized: %s", result.Root), useColorOut))
	c.debugf("init completed root=%s", result.Root)
	return exitOK
}

func (c *CLI) resolveInitRoot(rootFromFlag string) (string, error) {
	if strings.TrimSpace(rootFromFlag) != "" {
		return normalizeInitRoot(rootFromFlag)
	}

	if inFile, ok := c.In.(*os.File); ok && isatty.IsTerminal(inFile.Fd()) {
		defaultRootAbs, defaultRootLabel, err := defaultInitRootSuggestion()
		if err != nil {
			return "", err
		}
		line, err := c.promptLine(fmt.Sprintf("root path [%s]: ", defaultRootLabel))
		if err != nil {
			return "", err
		}
		selected := strings.TrimSpace(line)
		if selected == "" {
			selected = defaultRootAbs
		}
		return normalizeInitRoot(selected)
	}

	return "", fmt.Errorf("non-interactive init requires --root")
}

func defaultInitRootSuggestion() (abs string, label string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(home, "gionx"), "~/gionx", nil
}

func normalizeInitRoot(rootRaw string) (string, error) {
	root, err := filepath.Abs(rootRaw)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)
	if err := ensureDir(root); err != nil {
		return "", err
	}
	return root, nil
}

func ensureInitLayout(root string) error {
	didGitInit, err := ensureGitInit(root)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		return fmt.Errorf("create workspaces/: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		return fmt.Errorf("create archive/: %w", err)
	}

	if err := ensureRootAgents(root); err != nil {
		return err
	}
	if err := ensureRootGitignore(root); err != nil {
		return err
	}
	if didGitInit {
		if err := commitInitFiles(root); err != nil {
			return err
		}
	}
	return nil
}

func ensureRootAgents(root string) error {
	path := filepath.Join(root, rootAgentsFilename)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", rootAgentsFilename, err)
	}

	if err := os.WriteFile(path, []byte(defaultRootAgentsContent()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", rootAgentsFilename, err)
	}
	return nil
}

func ensureRootGitignore(root string) error {
	path := filepath.Join(root, gitignoreFilename)
	const pattern = "workspaces/**/repos/**"

	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", gitignoreFilename, err)
	}

	if hasGitignoreLine(string(b), pattern) {
		return nil
	}

	var out string
	if len(b) == 0 {
		out = "# gionx\n" + pattern + "\n"
	} else {
		out = string(b)
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += pattern + "\n"
	}

	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", gitignoreFilename, err)
	}
	return nil
}

func hasGitignoreLine(contents string, want string) bool {
	for _, line := range strings.Split(contents, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func ensureGitInit(root string) (bool, error) {
	gitMeta := filepath.Join(root, ".git")
	if _, err := os.Stat(gitMeta); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat .git: %w", err)
	}

	if _, err := exec.LookPath("git"); err != nil {
		return false, fmt.Errorf("git not found in PATH: %w", err)
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git init failed: %w (output=%s)", err, strings.TrimSpace(string(output)))
	}
	return true, nil
}

func commitInitFiles(root string) error {
	const commitMessage = "init: add gionx bootstrap files"
	ctx := context.Background()
	allowGitignore, err := toGitTopLevelPath(ctx, root, gitignoreFilename)
	if err != nil {
		return err
	}
	allowAgents, err := toGitTopLevelPath(ctx, root, rootAgentsFilename)
	if err != nil {
		return err
	}
	allowlist := map[string]struct{}{
		allowGitignore: {},
		allowAgents:    {},
	}

	addCmd := exec.Command("git", "add", "--", gitignoreFilename, rootAgentsFilename)
	addCmd.Dir = root
	addOutput, err := addCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add init files failed: %w (output=%s)", err, strings.TrimSpace(string(addOutput)))
	}

	listCmd := exec.Command("git", "diff", "--cached", "--name-only")
	listCmd.Dir = root
	listOutput, err := listCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git diff --cached failed: %w (output=%s)", err, strings.TrimSpace(string(listOutput)))
	}

	staged := 0
	for _, line := range strings.Split(strings.TrimSpace(string(listOutput)), "\n") {
		p := strings.TrimSpace(line)
		if p == "" {
			continue
		}
		staged++
		if _, ok := allowlist[p]; !ok {
			return fmt.Errorf("staged path outside allowlist during init: %s", p)
		}
	}
	if staged == 0 {
		return nil
	}

	commitCmd := exec.Command("git", "commit", "-m", commitMessage)
	commitCmd.Dir = root
	commitOutput, err := commitCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git commit init files failed: %w (output=%s)", err, strings.TrimSpace(string(commitOutput)))
	}
	return nil
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("not a directory: %s", path)
	}
	return nil
}

func defaultRootAgentsContent() string {
	return `# gionx AGENTS guide

## Purpose

This repository root is managed by gionx.
gionx helps you keep task workspaces organized and safely archived.

## Directory map

- workspaces/<id>/notes/: text-first logs (investigation notes, decisions, links)
- workspaces/<id>/artifacts/: file-first evidence (screenshots, logs, dumps, PoCs)
- workspaces/<id>/repos/<alias>/: git worktrees (NOT Git-tracked)
- archive/<id>/: archived workspaces (Git-tracked)

Notes vs artifacts:
- notes/: write what you learned and decided
- artifacts/: store evidence files you may need later

## Workflow (typical)

1) gionx ws create
2) gionx ws --act add-repo
3) work inside workspaces/<id>/repos/<alias>/
4) gionx ws --act close

## Git policy

- Track: everything except workspaces/**/repos/**
- Ignore: workspaces/**/repos/**
`
}
