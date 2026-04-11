package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/tasuku43/kra/internal/app/wsimport"
	"github.com/tasuku43/kra/internal/config"
)

type wsImportAllOpts struct {
	target       string
	limit        int
	apply        bool
	noPrompt     bool
	outputFormat string
}

type wsImportAllPlan struct {
	Targets []string            `json:"targets"`
	Summary wsImportAllSummary  `json:"summary"`
	Jira    *wsImportJiraPlan   `json:"jira,omitempty"`
	GitHub  *wsImportGitHubPlan `json:"github_review,omitempty"`
}

type wsImportAllSummary struct {
	Candidates int `json:"candidates"`
	ToCreate   int `json:"to_create"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
}

func (c *CLI) runWSImportAll(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSImportAllUsage(c.Out)
			return exitOK
		}
	}

	opts, err := parseWSImportAllOpts(args)
	if err != nil {
		return c.writeWSImportAllUsageError(wsImportAllWantsJSON(args), err.Error())
	}
	outputJSON := opts.outputFormat == "json"

	root, cfg, runtimeErrCode, runtimeErrMessage, usageExit := c.prepareWSImportGitHubRuntime("ws-import-all")
	if runtimeErrMessage != "" {
		return c.writeWSImportAllRuntimeError(outputJSON, runtimeErrCode, runtimeErrMessage, usageExit)
	}
	opts, err = applyWSImportAllConfigDefaults(opts, cfg)
	if err != nil {
		return c.writeWSImportAllUsageError(outputJSON, err.Error())
	}

	ctx := context.Background()
	plan := wsImportAllPlan{Targets: wsImportAllTargets(opts.target)}
	var jiraCreateInputs []wsimport.WorkspaceInput
	var githubCreateInputs []wsimport.GitHubWorkspaceInput
	type planResult struct {
		jiraPlan     *wsImportJiraPlan
		jiraInputs   []wsimport.WorkspaceInput
		githubPlan   *wsImportGitHubPlan
		githubInputs []wsimport.GitHubWorkspaceInput
		usageErr     string
		runtimeErr   string
	}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []planResult
	)
	run := func(fn func() planResult) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := fn()
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}()
	}
	if opts.target == config.ImportTargetJira || opts.target == config.ImportTargetBoth {
		run(func() planResult {
			jiraPlan, createInputs, err := c.prepareWSImportJiraPlan(ctx, root, cfg, wsImportJiraOpts{
				limit:        opts.limit,
				apply:        opts.apply,
				noPrompt:     opts.noPrompt,
				outputFormat: opts.outputFormat,
			})
			if err != nil {
				msg := err.Error()
				if strings.HasPrefix(msg, "usage: ") {
					return planResult{usageErr: strings.TrimPrefix(msg, "usage: ")}
				}
				return planResult{runtimeErr: msg}
			}
			return planResult{jiraPlan: &jiraPlan, jiraInputs: createInputs}
		})
	}
	if opts.target == config.ImportTargetGitHubReview || opts.target == config.ImportTargetBoth {
		run(func() planResult {
			githubOpts, err := applyWSImportGitHubReviewConfigDefaults(wsImportGitHubOpts{
				limit:        opts.limit,
				apply:        opts.apply,
				noPrompt:     opts.noPrompt,
				outputFormat: opts.outputFormat,
			}, cfg)
			if err != nil {
				return planResult{usageErr: err.Error()}
			}
			githubPlan, createInputs, err := c.prepareWSImportGitHubReviewPlan(ctx, root, cfg, githubOpts)
			if err != nil {
				return planResult{runtimeErr: err.Error()}
			}
			return planResult{githubPlan: &githubPlan, githubInputs: createInputs}
		})
	}
	wg.Wait()
	for _, result := range results {
		if result.usageErr != "" {
			return c.writeWSImportAllUsageError(outputJSON, result.usageErr)
		}
		if result.runtimeErr != "" {
			return c.writeWSImportAllRuntimeError(outputJSON, "internal_error", result.runtimeErr, exitError)
		}
		if result.jiraPlan != nil {
			plan.Jira = result.jiraPlan
			jiraCreateInputs = result.jiraInputs
		}
		if result.githubPlan != nil {
			plan.GitHub = result.githubPlan
			githubCreateInputs = result.githubInputs
		}
	}
	plan.Summary = summarizeWSImportAllPlan(plan)

	shouldApply := false
	interactivePromptFlow := !opts.noPrompt && !opts.apply
	if opts.noPrompt {
		shouldApply = opts.apply
	} else if opts.apply {
		shouldApply = true
	} else {
		if outputJSON {
			confirm, err := c.promptLine(renderWSImportJiraApplyPrompt(writerSupportsColor(c.Err)))
			if err != nil {
				return c.writeWSImportAllRuntimeError(true, "internal_error", fmt.Sprintf("read apply confirmation: %v", err), exitError)
			}
			confirm = strings.ToLower(strings.TrimSpace(confirm))
			shouldApply = confirm == "" || confirm == "y" || confirm == "yes"
		} else {
			c.printWSImportAllPlanHuman(plan)
			confirm, err := c.promptWSImportJiraApplyOnOut()
			if err != nil {
				return c.writeWSImportAllRuntimeError(false, "internal_error", fmt.Sprintf("read apply confirmation: %v", err), exitError)
			}
			confirm = strings.ToLower(strings.TrimSpace(confirm))
			shouldApply = confirm == "" || confirm == "y" || confirm == "yes"
		}
		if !shouldApply {
			if outputJSON {
				return c.writeWSImportAllJSONResult(plan, false, plan.Summary.Failed == 0, "conflict", "one or more import items failed in plan")
			}
			if plan.Summary.Failed > 0 {
				return exitError
			}
			return exitOK
		}
	}

	if shouldApply {
		if plan.Jira != nil {
			c.applyWSImportJiraPlan(root, plan.Jira, jiraCreateInputs)
		}
		if plan.GitHub != nil {
			c.applyWSImportGitHubPlan(ctx, root, cfg, plan.GitHub, githubCreateInputs)
		}
		plan.Summary = summarizeWSImportAllPlan(plan)
	}

	if outputJSON {
		if plan.Summary.Failed > 0 {
			return c.writeWSImportAllJSONResult(plan, shouldApply, false, "conflict", "import completed with failures")
		}
		return c.writeWSImportAllJSONResult(plan, shouldApply, true, "", "")
	}
	if shouldApply {
		c.printWSImportAllResultHuman(plan)
		if plan.Summary.Failed > 0 {
			return exitError
		}
		return exitOK
	}
	if !interactivePromptFlow {
		c.printWSImportAllPlanHuman(plan)
	}
	if plan.Summary.Failed > 0 {
		return exitError
	}
	return exitOK
}

func parseWSImportAllOpts(args []string) (wsImportAllOpts, error) {
	opts := wsImportAllOpts{
		limit:        wsImportJiraDefaultLimit,
		outputFormat: "human",
	}
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		switch rest[0] {
		case "--target":
			if len(rest) < 2 {
				return wsImportAllOpts{}, fmt.Errorf("--target requires a value")
			}
			opts.target = strings.ToLower(strings.TrimSpace(rest[1]))
			rest = rest[2:]
		case "--limit":
			if len(rest) < 2 {
				return wsImportAllOpts{}, fmt.Errorf("--limit requires a value")
			}
			n, err := strconv.Atoi(strings.TrimSpace(rest[1]))
			if err != nil {
				return wsImportAllOpts{}, fmt.Errorf("invalid --limit: %q", rest[1])
			}
			opts.limit = n
			rest = rest[2:]
		case "--apply":
			opts.apply = true
			rest = rest[1:]
		case "--no-prompt":
			opts.noPrompt = true
			rest = rest[1:]
		case "--json":
			opts.outputFormat = "json"
			rest = rest[1:]
		case "--format":
			if len(rest) < 2 {
				return wsImportAllOpts{}, fmt.Errorf("--format requires a value")
			}
			opts.outputFormat = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			if strings.HasPrefix(rest[0], "--format=") {
				opts.outputFormat = strings.TrimSpace(strings.TrimPrefix(rest[0], "--format="))
				rest = rest[1:]
				continue
			}
			if strings.HasPrefix(rest[0], "--target=") {
				opts.target = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(rest[0], "--target=")))
				rest = rest[1:]
				continue
			}
			return wsImportAllOpts{}, fmt.Errorf("unknown flag for ws import all: %q", rest[0])
		}
	}
	if len(rest) > 0 {
		return wsImportAllOpts{}, fmt.Errorf("unexpected args for ws import all: %q", strings.Join(rest, " "))
	}
	if opts.limit < wsImportJiraMinLimit || opts.limit > wsImportJiraMaxLimit {
		return wsImportAllOpts{}, fmt.Errorf("--limit must be in range %d..%d", wsImportJiraMinLimit, wsImportJiraMaxLimit)
	}
	switch opts.outputFormat {
	case "human", "json":
	default:
		return wsImportAllOpts{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.outputFormat)
	}
	return opts, nil
}

func wsImportAllWantsJSON(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--json":
			return true
		case strings.HasPrefix(arg, "--format="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "--format=")) == "json"
		case arg == "--format" && i+1 < len(args):
			return strings.TrimSpace(args[i+1]) == "json"
		}
	}
	return false
}

func applyWSImportAllConfigDefaults(opts wsImportAllOpts, cfg config.Config) (wsImportAllOpts, error) {
	out := opts
	if strings.TrimSpace(out.target) == "" {
		out.target = firstNonEmpty(cfg.Integration.Import.Defaults.Target, config.ImportTargetBoth)
	}
	switch out.target {
	case config.ImportTargetJira, config.ImportTargetGitHubReview, config.ImportTargetBoth:
		return out, nil
	default:
		return wsImportAllOpts{}, fmt.Errorf("integration.import.defaults.target must be one of: jira, github-review, both")
	}
}

func wsImportAllTargets(target string) []string {
	switch target {
	case config.ImportTargetJira:
		return []string{config.ImportTargetJira}
	case config.ImportTargetGitHubReview:
		return []string{config.ImportTargetGitHubReview}
	default:
		return []string{config.ImportTargetJira, config.ImportTargetGitHubReview}
	}
}

func summarizeWSImportAllPlan(plan wsImportAllPlan) wsImportAllSummary {
	var summary wsImportAllSummary
	if plan.Jira != nil {
		summary.Candidates += plan.Jira.Summary.Candidates
		summary.ToCreate += plan.Jira.Summary.ToCreate
		summary.Skipped += plan.Jira.Summary.Skipped
		summary.Failed += plan.Jira.Summary.Failed
	}
	if plan.GitHub != nil {
		summary.Candidates += plan.GitHub.Summary.Candidates
		summary.ToCreate += plan.GitHub.Summary.ToCreate
		summary.Skipped += plan.GitHub.Summary.Skipped
		summary.Failed += plan.GitHub.Summary.Failed
	}
	return summary
}

func (c *CLI) writeWSImportAllUsageError(outputJSON bool, message string) int {
	if outputJSON {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: "ws.import.all",
			Error:  &cliJSONError{Code: "invalid_argument", Message: message},
		})
		return exitUsage
	}
	fmt.Fprintln(c.Err, message)
	c.printWSImportAllUsage(c.Err)
	return exitUsage
}

func (c *CLI) writeWSImportAllRuntimeError(outputJSON bool, code string, message string, exitCode int) int {
	if outputJSON {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: "ws.import.all",
			Error:  &cliJSONError{Code: code, Message: message},
		})
		return exitCode
	}
	fmt.Fprintln(c.Err, message)
	return exitCode
}

func (c *CLI) writeWSImportAllJSONResult(plan wsImportAllPlan, applied bool, ok bool, code string, message string) int {
	resp := cliJSONResponse{
		OK:     ok,
		Action: "ws.import.all",
		Result: map[string]any{
			"targets":       plan.Targets,
			"summary":       plan.Summary,
			"applied":       applied,
			"jira":          plan.Jira,
			"github_review": plan.GitHub,
		},
	}
	if !ok {
		resp.Error = &cliJSONError{Code: code, Message: message}
	}
	_ = writeCLIJSON(c.Out, resp)
	if ok {
		return exitOK
	}
	return exitError
}

func (c *CLI) printWSImportAllPlanHuman(plan wsImportAllPlan) {
	useColor := writerSupportsColor(c.Out)
	bullet := styleMuted("•", useColor)
	body := []string{
		fmt.Sprintf("%s%s %s %s", uiIndent, bullet, styleAccent("targets:", useColor), strings.Join(plan.Targets, ", ")),
		fmt.Sprintf("%s%s %s (%d)", uiIndent, bullet, styleInfo("to create", useColor), plan.Summary.ToCreate),
	}
	body = append(body, renderWSImportAllCreateItems(plan, useColor)...)
	body = append(body, fmt.Sprintf("%s%s %s (%d)", uiIndent, bullet, styleWarn("skipped", useColor), plan.Summary.Skipped))
	body = append(body, renderWSImportAllNonCreateItems(plan, "skip", useColor)...)
	body = append(body, fmt.Sprintf("%s%s %s (%d)", uiIndent, bullet, styleError("failed", useColor), plan.Summary.Failed))
	body = append(body, renderWSImportAllNonCreateItems(plan, "fail", useColor)...)
	for _, line := range renderSectionAtoms(newSectionAtom(styleBold("Plan:", useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})) {
		fmt.Fprintln(c.Out, line)
	}
}

func (c *CLI) printWSImportAllResultHuman(plan wsImportAllPlan) {
	useColor := writerSupportsColor(c.Out)
	body := []string{
		fmt.Sprintf("%s%s %s %s %s", uiIndent, styleMuted("•", useColor), styleInfo(fmt.Sprintf("create=%d", plan.Summary.ToCreate), useColor), styleWarn(fmt.Sprintf("skipped=%d", plan.Summary.Skipped), useColor), styleError(fmt.Sprintf("failed=%d", plan.Summary.Failed), useColor)),
	}
	resultLine := styleSuccess("import completed", useColor)
	if plan.Summary.Failed > 0 {
		resultLine = styleWarn("import completed with failures", useColor)
	}
	body = append(body, fmt.Sprintf("%s%s %s", uiIndent, styleMuted("•", useColor), resultLine))
	printSection(c.Out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func renderWSImportAllCreateItems(plan wsImportAllPlan, useColor bool) []string {
	return renderWSImportAllItems(plan, "create", useColor)
}

func renderWSImportAllNonCreateItems(plan wsImportAllPlan, action string, useColor bool) []string {
	return renderWSImportAllItems(plan, action, useColor)
}

func renderWSImportAllItems(plan wsImportAllPlan, action string, useColor bool) []string {
	labels := make([]string, 0)
	if plan.Jira != nil {
		for _, it := range plan.Jira.Items {
			if it.Action != action {
				continue
			}
			labels = append(labels, "[jira] "+renderWSImportJiraPlanItemLabel(it))
		}
	}
	if plan.GitHub != nil {
		for _, it := range plan.GitHub.Items {
			if it.Action != action {
				continue
			}
			labels = append(labels, "[github-review] "+renderWSImportGitHubPlanItemLabel(it))
		}
	}
	if len(labels) == 0 {
		return nil
	}
	lines := make([]string, 0, len(labels))
	for i, label := range labels {
		connector := "├─ "
		if i == len(labels)-1 {
			connector = "└─ "
		}
		lines = append(lines, fmt.Sprintf("%s%s%s", uiIndent+uiIndent, styleMuted(connector, useColor), label))
	}
	return lines
}
