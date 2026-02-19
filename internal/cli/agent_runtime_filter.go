package cli

import (
	"strconv"
	"strings"
)

type agentRuntimeQueryOptions struct {
	workspaceID string
	state       string
	location    string
	kind        string
	all         bool
}

func normalizeAgentStateFilter(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "active":
		return "running"
	case "waiting":
		return "waiting_input"
	default:
		return strings.TrimSpace(strings.ToLower(v))
	}
}

func filterAgentRuntimeSessions(records []agentRuntimeSessionRecord, opts agentRuntimeQueryOptions) []agentRuntimeSessionRecord {
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

func formatUnixTS(ts int64) string {
	if ts <= 0 {
		return "-"
	}
	return strconv.FormatInt(ts, 10)
}
