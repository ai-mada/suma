package commandlocale

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

// This bounded source gate covers only the three runtime command entry points.
// It prevents newly added command-owned human copy from bypassing paired text;
// raw errors and machine JSON remain explicitly outside the translation path.
func TestRuntimeCommandOwnedMessagesUsePairedDisplayBoundary(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	files := []string{"cmd/suma-control/main.go", "cmd/suma-runner/main.go", "cmd/suma-runner/pool.go", "cmd/suma-deployer/main.go"}
	for _, relative := range files {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(root, relative), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			receiver, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			checkPair := func(start int) {
				if len(call.Args) != start+2 {
					t.Errorf("%s: missing paired display arguments", fset.Position(call.Pos()))
					return
				}
				for _, arg := range call.Args[start:] {
					literal, ok := arg.(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						t.Errorf("%s: display copy must have an explicit paired source", fset.Position(call.Pos()))
						continue
					}
					value, err := strconv.Unquote(literal.Value)
					if err != nil || value == "" {
						t.Errorf("%s: empty display copy", fset.Position(call.Pos()))
					}
				}
			}
			if receiver.Name == "app" {
				switch selector.Sel.Name {
				case "Text", "Failure":
					checkPair(0)
				case "String", "Bool":
					checkPair(2)
				}
			}
			if receiver.Name == "logger" && (selector.Sel.Name == "Error" || selector.Sel.Name == "Info") {
				if len(call.Args) < 1 {
					t.Errorf("%s: missing log message", fset.Position(call.Pos()))
					return true
				}
				text, ok := call.Args[0].(*ast.CallExpr)
				if !ok {
					t.Errorf("%s: log message bypasses display boundary", fset.Position(call.Pos()))
					return true
				}
				textSelector, ok := text.Fun.(*ast.SelectorExpr)
				if !ok || textSelector.Sel.Name != "Text" {
					t.Errorf("%s: log message bypasses display boundary", fset.Position(call.Pos()))
				}
			}
			if receiver.Name == "log" || receiver.Name == "slog" && (selector.Sel.Name == "Error" || selector.Sel.Name == "Info") {
				t.Errorf("%s: global human logger bypasses per-invocation display", fset.Position(call.Pos()))
			}
			if receiver.Name == "fmt" {
				for _, arg := range call.Args {
					if literal, ok := arg.(*ast.BasicLit); ok && literal.Kind == token.STRING && literal.Value != `"%s"` {
						t.Errorf("%s: direct formatted human literal bypasses display boundary", fset.Position(call.Pos()))
					}
				}
			}
			return true
		})
	}
}
