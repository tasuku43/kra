package cli

import (
	"context"
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
