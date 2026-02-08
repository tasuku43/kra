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
	FirstSeenAt int64  `json:"first_seen_at"`
	LastUsedAt  int64  `json:"last_used_at"`
}

type filePayload struct {
	Entries []fileEntryPayload `json:"entries"`
}

type fileEntryPayload struct {
	RootPath    string `json:"root_path"`
	StateDBPath string `json:"state_db_path,omitempty"` // legacy compatibility (read-only)
	FirstSeenAt int64  `json:"first_seen_at"`
	LastUsedAt  int64  `json:"last_used_at"`
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
		return nil, fmt.Errorf("read root registry %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil, nil
	}

	var p filePayload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("root registry is malformed: %s (fix or remove this file and retry): %w", path, err)
	}
	out := make([]Entry, 0, len(p.Entries))
	for _, e := range p.Entries {
		out = append(out, Entry{
			RootPath:    e.RootPath,
			FirstSeenAt: e.FirstSeenAt,
			LastUsedAt:  e.LastUsedAt,
		})
	}
	return out, nil
}

func Touch(rootPath string, now time.Time) error {
	rootAbs, err := cleanAbs(rootPath)
	if err != nil {
		return fmt.Errorf("resolve root_path: %w", err)
	}

	registryPath, err := Path()
	if err != nil {
		return fmt.Errorf("resolve root registry path: %w", err)
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
	entryPayloads := make([]fileEntryPayload, 0, len(entries))
	for _, e := range entries {
		entryPayloads = append(entryPayloads, fileEntryPayload{
			RootPath:    e.RootPath,
			FirstSeenAt: e.FirstSeenAt,
			LastUsedAt:  e.LastUsedAt,
		})
	}
	payload := filePayload{Entries: entryPayloads}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal root registry: %w", err)
	}
	b = append(b, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create root registry dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".registry-*.tmp")
	if err != nil {
		return fmt.Errorf("create root registry temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if _, err := tmp.Write(b); err != nil {
		cleanup()
		return fmt.Errorf("write root registry temp file %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close root registry temp file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace root registry %s: %w", path, err)
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
