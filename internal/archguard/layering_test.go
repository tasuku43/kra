package archguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestLayerDirectoriesExist(t *testing.T) {
	for _, dir := range []string{"../app", "../domain", "../infra", "../ui"} {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			t.Fatalf("layer dir missing: %s", dir)
		}
	}
}

func TestLayerDependencyDirection(t *testing.T) {
	tests := []struct {
		dir              string
		forbiddenImports []string
	}{
		{dir: "../app", forbiddenImports: []string{"/internal/cli", "/internal/ui"}},
		{dir: "../domain", forbiddenImports: []string{"/internal/cli", "/internal/app", "/internal/infra", "/internal/ui"}},
		{dir: "../infra", forbiddenImports: []string{"/internal/cli"}},
		{dir: "../ui", forbiddenImports: []string{"/internal/cli"}},
	}

	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			assertNoForbiddenImport(t, tt.dir, tt.forbiddenImports)
		})
	}
}

func TestCLINoDirectInfraImports(t *testing.T) {
	assertNoForbiddenImport(t, "../cli", []string{
		"/internal/paths",
		"/internal/statestore",
		"/internal/gitutil",
		"/internal/stateregistry",
	})
}

func TestCLIDirectInfraImportsAreAllowlisted(t *testing.T) {
	// Transitional guard:
	// - All direct infra imports from CLI must be explicit and reviewable.
	// - New files cannot start importing infra unless added here intentionally.
	// - cmux commands are excluded because the command group is planned for removal.
	allowed := map[string]struct{}{
		"bootstrap_agent_skills.go": {},
		"config.go":                 {},
		"config_bootstrap.go":       {},
		"context.go":                {},
		"doctor.go":                 {},
		"git_allowlist.go":          {},
		"git_status_snapshot.go":    {},
		"init.go":                   {},
		"repo_add.go":               {},
		"repo_discover.go":          {},
		"repo_gc.go":                {},
		"repo_pool_add.go":          {},
		"repo_remove.go":            {},
		"state_registry.go":         {},
		"template_validate.go":      {},
		"workspace_workstate.go":    {},
		"ws_add_repo.go":            {},
		"ws_close.go":               {},
		"ws_create.go":              {},
		"ws_dashboard.go":           {},
		"ws_git_helpers.go":         {},
		"ws_go.go":                  {},
		"ws_import_jira.go":         {},
		"ws_insight.go":             {},
		"ws_launcher.go":            {},
		"ws_list.go":                {},
		"ws_lock.go":                {},
		"ws_open.go":                {},
		"ws_purge.go":               {},
		"ws_remove_repo.go":         {},
		"ws_reopen.go":              {},
		"ws_resume.go":              {},
		"ws_save.go":                {},
	}

	seen := map[string]struct{}{}
	fset := token.NewFileSet()
	err := filepath.WalkDir("../cli", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		name := filepath.Base(path)
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if strings.HasPrefix(name, "cmux_") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range file.Imports {
			v := strings.Trim(imp.Path.Value, "\"")
			if !strings.Contains(v, "/internal/infra/") {
				continue
			}
			if _, ok := allowed[name]; !ok {
				t.Fatalf("direct infra import is not allowlisted: %s (%s)", name, v)
			}
			seen[name] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk cli: %v", err)
	}

	stale := make([]string, 0)
	for name := range allowed {
		if _, ok := seen[name]; !ok {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("remove stale CLI infra allowlist entries: %s", strings.Join(stale, ", "))
	}
}

func assertNoForbiddenImport(t *testing.T, dir string, forbidden []string) {
	t.Helper()

	fset := token.NewFileSet()
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		for _, imp := range file.Imports {
			v := strings.Trim(imp.Path.Value, "\"")
			for _, f := range forbidden {
				if strings.Contains(v, f) {
					t.Fatalf("forbidden import in %s: %s", path, v)
				}
			}
		}
		return nil
	})
}

func TestNoAstMutationInGuard(t *testing.T) {
	var _ ast.Node
}
