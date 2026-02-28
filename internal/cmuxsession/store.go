package cmuxsession

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	CurrentVersion = 1
	fileName       = "cmux-sessions.json"
)

type Entry struct {
	SessionID         string `json:"session_id"`
	Label             string `json:"label,omitempty"`
	CreatedAt         string `json:"created_at"`
	Path              string `json:"path"`
	PaneCount         int    `json:"pane_count"`
	SurfaceCount      int    `json:"surface_count"`
	BrowserStateSaved bool   `json:"browser_state_saved"`
}

type WorkspaceSessions struct {
	Sessions []Entry `json:"sessions"`
}

type File struct {
	Version    int                          `json:"version"`
	Workspaces map[string]WorkspaceSessions `json:"workspaces"`
}

type Store struct {
	path string
}

func NewStore(root string) Store {
	return Store{path: SessionPath(root)}
}

func SessionPath(root string) string {
	return filepath.Join(root, ".kra", "state", fileName)
}

func (s Store) Load() (File, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultFile(), nil
		}
		return File{}, fmt.Errorf("read cmux session store: %w", err)
	}
	var out File
	if err := json.Unmarshal(data, &out); err != nil {
		return File{}, fmt.Errorf("parse cmux session store: %w", err)
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
		return fmt.Errorf("marshal cmux session store: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create cmux session dir: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write cmux session temp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace cmux session store: %w", err)
	}
	return nil
}

func defaultFile() File {
	return File{
		Version:    CurrentVersion,
		Workspaces: map[string]WorkspaceSessions{},
	}
}

func normalize(in *File) error {
	if in.Version == 0 {
		in.Version = CurrentVersion
	}
	if in.Version != CurrentVersion {
		return fmt.Errorf("unsupported cmux session version: %d", in.Version)
	}
	if in.Workspaces == nil {
		in.Workspaces = map[string]WorkspaceSessions{}
	}
	for wsID, ws := range in.Workspaces {
		if ws.Sessions == nil {
			ws.Sessions = []Entry{}
		}
		seen := map[string]struct{}{}
		for _, s := range ws.Sessions {
			id := strings.TrimSpace(s.SessionID)
			if id == "" {
				return fmt.Errorf("workspace %s contains empty session_id", wsID)
			}
			if _, ok := seen[id]; ok {
				return fmt.Errorf("workspace %s contains duplicate session_id: %s", wsID, id)
			}
			seen[id] = struct{}{}
		}
		sort.SliceStable(ws.Sessions, func(i, j int) bool {
			a := strings.TrimSpace(ws.Sessions[i].CreatedAt)
			b := strings.TrimSpace(ws.Sessions[j].CreatedAt)
			if a != b {
				return a > b // newest first
			}
			return ws.Sessions[i].SessionID > ws.Sessions[j].SessionID
		})
		in.Workspaces[wsID] = ws
	}
	return nil
}
