package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/tasuku43/kra/internal/infra/paths"
)

type agentBoardOptions struct {
	format      string
	workspaceID string
	state       string
	location    string
	kind        string
	all         bool
	sessionID   string
	action      string
	noSelect    bool
}

func (c *CLI) runAgentBoard(args []string) int {
	opts, err := parseAgentBoardOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printAgentBoardUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printAgentBoardUsage(c.Err)
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

	records, err := loadAgentRuntimeSessionsPreferBroker(root)
	if err != nil {
		fmt.Fprintf(c.Err, "load agent runtime sessions: %v\n", err)
		return exitError
	}
	records = filterAgentRuntimeSessions(records, agentRuntimeQueryOptions{
		workspaceID: opts.workspaceID,
		state:       opts.state,
		location:    opts.location,
		kind:        opts.kind,
		all:         opts.all,
	})

	if opts.format == "human" && c.shouldRunAgentBoardSelection(opts) {
		return c.runAgentBoardInteractive(root, records, opts)
	}

	switch opts.format {
	case "tsv":
		printAgentRuntimeListTSV(c.Out, records)
	default:
		printAgentBoardHuman(c.Out, records, writerSupportsColor(c.Out))
	}
	return exitOK
}

func parseAgentBoardOptions(args []string) (agentBoardOptions, error) {
	opts := agentBoardOptions{format: "human"}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := rest[0]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return agentBoardOptions{}, errHelpRequested
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
			rest = rest[1:]
		case arg == "--format":
			if len(rest) < 2 {
				return agentBoardOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--workspace="):
			opts.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--workspace="))
			rest = rest[1:]
		case arg == "--workspace":
			if len(rest) < 2 {
				return agentBoardOptions{}, fmt.Errorf("--workspace requires a value")
			}
			opts.workspaceID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--state="):
			opts.state = normalizeAgentStateFilter(strings.TrimSpace(strings.ToLower(strings.TrimPrefix(arg, "--state="))))
			rest = rest[1:]
		case arg == "--state":
			if len(rest) < 2 {
				return agentBoardOptions{}, fmt.Errorf("--state requires a value")
			}
			opts.state = normalizeAgentStateFilter(strings.TrimSpace(strings.ToLower(rest[1])))
			rest = rest[2:]
		case strings.HasPrefix(arg, "--location="):
			opts.location = strings.TrimSpace(strings.TrimPrefix(arg, "--location="))
			rest = rest[1:]
		case arg == "--location":
			if len(rest) < 2 {
				return agentBoardOptions{}, fmt.Errorf("--location requires a value")
			}
			opts.location = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--kind="):
			opts.kind = strings.TrimSpace(strings.TrimPrefix(arg, "--kind="))
			rest = rest[1:]
		case arg == "--kind":
			if len(rest) < 2 {
				return agentBoardOptions{}, fmt.Errorf("--kind requires a value")
			}
			opts.kind = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case arg == "--all":
			opts.all = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--session="):
			opts.sessionID = strings.TrimSpace(strings.TrimPrefix(arg, "--session="))
			rest = rest[1:]
		case arg == "--session":
			if len(rest) < 2 {
				return agentBoardOptions{}, fmt.Errorf("--session requires a value")
			}
			opts.sessionID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--act="):
			opts.action = strings.TrimSpace(strings.ToLower(strings.TrimPrefix(arg, "--act=")))
			rest = rest[1:]
		case arg == "--act":
			if len(rest) < 2 {
				return agentBoardOptions{}, fmt.Errorf("--act requires a value")
			}
			opts.action = strings.TrimSpace(strings.ToLower(rest[1]))
			rest = rest[2:]
		case arg == "--no-select":
			opts.noSelect = true
			rest = rest[1:]
		default:
			return agentBoardOptions{}, fmt.Errorf("unknown flag for agent board: %q", arg)
		}
	}
	if len(rest) > 0 {
		return agentBoardOptions{}, fmt.Errorf("unexpected args for agent board: %q", strings.Join(rest, " "))
	}
	switch opts.format {
	case "human", "tsv":
	default:
		return agentBoardOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, tsv)", opts.format)
	}
	switch opts.state {
	case "", "running", "idle", "exited", "unknown":
	default:
		return agentBoardOptions{}, fmt.Errorf("unsupported --state: %q (supported: active, running, idle, exited, unknown)", opts.state)
	}
	switch opts.action {
	case "", "show", "stop":
	default:
		return agentBoardOptions{}, fmt.Errorf("unsupported --act: %q (supported: show, stop)", opts.action)
	}
	return opts, nil
}

func (c *CLI) shouldRunAgentBoardSelection(opts agentBoardOptions) bool {
	if opts.format != "human" || opts.noSelect {
		return false
	}
	if strings.TrimSpace(opts.action) != "" {
		return true
	}
	if strings.TrimSpace(opts.sessionID) != "" {
		return true
	}
	return cliInputIsTTY(c.In)
}

func (c *CLI) runAgentBoardInteractive(root string, records []agentRuntimeSessionRecord, opts agentBoardOptions) int {
	if len(records) == 0 {
		printAgentBoardHuman(c.Out, records, writerSupportsColor(c.Out))
		return exitOK
	}

	selected, err := c.selectAgentBoardSession(records, opts)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			return exitOK
		}
		fmt.Fprintf(c.Err, "resolve board session: %v\n", err)
		return exitUsage
	}

	action := strings.TrimSpace(strings.ToLower(opts.action))
	if action == "" {
		if !cliInputIsTTY(c.In) {
			action = "show"
		}
	}
	if action == "" {
		action, err = c.selectAgentBoardAction()
		if err != nil {
			if errors.Is(err, errSelectorCanceled) {
				return exitOK
			}
			fmt.Fprintf(c.Err, "resolve board action: %v\n", err)
			return exitUsage
		}
	}

	switch action {
	case "stop":
		return c.runAgentStop([]string{"--session", selected.SessionID})
	default:
		printAgentBoardSelectedSession(c.Out, selected, writerSupportsColor(c.Out))
		return exitOK
	}
}

func (c *CLI) selectAgentBoardSession(records []agentRuntimeSessionRecord, opts agentBoardOptions) (agentRuntimeSessionRecord, error) {
	if sessionID := strings.TrimSpace(opts.sessionID); sessionID != "" {
		for _, r := range records {
			if r.SessionID == sessionID {
				return r, nil
			}
		}
		return agentRuntimeSessionRecord{}, fmt.Errorf("session not found in board scope: %s", sessionID)
	}

	sorted := append([]agentRuntimeSessionRecord{}, records...)
	slices.SortFunc(sorted, func(a, b agentRuntimeSessionRecord) int {
		if cmp := strings.Compare(a.WorkspaceID, b.WorkspaceID); cmp != 0 {
			return cmp
		}
		if cmp := compareExecutionLocation(a, b); cmp != 0 {
			return cmp
		}
		if a.UpdatedAt != b.UpdatedAt {
			if a.UpdatedAt > b.UpdatedAt {
				return -1
			}
			return 1
		}
		return strings.Compare(a.SessionID, b.SessionID)
	})
	selectorItems := make([]workspaceSelectorCandidate, 0, len(sorted))
	for _, r := range sorted {
		scope := r.WorkspaceID + "  " + locationLabel(r)
		selectorItems = append(selectorItems, workspaceSelectorCandidate{
			ID:          r.SessionID,
			Description: fmt.Sprintf("%s  %s  state:%s  updated:%s", r.Kind, scope, displayRuntimeStateLabel(r.RuntimeState), formatRelativeAge(r.UpdatedAt)),
		})
	}

	title := "Session (board):"
	if ws := strings.TrimSpace(opts.workspaceID); ws != "" {
		title = fmt.Sprintf("Session (workspace: %s):", ws)
	}
	if !cliInputIsTTY(c.In) {
		return agentRuntimeSessionRecord{}, fmt.Errorf("--session is required in non-interactive mode when board selection is enabled")
	}
	selected, err := c.promptWorkspaceSelectorWithOptionsAndMode("active", "select", title, "session", selectorItems, true)
	if err != nil {
		return agentRuntimeSessionRecord{}, err
	}
	if len(selected) != 1 || strings.TrimSpace(selected[0]) == "" {
		return agentRuntimeSessionRecord{}, fmt.Errorf("session selection canceled")
	}
	sessionID := strings.TrimSpace(selected[0])
	for _, r := range sorted {
		if r.SessionID == sessionID {
			return r, nil
		}
	}
	return agentRuntimeSessionRecord{}, fmt.Errorf("selected session not found: %s", sessionID)
}

func (c *CLI) selectAgentBoardAction() (string, error) {
	items := []workspaceSelectorCandidate{
		{ID: "show", Description: "show selected session details"},
		{ID: "stop", Description: "stop selected active/idle session"},
	}
	selected, err := c.promptWorkspaceSelectorWithOptionsAndMode("active", "run", "Action:", "action", items, true)
	if err != nil {
		return "", err
	}
	if len(selected) != 1 || strings.TrimSpace(selected[0]) == "" {
		return "", fmt.Errorf("action selection canceled")
	}
	return strings.TrimSpace(strings.ToLower(selected[0])), nil
}

func printAgentBoardSelectedSession(out io.Writer, record agentRuntimeSessionRecord, useColor bool) {
	location := locationLabel(record)
	lines := []string{
		fmt.Sprintf("%sworkspace: %s", uiIndent, record.WorkspaceID),
		fmt.Sprintf("%slocation:  %s", uiIndent, location),
		fmt.Sprintf("%skind:      %s", uiIndent, record.Kind),
		fmt.Sprintf("%sstate:     %s", uiIndent, displayRuntimeStateLabel(record.RuntimeState)),
		fmt.Sprintf("%supdated:   %s", uiIndent, formatRelativeAge(record.UpdatedAt)),
		fmt.Sprintf("%ssession:   %s", uiIndent, record.SessionID),
	}
	if useColor {
		for i := range lines {
			lines[i] = styleMuted(lines[i], useColor)
		}
	}
	printSection(out, "Agent Session:", lines, sectionRenderOptions{
		blankAfterHeading: true,
		trailingBlank:     true,
	})
}

func printAgentBoardHuman(out io.Writer, rows []agentRuntimeSessionRecord, useColor bool) {
	if len(rows) == 0 {
		printSection(out, "Agent Board:", []string{fmt.Sprintf("%s(none)", uiIndent)}, sectionRenderOptions{
			blankAfterHeading: true,
			trailingBlank:     true,
		})
		return
	}

	byWorkspace := map[string][]agentRuntimeSessionRecord{}
	for _, r := range rows {
		byWorkspace[r.WorkspaceID] = append(byWorkspace[r.WorkspaceID], r)
	}
	workspaceIDs := make([]string, 0, len(byWorkspace))
	for ws := range byWorkspace {
		workspaceIDs = append(workspaceIDs, ws)
	}
	slices.Sort(workspaceIDs)

	lines := make([]string, 0, len(rows)+len(workspaceIDs))
	maxCols := listTerminalWidth()
	for _, ws := range workspaceIDs {
		header := fmt.Sprintf("%s%s", uiIndent, ws)
		if useColor {
			header = styleAccent(header, useColor)
		}
		lines = append(lines, truncateDisplay(header, maxCols))

		children := byWorkspace[ws]
		slices.SortFunc(children, func(a, b agentRuntimeSessionRecord) int {
			return compareExecutionLocation(a, b)
		})
		for _, child := range children {
			line := fmt.Sprintf(
				"%s  - %s  session:%s  kind:%s  state:%s  updated:%s",
				uiIndent,
				locationLabel(child),
				child.SessionID,
				child.Kind,
				displayRuntimeStateLabel(child.RuntimeState),
				formatUnixTS(child.UpdatedAt),
			)
			if useColor {
				line = styleMuted(line, useColor)
			}
			lines = append(lines, truncateDisplay(line, maxCols))
		}
	}

	printSection(out, "Agent Board:", lines, sectionRenderOptions{
		blankAfterHeading: true,
		trailingBlank:     true,
	})
}

func locationLabel(r agentRuntimeSessionRecord) string {
	if r.ExecutionScope == "repo" {
		return "repo:" + r.RepoKey
	}
	return "workspace"
}

func compareExecutionLocation(a, b agentRuntimeSessionRecord) int {
	aRank := executionScopeRank(a.ExecutionScope)
	bRank := executionScopeRank(b.ExecutionScope)
	if aRank != bRank {
		if aRank < bRank {
			return -1
		}
		return 1
	}
	if a.ExecutionScope == "repo" || b.ExecutionScope == "repo" {
		if cmp := strings.Compare(a.RepoKey, b.RepoKey); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func executionScopeRank(scope string) int {
	switch scope {
	case "workspace":
		return 0
	case "repo":
		return 1
	default:
		return 2
	}
}
