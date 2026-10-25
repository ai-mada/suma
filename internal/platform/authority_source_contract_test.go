package platform

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type authoritySourceIndex struct {
	aliases map[string]string
	fields  map[string]map[string]string
	results map[string][]string
	parents map[ast.Node]ast.Node
}

func authoritySourceType(t ast.Expr) string {
	switch t := t.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return authoritySourceType(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.ArrayType:
		return "[]" + authoritySourceType(t.Elt)
	case *ast.MapType:
		return "map:" + authoritySourceType(t.Value)
	case *ast.ParenExpr:
		return authoritySourceType(t.X)
	}
	return ""
}

func newAuthoritySourceIndex(files []*ast.File) *authoritySourceIndex {
	i := &authoritySourceIndex{map[string]string{}, map[string]map[string]string{}, map[string][]string{}, map[ast.Node]ast.Node{}}
	for _, file := range files {
		var stack []ast.Node
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return false
			}
			if len(stack) > 0 {
				i.parents[n] = stack[len(stack)-1]
			}
			stack = append(stack, n)
			switch n := n.(type) {
			case *ast.TypeSpec:
				if shape, ok := n.Type.(*ast.StructType); ok {
					fields := map[string]string{}
					for _, field := range shape.Fields.List {
						for _, name := range field.Names {
							fields[name.Name] = authoritySourceType(field.Type)
						}
					}
					i.fields[n.Name.Name] = fields
				} else {
					i.aliases[n.Name.Name] = authoritySourceType(n.Type)
				}
			case *ast.FuncDecl:
				if n.Type.Results != nil {
					var out []string
					for _, r := range n.Type.Results.List {
						count := len(r.Names)
						if count == 0 {
							count = 1
						}
						for j := 0; j < count; j++ {
							out = append(out, authoritySourceType(r.Type))
						}
					}
					i.results[n.Name.Name] = out
				}
			}
			return true
		})
	}
	return i
}

func (i *authoritySourceIndex) canonical(name string) string {
	for n := 0; n < 20; n++ {
		next, ok := i.aliases[name]
		if !ok || next == name {
			return name
		}
		name = next
	}
	return name
}

func sourceInitializer(id *ast.Ident) (ast.Expr, int) {
	if id.Obj == nil {
		return nil, 0
	}
	switch d := id.Obj.Decl.(type) {
	case *ast.ValueSpec:
		for n, name := range d.Names {
			if name.Name == id.Name {
				if len(d.Values) == 1 {
					return d.Values[0], n
				}
				if n < len(d.Values) {
					return d.Values[n], 0
				}
			}
		}
	case *ast.AssignStmt:
		for n, name := range d.Lhs {
			if name, ok := name.(*ast.Ident); ok && name.Name == id.Name {
				if len(d.Rhs) == 1 {
					return d.Rhs[0], n
				}
				if n < len(d.Rhs) {
					return d.Rhs[n], 0
				}
			}
		}
	}
	return nil, 0
}

func (i *authoritySourceIndex) exprType(x ast.Expr, result int, seen map[*ast.Object]bool) string {
	switch x := x.(type) {
	case *ast.Ident:
		if x.Obj == nil || seen[x.Obj] {
			return ""
		}
		seen[x.Obj] = true
		switch d := x.Obj.Decl.(type) {
		case *ast.Field:
			return i.canonical(authoritySourceType(d.Type))
		case *ast.ValueSpec:
			if d.Type != nil {
				return i.canonical(authoritySourceType(d.Type))
			}
		}
		value, n := sourceInitializer(x)
		if value != nil {
			return i.exprType(value, n, seen)
		}
	case *ast.CompositeLit:
		return i.literalType(x)
	case *ast.UnaryExpr:
		return i.exprType(x.X, result, seen)
	case *ast.StarExpr:
		return i.exprType(x.X, result, seen)
	case *ast.ParenExpr:
		return i.exprType(x.X, result, seen)
	case *ast.TypeAssertExpr:
		return i.canonical(authoritySourceType(x.Type))
	case *ast.IndexExpr:
		name := i.exprType(x.X, 0, seen)
		name = strings.TrimPrefix(name, "[]")
		name = strings.TrimPrefix(name, "map:")
		return i.canonical(name)
	case *ast.SelectorExpr:
		return i.canonical(i.fields[i.exprType(x.X, 0, seen)][x.Sel.Name])
	case *ast.CallExpr:
		name := ""
		switch f := x.Fun.(type) {
		case *ast.Ident:
			name = f.Name
		case *ast.SelectorExpr:
			name = f.Sel.Name
		}
		if (name == "new" || name == "make") && len(x.Args) > 0 {
			return i.canonical(authoritySourceType(x.Args[0]))
		}
		if kind := i.canonical(name); kind == "Actor" || kind == "Principal" {
			return kind
		}
		if out := i.results[name]; result < len(out) {
			return i.canonical(out[result])
		}
	}
	return ""
}

func (i *authoritySourceIndex) literalType(lit *ast.CompositeLit) string {
	if lit.Type != nil {
		return i.canonical(authoritySourceType(lit.Type))
	}
	parent := i.parents[lit]
	field := ""
	if pair, ok := parent.(*ast.KeyValueExpr); ok {
		if key, ok := pair.Key.(*ast.Ident); ok {
			field = key.Name
		}
		parent = i.parents[pair]
	}
	if outer, ok := parent.(*ast.CompositeLit); ok {
		name := i.literalType(outer)
		if strings.HasPrefix(name, "[]") {
			return i.canonical(strings.TrimPrefix(name, "[]"))
		}
		if strings.HasPrefix(name, "map:") {
			return i.canonical(strings.TrimPrefix(name, "map:"))
		}
		return i.canonical(i.fields[name][field])
	}
	return ""
}

func authorityZeroExpression(x ast.Expr) bool {
	if pair, ok := x.(*ast.KeyValueExpr); ok {
		return authorityZeroExpression(pair.Value)
	}
	switch x := x.(type) {
	case *ast.Ident:
		return x.Name == "nil" || x.Name == "false" || x.Name == "actorInvalid"
	case *ast.BasicLit:
		if x.Kind == token.STRING {
			s, e := strconv.Unquote(x.Value)
			return e == nil && s == ""
		}
		if n, e := strconv.ParseInt(x.Value, 0, 64); e == nil {
			return n == 0
		}
		if n, e := strconv.ParseFloat(x.Value, 64); e == nil {
			return n == 0
		}
	case *ast.CompositeLit:
		for _, e := range x.Elts {
			if !authorityZeroExpression(e) {
				return false
			}
		}
		return true
	case *ast.ParenExpr:
		return authorityZeroExpression(x.X)
	}
	return false
}

func (i *authoritySourceIndex) identityPath(x ast.Expr, seen map[*ast.Object]bool) bool {
	switch x := x.(type) {
	case *ast.SelectorExpr:
		if i.exprType(x.X, 0, map[*ast.Object]bool{}) == "Actor" && (x.Sel.Name == "kind" || x.Sel.Name == "credential" || x.Sel.Name == "delegation") {
			return true
		}
		return i.identityPath(x.X, seen)
	case *ast.Ident:
		if x.Obj == nil || seen[x.Obj] {
			return false
		}
		seen[x.Obj] = true
		value, _ := sourceInitializer(x)
		if value != nil {
			return i.identityPath(value, seen)
		}
	case *ast.UnaryExpr:
		return i.identityPath(x.X, seen)
	case *ast.StarExpr:
		return i.identityPath(x.X, seen)
	case *ast.ParenExpr:
		return i.identityPath(x.X, seen)
	case *ast.IndexExpr:
		return i.identityPath(x.X, seen)
	}
	return false
}

func authorityLegacyHelper(name string) bool {
	switch name {
	case "Authorize", "authorizeProjectTx", "authorizeProjectSnapshotTx", "currentSchedulingSessionTx", "receipt", "saveReceipt", "activity":
		return true
	}
	return false
}

func (i *authoritySourceIndex) hasPrincipalParameter(f *ast.FuncDecl) bool {
	if f.Type.Params != nil {
		for _, p := range f.Type.Params.List {
			if i.canonical(authoritySourceType(p.Type)) == i.canonical("Principal") {
				return true
			}
		}
	}
	return false
}

func authoritySourceViolations(file *ast.File, filename string, index *authoritySourceIndex, fset *token.FileSet) []string {
	if file.Name.Name != "platform" {
		return nil
	}
	trusted := filename == "command_actor.go" || filename == "command_delegation.go" || filename == "credential_lifecycle.go"
	var issues []string
	add := func(n ast.Node, rule string) {
		issues = append(issues, fmt.Sprintf("%s:%d: %s", filename, fset.Position(n.Pos()).Line, rule))
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			kind := index.literalType(n)
			if kind == "Actor" && !trusted && !authorityZeroExpression(n) {
				add(n, "nonzero Actor construction outside trusted source")
			}
			if kind == index.canonical("Principal") {
				for _, e := range n.Elts {
					if pair, ok := e.(*ast.KeyValueExpr); ok {
						if key, ok := pair.Key.(*ast.Ident); ok && key.Name == "internal" {
							add(n, "Principal.internal capability")
						}
					}
				}
			}
		case *ast.TypeSpec:
			if n.Name.Name == "Principal" {
				if _, exists := index.fields[index.canonical("Principal")]["internal"]; exists {
					add(n, "Principal.internal capability")
				}
				if shape, ok := n.Type.(*ast.StructType); ok {
					for _, f := range shape.Fields.List {
						for _, name := range f.Names {
							if name.Name == "internal" {
								add(n, "Principal.internal capability")
							}
						}
					}
				}
			}
		case *ast.SelectorExpr:
			if n.Sel.Name == "internal" && index.exprType(n.X, 0, map[*ast.Object]bool{}) == index.canonical("Principal") {
				add(n, "Principal.internal capability")
			}
		case *ast.AssignStmt:
			if !trusted {
				for _, left := range n.Lhs {
					// Rebinding a local copy of a display/scope value does not
					// mutate the Actor from which it was read.
					if _, local := left.(*ast.Ident); local {
						continue
					}
					if index.identityPath(left, map[*ast.Object]bool{}) {
						add(left, "Actor identity field mutation outside trusted source")
					}
				}
			}
		case *ast.IncDecStmt:
			if !trusted && index.identityPath(n.X, map[*ast.Object]bool{}) {
				add(n, "Actor identity field mutation outside trusted source")
			}
		case *ast.UnaryExpr:
			if !trusted && n.Op == token.AND && index.identityPath(n.X, map[*ast.Object]bool{}) {
				add(n, "mutable Actor identity field address outside trusted source")
			}
		case *ast.FuncDecl:
			service := n.Recv != nil && len(n.Recv.List) > 0 && index.canonical(authoritySourceType(n.Recv.List[0].Type)) == "Service"
			if service && authorityLegacyHelper(n.Name.Name) {
				add(n, "legacy Principal guard/receipt helper")
			}
			if index.hasPrincipalParameter(n) && strings.HasPrefix(strings.ToLower(n.Name.Name), "authorize") {
				add(n, "Principal authorization fallback")
			}
			if service && ast.IsExported(n.Name.Name) && index.hasPrincipalParameter(n) && n.Body != nil {
				// A renamed wrapper must not restore the old command signature
				// merely by moving its SQL into another private helper.
				for _, prefix := range []string{"Create", "Update", "Save", "Commit", "Delete", "Command", "Start", "Register", "Revoke", "Rotate", "Bind", "Attach", "Review", "Decompose", "Issue", "Capture", "Schedule", "Approve", "Write"} {
					if strings.HasPrefix(n.Name.Name, prefix) {
						add(n, "Principal command signature without Actor")
						break
					}
				}
				ast.Inspect(n.Body, func(b ast.Node) bool {
					if call, ok := b.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							switch sel.Sel.Name {
							case "Exec", "ExecContext", "Publish", "PublishWithOperation", "CreateRun", "Command", "WriteSchedulingPolicyTx":
								add(call, "Principal mutation entry without Actor")
							}
						}
					}
					return true
				})
			}
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok && authorityLegacyHelper(sel.Sel.Name) && index.exprType(sel.X, 0, map[*ast.Object]bool{}) == "Service" {
				add(n, "legacy Principal guard/receipt call")
			}
		}
		return true
	})
	return issues
}

func TestAuthoritySourceContractCalibration(t *testing.T) {
	for _, tc := range []struct {
		name, file, source string
		reject             bool
	}{
		{"display_principal", "view.go", `package platform;func Display(p Principal)string{return p.Name}`, false},
		{"display_read", "view.go", `package platform;func(s *Service) ReadDisplay(p Principal)(string,error){return p.Name,nil}`, false},
		{"zero_actor", "domain.go", `package platform;func F(){_=Actor{};_=Actor{kind:0};_=Actor{credential:credentialRef{}}}`, false},
		{"ordinary_kind", "domain.go", `package platform;type report struct{kind string};func F(){r:=report{};r.kind="task"}`, false},
		{"local_scope_copy", "domain.go", `package platform;func F(a Actor){id:=a.credential.projectID;id="other";_=id}`, false},
		{"kernel_api", "engine.go", `package dag;func(e *Engine)Publish(tenant,project string){}`, false},
		{"trusted_constructor", "command_actor.go", `package platform;func F(){_=Actor{kind:actorBrowser}}`, false},
		{"forged_actor", "domain.go", `package platform;func F(){_=Actor{kind:actorBrowser}}`, true},
		{"pointer_literal", "domain.go", `package platform;func F(){_=&Actor{credential:credentialRef{digest:"forged"}}}`, true},
		{"type_alias", "domain.go", `package platform;type borrowed=Actor;func F(){_=borrowed{kind:actorBrowser}}`, true},
		{"elided_literal", "domain.go", `package platform;func F(){_=[]Actor{{kind:actorBrowser}}}`, true},
		{"pointer_mutation", "domain.go", `package platform;func F(){var a Actor;p:=&a;p.kind=actorBrowser}`, true},
		{"asserted_actor", "domain.go", `package platform;func F(v any){v.(*Actor).kind=actorBrowser}`, true},
		{"scan_identity", "domain.go", `package platform;func F(a Actor){row.Scan(&a.kind)}`, true},
		{"nested_identity", "domain.go", `package platform;func F(a Actor){p:=&a.credential;p.digest="forged"}`, true},
		{"holder_identity", "domain.go", `package platform;type holder struct{actor Actor};func F(h *holder){h.actor.delegation.recordID="forged"}`, true},
		{"untrusted_same_basename", "untrusted/command_actor.go", `package platform;func F(){_=Actor{kind:actorBrowser}}`, true},
		{"principal_internal", "domain.go", `package platform;type Principal struct{internal bool}`, true},
		{"principal_internal_alias", "domain.go", `package platform;type projection struct{internal bool};type Principal=projection`, true},
		{"principal_internal_use", "domain.go", `package platform;func F(p Principal){if p.internal{}}`, true},
		{"raw_receipt", "domain.go", `package platform;func(s *Service)F(){s.receipt()}`, true},
		{"old_guard", "domain.go", `package platform;func(s *Service)authorizeProjectTx(p Principal)error{return nil}`, true},
		{"renamed_guard", "domain.go", `package platform;func authorizeRecord(p Principal)bool{return true}`, true},
		{"principal_write", "domain.go", `package platform;func(s *Service) Save(p Principal){s.DB.ExecContext(nil,"UPDATE data SET x=1")}`, true},
		{"principal_wrapper", "domain.go", `package platform;func(s *Service) CreateRecord(p Principal)error{return s.lowerInsert(p)}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, tc.file, tc.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			got := authoritySourceViolations(file, tc.file, newAuthoritySourceIndex([]*ast.File{file}), fset)
			if (len(got) > 0) != tc.reject {
				t.Fatalf("reject=%v want=%v: %v", len(got) > 0, tc.reject, got)
			}
		})
	}
}

func TestAuthoritySourceContractProduction(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
		names = append(names, name)
	}
	if len(files) == 0 {
		t.Fatal("no production Go sources checked")
	}
	index := newAuthoritySourceIndex(files)
	if _, ok := index.fields["Actor"]; !ok {
		t.Fatal("Actor contract source was not loaded")
	}
	for n, file := range files {
		for _, issue := range authoritySourceViolations(file, names[n], index, fset) {
			t.Error(issue)
		}
	}
}
