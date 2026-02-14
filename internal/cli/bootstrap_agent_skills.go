package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/kra/internal/infra/paths"
)

type bootstrapAgentSkillsConflict struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Hint   string `json:"hint"`
}

type bootstrapAgentSkillsResult struct {
	Root      string                         `json:"root"`
	Created   []string                       `json:"created"`
	Linked    []string                       `json:"linked"`
	Skipped   []string                       `json:"skipped"`
	Conflicts []bootstrapAgentSkillsConflict `json:"conflicts"`
}

type bootstrapAgentSkillsConflictError struct {
	conflicts []bootstrapAgentSkillsConflict
}

type bootstrapSkillReferencePlan struct {
	path         string
	target       string
	parent       string
	createParent bool
}

func (e *bootstrapAgentSkillsConflictError) Error() string {
	return "bootstrap agent-skills conflict"
}

func (c *CLI) runBootstrapAgentSkills(args []string) int {
	outputFormat := "human"
	writeJSONError := func(code string, message string, exitCode int) int {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: "bootstrap.agent-skills",
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
			c.printBootstrapAgentSkillsUsage(c.Out)
			return exitOK
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printBootstrapAgentSkillsUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--root", "--context":
			if outputFormat == "json" {
				return writeJSONError("invalid_argument", fmt.Sprintf("%s is not supported for bootstrap agent-skills", args[0]), exitUsage)
			}
			fmt.Fprintf(c.Err, "%s is not supported for bootstrap agent-skills\n", args[0])
			c.printBootstrapAgentSkillsUsage(c.Err)
			return exitUsage
		default:
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--root=") || strings.HasPrefix(args[0], "--context=") {
				if outputFormat == "json" {
					return writeJSONError("invalid_argument", "--root/--context are not supported for bootstrap agent-skills", exitUsage)
				}
				fmt.Fprintln(c.Err, "--root/--context are not supported for bootstrap agent-skills")
				c.printBootstrapAgentSkillsUsage(c.Err)
				return exitUsage
			}
			if outputFormat == "json" {
				return writeJSONError("invalid_argument", fmt.Sprintf("unexpected args for bootstrap agent-skills: %q", strings.Join(args, " ")), exitUsage)
			}
			fmt.Fprintf(c.Err, "unexpected args for bootstrap agent-skills: %q\n", strings.Join(args, " "))
			c.printBootstrapAgentSkillsUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printBootstrapAgentSkillsUsage(c.Err)
		return exitUsage
	}

	root, err := resolveBootstrapAgentSkillsRoot()
	if err != nil {
		message := fmt.Sprintf("resolve current context root: %v", err)
		if outputFormat == "json" {
			return writeJSONError("not_found", message, exitError)
		}
		fmt.Fprintln(c.Err, message)
		return exitError
	}

	result, code, execErr := runBootstrapAgentSkills(root, c.isExperimentEnabled(experimentAgentSkillpack))
	if execErr != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "bootstrap.agent-skills",
				Result: result.toJSONResult(),
				Error: &cliJSONError{
					Code:    code,
					Message: fmt.Sprintf("bootstrap agent-skills: %v", execErr),
				},
			})
			return exitError
		}
		c.printBootstrapAgentSkillsHumanResult(c.Err, result)
		fmt.Fprintf(c.Err, "bootstrap agent-skills: %v\n", execErr)
		return exitError
	}

	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "bootstrap.agent-skills",
			Result: result.toJSONResult(),
		})
		return exitOK
	}
	c.printBootstrapAgentSkillsHumanResult(c.Out, result)
	return exitOK
}

func resolveBootstrapAgentSkillsRoot() (string, error) {
	root, ok, err := paths.ReadCurrentContext()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("current context is not set (run `kra context use <name|root>` or `kra init --root <path> --context <name>`)")
	}
	if !looksLikeKRARoot(root) {
		return "", fmt.Errorf("current context does not look like a KRA_ROOT: %s", root)
	}
	return root, nil
}

func looksLikeKRARoot(root string) bool {
	if !isDir(filepath.Join(root, "workspaces")) {
		return false
	}
	if !isDir(filepath.Join(root, "archive")) {
		return false
	}
	return true
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func runBootstrapAgentSkills(root string, seedSkillpack bool) (bootstrapAgentSkillsResult, string, error) {
	result := bootstrapAgentSkillsResult{
		Root:      filepath.Clean(root),
		Created:   []string{},
		Linked:    []string{},
		Skipped:   []string{},
		Conflicts: []bootstrapAgentSkillsConflict{},
	}

	skillsRoot := filepath.Join(result.Root, ".agent", "skills")
	if err := ensureBootstrapSkillsRoot(skillsRoot, &result); err != nil {
		return result, "internal_error", err
	}
	if seedSkillpack {
		if err := ensureBootstrapDefaultSkillpack(skillsRoot, &result); err != nil {
			return result, "internal_error", err
		}
	}

	plans := make([]bootstrapSkillReferencePlan, 0, 2)
	for _, linkPath := range []string{
		filepath.Join(result.Root, ".codex", "skills"),
		filepath.Join(result.Root, ".claude", "skills"),
	} {
		plan, err := planBootstrapSkillReference(linkPath, skillsRoot, &result)
		if err != nil {
			return result, "internal_error", err
		}
		if plan != nil {
			plans = append(plans, *plan)
		}
	}

	if len(result.Conflicts) > 0 {
		return result, "conflict", &bootstrapAgentSkillsConflictError{conflicts: append([]bootstrapAgentSkillsConflict{}, result.Conflicts...)}
	}

	for _, plan := range plans {
		if err := applyBootstrapSkillReferencePlan(plan, &result); err != nil {
			return result, "internal_error", err
		}
	}
	return result, "", nil
}

func ensureBootstrapSkillsRoot(path string, result *bootstrapAgentSkillsResult) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			appendBootstrapConflict(result, path, "exists and is not a directory")
			return nil
		}
		appendUniquePath(&result.Skipped, path)
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	appendUniquePath(&result.Created, path)
	return nil
}

func planBootstrapSkillReference(path string, target string, result *bootstrapAgentSkillsResult) (*bootstrapSkillReferencePlan, error) {
	parent := filepath.Dir(path)
	parentInfo, err := os.Stat(parent)
	parentMissing := false
	if err == nil {
		if !parentInfo.IsDir() {
			appendBootstrapConflict(result, parent, "exists and is not a directory")
			return nil, nil
		}
	} else if os.IsNotExist(err) {
		parentMissing = true
	} else {
		return nil, fmt.Errorf("stat %s: %w", parent, err)
	}

	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 && symlinkPointsTo(path, target) {
			appendUniquePath(&result.Skipped, path)
			return nil, nil
		}
		appendBootstrapConflict(result, path, fmt.Sprintf("exists and is not the expected symlink to %s", target))
		return nil, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("lstat %s: %w", path, err)
	}
	return &bootstrapSkillReferencePlan{
		path:         path,
		target:       target,
		parent:       parent,
		createParent: parentMissing,
	}, nil
}

func applyBootstrapSkillReferencePlan(plan bootstrapSkillReferencePlan, result *bootstrapAgentSkillsResult) error {
	if plan.createParent {
		if err := os.MkdirAll(plan.parent, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", plan.parent, err)
		}
		appendUniquePath(&result.Created, plan.parent)
	}
	if _, err := os.Lstat(plan.path); err == nil {
		return fmt.Errorf("path already exists during bootstrap apply: %s", plan.path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("lstat %s: %w", plan.path, err)
	}
	if err := os.Symlink(plan.target, plan.path); err != nil {
		return fmt.Errorf("create symlink %s -> %s: %w", plan.path, plan.target, err)
	}
	appendUniquePath(&result.Linked, plan.path)
	return nil
}

func symlinkPointsTo(path string, target string) bool {
	targetCanonical, ok := canonicalPath(target)
	if !ok {
		return false
	}

	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		resolvedCanonical, resolvedOK := canonicalPath(resolved)
		if !resolvedOK {
			return false
		}
		return resolvedCanonical == targetCanonical
	}

	linkTarget, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(linkTarget) {
		linkTarget = filepath.Join(filepath.Dir(path), linkTarget)
	}
	linkTargetCanonical, linkTargetOK := canonicalPath(linkTarget)
	if !linkTargetOK {
		return false
	}
	return linkTargetCanonical == targetCanonical
}

func canonicalPath(path string) (string, bool) {
	candidate := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		candidate = resolved
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	return filepath.Clean(abs), true
}

func appendBootstrapConflict(result *bootstrapAgentSkillsResult, path string, reason string) {
	result.Conflicts = append(result.Conflicts, bootstrapAgentSkillsConflict{
		Path:   path,
		Reason: reason,
		Hint:   fmt.Sprintf("backup or rename %s, then rerun `kra bootstrap agent-skills`", path),
	})
}

func (r bootstrapAgentSkillsResult) toJSONResult() map[string]any {
	return map[string]any{
		"root":      r.Root,
		"created":   append([]string{}, r.Created...),
		"linked":    append([]string{}, r.Linked...),
		"skipped":   append([]string{}, r.Skipped...),
		"conflicts": append([]bootstrapAgentSkillsConflict{}, r.Conflicts...),
	}
}

func (c *CLI) printBootstrapAgentSkillsHumanResult(w io.Writer, result bootstrapAgentSkillsResult) {
	useColorOut := writerSupportsColor(w)
	lines := []string{
		styleSuccess(fmt.Sprintf("target root: %s", result.Root), useColorOut),
		fmt.Sprintf("created: %d", len(result.Created)),
		fmt.Sprintf("linked: %d", len(result.Linked)),
		fmt.Sprintf("skipped: %d", len(result.Skipped)),
	}
	for _, conflict := range result.Conflicts {
		lines = append(lines, styleWarn(fmt.Sprintf("conflict: %s (%s)", conflict.Path, conflict.Reason), useColorOut))
		lines = append(lines, styleMuted(fmt.Sprintf("hint: %s", conflict.Hint), useColorOut))
	}
	printResultSection(w, useColorOut, lines...)
}
