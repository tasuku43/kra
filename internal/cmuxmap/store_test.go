package cmuxmap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMappingPath(t *testing.T) {
	root := "/tmp/kra-root"
	got := MappingPath(root)
	want := filepath.Join(root, ".kra", "state", "cmux-workspaces.json")
	if got != want {
		t.Fatalf("MappingPath() = %q, want %q", got, want)
	}
}

func TestStoreAcquireLock_SerializesAndHonorsCancellation(t *testing.T) {
	store := NewStore(t.TempDir())
	release, err := store.AcquireLock(context.Background())
	if err != nil {
		t.Fatalf("AcquireLock(first) error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := store.AcquireLock(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AcquireLock(contended) error = %v, want deadline exceeded", err)
	}

	release()
	release()
	releaseAgain, err := store.AcquireLock(context.Background())
	if err != nil {
		t.Fatalf("AcquireLock(after release) error: %v", err)
	}
	releaseAgain()
}

func TestStoreAcquireLock_AlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewStore(t.TempDir()).AcquireLock(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("AcquireLock() error = %v, want context canceled", err)
	}
}

func TestStoreLoad_NotExist_ReturnsDefault(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got.Version != CurrentVersion {
		t.Fatalf("version = %d, want %d", got.Version, CurrentVersion)
	}
	if got.Workspaces == nil {
		t.Fatalf("workspaces should not be nil")
	}
	if len(got.Workspaces) != 0 {
		t.Fatalf("workspaces len = %d, want 0", len(got.Workspaces))
	}
}

func TestStoreSaveLoad_RoundTripAndNormalize(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)

	in := File{
		Version: CurrentVersion,
		Workspaces: map[string]WorkspaceMapping{
			"WS1": {
				NextOrdinal: 0,
				Entries: []Entry{
					{CMUXWorkspaceID: "B", Ordinal: 2},
					{CMUXWorkspaceID: "A", Ordinal: 0},
				},
			},
		},
	}
	if err := s.Save(in); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	ws, ok := got.Workspaces["WS1"]
	if !ok {
		t.Fatalf("workspace WS1 not found")
	}
	if ws.Entries[0].CMUXWorkspaceID != "A" || ws.Entries[0].Ordinal != 1 {
		t.Fatalf("first entry = %+v, want id=A ordinal=1", ws.Entries[0])
	}
	if ws.NextOrdinal != 3 {
		t.Fatalf("next_ordinal = %d, want 3", ws.NextOrdinal)
	}

	if _, err := os.Stat(MappingPath(root)); err != nil {
		t.Fatalf("mapping file stat error: %v", err)
	}
}

func TestStoreSave_DoesNotUseLegacyFixedTemp(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	path := MappingPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	legacyTemp := path + ".tmp"
	const sentinel = "legacy temp must remain untouched"
	if err := os.WriteFile(legacyTemp, []byte(sentinel), 0o644); err != nil {
		t.Fatalf("WriteFile(legacy temp) error: %v", err)
	}

	if err := store.Save(defaultFile()); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	got, err := os.ReadFile(legacyTemp)
	if err != nil {
		t.Fatalf("ReadFile(legacy temp) error: %v", err)
	}
	if string(got) != sentinel {
		t.Fatalf("legacy temp = %q, want unchanged sentinel", got)
	}
	if _, err := store.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
}

func TestStoreLoad_UnsupportedVersionErrors(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	path := MappingPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":99,"workspaces":{}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	_, err := s.Load()
	if err == nil {
		t.Fatalf("Load() error = nil, want unsupported version error")
	}
}

func TestStoreLoad_MigratesVersionOneAndNormalizesNativeState(t *testing.T) {
	root := t.TempDir()
	path := MappingPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	legacy := `{"version":1,"workspaces":{"WS1":{"next_ordinal":1,"entries":[]}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	got, err := NewStore(root).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got.Version != CurrentVersion {
		t.Fatalf("version = %d, want %d", got.Version, CurrentVersion)
	}
	if got.RootGroup != nil {
		t.Fatalf("root group = %#v, want nil", got.RootGroup)
	}
	if got.Todos == nil {
		t.Fatal("todos must be normalized to an empty map")
	}
}

func TestStoreLoad_IncompleteRootGroupFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "missing window",
			json: `{"version":2,"workspaces":{},"root_group":{"cmux_group_id":"group-id","anchor_workspace_id":"anchor-id"}}`,
			want: "cmux_window_id",
		},
		{
			name: "missing group",
			json: `{"version":2,"workspaces":{},"root_group":{"cmux_window_id":"window-id","anchor_workspace_id":"anchor-id"}}`,
			want: "cmux_group_id",
		},
		{
			name: "missing anchor",
			json: `{"version":2,"workspaces":{},"root_group":{"cmux_window_id":"window-id","cmux_group_id":"group-id"}}`,
			want: "anchor_workspace_id",
		},
		{
			name: "whitespace identity",
			json: `{"version":2,"workspaces":{},"root_group":{"cmux_window_id":"window-id","cmux_group_id":"   ","anchor_workspace_id":"anchor-id"}}`,
			want: "cmux_group_id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := writeMappingJSON(t, tt.json)
			_, err := store.Load()
			if err == nil || !strings.Contains(err.Error(), "invalid cmux mapping root_group") || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %v, want fail-closed root_group error containing %q", err, tt.want)
			}
		})
	}
}

func TestStoreLoad_InvalidTodoBaseFailsClosed(t *testing.T) {
	const validItem = `{"kra_task_id":"TASK-001","cmux_item_id":"item-1","kra_status":"todo","cmux_state":"pending","text":"TASK-001 work"}`
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "empty logical workspace id",
			json: `{"version":2,"workspaces":{},"todos":{"":{"cmux_workspace_id":"workspace-id","items":[]}}}`,
			want: "empty workspace id",
		},
		{
			name: "logical workspace id whitespace",
			json: `{"version":2,"workspaces":{},"todos":{" WS1 ":{"cmux_workspace_id":"workspace-id","items":[]}}}`,
			want: "surrounding whitespace",
		},
		{
			name: "missing cmux workspace id",
			json: `{"version":2,"workspaces":{},"todos":{"WS1":{"items":[]}}}`,
			want: "cmux_workspace_id",
		},
		{
			name: "missing task id",
			json: `{"version":2,"workspaces":{},"todos":{"WS1":{"cmux_workspace_id":"workspace-id","items":[{"cmux_item_id":"item-1","kra_status":"todo","cmux_state":"pending","text":"work"}]}}}`,
			want: "kra_task_id",
		},
		{
			name: "missing item id",
			json: `{"version":2,"workspaces":{},"todos":{"WS1":{"cmux_workspace_id":"workspace-id","items":[{"kra_task_id":"TASK-001","kra_status":"todo","cmux_state":"pending","text":"work"}]}}}`,
			want: "cmux_item_id",
		},
		{
			name: "missing status state and text",
			json: `{"version":2,"workspaces":{},"todos":{"WS1":{"cmux_workspace_id":"workspace-id","items":[{"kra_task_id":"TASK-001","cmux_item_id":"item-1"}]}}}`,
			want: "kra_status, cmux_state, text",
		},
		{
			name: "duplicate task id",
			json: `{"version":2,"workspaces":{},"todos":{"WS1":{"cmux_workspace_id":"workspace-id","items":[` + validItem + `,{"kra_task_id":"TASK-001","cmux_item_id":"item-2","kra_status":"todo","cmux_state":"pending","text":"TASK-001 other"}]}}}`,
			want: "duplicate kra_task_id",
		},
		{
			name: "duplicate cmux item id",
			json: `{"version":2,"workspaces":{},"todos":{"WS1":{"cmux_workspace_id":"workspace-id","items":[` + validItem + `,{"kra_task_id":"TASK-002","cmux_item_id":"item-1","kra_status":"done","cmux_state":"completed","text":"TASK-002 other"}]}}}`,
			want: "duplicate cmux_item_id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := writeMappingJSON(t, tt.json)
			_, err := store.Load()
			if err == nil || !strings.Contains(err.Error(), "invalid cmux mapping Todo base") || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %v, want fail-closed Todo base error containing %q", err, tt.want)
			}
		})
	}
}

func TestStoreSave_InvalidNativeOwnershipDoesNotReplaceExistingMapping(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*File)
		want   string
	}{
		{
			name: "incomplete root group",
			mutate: func(file *File) {
				file.RootGroup = &GroupMapping{CMUXWindowID: "window-id", CMUXGroupID: "group-id"}
			},
			want: "anchor_workspace_id",
		},
		{
			name: "incomplete Todo item",
			mutate: func(file *File) {
				file.Todos["WS1"] = TodoMapping{
					CMUXWorkspaceID: "workspace-id",
					Items: []TodoItemBase{{
						KRATaskID:  "TASK-001",
						CMUXItemID: "item-1",
					}},
				}
			},
			want: "kra_status, cmux_state, text",
		},
		{
			name: "duplicate Todo item ownership",
			mutate: func(file *File) {
				file.Todos["WS1"] = TodoMapping{
					CMUXWorkspaceID: "workspace-id",
					Items: []TodoItemBase{
						{KRATaskID: "TASK-001", CMUXItemID: "item-1", KRAStatus: "todo", CMUXState: "pending", Text: "TASK-001 one"},
						{KRATaskID: "TASK-001", CMUXItemID: "item-2", KRAStatus: "done", CMUXState: "completed", Text: "TASK-001 two"},
					},
				}
			},
			want: "duplicate kra_task_id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			if err := store.Save(defaultFile()); err != nil {
				t.Fatalf("seed Save() error: %v", err)
			}
			before, err := os.ReadFile(store.path)
			if err != nil {
				t.Fatalf("read seed mapping: %v", err)
			}

			invalid := defaultFile()
			tt.mutate(&invalid)
			err = store.Save(invalid)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Save() error = %v, want %q", err, tt.want)
			}
			after, err := os.ReadFile(store.path)
			if err != nil {
				t.Fatalf("read mapping after rejected Save: %v", err)
			}
			if string(after) != string(before) {
				t.Fatalf("rejected Save replaced mapping:\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func TestStoreSave_RoundTripsGroupAndTodoBase(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	in := defaultFile()
	in.RootGroup = &GroupMapping{
		CMUXWindowID:      "window-id",
		CMUXGroupID:       "group-id",
		AnchorWorkspaceID: "anchor-id",
		NameSnapshot:      "kra",
	}
	in.Todos["WS1"] = TodoMapping{
		CMUXWorkspaceID: "workspace-id",
		SyncedAt:        "2026-08-13T00:00:00Z",
		Items: []TodoItemBase{{
			KRATaskID:  "TASK-001",
			CMUXItemID: "todo-id",
			KRAStatus:  "doing",
			CMUXState:  "in-progress",
			Text:       "TASK-001 test",
		}},
	}
	if err := store.Save(in); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got.RootGroup == nil || got.RootGroup.CMUXGroupID != "group-id" || got.RootGroup.AnchorWorkspaceID != "anchor-id" {
		t.Fatalf("root group = %#v", got.RootGroup)
	}
	items := got.Todos["WS1"].Items
	if len(items) != 1 || items[0].CMUXItemID != "todo-id" || items[0].KRATaskID != "TASK-001" {
		t.Fatalf("todo items = %#v", items)
	}
}

func writeMappingJSON(t *testing.T, content string) Store {
	t.Helper()
	root := t.TempDir()
	path := MappingPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	return NewStore(root)
}
