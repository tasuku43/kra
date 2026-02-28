package cli

import (
	"context"
	"strings"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type cmuxRuntimeClient interface {
	ListWorkspaces(ctx context.Context) ([]cmuxctl.Workspace, error)
}

var newCMUXRuntimeClient = func() cmuxRuntimeClient { return cmuxctl.NewClient() }

func reconcileCMUXMappingWithRuntime(store cmuxmap.Store, mapping cmuxmap.File, runtime []cmuxctl.Workspace, prune bool) (cmuxmap.File, map[string]bool, int, error) {
	exists := map[string]bool{}
	for _, row := range runtime {
		id := strings.TrimSpace(row.ID)
		if id != "" {
			exists[id] = true
		}
	}
	if !prune {
		return mapping, exists, 0, nil
	}

	prunedCount := 0
	for wsID, ws := range mapping.Workspaces {
		keep := make([]cmuxmap.Entry, 0, len(ws.Entries))
		for _, e := range ws.Entries {
			if exists[strings.TrimSpace(e.CMUXWorkspaceID)] {
				keep = append(keep, e)
				continue
			}
			prunedCount++
		}
		ws.Entries = keep
		mapping.Workspaces[wsID] = ws
	}
	if prunedCount > 0 {
		if err := store.Save(mapping); err != nil {
			return mapping, exists, prunedCount, err
		}
	}
	return mapping, exists, prunedCount, nil
}
