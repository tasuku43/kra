package appports

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type CMUXTaskSyncClient interface {
	ListStatus(ctx context.Context, workspace string) ([]cmuxctl.StatusEntry, error)
	SetStatus(ctx context.Context, workspace string, label string, text string, icon string, color string) error
	ClearStatus(ctx context.Context, workspace string, label string) error
}

type WSTaskSyncPort struct {
	newClient func() CMUXTaskSyncClient
	newStore  func(root string) cmuxmap.Store
}

func NewWSTaskSyncPort(newClient func() CMUXTaskSyncClient, newStore func(root string) cmuxmap.Store) *WSTaskSyncPort {
	return &WSTaskSyncPort{
		newClient: newClient,
		newStore:  newStore,
	}
}

func (p *WSTaskSyncPort) ListCMUXWorkspaceIDs(root string, workspaceID string) ([]string, error) {
	if p == nil || p.newStore == nil {
		return fallbackCMUXWorkspaceIDs(), nil
	}
	store := p.newStore(root)
	mapping, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("load cmux mapping: %w", err)
	}
	ws, ok := mapping.Workspaces[workspaceID]
	if !ok || len(ws.Entries) == 0 {
		return fallbackCMUXWorkspaceIDs(), nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ws.Entries))
	for _, entry := range ws.Entries {
		id := strings.TrimSpace(entry.CMUXWorkspaceID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return fallbackCMUXWorkspaceIDs(), nil
	}
	return out, nil
}

func fallbackCMUXWorkspaceIDs() []string {
	id := strings.TrimSpace(os.Getenv("CMUX_WORKSPACE_ID"))
	if id == "" {
		return nil
	}
	return []string{id}
}

func (p *WSTaskSyncPort) ListStatuses(ctx context.Context, cmuxWorkspaceID string) ([]wstask.SyncStatusEntry, error) {
	client, err := p.client()
	if err != nil {
		return nil, err
	}
	rows, err := client.ListStatus(ctx, cmuxWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("list cmux sidebar status: %w", err)
	}
	out := make([]wstask.SyncStatusEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, wstask.SyncStatusEntry{
			Key:   strings.TrimSpace(row.Key),
			Value: strings.TrimSpace(row.Value),
			Icon:  strings.TrimSpace(row.Icon),
			Color: strings.TrimSpace(row.Color),
		})
	}
	return out, nil
}

func (p *WSTaskSyncPort) SetStatus(ctx context.Context, cmuxWorkspaceID string, entry wstask.SyncStatusEntry) error {
	client, err := p.client()
	if err != nil {
		return err
	}
	if err := client.SetStatus(ctx, cmuxWorkspaceID, entry.Key, entry.Value, entry.Icon, entry.Color); err != nil {
		return fmt.Errorf("set cmux sidebar status: %w", err)
	}
	return nil
}

func (p *WSTaskSyncPort) ClearStatus(ctx context.Context, cmuxWorkspaceID string, key string) error {
	client, err := p.client()
	if err != nil {
		return err
	}
	if err := client.ClearStatus(ctx, cmuxWorkspaceID, key); err != nil {
		return fmt.Errorf("clear cmux sidebar status: %w", err)
	}
	return nil
}

func (p *WSTaskSyncPort) client() (CMUXTaskSyncClient, error) {
	if p == nil || p.newClient == nil {
		return nil, fmt.Errorf("cmux task sync client is not configured")
	}
	client := p.newClient()
	if client == nil {
		return nil, fmt.Errorf("cmux task sync client is not configured")
	}
	return client, nil
}
