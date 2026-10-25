package localization

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCandidateTransferOwnedDiagnosticsHavePairedExactSources(t *testing.T) {
	files, err := filepath.Glob("../platform/candidate_transfer*.go")
	if err != nil || len(files) == 0 {
		t.Fatal("candidate source scan unavailable", err)
	}
	count := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			name, ok := call.Fun.(*ast.Ident)
			if !ok || name.Name != "invalid" && name.Name != "conflict" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			source, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			key, found := LookupSource(source)
			if !found || !strings.HasPrefix(key, "candidate_transfer.") {
				t.Errorf("owned candidate diagnostic lacks an exact domain key: %q", source)
				return true
			}
			for _, locale := range []Locale{ZhCN, EnUS} {
				out, err := Format(locale, key, nil)
				if err != nil || !out.Localized || out.Text == "" {
					t.Errorf("missing candidate translation: %s/%s", key, locale)
				}
			}
			count++
			return true
		})
	}
	if count < 19 {
		t.Fatal("source coverage unexpectedly incomplete", count)
	}
	if _, found := LookupSource("Candidate transfer lease is no longer current: private-value"); found {
		t.Fatal("unowned appended evidence was inferred as a diagnostic")
	}
}
