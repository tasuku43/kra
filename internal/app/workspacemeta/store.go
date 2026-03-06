package workspacemeta

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const FileName = ".kra.meta.json"

type File struct {
	SchemaVersion int           `json:"schema_version"`
	Workspace     Workspace     `json:"workspace"`
	ReposRestore  []RepoRestore `json:"repos_restore"`
	Baseline      *Baseline     `json:"baseline,omitempty"`
	Protection    Protection    `json:"protection,omitempty"`
}

type Workspace struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	SourceURL string `json:"source_url"`
	Status    string `json:"status"`
	WorkState string `json:"work_state,omitempty"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type RepoRestore struct {
	RepoUID   string `json:"repo_uid"`
	RepoKey   string `json:"repo_key"`
	RemoteURL string `json:"remote_url"`
	Alias     string `json:"alias"`
	Branch    string `json:"branch"`
	BaseRef   string `json:"base_ref"`
}

type Protection struct {
	PurgeGuard PurgeGuard `json:"purge_guard,omitempty"`
}

type PurgeGuard struct {
	Enabled   *bool `json:"enabled,omitempty"`
	UpdatedAt int64 `json:"updated_at,omitempty"`
}

type BaselineRepo struct {
	BaselineHead string `json:"baseline_head,omitempty"`
}

type Baseline struct {
	Version   int                     `json:"version"`
	CreatedAt int64                   `json:"created_at"`
	Repos     map[string]BaselineRepo `json:"repos,omitempty"`
	FS        map[string]string       `json:"fs,omitempty"`
}

func NewForCreate(id string, title string, sourceURL string, now int64) File {
	enabled := true
	return File{
		SchemaVersion: 1,
		Workspace: Workspace{
			ID:        id,
			Title:     title,
			SourceURL: sourceURL,
			Status:    "active",
			WorkState: "todo",
			CreatedAt: now,
			UpdatedAt: now,
		},
		ReposRestore: make([]RepoRestore, 0),
		Protection: Protection{
			PurgeGuard: PurgeGuard{
				Enabled:   &enabled,
				UpdatedAt: now,
			},
		},
	}
}

func Write(wsPath string, meta File) error {
	if strings.TrimSpace(wsPath) == "" {
		return fmt.Errorf("workspace path is required")
	}
	metaPath := filepath.Join(wsPath, FileName)
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace meta: %w", err)
	}
	b = append(b, '\n')

	tmp, err := os.CreateTemp(wsPath, ".kra-meta-*.tmp")
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

func Load(wsPath string) (File, error) {
	if strings.TrimSpace(wsPath) == "" {
		return File{}, fmt.Errorf("workspace path is required")
	}
	metaPath := filepath.Join(wsPath, FileName)
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return File{}, fmt.Errorf("read workspace meta file %s: %w", metaPath, err)
	}
	var meta File
	if err := json.Unmarshal(b, &meta); err != nil {
		return File{}, fmt.Errorf("parse workspace meta file %s: %w", metaPath, err)
	}
	if meta.SchemaVersion != 1 {
		return File{}, fmt.Errorf("unsupported workspace meta schema_version: %d", meta.SchemaVersion)
	}
	if strings.TrimSpace(meta.Workspace.ID) == "" {
		return File{}, fmt.Errorf("workspace.id is required in %s", metaPath)
	}
	switch strings.TrimSpace(meta.Workspace.WorkState) {
	case "", "todo", "in-progress":
	default:
		return File{}, fmt.Errorf("workspace.work_state must be one of: todo, in-progress")
	}
	if meta.ReposRestore == nil {
		meta.ReposRestore = make([]RepoRestore, 0)
	}
	if meta.Baseline != nil {
		baseline := NormalizeBaseline(*meta.Baseline)
		meta.Baseline = &baseline
	}
	return meta, nil
}

func NormalizeBaseline(baseline Baseline) Baseline {
	if baseline.Version <= 0 {
		baseline.Version = 1
	}
	if baseline.Repos == nil {
		baseline.Repos = map[string]BaselineRepo{}
	}
	if baseline.FS == nil {
		baseline.FS = map[string]string{}
	}
	return baseline
}

func SetWorkState(wsPath string, next string, now int64) (bool, error) {
	meta, err := Load(wsPath)
	if err != nil {
		return false, err
	}
	curRaw := strings.TrimSpace(meta.Workspace.WorkState)
	cur := normalizeWorkState(curRaw)
	target := normalizeWorkState(next)
	if cur == target {
		if curRaw == "" && target == "todo" {
			meta.Workspace.WorkState = target
			if now > 0 {
				meta.Workspace.UpdatedAt = now
			}
			if err := Write(wsPath, meta); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, nil
	}
	if cur == "in-progress" && target == "todo" {
		return false, nil
	}
	meta.Workspace.WorkState = target
	if now > 0 {
		meta.Workspace.UpdatedAt = now
	}
	if err := Write(wsPath, meta); err != nil {
		return false, err
	}
	return true, nil
}

func UpsertReposRestore(wsPath string, repos []RepoRestore, now int64) error {
	meta, err := Load(wsPath)
	if err != nil {
		return err
	}
	byAlias := make(map[string]RepoRestore, len(meta.ReposRestore)+len(repos))
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
	merged := make([]RepoRestore, 0, len(aliases))
	for _, alias := range aliases {
		merged = append(merged, byAlias[alias])
	}
	meta.ReposRestore = merged
	if now > 0 {
		meta.Workspace.UpdatedAt = now
	}
	return Write(wsPath, meta)
}

func PurgeGuardEnabled(meta File) bool {
	if meta.Protection.PurgeGuard.Enabled == nil {
		return true
	}
	return *meta.Protection.PurgeGuard.Enabled
}

func SetPurgeGuard(meta *File, enabled bool, now int64) {
	if meta == nil {
		return
	}
	meta.Protection.PurgeGuard.Enabled = boolPtr(enabled)
	if now > 0 {
		meta.Protection.PurgeGuard.UpdatedAt = now
	}
}

func RemoveReposRestoreByAlias(wsPath string, aliases []string, now int64) error {
	meta, err := Load(wsPath)
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
	next := make([]RepoRestore, 0, len(meta.ReposRestore))
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
	return Write(wsPath, meta)
}

func normalizeWorkState(raw string) string {
	switch strings.TrimSpace(raw) {
	case "in-progress":
		return "in-progress"
	default:
		return "todo"
	}
}

func boolPtr(v bool) *bool {
	b := v
	return &b
}
