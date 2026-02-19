package cli

import (
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
	records = filterAgentRuntimeSessions(records, agentListOptions{
		workspaceID: opts.workspaceID,
		state:       opts.state,
		location:    opts.location,
		kind:        opts.kind,
		all:         opts.all,
	})
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
	return opts, nil
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
