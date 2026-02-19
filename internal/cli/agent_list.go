package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/tasuku43/kra/internal/infra/paths"
)

const agentActivitiesFilename = "agents.json"

type agentListOptions struct {
	format      string
	workspaceID string
	state       string
	location    string
	kind        string
	all         bool
}

type agentActivityRecord struct {
	WorkspaceID        string `json:"workspace_id"`
	RepoKey            string `json:"repo_key"`
	AgentKind          string `json:"agent_kind"`
	TaskSummary        string `json:"task_summary"`
	InstructionSummary string `json:"instruction_summary"`
	StartedAt          int64  `json:"started_at"`
	LastHeartbeatAt    int64  `json:"last_heartbeat_at"`
	Status             string `json:"status"`
	LogPath            string `json:"log_path"`
}

func (c *CLI) runAgentList(args []string) int {
	opts, err := parseAgentListOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printAgentListUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printAgentListUsage(c.Err)
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
	records = filterAgentRuntimeSessions(records, opts)

	switch opts.format {
	case "tsv":
		printAgentRuntimeListTSV(c.Out, records)
	default:
		useColorOut := writerSupportsColor(c.Out)
		printAgentRuntimeListHuman(c.Out, records, useColorOut)
	}
	return exitOK
}

func parseAgentListOptions(args []string) (agentListOptions, error) {
	opts := agentListOptions{format: "human"}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := rest[0]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return agentListOptions{}, errHelpRequested
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
			rest = rest[1:]
		case arg == "--format":
			if len(rest) < 2 {
				return agentListOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--workspace="):
			opts.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--workspace="))
			rest = rest[1:]
		case arg == "--workspace":
			if len(rest) < 2 {
				return agentListOptions{}, fmt.Errorf("--workspace requires a value")
			}
			opts.workspaceID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--state="):
			opts.state = normalizeAgentStateFilter(strings.TrimSpace(strings.ToLower(strings.TrimPrefix(arg, "--state="))))
			rest = rest[1:]
		case arg == "--state":
			if len(rest) < 2 {
				return agentListOptions{}, fmt.Errorf("--state requires a value")
			}
			opts.state = normalizeAgentStateFilter(strings.TrimSpace(strings.ToLower(rest[1])))
			rest = rest[2:]
		case strings.HasPrefix(arg, "--location="):
			opts.location = strings.TrimSpace(strings.TrimPrefix(arg, "--location="))
			rest = rest[1:]
		case arg == "--location":
			if len(rest) < 2 {
				return agentListOptions{}, fmt.Errorf("--location requires a value")
			}
			opts.location = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--kind="):
			opts.kind = strings.TrimSpace(strings.TrimPrefix(arg, "--kind="))
			rest = rest[1:]
		case arg == "--kind":
			if len(rest) < 2 {
				return agentListOptions{}, fmt.Errorf("--kind requires a value")
			}
			opts.kind = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case arg == "--all":
			opts.all = true
			rest = rest[1:]
		default:
			return agentListOptions{}, fmt.Errorf("unknown flag for agent list: %q", arg)
		}
	}
	if len(rest) > 0 {
		return agentListOptions{}, fmt.Errorf("unexpected args for agent list: %q", strings.Join(rest, " "))
	}
	switch opts.format {
	case "human", "tsv":
	default:
		return agentListOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, tsv)", opts.format)
	}
	switch opts.state {
	case "", "running", "idle", "exited", "unknown":
	default:
		return agentListOptions{}, fmt.Errorf("unsupported --state: %q (supported: active, running, idle, exited, unknown)", opts.state)
	}
	return opts, nil
}

func normalizeAgentStateFilter(v string) string {
	if strings.TrimSpace(strings.ToLower(v)) == "active" {
		return "running"
	}
	return strings.TrimSpace(strings.ToLower(v))
}

func filterAgentRuntimeSessions(records []agentRuntimeSessionRecord, opts agentListOptions) []agentRuntimeSessionRecord {
	filtered := make([]agentRuntimeSessionRecord, 0, len(records))
	workspaceQuery := strings.TrimSpace(opts.workspaceID)
	locationQuery := strings.TrimSpace(strings.ToLower(opts.location))
	kindQuery := strings.TrimSpace(strings.ToLower(opts.kind))
	for _, r := range records {
		if workspaceQuery != "" && r.WorkspaceID != workspaceQuery {
			continue
		}
		if !opts.all && opts.state == "" && r.RuntimeState == "exited" {
			continue
		}
		if opts.state != "" && r.RuntimeState != opts.state {
			continue
		}
		if kindQuery != "" && strings.ToLower(r.Kind) != kindQuery {
			continue
		}
		if locationQuery != "" {
			location := "workspace"
			if r.ExecutionScope == "repo" {
				location = "repo:" + strings.ToLower(r.RepoKey)
			}
			if !strings.Contains(location, locationQuery) {
				continue
			}
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func loadAgentActivities(root string) ([]agentActivityRecord, error) {
	path := filepath.Join(root, ".kra", "state", agentActivitiesFilename)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []agentActivityRecord{}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(string(b)) == "" {
		return []agentActivityRecord{}, nil
	}

	var records []agentActivityRecord
	if err := json.Unmarshal(b, &records); err != nil {
		return nil, err
	}
	for i := range records {
		records[i].WorkspaceID = strings.TrimSpace(records[i].WorkspaceID)
		records[i].RepoKey = strings.TrimSpace(records[i].RepoKey)
		records[i].AgentKind = strings.TrimSpace(records[i].AgentKind)
		records[i].TaskSummary = strings.TrimSpace(records[i].TaskSummary)
		records[i].InstructionSummary = strings.TrimSpace(records[i].InstructionSummary)
		records[i].Status = strings.TrimSpace(strings.ToLower(records[i].Status))
		if records[i].Status == "" {
			records[i].Status = "unknown"
		}
		records[i].LogPath = strings.TrimSpace(records[i].LogPath)
	}

	slices.SortFunc(records, func(a, b agentActivityRecord) int {
		if a.StartedAt != b.StartedAt {
			if a.StartedAt > b.StartedAt {
				return -1
			}
			return 1
		}
		if cmp := strings.Compare(a.WorkspaceID, b.WorkspaceID); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.AgentKind, b.AgentKind)
	})
	return records, nil
}

func saveAgentActivities(root string, records []agentActivityRecord) error {
	stateDir := filepath.Join(root, ".kra", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(stateDir, agentActivitiesFilename)
	b, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func printAgentListTSV(out io.Writer, rows []agentActivityRecord) {
	fmt.Fprintln(out, "workspace_id\trepo_key\tagent_kind\ttask_summary\tinstruction_summary\tstarted_at\tlast_heartbeat_at\tstatus\tlog_path")
	for _, r := range rows {
		fmt.Fprintf(
			out,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.WorkspaceID,
			r.RepoKey,
			r.AgentKind,
			r.TaskSummary,
			r.InstructionSummary,
			formatUnixTS(r.StartedAt),
			formatUnixTS(r.LastHeartbeatAt),
			r.Status,
			r.LogPath,
		)
	}
}

func printAgentListHuman(out io.Writer, rows []agentActivityRecord, useColor bool) {
	body := make([]string, 0, len(rows))
	if len(rows) == 0 {
		body = append(body, fmt.Sprintf("%s(none)", uiIndent))
		printSection(out, "Agents:", body, sectionRenderOptions{
			blankAfterHeading: true,
			trailingBlank:     true,
		})
		return
	}
	maxCols := listTerminalWidth()
	for _, r := range rows {
		line := fmt.Sprintf(
			"%s• %s  kind:%s  status:%s  hb:%s",
			uiIndent,
			r.WorkspaceID,
			r.AgentKind,
			r.Status,
			formatUnixTS(r.LastHeartbeatAt),
		)
		if r.RepoKey != "" {
			line += "  repo:" + r.RepoKey
		}
		if r.TaskSummary != "" {
			line += "  task:" + r.TaskSummary
		}
		if r.InstructionSummary != "" {
			line += "  instruction:" + r.InstructionSummary
		}
		if r.LogPath != "" {
			line += "  log:" + r.LogPath
		}
		if useColor {
			line = styleMuted(line, useColor)
		}
		body = append(body, truncateDisplay(line, maxCols))
	}
	printSection(out, "Agents:", body, sectionRenderOptions{
		blankAfterHeading: true,
		trailingBlank:     true,
	})
}

func formatUnixTS(v int64) string {
	if v <= 0 {
		return "-"
	}
	return strconv.FormatInt(v, 10)
}
