package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type wsRunFunctionShape struct {
	usesPromptWorkspaceSelector bool
	usesSharedFlow              bool
	callsRunWorkspaceSelector   bool
}

func TestWSArchitectureGuard_RunWSCloseMustUseSharedFlow(t *testing.T) {
	shapes := loadWSRunFunctionShapes(t)
	shape, ok := shapes["runWSClose"]
	if !ok {
		t.Fatalf("runWSClose not found")
	}
	if !shape.usesSharedFlow {
		t.Fatalf("runWSClose must call runWorkspaceSelectRiskResultFlow")
	}
}

func TestWSArchitectureGuard_SelectorHandlersMustUseSharedFlow(t *testing.T) {
	shapes := loadWSRunFunctionShapes(t)
	var violators []string

	for fn, shape := range shapes {
		if shape.usesPromptWorkspaceSelector && !shape.usesSharedFlow {
			violators = append(violators, fn)
		}
		if shape.callsRunWorkspaceSelector {
			violators = append(violators, fn+" (calls runWorkspaceSelector directly)")
		}
	}

	if len(violators) > 0 {
		slices.Sort(violators)
		t.Fatalf("selector architecture violation: %s", strings.Join(violators, ", "))
	}
}

func loadWSRunFunctionShapes(t *testing.T) map[string]wsRunFunctionShape {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	cliDir := filepath.Dir(currentFile)

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, cliDir, func(fi fs.FileInfo) bool {
		name := fi.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse cli package: %v", err)
	}

	pkg, ok := pkgs["cli"]
	if !ok {
		t.Fatalf("cli package not found in %s", cliDir)
	}

	shapes := make(map[string]wsRunFunctionShape, 8)
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if !strings.HasPrefix(fn.Name.Name, "runWS") {
				continue
			}

			shape := wsRunFunctionShape{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				called := calledName(call.Fun)
				switch called {
				case "promptWorkspaceSelector":
					shape.usesPromptWorkspaceSelector = true
				case "runWorkspaceSelectRiskResultFlow":
					shape.usesSharedFlow = true
				case "runWorkspaceSelector":
					shape.callsRunWorkspaceSelector = true
				}
				return true
			})
			shapes[fn.Name.Name] = shape
		}
	}
	return shapes
}

func calledName(fun ast.Expr) string {
	switch v := fun.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	default:
		return ""
	}
}
