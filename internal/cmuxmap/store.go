package cmuxmap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tasuku43/kra/internal/filelock"
)

const (
	CurrentVersion = 2
	fileName       = "cmux-workspaces.json"
	operationsDir  = "operations"
)

type Entry struct {
	CMUXWorkspaceID string `json:"cmux_workspace_id"`
	Ordinal         int    `json:"ordinal"`
	TitleSnapshot   string `json:"title_snapshot"`
	CreatedAt       string `json:"created_at"`
	LastUsedAt      string `json:"last_used_at"`
}

type WorkspaceMapping struct {
	NextOrdinal int     `json:"next_ordinal"`
	Entries     []Entry `json:"entries"`
}

type GroupMapping struct {
	CMUXWindowID      string `json:"cmux_window_id"`
	CMUXGroupID       string `json:"cmux_group_id"`
	AnchorWorkspaceID string `json:"anchor_workspace_id"`
	NameSnapshot      string `json:"name_snapshot"`
	CreatedAt         string `json:"created_at"`
	LastUsedAt        string `json:"last_used_at"`
}

type TodoItemBase struct {
	KRATaskID  string `json:"kra_task_id"`
	CMUXItemID string `json:"cmux_item_id"`
	KRAStatus  string `json:"kra_status"`
	CMUXState  string `json:"cmux_state"`
	Text       string `json:"text"`
}

type TodoMapping struct {
	CMUXWorkspaceID string         `json:"cmux_workspace_id"`
	Items           []TodoItemBase `json:"items"`
	SyncedAt        string         `json:"synced_at"`
}

type File struct {
	Version    int                         `json:"version"`
	RootGroup  *GroupMapping               `json:"root_group,omitempty"`
	Workspaces map[string]WorkspaceMapping `json:"workspaces"`
	Todos      map[string]TodoMapping      `json:"todos,omitempty"`
}

type Store struct {
	path string
}

func NewStore(root string) Store {
	return Store{path: MappingPath(root)}
}

func MappingPath(root string) string {
	return filepath.Join(root, ".kra", "state", fileName)
}

func (s Store) Load() (File, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultFile(), nil
		}
		return File{}, fmt.Errorf("read cmux mapping: %w", err)
	}

	var out File
	if err := json.Unmarshal(data, &out); err != nil {
		return File{}, fmt.Errorf("parse cmux mapping: %w", err)
	}
	if err := normalize(&out); err != nil {
		return File{}, err
	}
	return out, nil
}

// AcquireLock serializes read-modify-write mapping transactions across kra
// processes. The lock file is intentionally retained: the operating system
// owns the lock lifetime and releases it if the process exits.
func (s Store) AcquireLock(ctx context.Context) (func(), error) {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create cmux mapping dir: %w", err)
	}
	lockPath := filepath.Join(dir, operationsDir, "cmux-workspaces.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create cmux mapping lock dir: %w", err)
	}
	release, err := filelock.Acquire(ctx, lockPath)
	if err != nil {
		return nil, fmt.Errorf("lock cmux mapping: %w", err)
	}
	return release, nil
}

func (s Store) Save(in File) error {
	if err := normalize(&in); err != nil {
		return err
	}

	data, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cmux mapping: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create cmux mapping dir: %w", err)
	}

	tmpDir := filepath.Join(filepath.Dir(s.path), operationsDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return fmt.Errorf("create cmux mapping temp dir: %w", err)
	}
	tmpFile, err := os.CreateTemp(tmpDir, fileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("create cmux mapping temp: %w", err)
	}
	tmp := tmpFile.Name()
	defer func() { _ = os.Remove(tmp) }()
	if err := tmpFile.Chmod(0o644); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("chmod cmux mapping temp: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write cmux mapping temp: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close cmux mapping temp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace cmux mapping: %w", err)
	}
	return nil
}

func defaultFile() File {
	return File{
		Version:    CurrentVersion,
		Workspaces: map[string]WorkspaceMapping{},
		Todos:      map[string]TodoMapping{},
	}
}

func normalize(in *File) error {
	if in.Version == 0 || in.Version == 1 {
		in.Version = CurrentVersion
	}
	if in.Version != CurrentVersion {
		return fmt.Errorf("unsupported cmux mapping version: %d", in.Version)
	}
	if in.Workspaces == nil {
		in.Workspaces = map[string]WorkspaceMapping{}
	}
	if in.Todos == nil {
		in.Todos = map[string]TodoMapping{}
	}
	if in.RootGroup != nil {
		in.RootGroup.CMUXWindowID = strings.TrimSpace(in.RootGroup.CMUXWindowID)
		in.RootGroup.CMUXGroupID = strings.TrimSpace(in.RootGroup.CMUXGroupID)
		in.RootGroup.AnchorWorkspaceID = strings.TrimSpace(in.RootGroup.AnchorWorkspaceID)
		in.RootGroup.NameSnapshot = strings.TrimSpace(in.RootGroup.NameSnapshot)
		in.RootGroup.CreatedAt = strings.TrimSpace(in.RootGroup.CreatedAt)
		in.RootGroup.LastUsedAt = strings.TrimSpace(in.RootGroup.LastUsedAt)
		missing := make([]string, 0, 3)
		if in.RootGroup.CMUXWindowID == "" {
			missing = append(missing, "cmux_window_id")
		}
		if in.RootGroup.CMUXGroupID == "" {
			missing = append(missing, "cmux_group_id")
		}
		if in.RootGroup.AnchorWorkspaceID == "" {
			missing = append(missing, "anchor_workspace_id")
		}
		if len(missing) > 0 {
			return fmt.Errorf("invalid cmux mapping root_group: missing %s", strings.Join(missing, ", "))
		}
	}

	for wsID, ws := range in.Workspaces {
		if ws.Entries == nil {
			ws.Entries = []Entry{}
		}

		sort.SliceStable(ws.Entries, func(i, j int) bool {
			if ws.Entries[i].Ordinal != ws.Entries[j].Ordinal {
				return ws.Entries[i].Ordinal < ws.Entries[j].Ordinal
			}
			return ws.Entries[i].CMUXWorkspaceID < ws.Entries[j].CMUXWorkspaceID
		})

		maxOrdinal := 0
		for i := range ws.Entries {
			if ws.Entries[i].Ordinal < 1 {
				ws.Entries[i].Ordinal = i + 1
			}
			if ws.Entries[i].Ordinal > maxOrdinal {
				maxOrdinal = ws.Entries[i].Ordinal
			}
		}
		if ws.NextOrdinal < 1 {
			ws.NextOrdinal = 1
		}
		if ws.NextOrdinal <= maxOrdinal {
			ws.NextOrdinal = maxOrdinal + 1
		}
		in.Workspaces[wsID] = ws
	}
	for workspaceID, todo := range in.Todos {
		normalizedWorkspaceID := strings.TrimSpace(workspaceID)
		if normalizedWorkspaceID == "" {
			return fmt.Errorf("invalid cmux mapping Todo base: empty workspace id")
		}
		if normalizedWorkspaceID != workspaceID {
			return fmt.Errorf("invalid cmux mapping Todo base %q: workspace id has surrounding whitespace", workspaceID)
		}
		todo.CMUXWorkspaceID = strings.TrimSpace(todo.CMUXWorkspaceID)
		todo.SyncedAt = strings.TrimSpace(todo.SyncedAt)
		if todo.CMUXWorkspaceID == "" {
			return fmt.Errorf("invalid cmux mapping Todo base %q: missing cmux_workspace_id", workspaceID)
		}
		if todo.Items == nil {
			todo.Items = []TodoItemBase{}
		}
		seenTaskIDs := make(map[string]struct{}, len(todo.Items))
		seenCMUXItemIDs := make(map[string]struct{}, len(todo.Items))
		items := make([]TodoItemBase, 0, len(todo.Items))
		for i, item := range todo.Items {
			item.KRATaskID = strings.TrimSpace(item.KRATaskID)
			item.CMUXItemID = strings.TrimSpace(item.CMUXItemID)
			item.KRAStatus = strings.TrimSpace(item.KRAStatus)
			item.CMUXState = strings.TrimSpace(item.CMUXState)
			item.Text = strings.TrimSpace(item.Text)
			missing := make([]string, 0, 5)
			if item.KRATaskID == "" {
				missing = append(missing, "kra_task_id")
			}
			if item.CMUXItemID == "" {
				missing = append(missing, "cmux_item_id")
			}
			if item.KRAStatus == "" {
				missing = append(missing, "kra_status")
			}
			if item.CMUXState == "" {
				missing = append(missing, "cmux_state")
			}
			if item.Text == "" {
				missing = append(missing, "text")
			}
			if len(missing) > 0 {
				return fmt.Errorf(
					"invalid cmux mapping Todo base %q item %d: missing %s",
					workspaceID,
					i,
					strings.Join(missing, ", "),
				)
			}
			if _, exists := seenTaskIDs[item.KRATaskID]; exists {
				return fmt.Errorf(
					"invalid cmux mapping Todo base %q: duplicate kra_task_id %q",
					workspaceID,
					item.KRATaskID,
				)
			}
			if _, exists := seenCMUXItemIDs[item.CMUXItemID]; exists {
				return fmt.Errorf(
					"invalid cmux mapping Todo base %q: duplicate cmux_item_id %q",
					workspaceID,
					item.CMUXItemID,
				)
			}
			seenTaskIDs[item.KRATaskID] = struct{}{}
			seenCMUXItemIDs[item.CMUXItemID] = struct{}{}
			items = append(items, item)
		}
		todo.Items = items
		in.Todos[workspaceID] = todo
	}
	return nil
}
