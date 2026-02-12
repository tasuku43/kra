package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const workspaceMetaFilename = ".gionx.meta.json"

type workspaceMetaFile struct {
	SchemaVersion int                        `json:"schema_version"`
	Workspace     workspaceMetaWorkspace     `json:"workspace"`
	ReposRestore  []workspaceMetaRepoRestore `json:"repos_restore"`
}

type workspaceMetaWorkspace struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	SourceURL string `json:"source_url"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type workspaceMetaRepoRestore struct {
	RepoUID   string `json:"repo_uid"`
	RepoKey   string `json:"repo_key"`
	RemoteURL string `json:"remote_url"`
	Alias     string `json:"alias"`
	Branch    string `json:"branch"`
	BaseRef   string `json:"base_ref"`
}

func newWorkspaceMetaFileForCreate(id string, title string, sourceURL string, now int64) workspaceMetaFile {
	return workspaceMetaFile{
		SchemaVersion: 1,
		Workspace: workspaceMetaWorkspace{
			ID:        id,
			Title:     title,
			SourceURL: sourceURL,
			Status:    "active",
			CreatedAt: now,
			UpdatedAt: now,
		},
		ReposRestore: make([]workspaceMetaRepoRestore, 0),
	}
}

func writeWorkspaceMetaFile(wsPath string, meta workspaceMetaFile) error {
	if strings.TrimSpace(wsPath) == "" {
		return fmt.Errorf("workspace path is required")
	}
	metaPath := filepath.Join(wsPath, workspaceMetaFilename)
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace meta: %w", err)
	}
	b = append(b, '\n')

	tmp, err := os.CreateTemp(wsPath, ".gionx-meta-*.tmp")
	if err != nil {
		return fmt.Errorf("create workspace meta temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write workspace meta temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close workspace meta temp file: %w", err)
	}
	if err := os.Rename(tmpPath, metaPath); err != nil {
		return fmt.Errorf("replace workspace meta file %s: %w", metaPath, err)
	}
	return nil
}

func loadWorkspaceMetaFile(wsPath string) (workspaceMetaFile, error) {
	if strings.TrimSpace(wsPath) == "" {
		return workspaceMetaFile{}, fmt.Errorf("workspace path is required")
	}
	metaPath := filepath.Join(wsPath, workspaceMetaFilename)
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return workspaceMetaFile{}, fmt.Errorf("read workspace meta file %s: %w", metaPath, err)
	}
	var meta workspaceMetaFile
	if err := json.Unmarshal(b, &meta); err != nil {
		return workspaceMetaFile{}, fmt.Errorf("parse workspace meta file %s: %w", metaPath, err)
	}
	if meta.SchemaVersion != 1 {
		return workspaceMetaFile{}, fmt.Errorf("unsupported workspace meta schema_version: %d", meta.SchemaVersion)
	}
	if strings.TrimSpace(meta.Workspace.ID) == "" {
		return workspaceMetaFile{}, fmt.Errorf("workspace.id is required in %s", metaPath)
	}
	if meta.ReposRestore == nil {
		meta.ReposRestore = make([]workspaceMetaRepoRestore, 0)
	}
	return meta, nil
}

func upsertWorkspaceMetaReposRestore(wsPath string, repos []workspaceMetaRepoRestore, now int64) error {
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		return err
	}
	byAlias := make(map[string]workspaceMetaRepoRestore, len(meta.ReposRestore)+len(repos))
	for _, r := range meta.ReposRestore {
		alias := strings.TrimSpace(r.Alias)
		if alias == "" {
			return fmt.Errorf("repos_restore alias is required")
		}
		byAlias[alias] = r
	}
	for _, r := range repos {
		alias := strings.TrimSpace(r.Alias)
		if alias == "" {
			return fmt.Errorf("repos_restore alias is required")
		}
		byAlias[alias] = r
	}
	aliases := make([]string, 0, len(byAlias))
	for alias := range byAlias {
		aliases = append(aliases, alias)
	}
	slices.Sort(aliases)
	merged := make([]workspaceMetaRepoRestore, 0, len(aliases))
	for _, alias := range aliases {
		merged = append(merged, byAlias[alias])
	}
	meta.ReposRestore = merged
	if now > 0 {
		meta.Workspace.UpdatedAt = now
	}
	return writeWorkspaceMetaFile(wsPath, meta)
}

func removeWorkspaceMetaReposRestoreByAlias(wsPath string, aliases []string, now int64) error {
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		return err
	}
	toDelete := make(map[string]bool, len(aliases))
	for _, alias := range aliases {
		trimmed := strings.TrimSpace(alias)
		if trimmed == "" {
			continue
		}
		toDelete[trimmed] = true
	}
	if len(toDelete) == 0 {
		return nil
	}
	next := make([]workspaceMetaRepoRestore, 0, len(meta.ReposRestore))
	for _, r := range meta.ReposRestore {
		if toDelete[strings.TrimSpace(r.Alias)] {
			continue
		}
		next = append(next, r)
	}
	meta.ReposRestore = next
	if now > 0 {
		meta.Workspace.UpdatedAt = now
	}
	return writeWorkspaceMetaFile(wsPath, meta)
}
