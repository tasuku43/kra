package cli

import (
	"context"
	"sort"
	"time"
)

func listWorkspaceCandidatesByStatus(ctx context.Context, root string, status string) ([]workspaceSelectorCandidate, error) {
	rows, _, err := buildWSListRows(ctx, root, status, time.Now().Unix(), false)
	if err != nil {
		return nil, err
	}
	out := make([]workspaceSelectorCandidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, workspaceSelectorCandidate{
			ID:        row.ID,
			Title:     formatWorkspaceTitle(row.Title),
			WorkState: normalizeWorkspaceWorkState(row.WorkState),
		})
	}
	return out, nil
}

func listWorkspaceIDsByStatus(ctx context.Context, root string, status string) ([]string, error) {
	candidates, err := listWorkspaceCandidatesByStatus(ctx, root, status)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID == "" {
			continue
		}
		out = append(out, candidate.ID)
	}
	sort.Strings(out)
	return out, nil
}
