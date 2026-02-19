package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/infra/paths"
)

type agentRuntimeSignalEventRecord struct {
	SessionID string `json:"session_id"`
	At        int64  `json:"at"`
	Name      string `json:"name"`
	StateHint string `json:"state_hint,omitempty"`
	Details   string `json:"details,omitempty"`
}

func appendAgentRuntimeSignalEvents(record agentRuntimeSessionRecord, events []agentRuntimeSignalEvent) error {
	if len(events) == 0 {
		return nil
	}
	kraHome, err := paths.KraHomeDir()
	if err != nil {
		return fmt.Errorf("resolve KRA_HOME: %w", err)
	}
	rootHash := hashRootPath(record.RootPath)
	dir := filepath.Join(kraHome, "state", "agents", rootHash, "events")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create runtime event dir: %w", err)
	}
	path := filepath.Join(dir, strings.TrimSpace(record.SessionID)+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open runtime event file: %w", err)
	}
	defer func() { _ = f.Close() }()

	now := time.Now().Unix()
	for _, ev := range events {
		name := strings.TrimSpace(ev.Name)
		if name == "" {
			continue
		}
		row := agentRuntimeSignalEventRecord{
			SessionID: strings.TrimSpace(record.SessionID),
			At:        now,
			Name:      name,
			StateHint: strings.TrimSpace(ev.StateHint),
			Details:   strings.TrimSpace(ev.Details),
		}
		b, err := json.Marshal(row)
		if err != nil {
			continue
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			return fmt.Errorf("append runtime event: %w", err)
		}
	}
	return nil
}
