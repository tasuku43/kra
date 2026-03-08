package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveWorkspaceOutputCoverageForPath_ClassifiesAllStates(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, wsPath string)
		wantCoverage  workspaceOutputCoverage
		wantNotes     int
		wantArtifacts int
	}{
		{
			name:          "empty",
			setup:         func(t *testing.T, wsPath string) {},
			wantCoverage:  workspaceOutputCoverageEmpty,
			wantNotes:     0,
			wantArtifacts: 0,
		},
		{
			name: "notes-only",
			setup: func(t *testing.T, wsPath string) {
				mustWriteCoverageFile(t, filepath.Join(wsPath, "notes", "summary.md"))
			},
			wantCoverage:  workspaceOutputCoverageNotesOnly,
			wantNotes:     1,
			wantArtifacts: 0,
		},
		{
			name: "artifacts-only",
			setup: func(t *testing.T, wsPath string) {
				mustWriteCoverageFile(t, filepath.Join(wsPath, "artifacts", "capture.txt"))
			},
			wantCoverage:  workspaceOutputCoverageArtifactsOnly,
			wantNotes:     0,
			wantArtifacts: 1,
		},
		{
			name: "documented",
			setup: func(t *testing.T, wsPath string) {
				mustWriteCoverageFile(t, filepath.Join(wsPath, "notes", "summary.md"))
				mustWriteCoverageFile(t, filepath.Join(wsPath, "artifacts", "capture.txt"))
			},
			wantCoverage:  workspaceOutputCoverageDocumented,
			wantNotes:     1,
			wantArtifacts: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wsPath := t.TempDir()
			tc.setup(t, wsPath)

			got, err := deriveWorkspaceOutputCoverageForPath(wsPath)
			if err != nil {
				t.Fatalf("deriveWorkspaceOutputCoverageForPath() error = %v", err)
			}
			if got.Coverage != tc.wantCoverage {
				t.Fatalf("coverage = %q, want %q", got.Coverage, tc.wantCoverage)
			}
			if got.NotesCount != tc.wantNotes {
				t.Fatalf("notes_count = %d, want %d", got.NotesCount, tc.wantNotes)
			}
			if got.ArtifactsCount != tc.wantArtifacts {
				t.Fatalf("artifacts_count = %d, want %d", got.ArtifactsCount, tc.wantArtifacts)
			}
		})
	}
}

func TestDeriveWorkspaceOutputCoverageForPath_IgnoresScaffolding(t *testing.T) {
	wsPath := t.TempDir()
	mustWriteCoverageFile(t, filepath.Join(wsPath, "notes", "AGENTS.md"))
	mustWriteCoverageFile(t, filepath.Join(wsPath, "artifacts", "CLAUDE.md"))
	mustWriteCoverageFile(t, filepath.Join(wsPath, "artifacts", ".claude", "settings.local.json"))

	got, err := deriveWorkspaceOutputCoverageForPath(wsPath)
	if err != nil {
		t.Fatalf("deriveWorkspaceOutputCoverageForPath() error = %v", err)
	}
	if got.Coverage != workspaceOutputCoverageEmpty {
		t.Fatalf("coverage = %q, want %q", got.Coverage, workspaceOutputCoverageEmpty)
	}
	if got.NotesCount != 0 || got.ArtifactsCount != 0 {
		t.Fatalf("counts = %d/%d, want 0/0", got.NotesCount, got.ArtifactsCount)
	}
}

func mustWriteCoverageFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
