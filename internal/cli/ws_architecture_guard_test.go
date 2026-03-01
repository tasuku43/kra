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

var wsFlowRequiredHandlers = []string{
	"runWSClose",
	"runWSReopen",
	"runWSPurge",
}

func TestWSArchitectureGuard_WSFlowHandlersMustUseSharedFlow(t *testing.T) {
	shapes := loadWSRunFunctionShapes(t)

	missing := make([]string, 0, len(wsFlowRequiredHandlers))
	noSharedFlow := make([]string, 0, len(wsFlowRequiredHandlers))
	directSelectorCalls := make([]string, 0, len(wsFlowRequiredHandlers))
	for _, fn := range wsFlowRequiredHandlers {
		shape, ok := shapes[fn]
		if !ok {
			missing = append(missing, fn)
			continue
		}
		if !shape.usesSharedFlow {
			noSharedFlow = append(noSharedFlow, fn)
		}
		if shape.callsRunWorkspaceSelector {
			directSelectorCalls = append(directSelectorCalls, fn)
		}
	}

	if len(missing) > 0 {
		slices.Sort(missing)
		t.Fatalf("ws flow handler not found: %s", strings.Join(missing, ", "))
	}
	if len(noSharedFlow) > 0 {
		slices.Sort(noSharedFlow)
		t.Fatalf("ws flow handlers must call runWorkspaceSelectRiskResultFlow: %s", strings.Join(noSharedFlow, ", "))
	}
	if len(directSelectorCalls) > 0 {
		slices.Sort(directSelectorCalls)
		t.Fatalf("ws flow handlers must not call runWorkspaceSelector directly: %s", strings.Join(directSelectorCalls, ", "))
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

func TestWSArchitectureGuard_RiskResultRenderersMustUseSectionAtoms(t *testing.T) {
	// Keep section heading/body/trailing-blank contract centralized via printSection.
	required := map[string]string{
		"printWorkspaceFlowAbortedResult": "printSection",
		"printWorkspaceFlowResult":        "printSection",
		"printRiskSection":                "printSection",
		"printPurgeRiskSection":           "printSection",
	}
	forbidden := []string{"Fprint", "Fprintf", "Fprintln"}

	calls := loadCLIFunctionCalls(t)
	var violations []string
	for fn, mustCall := range required {
		fnCalls, ok := calls[fn]
		if !ok {
			violations = append(violations, fn+" (function not found)")
			continue
		}
		if !fnCalls[mustCall] {
			violations = append(violations, fn+" (missing call: "+mustCall+")")
		}
		for _, ban := range forbidden {
			if fnCalls[ban] {
				violations = append(violations, fn+" (forbidden direct output call: "+ban+")")
			}
		}
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("section-atom architecture violation: %s", strings.Join(violations, ", "))
	}
}

func loadWSRunFunctionShapes(t *testing.T) map[string]wsRunFunctionShape {
	t.Helper()

	pkg := parseCLIPackage(t)
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

func loadCLIFunctionCalls(t *testing.T) map[string]map[string]bool {
	t.Helper()

	pkg := parseCLIPackage(t)
	calls := make(map[string]map[string]bool, 16)
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			callSet := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := calledName(call.Fun)
				if strings.TrimSpace(name) != "" {
					callSet[name] = true
				}
				return true
			})
			calls[fn.Name.Name] = callSet
		}
	}
	return calls
}

func parseCLIPackage(t *testing.T) *ast.Package {
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
	return pkg
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
