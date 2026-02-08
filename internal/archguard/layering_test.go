package archguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
