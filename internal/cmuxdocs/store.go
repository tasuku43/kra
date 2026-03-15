package cmuxdocs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	CurrentVersion = 1
	fileName       = "cmux-docs.json"
)

type Slot struct {
	CMUXWorkspaceID string `json:"cmux_workspace_id"`
	PaneRef         string `json:"pane_ref"`
	LastViewerRef   string `json:"last_viewer_ref,omitempty"`
	LastUsedAt      string `json:"last_used_at,omitempty"`
}

type Stage struct {
	WorkspaceRef string `json:"workspace_ref,omitempty"`
	PaneRef      string `json:"pane_ref,omitempty"`
	SurfaceRef   string `json:"surface_ref,omitempty"`
	LastUsedAt   string `json:"last_used_at,omitempty"`
}

type File struct {
	Version    int             `json:"version"`
	Stage      Stage           `json:"stage,omitempty"`
	Workspaces map[string]Slot `json:"workspaces"`
}

type Store struct {
	path string
}

func NewStore(root string) Store {
	return Store{path: filepath.Join(root, ".kra", "state", fileName)}
}

func (s Store) Load() (File, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultFile(), nil
		}
		return File{}, fmt.Errorf("read cmux docs state: %w", err)
	}

	var out File
	if err := json.Unmarshal(data, &out); err != nil {
		return File{}, fmt.Errorf("parse cmux docs state: %w", err)
	}
	if err := normalize(&out); err != nil {
		return File{}, err
	}
	return out, nil
}

func (s Store) Save(in File) error {
	if err := normalize(&in); err != nil {
		return err
	}
	data, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cmux docs state: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create cmux docs state dir: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write cmux docs state temp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace cmux docs state: %w", err)
	}
	return nil
}

func defaultFile() File {
	return File{
		Version:    CurrentVersion,
		Stage:      Stage{},
		Workspaces: map[string]Slot{},
	}
}

func normalize(in *File) error {
	if in.Version == 0 {
		in.Version = CurrentVersion
	}
	if in.Version != CurrentVersion {
		return fmt.Errorf("unsupported cmux docs state version: %d", in.Version)
	}
	if in.Workspaces == nil {
		in.Workspaces = map[string]Slot{}
	}
	in.Stage = Stage{
		WorkspaceRef: in.Stage.WorkspaceRef,
		PaneRef:      in.Stage.PaneRef,
		SurfaceRef:   in.Stage.SurfaceRef,
		LastUsedAt:   in.Stage.LastUsedAt,
	}

	keys := make([]string, 0, len(in.Workspaces))
	for k := range in.Workspaces {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		slot := in.Workspaces[k]
		in.Workspaces[k] = Slot{
			CMUXWorkspaceID: slot.CMUXWorkspaceID,
			PaneRef:         slot.PaneRef,
			LastViewerRef:   slot.LastViewerRef,
			LastUsedAt:      slot.LastUsedAt,
		}
	}
	return nil
}
