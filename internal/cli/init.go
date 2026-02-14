package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/tasuku43/kra/internal/app/initcmd"
	"github.com/tasuku43/kra/internal/infra/appports"
	"github.com/tasuku43/kra/internal/infra/gitutil"
	"github.com/tasuku43/kra/internal/infra/paths"
)

const (
	rootAgentsFilename = "AGENTS.md"
	gitignoreFilename  = ".gitignore"
)

func (c *CLI) runInit(args []string) int {
	rootFromFlag := ""
	contextFromFlag := ""
	bootstrapMode := ""
	outputFormat := "human"
	writeJSONError := func(code string, message string, exitCode int) int {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: "init",
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitCode
	}
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
		case "--context":
			if len(args) < 2 {
				if outputFormat == "json" {
					return writeJSONError("invalid_argument", "--context requires a value", exitUsage)
				}
				fmt.Fprintln(c.Err, "--context requires a value")
				c.printInitUsage(c.Err)
				return exitUsage
			}
			contextFromFlag = strings.TrimSpace(args[1])
			args = args[2:]
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printInitUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--bootstrap":
			if len(args) < 2 {
				if outputFormat == "json" {
					return writeJSONError("invalid_argument", "--bootstrap requires a value", exitUsage)
				}
				fmt.Fprintln(c.Err, "--bootstrap requires a value")
				c.printInitUsage(c.Err)
				return exitUsage
			}
			if strings.TrimSpace(bootstrapMode) != "" {
				if outputFormat == "json" {
					return writeJSONError("invalid_argument", "only one --bootstrap value is supported", exitUsage)
				}
				fmt.Fprintln(c.Err, "only one --bootstrap value is supported")
				c.printInitUsage(c.Err)
				return exitUsage
			}
			bootstrapMode = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--root=") {
				rootFromFlag = strings.TrimSpace(strings.TrimPrefix(args[0], "--root="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--context=") {
				contextFromFlag = strings.TrimSpace(strings.TrimPrefix(args[0], "--context="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--bootstrap=") {
				if strings.TrimSpace(bootstrapMode) != "" {
					if outputFormat == "json" {
						return writeJSONError("invalid_argument", "only one --bootstrap value is supported", exitUsage)
					}
					fmt.Fprintln(c.Err, "only one --bootstrap value is supported")
					c.printInitUsage(c.Err)
					return exitUsage
				}
				bootstrapMode = strings.TrimSpace(strings.TrimPrefix(args[0], "--bootstrap="))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unexpected args for init: %q\n", strings.Join(args, " "))
			c.printInitUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printInitUsage(c.Err)
		return exitUsage
	}
	if outputFormat == "json" {
		if strings.TrimSpace(rootFromFlag) == "" {
			return writeJSONError("invalid_argument", "--root is required in --format json mode", exitUsage)
		}
		if strings.TrimSpace(contextFromFlag) == "" {
			return writeJSONError("invalid_argument", "--context is required in --format json mode", exitUsage)
		}
	}
	if bootstrapMode != "" && bootstrapMode != "agent-skills" {
		if outputFormat == "json" {
			return writeJSONError("invalid_argument", fmt.Sprintf("unsupported --bootstrap: %q (supported: agent-skills)", bootstrapMode), exitUsage)
		}
		fmt.Fprintf(c.Err, "unsupported --bootstrap: %q (supported: agent-skills)\n", bootstrapMode)
		c.printInitUsage(c.Err)
		return exitUsage
	}

	root, contextName, err := c.resolveInitInputs(rootFromFlag, contextFromFlag)
	if err != nil {
		if outputFormat == "json" {
			return writeJSONError("invalid_argument", fmt.Sprintf("resolve init inputs: %v", err), exitUsage)
		}
		fmt.Fprintf(c.Err, "resolve init inputs: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "init"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run init args=%q", args)

	ctx := context.Background()
	svc := initcmd.NewService(appports.NewInitPort(ensureInitLayout, c.touchStateRegistry))
	result, err := svc.Run(ctx, initcmd.Request{Root: root, ContextName: contextName})
	if err != nil {
		if outputFormat == "json" {
			return writeJSONError("internal_error", fmt.Sprintf("run init usecase: %v", err), exitError)
		}
		switch {
		case strings.HasPrefix(err.Error(), "init layout:"):
			fmt.Fprintf(c.Err, "%v\n", err)
		case strings.HasPrefix(err.Error(), "update root registry:"):
			fmt.Fprintf(c.Err, "%v\n", err)
		case strings.HasPrefix(err.Error(), "update context registry:"):
			fmt.Fprintf(c.Err, "%v\n", err)
		default:
			fmt.Fprintf(c.Err, "run init usecase: %v\n", err)
		}
		return exitError
	}
	var bootstrapResult *bootstrapAgentSkillsResult
	if bootstrapMode == "agent-skills" {
		bootstrapRunResult, code, bootstrapErr := runBootstrapAgentSkills(result.Root, c.isExperimentEnabled(experimentAgentSkillpack))
		if bootstrapErr != nil {
			if outputFormat == "json" {
				return writeJSONError(code, fmt.Sprintf("bootstrap agent-skills: %v", bootstrapErr), exitError)
			}
			c.printBootstrapAgentSkillsHumanResult(c.Err, bootstrapRunResult)
			fmt.Fprintf(c.Err, "bootstrap agent-skills: %v\n", bootstrapErr)
			return exitError
		}
		bootstrapResult = &bootstrapRunResult
	}
	if err := paths.WriteCurrentContext(result.Root); err != nil {
		if outputFormat == "json" {
			return writeJSONError("internal_error", fmt.Sprintf("update current context: %v", err), exitError)
		}
		fmt.Fprintf(c.Err, "update current context: %v\n", err)
		return exitError
	}
	if outputFormat == "json" {
		resultPayload := map[string]any{
			"root":         result.Root,
			"context_name": contextName,
		}
		if bootstrapResult != nil {
			resultPayload["bootstrap"] = map[string]any{
				"agent_skills": bootstrapResult.toJSONResult(),
			}
		}
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "init",
			Result: resultPayload,
		})
		c.debugf("init completed root=%s", result.Root)
		return exitOK
	}

	useColorOut := writerSupportsColor(c.Out)
	resultLines := []string{
		styleSuccess(fmt.Sprintf("Initialized: %s", result.Root), useColorOut),
		styleSuccess(fmt.Sprintf("Context selected: %s", contextName), useColorOut),
	}
	if bootstrapResult != nil {
		resultLines = append(resultLines, styleSuccess(fmt.Sprintf("Bootstrap agent-skills: created=%d linked=%d skipped=%d", len(bootstrapResult.Created), len(bootstrapResult.Linked), len(bootstrapResult.Skipped)), useColorOut))
	}
	printResultSection(c.Out, useColorOut, resultLines...)
	c.debugf("init completed root=%s", result.Root)
	return exitOK
}

func (c *CLI) resolveInitInputs(rootFromFlag string, contextFromFlag string) (root string, contextName string, err error) {
	isTTY := false
	if inFile, ok := c.In.(*os.File); ok && isatty.IsTerminal(inFile.Fd()) {
		isTTY = true
	}

	if strings.TrimSpace(rootFromFlag) == "" {
		if !isTTY {
			return "", "", fmt.Errorf("non-interactive init requires --root")
		}
		defaultRootAbs, defaultRootLabel, err := defaultInitRootSuggestion()
		if err != nil {
			return "", "", err
		}
		line, err := c.promptLine(fmt.Sprintf("root path [%s]: ", defaultRootLabel))
		if err != nil {
			return "", "", err
		}
		selected := strings.TrimSpace(line)
		if selected == "" {
			selected = defaultRootAbs
		}
		rootFromFlag = selected
	}
	root, err = normalizeInitRoot(rootFromFlag)
	if err != nil {
		return "", "", err
	}

	contextFromFlag = strings.TrimSpace(contextFromFlag)
	if contextFromFlag == "" {
		if !isTTY {
			return "", "", fmt.Errorf("non-interactive init requires --context")
		}
		defaultName, err := defaultContextNameSuggestion()
		if err != nil {
			return "", "", err
		}
		line, err := c.promptLine(fmt.Sprintf("context name [%s]: ", defaultName))
		if err != nil {
			return "", "", err
		}
		contextFromFlag = strings.TrimSpace(line)
		if contextFromFlag == "" {
			contextFromFlag = defaultName
		}
	}
	if strings.TrimSpace(contextFromFlag) == "" {
		return "", "", fmt.Errorf("context name is required")
	}
	return root, contextFromFlag, nil
}

func defaultInitRootSuggestion() (abs string, label string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(home, "kra"), "~/kra", nil
}

func defaultContextNameSuggestion() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	base := strings.TrimSpace(filepath.Base(filepath.Clean(wd)))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "default", nil
	}
	return base, nil
}

func normalizeInitRoot(rootRaw string) (string, error) {
	expanded, err := expandInitRootHome(rootRaw)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)
	if err := ensureDir(root); err != nil {
		return "", err
	}
	return root, nil
}

func expandInitRootHome(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "~" || strings.HasPrefix(trimmed, "~/") || strings.HasPrefix(trimmed, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir for ~ expansion: %w", err)
		}
		if trimmed == "~" {
			return home, nil
		}
		suffix := trimmed[2:]
		return filepath.Join(home, suffix), nil
	}
	return raw, nil
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
	if err := ensureDefaultWorkspaceTemplate(root); err != nil {
		return err
	}
	if err := ensureRootConfig(root); err != nil {
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
		out = "# kra\n" + pattern + "\n"
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

	if err := gitutil.EnsureGitInPath(); err != nil {
		return false, err
	}
	if _, err := gitutil.Run(context.Background(), root, "init"); err != nil {
		return false, err
	}
	return true, nil
}

func commitInitFiles(root string) error {
	const commitMessage = "init: add kra bootstrap files"
	ctx := context.Background()
	allowGitignore, err := toGitTopLevelPath(ctx, root, gitignoreFilename)
	if err != nil {
		return err
	}
	allowAgents, err := toGitTopLevelPath(ctx, root, rootAgentsFilename)
	if err != nil {
		return err
	}
	defaultTemplateAgentsRel := filepath.Join(workspaceTemplatesDirName, defaultWorkspaceTemplateName, rootAgentsFilename)
	rootConfigRel := filepath.Join(".kra", "config.yaml")
	allowlist := map[string]struct{}{
		allowGitignore: {},
		allowAgents:    {},
	}

	addArgs := []string{"add", "--", gitignoreFilename, rootAgentsFilename}
	if _, statErr := os.Stat(filepath.Join(root, defaultTemplateAgentsRel)); statErr == nil {
		allowTemplateAgents, allowErr := toGitTopLevelPath(ctx, root, defaultTemplateAgentsRel)
		if allowErr != nil {
			return allowErr
		}
		allowlist[allowTemplateAgents] = struct{}{}
		addArgs = append(addArgs, filepath.ToSlash(defaultTemplateAgentsRel))
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("stat default template AGENTS.md: %w", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, rootConfigRel)); statErr == nil {
		allowRootConfig, allowErr := toGitTopLevelPath(ctx, root, rootConfigRel)
		if allowErr != nil {
			return allowErr
		}
		allowlist[allowRootConfig] = struct{}{}
		addArgs = append(addArgs, filepath.ToSlash(rootConfigRel))
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("stat root config: %w", statErr)
	}

	if _, err := gitutil.Run(ctx, root, addArgs...); err != nil {
		return err
	}

	listOutput, err := gitutil.Run(ctx, root, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}

	staged := 0
	for _, line := range strings.Split(strings.TrimSpace(listOutput), "\n") {
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

	if _, err := gitutil.Run(ctx, root, "commit", "-m", commitMessage); err != nil {
		return err
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
	return `# kra AGENTS guide

## Purpose

This repository root is managed by kra.
kra helps you keep task workspaces organized and safely archived.

## Directory map

- workspaces/<id>/notes/: text-first logs (investigation notes, decisions, links)
- workspaces/<id>/artifacts/: file-first evidence (screenshots, logs, dumps, PoCs)
- workspaces/<id>/repos/<alias>/: git worktrees (NOT Git-tracked)
- archive/<id>/: archived workspaces (Git-tracked)

Notes vs artifacts:
- notes/: write what you learned and decided
- artifacts/: store evidence files you may need later

## Workflow (typical)

1) kra ws create
2) kra ws --act add-repo
3) work inside workspaces/<id>/repos/<alias>/
4) kra ws --act close

## Git policy

- Track: everything except workspaces/**/repos/**
- Ignore: workspaces/**/repos/**
`
}

func ensureRootConfig(root string) error {
	path := paths.RootConfigPath(root)
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("root config path is a directory: %s", path)
		}
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat root config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create root config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(defaultRootConfigContent()), 0o644); err != nil {
		return fmt.Errorf("write root config: %w", err)
	}
	return nil
}

func defaultRootConfigContent() string {
	return `# kra root config
# Precedence (high -> low):
#   1) CLI flags
#   2) this file: <KRA_ROOT>/.kra/config.yaml
#   3) global: ~/.kra/config.yaml
#   4) built-in defaults
#
# Empty string values are treated as unset.

workspace:
  defaults:
    template: default

integration:
  jira:
    # base_url: https://jira.example.com
    # defaults:
    #   space: SRE
    #   type: sprint # sprint | jql
`
}

func ensureDefaultWorkspaceTemplate(root string) error {
	templatesDir := workspaceTemplatesPath(root)
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		return fmt.Errorf("create templates/: %w", err)
	}

	defaultPath := workspaceTemplatePath(root, defaultWorkspaceTemplateName)
	if info, err := os.Stat(defaultPath); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("default template path is not a directory: %s", defaultPath)
		}
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat default template: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(defaultPath, "notes"), 0o755); err != nil {
		return fmt.Errorf("create default template notes/: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(defaultPath, "artifacts"), 0o755); err != nil {
		return fmt.Errorf("create default template artifacts/: %w", err)
	}
	if err := os.WriteFile(filepath.Join(defaultPath, rootAgentsFilename), []byte(defaultWorkspaceTemplateAgentsContent()), 0o644); err != nil {
		return fmt.Errorf("write default template AGENTS.md: %w", err)
	}
	return nil
}

func defaultWorkspaceTemplateAgentsContent() string {
	return `# workspace AGENTS guide

## Directory map

- notes/: investigation notes, decisions, TODOs, links
- artifacts/: files and evidence (screenshots, logs, dumps, PoCs)
- repos/: git worktrees (NOT Git-tracked; added via kra ws --act add-repo)

Notes vs artifacts:
- notes/: write what you learned and decided
- artifacts/: store evidence files you may need later

## Closing

When you are done, run:
  kra ws --act close <workspace-id>
`
}
