package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const rootRepoRegistryVersion = 1
const rootRepoRegistryFilename = "root-repos.json"

type rootRepoRegistryFile struct {
	Version int                     `json:"version"`
	Repos   []rootRepoRegistryEntry `json:"repos"`
}

type rootRepoRegistryEntry struct {
	RepoUID   string `json:"repo_uid"`
	RepoKey   string `json:"repo_key"`
	RemoteURL string `json:"remote_url"`
}

func rootRepoRegistryPath(root string) string {
	return filepath.Join(root, ".kra", "state", rootRepoRegistryFilename)
}

func loadRootRepoRegistry(root string) ([]rootRepoRegistryEntry, error) {
	path := rootRepoRegistryPath(root)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var data rootRepoRegistryFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	out := make([]rootRepoRegistryEntry, 0, len(data.Repos))
	seen := map[string]bool{}
	for _, it := range data.Repos {
		entry := rootRepoRegistryEntry{
			RepoUID:   strings.TrimSpace(it.RepoUID),
			RepoKey:   strings.TrimSpace(it.RepoKey),
			RemoteURL: strings.TrimSpace(it.RemoteURL),
		}
		if entry.RepoUID == "" || entry.RepoKey == "" || entry.RemoteURL == "" {
			continue
		}
		if seen[entry.RepoUID] {
			continue
		}
		seen[entry.RepoUID] = true
		out = append(out, entry)
	}
	slices.SortFunc(out, func(a, b rootRepoRegistryEntry) int {
		if a.RepoKey != b.RepoKey {
			return strings.Compare(a.RepoKey, b.RepoKey)
		}
		return strings.Compare(a.RepoUID, b.RepoUID)
	})
	return out, nil
}

func saveRootRepoRegistry(root string, entries []rootRepoRegistryEntry) error {
	path := rootRepoRegistryPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	normalized := make([]rootRepoRegistryEntry, 0, len(entries))
	seen := map[string]bool{}
	for _, it := range entries {
		entry := rootRepoRegistryEntry{
			RepoUID:   strings.TrimSpace(it.RepoUID),
			RepoKey:   strings.TrimSpace(it.RepoKey),
			RemoteURL: strings.TrimSpace(it.RemoteURL),
		}
		if entry.RepoUID == "" || entry.RepoKey == "" || entry.RemoteURL == "" {
			continue
		}
		if seen[entry.RepoUID] {
			continue
		}
		seen[entry.RepoUID] = true
		normalized = append(normalized, entry)
	}
	slices.SortFunc(normalized, func(a, b rootRepoRegistryEntry) int {
		if a.RepoKey != b.RepoKey {
			return strings.Compare(a.RepoKey, b.RepoKey)
		}
		return strings.Compare(a.RepoUID, b.RepoUID)
	})
	payload := rootRepoRegistryFile{
		Version: rootRepoRegistryVersion,
		Repos:   normalized,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeAtomic(path, raw, 0o644)
}

func upsertRootRepoRegistryEntries(root string, additions []rootRepoRegistryEntry) error {
	current, err := loadRootRepoRegistry(root)
	if err != nil {
		return err
	}
	byUID := map[string]rootRepoRegistryEntry{}
	for _, it := range current {
		byUID[it.RepoUID] = it
	}
	for _, it := range additions {
		uid := strings.TrimSpace(it.RepoUID)
		key := strings.TrimSpace(it.RepoKey)
		remote := strings.TrimSpace(it.RemoteURL)
		if uid == "" || key == "" || remote == "" {
			continue
		}
		byUID[uid] = rootRepoRegistryEntry{RepoUID: uid, RepoKey: key, RemoteURL: remote}
	}
	out := make([]rootRepoRegistryEntry, 0, len(byUID))
	for _, it := range byUID {
		out = append(out, it)
	}
	return saveRootRepoRegistry(root, out)
}

func removeRootRepoRegistryEntries(root string, repoUIDs []string) error {
	if len(repoUIDs) == 0 {
		return nil
	}
	current, err := loadRootRepoRegistry(root)
	if err != nil {
		return err
	}
	removeSet := map[string]bool{}
	for _, uid := range repoUIDs {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		removeSet[uid] = true
	}
	out := make([]rootRepoRegistryEntry, 0, len(current))
	for _, it := range current {
		if removeSet[it.RepoUID] {
			continue
		}
		out = append(out, it)
	}
	return saveRootRepoRegistry(root, out)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
