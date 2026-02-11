package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/tasuku43/gionx/internal/app/wsimport"
	"github.com/tasuku43/gionx/internal/infra/appports"
	"github.com/tasuku43/gionx/internal/infra/paths"
)

const (
	wsImportJiraDefaultLimit = 30
	wsImportJiraMinLimit     = 1
	wsImportJiraMaxLimit     = 200
)

type wsImportJiraOpts struct {
	sprintValue string
	sprintSet   bool
	jql         string
	board       string
	spaceKey    string
	limit       int
	apply       bool
	noPrompt    bool
	jsonOutput  bool
}

type wsImportJiraPlan struct {
	Source  wsImportJiraSource  `json:"source"`
	Filters wsImportJiraFilters `json:"filters"`
	Summary wsImportJiraSummary `json:"summary"`
	Items   []wsImportJiraItem  `json:"items"`
}

type wsImportJiraSource struct {
	Type   string `json:"type"`
	Mode   string `json:"mode"`
	Board  string `json:"board,omitempty"`
	Sprint string `json:"sprint,omitempty"`
	JQL    string `json:"jql,omitempty"`
}

type wsImportJiraFilters struct {
	Assignee       string `json:"assignee"`
	StatusCategory string `json:"statusCategory"`
	Limit          int    `json:"limit"`
}

type wsImportJiraSummary struct {
	Candidates int `json:"candidates"`
	ToCreate   int `json:"to_create"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
}

type wsImportJiraItem struct {
	IssueKey    string `json:"issue_key,omitempty"`
	Title       string `json:"title,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Action      string `json:"action"`
	Reason      string `json:"reason,omitempty"`
	Message     string `json:"message,omitempty"`
}

type wsImportJiraSprintCandidate struct {
	ID        int
	Name      string
	State     string
	BoardID   int
	BoardName string
}

func (c *CLI) runWSImportJira(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSImportJiraUsage(c.Out)
			return exitOK
		}
	}

	opts, err := c.parseWSImportJiraOpts(args)
	if err != nil {
		fmt.Fprintln(c.Err, err.Error())
		c.printWSImportJiraUsage(c.Err)
		return exitUsage
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve GIONX_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "ws-import-jira"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws import jira args=%q", args)

	ctx := context.Background()
	svc := wsimport.NewService(appports.NewWSImportJiraPort())
	jql := ""
	source := wsImportJiraSource{Type: "jira", Mode: "jql"}
	if opts.sprintSet {
		sprintQueryValue := opts.sprintValue
		sprintDisplayValue := opts.sprintValue
		if strings.TrimSpace(sprintQueryValue) == "" {
			chosen, err := c.selectWSImportJiraSprintFromSpace(ctx, svc, opts.spaceKey)
			if err != nil {
				fmt.Fprintf(c.Err, "resolve sprint: %v\n", err)
				return exitUsage
			}
			sprintQueryValue = strconv.Itoa(chosen.ID)
			sprintDisplayValue = chosen.Name
		}
		jql = buildWSImportJiraSprintJQL(opts.spaceKey, sprintQueryValue)
		source.Mode = "sprint"
		source.Sprint = sprintDisplayValue
	} else {
		jql = strings.TrimSpace(opts.jql)
		source.JQL = jql
	}
	c.debugf("ws import jira: resolved mode=%s jql=%q", source.Mode, jql)

	inputs, err := svc.ResolveWorkspaceInputsByJQL(ctx, jql, opts.limit)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve jira issues: %v\n", err)
		return exitError
	}

	plan, createInputs := buildWSImportJiraPlan(source, opts.limit, root, inputs)

	shouldApply := false
	interactivePromptFlow := !opts.noPrompt && !opts.apply
	if opts.noPrompt {
		shouldApply = opts.apply
	} else if opts.apply {
		shouldApply = true
	} else {
		if opts.jsonOutput {
			if err := c.printWSImportJiraPlanJSON(plan); err != nil {
				fmt.Fprintf(c.Err, "write json: %v\n", err)
				return exitError
			}
			confirm, err := c.promptLine(renderWSImportJiraApplyPrompt(writerSupportsColor(c.Err)))
			if err != nil {
				fmt.Fprintf(c.Err, "read apply confirmation: %v\n", err)
				return exitError
			}
			confirm = strings.ToLower(strings.TrimSpace(confirm))
			shouldApply = confirm == "" || confirm == "y" || confirm == "yes"
		} else {
			c.printWSImportJiraPlanHuman(plan)
			confirm, err := c.promptWSImportJiraApplyOnOut()
			if err != nil {
				fmt.Fprintf(c.Err, "read apply confirmation: %v\n", err)
				return exitError
			}
			confirm = strings.ToLower(strings.TrimSpace(confirm))
			shouldApply = confirm == "" || confirm == "y" || confirm == "yes"
		}
		if !shouldApply {
			if plan.Summary.Failed > 0 {
				return exitError
			}
			return exitOK
		}
	}
	if shouldApply {
		createdCount := 0
		for _, in := range createInputs {
			if _, err := c.createWorkspaceAtRoot(root, in.ID, in.Title, in.SourceURL); err != nil {
				markWSImportJiraCreateItemAsFailed(&plan, in, classifyWSImportJiraCreateFailureReason(err), err.Error())
				plan.Summary.Failed++
				continue
			}
			createdCount++
		}
		plan.Summary.ToCreate = createdCount
	}

	if opts.jsonOutput {
		if err := c.printWSImportJiraPlanJSON(plan); err != nil {
			fmt.Fprintf(c.Err, "write json: %v\n", err)
			return exitError
		}
		if plan.Summary.Failed > 0 {
			return exitError
		}
		return exitOK
	}

	if shouldApply {
		c.printWSImportJiraResultHuman(plan)
		if plan.Summary.Failed > 0 {
			return exitError
		}
		return exitOK
	}

	if !interactivePromptFlow {
		c.printWSImportJiraPlanHuman(plan)
	}

	if plan.Summary.Failed > 0 {
		return exitError
	}
	return exitOK
}

func (c *CLI) parseWSImportJiraOpts(args []string) (wsImportJiraOpts, error) {
	opts := wsImportJiraOpts{limit: wsImportJiraDefaultLimit}
	rest := args

	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		switch rest[0] {
		case "--sprint":
			opts.sprintSet = true
			if len(rest) > 1 && !strings.HasPrefix(rest[1], "-") {
				opts.sprintValue = strings.TrimSpace(rest[1])
				rest = rest[2:]
				continue
			}
			rest = rest[1:]
		case "--jql":
			if len(rest) < 2 {
				return wsImportJiraOpts{}, fmt.Errorf("--jql requires a value")
			}
			opts.jql = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case "--board":
			if len(rest) < 2 {
				return wsImportJiraOpts{}, fmt.Errorf("--board requires a value")
			}
			opts.board = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case "--space":
			if len(rest) < 2 {
				return wsImportJiraOpts{}, fmt.Errorf("--space requires a value")
			}
			if strings.TrimSpace(opts.spaceKey) != "" {
				return wsImportJiraOpts{}, fmt.Errorf("--space and --project cannot be combined")
			}
			opts.spaceKey = strings.ToUpper(strings.TrimSpace(rest[1]))
			rest = rest[2:]
		case "--project":
			if len(rest) < 2 {
				return wsImportJiraOpts{}, fmt.Errorf("--project requires a value")
			}
			if strings.TrimSpace(opts.spaceKey) != "" {
				return wsImportJiraOpts{}, fmt.Errorf("--space and --project cannot be combined")
			}
			opts.spaceKey = strings.ToUpper(strings.TrimSpace(rest[1]))
			rest = rest[2:]
		case "--limit":
			if len(rest) < 2 {
				return wsImportJiraOpts{}, fmt.Errorf("--limit requires a value")
			}
			n, err := strconv.Atoi(strings.TrimSpace(rest[1]))
			if err != nil {
				return wsImportJiraOpts{}, fmt.Errorf("invalid --limit: %q", rest[1])
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
			opts.jsonOutput = true
			rest = rest[1:]
		default:
			return wsImportJiraOpts{}, fmt.Errorf("unknown flag for ws import jira: %q", rest[0])
		}
	}

	if len(rest) > 0 {
		return wsImportJiraOpts{}, fmt.Errorf("unexpected args for ws import jira: %q", strings.Join(rest, " "))
	}
	if opts.sprintSet && opts.jql != "" {
		return wsImportJiraOpts{}, fmt.Errorf("--sprint and --jql cannot be combined")
	}
	if !opts.sprintSet && opts.jql == "" {
		return wsImportJiraOpts{}, fmt.Errorf("one of --sprint or --jql is required")
	}
	if !opts.sprintSet && opts.board != "" {
		return wsImportJiraOpts{}, fmt.Errorf("--board is only valid with --sprint")
	}
	if opts.sprintSet && opts.board != "" {
		return wsImportJiraOpts{}, fmt.Errorf("--board is not supported with --sprint; use --space/--project")
	}
	if !opts.sprintSet && opts.spaceKey != "" {
		return wsImportJiraOpts{}, fmt.Errorf("--space/--project is only valid with --sprint")
	}
	if opts.sprintSet && strings.TrimSpace(opts.spaceKey) == "" {
		return wsImportJiraOpts{}, fmt.Errorf("--sprint requires --space (or --project)")
	}
	if opts.limit < wsImportJiraMinLimit || opts.limit > wsImportJiraMaxLimit {
		return wsImportJiraOpts{}, fmt.Errorf("--limit must be in range %d..%d", wsImportJiraMinLimit, wsImportJiraMaxLimit)
	}
	if opts.sprintSet && strings.TrimSpace(opts.sprintValue) == "" {
		if opts.noPrompt {
			return wsImportJiraOpts{}, fmt.Errorf("--no-prompt requires --sprint <id|name> or --jql")
		}
	}
	if opts.jql != "" && strings.TrimSpace(opts.jql) == "" {
		return wsImportJiraOpts{}, fmt.Errorf("--jql requires a non-empty value")
	}

	return opts, nil
}

func buildWSImportJiraPlan(source wsImportJiraSource, limit int, root string, inputs []wsimport.WorkspaceInput) (wsImportJiraPlan, []wsimport.WorkspaceInput) {
	plan := wsImportJiraPlan{
		Source: source,
		Filters: wsImportJiraFilters{
			Assignee:       "currentUser()",
			StatusCategory: "not_done",
			Limit:          limit,
		},
		Summary: wsImportJiraSummary{
			Candidates: len(inputs),
			ToCreate:   0,
			Skipped:    0,
			Failed:     0,
		},
		Items: make([]wsImportJiraItem, 0, len(inputs)),
	}

	createInputs := make([]wsimport.WorkspaceInput, 0, len(inputs))
	for _, in := range inputs {
		if err := validateWorkspaceID(in.ID); err != nil {
			plan.Items = append(plan.Items, wsImportJiraItem{
				IssueKey:    in.ID,
				Title:       in.Title,
				WorkspaceID: in.ID,
				Action:      "fail",
				Reason:      "invalid_workspace_id",
				Message:     err.Error(),
			})
			plan.Summary.Failed++
			continue
		}
		activePath := filepath.Join(root, "workspaces", in.ID)
		if _, err := os.Stat(activePath); err == nil {
			plan.Items = append(plan.Items, wsImportJiraItem{
				IssueKey:    in.ID,
				Title:       in.Title,
				WorkspaceID: in.ID,
				Action:      "skip",
				Reason:      "already_active",
			})
			plan.Summary.Skipped++
			continue
		}
		archivedPath := filepath.Join(root, "archive", in.ID)
		if _, err := os.Stat(archivedPath); err == nil {
			plan.Items = append(plan.Items, wsImportJiraItem{
				IssueKey:    in.ID,
				Title:       in.Title,
				WorkspaceID: in.ID,
				Action:      "skip",
				Reason:      "archived_exists",
			})
			plan.Summary.Skipped++
			continue
		}
		plan.Items = append(plan.Items, wsImportJiraItem{
			IssueKey:    in.ID,
			Title:       in.Title,
			WorkspaceID: in.ID,
			Action:      "create",
		})
		createInputs = append(createInputs, in)
		plan.Summary.ToCreate++
	}
	return plan, createInputs
}

func buildWSImportJiraSprintJQL(spaceKey string, sprintValue string) string {
	key := strings.ToUpper(strings.TrimSpace(spaceKey))
	if isDigitsOnly(strings.TrimSpace(sprintValue)) {
		return fmt.Sprintf("project = %s AND sprint = %s AND assignee = currentUser() AND statusCategory != Done ORDER BY Rank ASC", key, strings.TrimSpace(sprintValue))
	}
	escaped := strings.ReplaceAll(strings.TrimSpace(sprintValue), `"`, `\"`)
	return fmt.Sprintf(`project = %s AND sprint = "%s" AND assignee = currentUser() AND statusCategory != Done ORDER BY Rank ASC`, key, escaped)
}

func isDigitsOnly(v string) bool {
	if strings.TrimSpace(v) == "" {
		return false
	}
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (c *CLI) selectWSImportJiraSprintFromSpace(ctx context.Context, svc *wsimport.Service, spaceKey string) (wsImportJiraSprintCandidate, error) {
	sprints, err := svc.ListProjectOpenSprints(ctx, spaceKey, 200)
	if err != nil {
		return wsImportJiraSprintCandidate{}, err
	}
	c.debugf("ws import jira: space=%q sprint candidates by jql=%d", spaceKey, len(sprints))

	candidates := make([]wsImportJiraSprintCandidate, 0, len(sprints))
	seen := map[int]struct{}{}
	for _, s := range sprints {
		state := strings.ToLower(strings.TrimSpace(s.State))
		if state != "active" && state != "future" {
			continue
		}
		if _, ok := seen[s.ID]; ok {
			continue
		}
		seen[s.ID] = struct{}{}
		candidates = append(candidates, wsImportJiraSprintCandidate{
			ID:        s.ID,
			Name:      s.Name,
			State:     state,
			BoardID:   s.OriginBoardID,
			BoardName: "",
		})
	}
	if len(candidates) == 0 {
		return wsImportJiraSprintCandidate{}, fmt.Errorf("no active/future sprints found for --space %q", spaceKey)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		priority := func(state string) int {
			switch strings.ToLower(strings.TrimSpace(state)) {
			case "active":
				return 0
			case "future":
				return 1
			default:
				return 9
			}
		}
		pi, pj := priority(candidates[i].State), priority(candidates[j].State)
		if pi != pj {
			return pi < pj
		}
		if candidates[i].ID != candidates[j].ID {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].Name < candidates[j].Name
	})

	if selected, ok, err := c.trySelectWSImportJiraSprintWithSelector(candidates); err != nil {
		return wsImportJiraSprintCandidate{}, err
	} else if ok {
		return selected, nil
	}

	labels := make([]string, 0, len(candidates))
	for _, s := range candidates {
		boardLabel := strings.TrimSpace(s.BoardName)
		if boardLabel == "" && s.BoardID > 0 {
			boardLabel = strconv.Itoa(s.BoardID)
		}
		labels = append(labels, fmt.Sprintf("%d: %s (%s / board=%s)", s.ID, s.Name, strings.ToLower(strings.TrimSpace(s.State)), boardLabel))
	}
	idx, err := c.promptWSImportJiraChoice("Select sprint:", labels)
	if err != nil {
		return wsImportJiraSprintCandidate{}, err
	}
	return candidates[idx], nil
}

func (c *CLI) trySelectWSImportJiraSprintWithSelector(candidates []wsImportJiraSprintCandidate) (wsImportJiraSprintCandidate, bool, error) {
	selectorCandidates := make([]workspaceSelectorCandidate, 0, len(candidates))
	byID := make(map[string]wsImportJiraSprintCandidate, len(candidates))
	for _, s := range candidates {
		id := strconv.Itoa(s.ID)
		state := strings.ToLower(strings.TrimSpace(s.State))
		description := fmt.Sprintf("%s (%s)", s.Name, state)
		selectorCandidates = append(selectorCandidates, workspaceSelectorCandidate{
			ID:          id,
			Title:       s.Name,
			Description: description,
		})
		byID[id] = s
	}

	selectedIDs, err := c.promptWorkspaceSelectorWithOptionsAndMode(
		"active",
		"select",
		"Sprints(active/future):",
		"sprint",
		selectorCandidates,
		true,
	)
	if err != nil {
		// Keep non-interactive test/pipe behavior: fallback to numbered prompt.
		if strings.Contains(strings.ToLower(err.Error()), "requires a tty") {
			return wsImportJiraSprintCandidate{}, false, nil
		}
		if err == errSelectorCanceled {
			return wsImportJiraSprintCandidate{}, false, fmt.Errorf("sprint selection canceled")
		}
		return wsImportJiraSprintCandidate{}, false, err
	}
	if len(selectedIDs) == 0 {
		return wsImportJiraSprintCandidate{}, false, fmt.Errorf("sprint selection canceled")
	}
	selected, ok := byID[selectedIDs[0]]
	if !ok {
		return wsImportJiraSprintCandidate{}, false, fmt.Errorf("selected sprint not found: %s", selectedIDs[0])
	}
	return selected, true, nil
}

func (c *CLI) promptWSImportJiraChoice(title string, options []string) (int, error) {
	if len(options) == 0 {
		return 0, fmt.Errorf("no options")
	}
	useColor := writerSupportsColor(c.Out)
	body := make([]string, 0, len(options))
	for i, opt := range options {
		body = append(body, fmt.Sprintf("%s%d) %s", uiIndent, i+1, opt))
	}
	for _, line := range renderSectionAtoms(newSectionAtom(styleBold(title, useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     false,
	})) {
		fmt.Fprintln(c.Out, line)
	}
	for {
		in, err := c.promptLine("choice: ")
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(strings.TrimSpace(in))
		if err != nil || n < 1 || n > len(options) {
			fmt.Fprintln(c.Err, "invalid choice")
			continue
		}
		return n - 1, nil
	}
}

func (c *CLI) printWSImportJiraPlanHuman(plan wsImportJiraPlan) {
	useColor := writerSupportsColor(c.Out)
	bullet := styleMuted("•", useColor)
	connectorMuted := func(connector string) string {
		return styleMuted(connector, useColor)
	}
	styleLabel := func(label string) string {
		return styleAccent(label, useColor)
	}

	sourceLine := ""
	if plan.Source.Mode == "sprint" {
		if strings.TrimSpace(plan.Source.Board) != "" {
			sourceLine = fmt.Sprintf("%s jira mode=sprint sprint=%s board=%s", styleLabel("source:"), plan.Source.Sprint, plan.Source.Board)
		} else {
			sourceLine = fmt.Sprintf("%s jira mode=sprint sprint=%s", styleLabel("source:"), plan.Source.Sprint)
		}
	} else {
		sourceLine = fmt.Sprintf("%s jira mode=jql jql=%s", styleLabel("source:"), plan.Source.JQL)
	}
	filtersLabel := styleLabel("filters:")
	toCreateLabel := styleMuted("to create", useColor)
	if plan.Summary.ToCreate > 0 {
		toCreateLabel = styleInfo("to create", useColor)
	}
	skippedLabel := styleMuted("skipped", useColor)
	if plan.Summary.Skipped > 0 {
		skippedLabel = styleWarn("skipped", useColor)
	}
	failedLabel := styleMuted("failed", useColor)
	if plan.Summary.Failed > 0 {
		failedLabel = styleError("failed", useColor)
	}

	body := []string{
		fmt.Sprintf("%s%s %s", uiIndent, bullet, sourceLine),
		fmt.Sprintf("%s%s %s assignee=%s statusCategory!=Done limit=%d", uiIndent, bullet, filtersLabel, plan.Filters.Assignee, plan.Filters.Limit),
	}
	body = append(body, fmt.Sprintf("%s%s %s (%d)", uiIndent, bullet, toCreateLabel, plan.Summary.ToCreate))
	body = append(body, renderWSImportJiraPlanItems(plan.Items, "create", connectorMuted)...)
	body = append(body, fmt.Sprintf("%s%s %s (%d)", uiIndent, bullet, skippedLabel, plan.Summary.Skipped))
	body = append(body, renderWSImportJiraPlanItems(plan.Items, "skip", connectorMuted)...)
	body = append(body, fmt.Sprintf("%s%s %s (%d)", uiIndent, bullet, failedLabel, plan.Summary.Failed))
	body = append(body, renderWSImportJiraPlanItems(plan.Items, "fail", connectorMuted)...)

	for _, line := range renderSectionAtoms(newSectionAtom(styleBold("Plan:", useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})) {
		fmt.Fprintln(c.Out, line)
	}
}

func (c *CLI) printWSImportJiraResultHuman(plan wsImportJiraPlan) {
	useColor := writerSupportsColor(c.Out)
	bullet := styleMuted("•", useColor)

	createStat := styleMuted(fmt.Sprintf("create=%d", plan.Summary.ToCreate), useColor)
	if plan.Summary.ToCreate > 0 {
		createStat = styleInfo(fmt.Sprintf("create=%d", plan.Summary.ToCreate), useColor)
	}
	skippedStat := styleMuted(fmt.Sprintf("skipped=%d", plan.Summary.Skipped), useColor)
	if plan.Summary.Skipped > 0 {
		skippedStat = styleWarn(fmt.Sprintf("skipped=%d", plan.Summary.Skipped), useColor)
	}
	failedStat := styleMuted(fmt.Sprintf("failed=%d", plan.Summary.Failed), useColor)
	if plan.Summary.Failed > 0 {
		failedStat = styleError(fmt.Sprintf("failed=%d", plan.Summary.Failed), useColor)
	}

	resultLine := styleSuccess("import completed", useColor)
	if plan.Summary.Failed > 0 {
		resultLine = styleWarn("import completed with failures", useColor)
	}

	body := []string{
		fmt.Sprintf("%s%s %s %s %s", uiIndent, bullet, createStat, skippedStat, failedStat),
		fmt.Sprintf("%s%s %s", uiIndent, bullet, resultLine),
	}
	printSection(c.Out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func (c *CLI) printWSImportJiraPlanJSON(plan wsImportJiraPlan) error {
	enc := json.NewEncoder(c.Out)
	enc.SetEscapeHTML(false)
	return enc.Encode(plan)
}

func classifyWSImportJiraCreateFailureReason(err error) string {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "permission denied"):
		return "permission_denied"
	case strings.Contains(msg, "not found"),
		strings.Contains(msg, "no such file or directory"):
		return "not_found"
	default:
		return "create_failed"
	}
}

func renderWSImportJiraPlanItems(items []wsImportJiraItem, action string, connectorMuted func(string) string) []string {
	group := make([]wsImportJiraItem, 0, len(items))
	for _, it := range items {
		if it.Action != action {
			continue
		}
		group = append(group, it)
	}
	if len(group) == 0 {
		return nil
	}
	lines := make([]string, 0, len(group))
	for i, it := range group {
		connector := "├─ "
		if i == len(group)-1 {
			connector = "└─ "
		}
		lines = append(lines, fmt.Sprintf("%s%s%s", uiIndent+uiIndent, connectorMuted(connector), renderWSImportJiraPlanItemLabel(it)))
	}
	return lines
}

func renderWSImportJiraPlanItemLabel(it wsImportJiraItem) string {
	base := fmt.Sprintf("%s: %s", strings.TrimSpace(it.IssueKey), formatWorkspaceTitle(it.Title))
	switch it.Action {
	case "skip":
		reason := strings.TrimSpace(it.Reason)
		if reason == "" || strings.EqualFold(reason, "already_active") {
			return base
		}
		return fmt.Sprintf("%s (%s)", base, reason)
	case "fail":
		msg := strings.TrimSpace(it.Message)
		if msg == "" {
			return fmt.Sprintf("%s (%s)", base, strings.TrimSpace(it.Reason))
		}
		return fmt.Sprintf("%s (%s: %s)", base, strings.TrimSpace(it.Reason), msg)
	default:
		return base
	}
}

func renderWSImportJiraApplyPrompt(useColor bool) string {
	bullet := styleMuted("•", useColor)
	guide := styleMuted("[Enter=yes / n=no]", useColor)
	return fmt.Sprintf("%s%s apply this plan? %s: ", uiIndent, bullet, guide)
}

func (c *CLI) promptWSImportJiraApplyOnOut() (string, error) {
	prompt := renderWSImportJiraApplyPrompt(writerSupportsColor(c.Out))

	inFile, inOK := c.In.(*os.File)
	outFile, outOK := c.Out.(*os.File)
	if inOK && outOK && isatty.IsTerminal(inFile.Fd()) && isatty.IsTerminal(outFile.Fd()) {
		return runInlineTextInput(inFile, c.Out, prompt)
	}

	if prompt != "" {
		fmt.Fprint(c.Out, prompt)
	}
	if c.inReader == nil {
		c.inReader = bufio.NewReader(c.In)
	}
	line, err := c.inReader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	fmt.Fprintln(c.Out)
	return strings.TrimSpace(line), nil
}

func markWSImportJiraCreateItemAsFailed(plan *wsImportJiraPlan, in wsimport.WorkspaceInput, reason string, message string) {
	for i := range plan.Items {
		if plan.Items[i].IssueKey != in.ID {
			continue
		}
		if plan.Items[i].Action != "create" {
			continue
		}
		plan.Items[i].Action = "fail"
		plan.Items[i].Reason = reason
		plan.Items[i].Message = message
		return
	}
	plan.Items = append(plan.Items, wsImportJiraItem{
		IssueKey:    in.ID,
		Title:       in.Title,
		WorkspaceID: in.ID,
		Action:      "fail",
		Reason:      reason,
		Message:     message,
	})
}
