package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type workspaceOutputCoverage string

const (
	workspaceOutputCoverageEmpty         workspaceOutputCoverage = "empty"
	workspaceOutputCoverageNotesOnly     workspaceOutputCoverage = "notes-only"
	workspaceOutputCoverageArtifactsOnly workspaceOutputCoverage = "artifacts-only"
	workspaceOutputCoverageDocumented    workspaceOutputCoverage = "documented"
)

type workspaceOutputCoverageSummary struct {
	Coverage       workspaceOutputCoverage
	NotesCount     int
	ArtifactsCount int
}

func deriveWorkspaceOutputCoverage(root string, scope string, workspaceID string) (workspaceOutputCoverageSummary, error) {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if scope == "archived" {
		wsPath = filepath.Join(root, "archive", workspaceID)
	}
	return deriveWorkspaceOutputCoverageForPath(wsPath)
}

func deriveWorkspaceOutputCoverageForPath(wsPath string) (workspaceOutputCoverageSummary, error) {
	notesCount, err := countWorkspaceOutputCoverageFiles(wsPath, "notes")
	if err != nil {
		return workspaceOutputCoverageSummary{}, err
	}
	artifactsCount, err := countWorkspaceOutputCoverageFiles(wsPath, "artifacts")
	if err != nil {
		return workspaceOutputCoverageSummary{}, err
	}
	return workspaceOutputCoverageSummary{
		Coverage:       classifyWorkspaceOutputCoverage(notesCount, artifactsCount),
		NotesCount:     notesCount,
		ArtifactsCount: artifactsCount,
	}, nil
}

func countWorkspaceOutputCoverageFiles(wsPath string, subdir string) (int, error) {
	basePath := filepath.Join(wsPath, subdir)
	if fi, err := os.Stat(basePath); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	} else if !fi.IsDir() {
		return 0, nil
	}

	count := 0
	err := filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(wsPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if isWorkspaceOutputCoverageScaffolding(rel) {
			return nil
		}
		count++
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func classifyWorkspaceOutputCoverage(notesCount int, artifactsCount int) workspaceOutputCoverage {
	switch {
	case notesCount > 0 && artifactsCount > 0:
		return workspaceOutputCoverageDocumented
	case notesCount > 0:
		return workspaceOutputCoverageNotesOnly
	case artifactsCount > 0:
		return workspaceOutputCoverageArtifactsOnly
	default:
		return workspaceOutputCoverageEmpty
	}
}

func isWorkspaceOutputCoverageScaffolding(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(filepath.Clean(rel)))
	if rel == "." || rel == "" {
		return true
	}
	base := filepath.Base(rel)
	switch base {
	case "AGENTS.md", "CLAUDE.md", workspaceMetaFilename:
		return true
	}
	return rel == ".claude/settings.local.json" || strings.HasSuffix(rel, "/.claude/settings.local.json")
}
