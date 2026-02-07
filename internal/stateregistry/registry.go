package stateregistry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tasuku43/gionx/internal/paths"
)

type Entry struct {
	RootPath    string `json:"root_path"`
	StateDBPath string `json:"state_db_path"`
	FirstSeenAt int64  `json:"first_seen_at"`
	LastUsedAt  int64  `json:"last_used_at"`
}

type filePayload struct {
	Entries []Entry `json:"entries"`
}

func Path() (string, error) {
	return paths.RegistryPath()
}

func Load(path string) ([]Entry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("registry path is required")
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state registry %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil, nil
	}

	var p filePayload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("state registry is malformed: %s (fix or remove this file and retry): %w", path, err)
	}
	return p.Entries, nil
}

func Touch(rootPath string, stateDBPath string, now time.Time) error {
	rootAbs, err := cleanAbs(rootPath)
	if err != nil {
		return fmt.Errorf("resolve root_path: %w", err)
	}
	stateAbs, err := cleanAbs(stateDBPath)
	if err != nil {
		return fmt.Errorf("resolve state_db_path: %w", err)
	}
	expected, err := paths.StateDBPathForRoot(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve expected state_db_path: %w", err)
	}
	expectedAbs, err := cleanAbs(expected)
	if err != nil {
		return fmt.Errorf("resolve expected state_db_path: %w", err)
	}
	if stateAbs != expectedAbs {
		return fmt.Errorf("state_db_path mismatch for root_path: got=%s want=%s", stateAbs, expectedAbs)
	}

	registryPath, err := Path()
	if err != nil {
		return fmt.Errorf("resolve state registry path: %w", err)
	}

	entries, err := Load(registryPath)
	if err != nil {
		return err
	}

	nowUnix := now.Unix()
	if nowUnix <= 0 {
		nowUnix = time.Now().Unix()
	}

	updated := false
	for i := range entries {
		if entries[i].RootPath != rootAbs {
			continue
		}
		if entries[i].StateDBPath != stateAbs {
			return fmt.Errorf("state registry entry mismatch for root_path=%s: state_db_path=%s", rootAbs, entries[i].StateDBPath)
		}
		if entries[i].FirstSeenAt <= 0 {
			entries[i].FirstSeenAt = nowUnix
		}
		if entries[i].LastUsedAt < nowUnix {
			entries[i].LastUsedAt = nowUnix
		}
		updated = true
		break
	}
	if !updated {
		entries = append(entries, Entry{
			RootPath:    rootAbs,
			StateDBPath: stateAbs,
			FirstSeenAt: nowUnix,
			LastUsedAt:  nowUnix,
		})
	}

	slices.SortFunc(entries, func(a, b Entry) int {
		return strings.Compare(a.RootPath, b.RootPath)
	})

	if err := writeAtomic(registryPath, entries); err != nil {
		return err
	}
	return nil
}

func writeAtomic(path string, entries []Entry) error {
	payload := filePayload{Entries: entries}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state registry: %w", err)
	}
	b = append(b, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state registry dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".registry-*.tmp")
	if err != nil {
		return fmt.Errorf("create state registry temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if _, err := tmp.Write(b); err != nil {
		cleanup()
		return fmt.Errorf("write state registry temp file %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close state registry temp file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace state registry %s: %w", path, err)
	}
	return nil
}

func cleanAbs(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
