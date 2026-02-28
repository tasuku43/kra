package cmuxsession

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionPath(t *testing.T) {
	root := "/tmp/kra-root"
	got := SessionPath(root)
	want := filepath.Join(root, ".kra", "state", "cmux-sessions.json")
	if got != want {
		t.Fatalf("SessionPath() = %q, want %q", got, want)
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
		Workspaces: map[string]WorkspaceSessions{
			"WS1": {
				Sessions: []Entry{
					{
						SessionID: "older",
						CreatedAt: "2026-02-28T10:00:00Z",
						Path:      "workspaces/WS1/artifacts/cmux/sessions/older",
					},
					{
						SessionID: "newer",
						CreatedAt: "2026-02-28T11:00:00Z",
						Path:      "workspaces/WS1/artifacts/cmux/sessions/newer",
					},
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
	if len(ws.Sessions) != 2 {
		t.Fatalf("sessions len = %d, want 2", len(ws.Sessions))
	}
	if ws.Sessions[0].SessionID != "newer" {
		t.Fatalf("first session = %q, want %q", ws.Sessions[0].SessionID, "newer")
	}
	if _, err := os.Stat(SessionPath(root)); err != nil {
		t.Fatalf("session file stat error: %v", err)
	}
}

func TestStoreLoad_UnsupportedVersionErrors(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	path := SessionPath(root)
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

func TestStoreLoad_DuplicateSessionIDErrors(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	path := SessionPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	raw := `{
  "version": 1,
  "workspaces": {
    "WS1": {
      "sessions": [
        {"session_id":"dup","created_at":"2026-02-28T11:00:00Z","path":"a"},
        {"session_id":"dup","created_at":"2026-02-28T10:00:00Z","path":"b"}
      ]
    }
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	_, err := s.Load()
	if err == nil {
		t.Fatalf("Load() error = nil, want duplicate session_id error")
	}
}
