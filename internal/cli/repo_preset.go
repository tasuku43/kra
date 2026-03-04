package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/tasuku43/kra/internal/app/repocmd"
	"github.com/tasuku43/kra/internal/config"
	"github.com/tasuku43/kra/internal/infra/appports"
	"gopkg.in/yaml.v3"
)

var promptRepoPresetSelection = func(c *CLI, candidates []workspaceSelectorCandidate) ([]string, error) {
	return c.promptWorkspaceSelectorWithOptions("active", "add", "Repo pool:", "repo", candidates)
}

func (c *CLI) runRepoPreset(args []string) int {
	if len(args) == 0 {
		c.printRepoPresetUsage(c.Err)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		c.printRepoPresetUsage(c.Out)
		return exitOK
	case "add":
		return c.runRepoPresetAdd(args[1:])
	case "rm", "remove":
		return c.runRepoPresetRemove(args[1:])
	case "list":
		return c.runRepoPresetList(args[1:])
	case "show":
		return c.runRepoPresetShow(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"repo", "preset"}, args[0]), " "))
		c.printRepoPresetUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runRepoPresetAdd(args []string) int {
	forceOverwrite := false
	pos := make([]string, 0, 1)
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		switch arg {
		case "-h", "--help", "help":
			c.printRepoPresetUsage(c.Out)
			return exitOK
		case "--yes":
			forceOverwrite = true
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(c.Err, "unknown flag for repo preset add: %q\n", arg)
				c.printRepoPresetUsage(c.Err)
				return exitUsage
			}
			pos = append(pos, arg)
		}
	}
	if len(pos) != 1 {
		fmt.Fprintln(c.Err, "repo preset add requires <name>")
		c.printRepoPresetUsage(c.Err)
		return exitUsage
	}
	presetName := strings.TrimSpace(pos[0])
	if err := validateRepoPresetName(presetName); err != nil {
		fmt.Fprintf(c.Err, "invalid preset name: %v\n", err)
		c.printRepoPresetUsage(c.Err)
		return exitUsage
	}

	session, err := c.runRepoPresetSession("repo-preset-add")
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}

	candidates, err := listRootRepoCandidatesFromFilesystem(context.Background(), session.Root, session.RepoPoolPath)
	if err != nil {
		fmt.Fprintf(c.Err, "list repos: %v\n", err)
		return exitError
	}
	if len(candidates) == 0 {
		fmt.Fprintln(c.Err, "no repos registered in current root")
		return exitError
	}

	selectorCandidates := make([]workspaceSelectorCandidate, 0, len(candidates))
	for _, cand := range candidates {
		selectorCandidates = append(selectorCandidates, workspaceSelectorCandidate{
			ID:    cand.RepoKey,
			Title: "",
		})
	}
	selectedIDs, err := promptRepoPresetSelection(c, selectorCandidates)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
		fmt.Fprintf(c.Err, "select repos: %v\n", err)
		return exitError
	}
	selected := normalizeRepoPresetRepos(selectedIDs)
	if len(selected) == 0 {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}

	rootConfigPath := filepath.Join(session.Root, ".kra", "config.yaml")
	cfg, err := config.LoadFile(rootConfigPath)
	if err != nil {
		fmt.Fprintf(c.Err, "load root config: %v\n", err)
		return exitError
	}
	_, exists := cfg.Workspace.RepoPresets[presetName]
	if exists {
		allow, confirmErr := c.confirmRepoPresetOverwrite(forceOverwrite)
		if confirmErr != nil {
			fmt.Fprintf(c.Err, "%v\n", confirmErr)
			return exitUsage
		}
		if !allow {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
	}
	if cfg.Workspace.RepoPresets == nil {
		cfg.Workspace.RepoPresets = map[string]config.WorkspaceRepoPreset{}
	}
	cfg.Workspace.RepoPresets[presetName] = config.WorkspaceRepoPreset{Repos: selected}
	if err := saveRootConfigFile(rootConfigPath, cfg); err != nil {
		fmt.Fprintf(c.Err, "save root config: %v\n", err)
		return exitError
	}

	printRepoPresetAddResult(c.Out, presetName, selected, exists, writerSupportsColor(c.Out))
	return exitOK
}

func (c *CLI) runRepoPresetRemove(args []string) int {
	if len(args) != 1 {
		if len(args) == 1 && isHelpArg(args[0]) {
			c.printRepoPresetUsage(c.Out)
			return exitOK
		}
		fmt.Fprintln(c.Err, "repo preset rm/remove requires <name>")
		c.printRepoPresetUsage(c.Err)
		return exitUsage
	}
	presetName := strings.TrimSpace(args[0])
	if isHelpArg(presetName) {
		c.printRepoPresetUsage(c.Out)
		return exitOK
	}
	if err := validateRepoPresetName(presetName); err != nil {
		fmt.Fprintf(c.Err, "invalid preset name: %v\n", err)
		c.printRepoPresetUsage(c.Err)
		return exitUsage
	}

	session, err := c.runRepoPresetSession("repo-preset-remove")
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	rootConfigPath := filepath.Join(session.Root, ".kra", "config.yaml")
	cfg, err := config.LoadFile(rootConfigPath)
	if err != nil {
		fmt.Fprintf(c.Err, "load root config: %v\n", err)
		return exitError
	}
	if len(cfg.Workspace.RepoPresets) == 0 {
		fmt.Fprintf(c.Err, "preset not found: %s\n", presetName)
		return exitError
	}
	if _, ok := cfg.Workspace.RepoPresets[presetName]; !ok {
		fmt.Fprintf(c.Err, "preset not found: %s\n", presetName)
		return exitError
	}
	delete(cfg.Workspace.RepoPresets, presetName)
	if len(cfg.Workspace.RepoPresets) == 0 {
		cfg.Workspace.RepoPresets = nil
	}
	if err := saveRootConfigFile(rootConfigPath, cfg); err != nil {
		fmt.Fprintf(c.Err, "save root config: %v\n", err)
		return exitError
	}

	printRepoPresetRemoveResult(c.Out, presetName, writerSupportsColor(c.Out))
	return exitOK
}

func (c *CLI) runRepoPresetList(args []string) int {
	if len(args) > 0 {
		if len(args) == 1 && isHelpArg(args[0]) {
			c.printRepoPresetUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "unexpected args for repo preset list: %q\n", strings.Join(args, " "))
		c.printRepoPresetUsage(c.Err)
		return exitUsage
	}
	session, err := c.runRepoPresetSession("repo-preset-list")
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	rootConfigPath := filepath.Join(session.Root, ".kra", "config.yaml")
	cfg, err := config.LoadFile(rootConfigPath)
	if err != nil {
		fmt.Fprintf(c.Err, "load root config: %v\n", err)
		return exitError
	}
	names := sortedRepoPresetNames(cfg.Workspace.RepoPresets)
	printRepoPresetList(c.Out, names, writerSupportsColor(c.Out))
	return exitOK
}

func (c *CLI) runRepoPresetShow(args []string) int {
	if len(args) != 1 {
		if len(args) == 1 && isHelpArg(args[0]) {
			c.printRepoPresetUsage(c.Out)
			return exitOK
		}
		fmt.Fprintln(c.Err, "repo preset show requires <name>")
		c.printRepoPresetUsage(c.Err)
		return exitUsage
	}
	presetName := strings.TrimSpace(args[0])
	if isHelpArg(presetName) {
		c.printRepoPresetUsage(c.Out)
		return exitOK
	}
	if err := validateRepoPresetName(presetName); err != nil {
		fmt.Fprintf(c.Err, "invalid preset name: %v\n", err)
		c.printRepoPresetUsage(c.Err)
		return exitUsage
	}
	session, err := c.runRepoPresetSession("repo-preset-show")
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	rootConfigPath := filepath.Join(session.Root, ".kra", "config.yaml")
	cfg, err := config.LoadFile(rootConfigPath)
	if err != nil {
		fmt.Fprintf(c.Err, "load root config: %v\n", err)
		return exitError
	}
	preset, ok := cfg.Workspace.RepoPresets[presetName]
	if !ok {
		fmt.Fprintf(c.Err, "preset not found: %s\n", presetName)
		return exitError
	}
	printRepoPresetShow(c.Out, presetName, preset.Repos, writerSupportsColor(c.Out))
	return exitOK
}

func (c *CLI) runRepoPresetSession(debugTag string) (repocmd.Session, error) {
	wd, err := os.Getwd()
	if err != nil {
		return repocmd.Session{}, fmt.Errorf("get working dir: %w", err)
	}
	repoUC := repocmd.NewService(appports.NewRepoPort(c.ensureDebugLog, c.touchStateRegistry))
	return repoUC.Run(context.Background(), repocmd.Request{
		CWD:           wd,
		DebugTag:      debugTag,
		TouchRegistry: true,
	})
}

func (c *CLI) confirmRepoPresetOverwrite(force bool) (bool, error) {
	if force {
		return true, nil
	}
	if inputIsTTY(c.In) {
		line, err := c.promptLine("preset already exists. overwrite? [y/N]: ")
		if err != nil {
			return false, fmt.Errorf("read confirmation: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		default:
			return false, nil
		}
	}
	return false, fmt.Errorf("--yes is required to overwrite an existing preset in non-interactive mode")
}

func saveRootConfigFile(path string, cfg config.Config) error {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeAtomic(path, raw, 0o644)
}

func validateRepoPresetName(name string) error {
	if err := validateWorkspaceID(strings.TrimSpace(name)); err != nil {
		return err
	}
	return nil
}

func sortedRepoPresetNames(presets map[string]config.WorkspaceRepoPreset) []string {
	names := make([]string, 0, len(presets))
	for name := range presets {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		names = append(names, trimmed)
	}
	slices.Sort(names)
	return names
}

func normalizeRepoPresetRepos(repos []string) []string {
	out := make([]string, 0, len(repos))
	seen := map[string]bool{}
	for _, raw := range repos {
		repoKey := strings.TrimSpace(raw)
		if repoKey == "" || seen[repoKey] {
			continue
		}
		seen[repoKey] = true
		out = append(out, repoKey)
	}
	return out
}

func inputIsTTY(in io.Reader) bool {
	f, ok := in.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd())
}

func isHelpArg(v string) bool {
	switch strings.TrimSpace(v) {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func printRepoPresetAddResult(out io.Writer, name string, repos []string, replaced bool, useColor bool) {
	bullet := styleMuted("•", useColor)
	check := styleSuccess("✔", useColor)
	verb := "created"
	if replaced {
		verb = "updated"
	}
	body := []string{
		fmt.Sprintf("%s%s %s %s (%s)", uiIndent, bullet, check, name, verb),
		fmt.Sprintf("%s%s repos: %d", uiIndent, bullet, len(repos)),
	}
	for _, repoKey := range repos {
		body = append(body, fmt.Sprintf("%s%s - %s", uiIndent, bullet, repoKey))
	}
	printSection(out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func printRepoPresetRemoveResult(out io.Writer, name string, useColor bool) {
	bullet := styleMuted("•", useColor)
	check := styleSuccess("✔", useColor)
	body := []string{
		fmt.Sprintf("%s%s %s removed %s", uiIndent, bullet, check, name),
	}
	printSection(out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func printRepoPresetList(out io.Writer, names []string, useColor bool) {
	bullet := styleMuted("•", useColor)
	body := make([]string, 0, len(names)+1)
	if len(names) == 0 {
		body = append(body, fmt.Sprintf("%s%s (none)", uiIndent, bullet))
	} else {
		for _, name := range names {
			body = append(body, fmt.Sprintf("%s%s %s", uiIndent, bullet, name))
		}
	}
	printSection(out, styleBold("Presets:", useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func printRepoPresetShow(out io.Writer, name string, repos []string, useColor bool) {
	bullet := styleMuted("•", useColor)
	label := styleAccent("repos", useColor)
	body := []string{
		fmt.Sprintf("%s%s name: %s", uiIndent, bullet, name),
		fmt.Sprintf("%s%s %s:", uiIndent, bullet, label),
	}
	for _, repoKey := range repos {
		body = append(body, fmt.Sprintf("%s- %s", uiIndent+uiIndent, repoKey))
	}
	printSection(out, styleBold("Preset:", useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}
