package localization

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// This is a bounded source gate, not a claim that every backend diagnostic is
// translated. It inspects all build variants of non-test internal/platform Go
// files and the message positions of the constructors below. English literals,
// external errors and variable forwarding are counted separately, not reported
// as localized. It does not inspect stored logs, user content, other packages,
// arbitrary wrappers, or perform type checking / general variable data flow.
// Referenced package constants with multiple build-variant definitions fail
// closed; this checker never chooses one value without a build context.
// Keeping the checker in a _test.go file adds nothing to runtime binaries.
func TestPlatformDiagnosticCoverage(t *testing.T) {
	files := map[string][]byte{}
	err := filepath.WalkDir(filepath.Join("..", "platform"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(path)] = raw
		return nil
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("read platform source: files=%d err=%v", len(files), err)
	}
	report, err := inspectPlatformDiagnostics(files)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("scope=internal/platform non-test Go; files=%d constructors=%d static_han=%d unique_static_han=%d annotated_dynamic=%d unique_annotation_keys=%d non_han_static_out_of_scope=%d raw_dynamic_out_of_scope=%d source_sha256=%s", len(files), report.total, report.staticHan, len(report.sources), report.annotated, len(report.annotationKeys), report.nonHanStatic, report.rawDynamic, platformSourceDigest(files))
	for _, issue := range report.issues {
		t.Errorf("%s: %s", issue.kind, issue.detail)
	}
}

// These fixtures exercise the checker, rather than adding fixture messages to
// the product catalog. Unknown Chinese must fail even when a known prefix, a
// template, an error code, or a nearby catalog entry looks similar.
func TestPlatformDiagnosticParserFixtures(t *testing.T) {
	const known = `"输入无效"`
	wrap := func(body string) string {
		return "package platform\nfunc fixture(field string, err error) {\n" + body + "\n}\n"
	}
	tests := []struct {
		name      string
		source    string
		extra     string
		variant   string
		issue     string
		static    int
		annotated int
		raw       int
		neutral   int
	}{
		{name: "all message positions", source: `package platform
import ("errors"; "fmt")
func fixture() {
 _ = invalid(` + known + `)
 _ = conflict(` + known + `)
 _ = errf(` + known + `)
 _ = errors.New(` + known + `)
 _ = fmt.Errorf(` + known + `)
 _ = &APIError{400, "code", ` + known + `, nil}
 _ = &APIError{Code: "code", Message: ` + known + `, Status: 400}
 _ = releaseError(400, "metadata中文", ` + known + `)
 _ = releaseConflict("metadata中文", ` + known + `)
}`, static: 9},
		{name: "constant parentheses and addition", source: `package platform
const beginning = "输入"
const (ending = "无效"; repeated)
const whole = beginning + (repeated)
func fixture() { _ = invalid((whole)) }`, static: 1},
		{name: "cross file constant", source: wrap(`_ = invalid(sharedMessage)`), extra: `package platform; const sharedMessage = "输入" + "无效"`, static: 1},
		{name: "ambiguous build variant constants fail closed", source: wrap(`_ = invalid(sharedMessage)`), extra: `package platform; const sharedMessage = "新增未知错误"`, variant: `package platform; const sharedMessage = "输入无效"`, issue: "ambiguous_constant"},
		{name: "standard import aliases", source: `package platform
import (problem "errors"; format "fmt")
func fixture() { _ = problem.New("新增未知错误"); _ = format.Errorf("另一未知错误") }`, issue: "unmapped_static"},
		{name: "unknown invalid", source: wrap(`_ = invalid("新增未知错误")`), issue: "unmapped_static"},
		{name: "unknown conflict", source: wrap(`_ = conflict("新增未知错误")`), issue: "unmapped_static"},
		{name: "unknown errf", source: wrap(`_ = errf("新增未知错误")`), issue: "unmapped_static"},
		{name: "positional APIError", source: wrap(`_ = &APIError{400, "code", "新增未知错误", nil}`), issue: "unmapped_static"},
		{name: "keyed APIError", source: wrap(`_ = &APIError{Code: "code", Status: 400, Message: "新增未知错误"}`), issue: "unmapped_static"},
		{name: "releaseError third argument", source: wrap(`_ = releaseError(400, ` + known + `, "新增未知错误")`), issue: "unmapped_static"},
		{name: "releaseConflict second argument", source: wrap(`_ = releaseConflict(` + known + `, "新增未知错误")`), issue: "unmapped_static"},
		{name: "concatenated full source", source: wrap(`_ = invalid("输入" + ("无效附加内容"))`), issue: "unmapped_static"},
		{name: "prefix is not a message", source: wrap(`_ = invalid("缺少")`), issue: "unmapped_static"},
		{name: "template is not rendered source", source: wrap(`_ = invalid("缺少{{.field}}")`), issue: "unmapped_static"},
		{name: "dynamic Chinese prefix", source: wrap(`_ = invalid("缺少" + field)`), issue: "unannotated_dynamic"},
		{name: "dynamic Chinese suffix", source: wrap(`_ = invalid(field + "必须为整数")`), issue: "unannotated_dynamic"},
		{name: "constant dynamic prefix", source: "package platform\nconst prefix = \"缺少\"\nfunc fixture(field string) { _ = invalid(prefix + field) }", issue: "unannotated_dynamic"},
		{name: "nested formatted source", source: `package platform; import "fmt"; func fixture(field string) { _ = invalid(fmt.Sprintf("缺少%s", field)) }`, issue: "unannotated_dynamic"},
		{name: "formatter runtime arguments", source: wrap(`_ = errf("输入无效%s", field)`), issue: "unannotated_dynamic"},
		{name: "formatter Chinese argument", source: `package platform; import "fmt"; func fixture() { _ = fmt.Errorf("%s", "新增未知错误") }`, issue: "unannotated_dynamic"},
		{name: "specific registered annotation", source: wrap(`_ = invalidDiagnostic("validation.field_required", "缺少" + field, map[string]any{"field": field})`), annotated: 1},
		{name: "constant annotation key", source: "package platform\nconst key = \"validation.\" + \"field_required\"\nfunc fixture(field string) { _ = invalidDiagnostic(key, \"缺少\" + field, map[string]any{\"field\": field}) }", annotated: 1},
		{name: "annotation cannot hide unknown static source", source: wrap(`_ = invalidDiagnostic("validation.field_required", "新增未知完整中文", map[string]any{"field": field})`), issue: "unmapped_static"},
		{name: "annotation cannot hide static prefix", source: wrap(`_ = invalidDiagnostic("validation.field_required", "缺少", map[string]any{"field": field})`), issue: "unmapped_static"},
		{name: "unknown annotation key", source: wrap(`_ = invalidDiagnostic("validation.no_such_key", "缺少" + field, map[string]any{"field": field})`), issue: "annotation_key"},
		{name: "runtime annotation key", source: wrap(`_ = invalidDiagnostic(field, "缺少" + field, map[string]any{"field": field})`), issue: "annotation_key"},
		{name: "generic annotation key", source: wrap(`_ = invalidDiagnostic("common.invalid_input", "缺少" + field, map[string]any{})`), issue: "annotation_key"},
		{name: "missing annotation parameter", source: wrap(`_ = invalidDiagnostic("validation.field_required", "缺少" + field, map[string]any{})`), issue: "annotation_params"},
		{name: "extra annotation parameter", source: wrap(`_ = invalidDiagnostic("validation.field_required", "缺少" + field, map[string]any{"field": field, "extra": field})`), issue: "annotation_params"},
		{name: "duplicate annotation parameter", source: wrap(`_ = invalidDiagnostic("validation.field_required", "缺少" + field, map[string]any{"field": field, "field": field})`), issue: "annotation_params"},
		{name: "runtime annotation parameter name", source: wrap(`_ = invalidDiagnostic("validation.field_required", "缺少" + field, map[string]any{field: field})`), issue: "annotation_params"},
		{name: "nonliteral annotation parameters", source: wrap(`params := map[string]any{"field": field}; _ = invalidDiagnostic("validation.field_required", "缺少" + field, params)`), issue: "annotation_params"},
		{name: "forwarding and external errors stay raw", source: wrap(`_ = invalid(field); _ = &APIError{400, "code", err.Error(), nil}; _ = &APIError{Message: field}`), raw: 3},
		{name: "parameter shadows package constant", source: "package platform\nconst message = \"新增未知错误\"\nfunc fixture(message string) { _ = invalid(message) }", raw: 1},
		{name: "selector field is not a package constant", source: `package platform; const Message = "新增未知错误"; func fixture(input struct{ Message string }) { _ = invalid(input.Message) }`, raw: 1},
		{name: "error method is not a package constant", source: `package platform; const Error = "新增未知错误"; func fixture(err error) { _ = invalid(err.Error()) }`, raw: 1},
		{name: "English stays outside Chinese gate", source: wrap(`_ = invalid("English diagnostic remains raw")`), neutral: 1},
		{name: "user content is not a diagnostic", source: wrap(`field = "用户自行编写的中文"; _ = field`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string][]byte{"fixture.go": []byte(tt.source)}
			if tt.extra != "" {
				files["constants_linux.go"] = []byte(tt.extra)
			}
			if tt.variant != "" {
				files["constants_windows.go"] = []byte(tt.variant)
			}
			report, err := inspectPlatformDiagnostics(files)
			if err != nil {
				t.Fatalf("fixture must parse before checking coverage: %v", err)
			}
			if tt.issue != "" {
				if len(report.issues) == 0 {
					t.Fatalf("checker accepted diagnostic that must fail with %s", tt.issue)
				}
				for _, issue := range report.issues {
					if issue.kind != tt.issue {
						t.Errorf("issue kind = %s, want %s: %s", issue.kind, tt.issue, issue.detail)
					}
				}
				return
			}
			if len(report.issues) != 0 {
				t.Fatalf("valid fixture rejected: %+v", report.issues)
			}
			if report.staticHan != tt.static || report.annotated != tt.annotated || report.rawDynamic != tt.raw || report.nonHanStatic != tt.neutral {
				t.Fatalf("classification = static:%d annotated:%d raw:%d neutral:%d; want %d/%d/%d/%d", report.staticHan, report.annotated, report.rawDynamic, report.nonHanStatic, tt.static, tt.annotated, tt.raw, tt.neutral)
			}
		})
	}
}

type platformDiagnosticIssue struct {
	kind   string
	detail string
}

type platformDiagnosticReport struct {
	issues         []platformDiagnosticIssue
	total          int
	staticHan      int
	annotated      int
	rawDynamic     int
	nonHanStatic   int
	sources        map[string]bool
	annotationKeys map[string]bool
}

func inspectPlatformDiagnostics(files map[string][]byte) (platformDiagnosticReport, error) {
	fset := token.NewFileSet()
	trees := map[string]*ast.File{}
	constants := platformStringConstants{objects: map[*ast.Object]ast.Expr{}, packageNames: map[string][]ast.Expr{}}
	for _, name := range platformSortedNames(files) {
		tree, err := parser.ParseFile(fset, name, files[name], parser.AllErrors)
		if err != nil {
			return platformDiagnosticReport{}, err
		}
		trees[name] = tree
		constants.collect(tree)
	}
	report := platformDiagnosticReport{sources: map[string]bool{}, annotationKeys: map[string]bool{}}
	issue := func(kind string, expr ast.Expr, detail string) {
		report.issues = append(report.issues, platformDiagnosticIssue{kind, fmt.Sprintf("%s: %s", fset.Position(expr.Pos()), detail)})
	}
	check := func(expr ast.Expr, formatted bool) {
		report.total++
		if name := constants.ambiguousName(expr); name != "" {
			issue("ambiguous_constant", expr, "diagnostic references a package constant with multiple build-variant definitions: "+name)
			return
		}
		source, static := constants.stringValue(expr)
		if static && !formatted {
			if !platformHasHan(source) {
				report.nonHanStatic++
				return
			}
			report.staticHan++
			report.sources[source] = true
			if key, ok := LookupSource(source); !ok {
				issue("unmapped_static", expr, fmt.Sprintf("complete Chinese source has no exact LookupSource mapping: %q", source))
			} else if _, exists := definitions[key]; !exists {
				issue("unmapped_static", expr, "source refers to an unregistered key: "+key)
			}
			return
		}
		if constants.containsHan(expr) {
			issue("unannotated_dynamic", expr, "Chinese dynamic diagnostic requires invalidDiagnostic with a specific registered key and literal parameter names")
			return
		}
		report.rawDynamic++
	}
	for _, name := range platformSortedNames(files) {
		tree := trees[name]
		imports := platformImports(tree)
		ast.Inspect(tree, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CallExpr:
				constructor, index, formatter := platformConstructor(n, imports)
				if constructor == "" || len(n.Args) <= index {
					return true
				}
				if constructor == "invalidDiagnostic" {
					report.total++
					if name := constants.ambiguousName(n.Args[1]); name != "" {
						issue("ambiguous_constant", n.Args[1], "diagnostic references a package constant with multiple build-variant definitions: "+name)
						return true
					}
					key, ok := constants.stringValue(n.Args[0])
					definition, exists := definitions[key]
					if !ok || !exists || len(definition.Parameters) == 0 {
						issue("annotation_key", n.Args[0], "dynamic annotation requires a constant, registered, parameterized diagnostic key")
						return true
					}
					if len(n.Args) != 3 || !platformParameterNames(n.Args[2], definition.Parameters) {
						issue("annotation_params", n, "literal parameter names must match the registered key exactly: "+key)
						return true
					}
					// Metadata cannot exempt a complete static source (including a
					// template fragment) from exact-source ownership.
					if source, static := constants.stringValue(n.Args[1]); static && platformHasHan(source) {
						report.staticHan++
						report.sources[source] = true
						sourceKey, mapped := LookupSource(source)
						if !mapped {
							issue("unmapped_static", n.Args[1], fmt.Sprintf("complete Chinese source has no exact LookupSource mapping: %q", source))
						} else if sourceKey != key {
							issue("annotation_key", n.Args[0], "static source key differs from annotation: "+sourceKey)
						}
						return true
					}
					report.annotated++
					report.annotationKeys[key] = true
					return true
				}
				if formatter && len(n.Args) > index+1 {
					// Formatting arguments can introduce owned Chinese even when the
					// format is only "%s"; inspect the whole formatting operation.
					check(n, true)
				} else {
					check(n.Args[index], false)
				}
			case *ast.CompositeLit:
				id, ok := n.Type.(*ast.Ident)
				if !ok || id.Name != "APIError" {
					return true
				}
				for index, element := range n.Elts {
					if keyed, ok := element.(*ast.KeyValueExpr); ok {
						if field, ok := keyed.Key.(*ast.Ident); ok && field.Name == "Message" {
							check(keyed.Value, false)
						}
					} else if index == 2 {
						check(element, false)
					}
				}
			}
			return true
		})
	}
	return report, nil
}

// String constants are resolved across files and within lexical scopes. Runtime
// variables are deliberately not treated as constant strings or translations.
type platformStringConstants struct {
	objects      map[*ast.Object]ast.Expr
	packageNames map[string][]ast.Expr
}

func (c platformStringConstants) collect(tree *ast.File) {
	packageDecls := map[*ast.GenDecl]bool{}
	for _, decl := range tree.Decls {
		if group, ok := decl.(*ast.GenDecl); ok {
			packageDecls[group] = true
		}
	}
	ast.Inspect(tree, func(node ast.Node) bool {
		group, ok := node.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			return true
		}
		var values []ast.Expr
		for _, spec := range group.Specs {
			binding := spec.(*ast.ValueSpec)
			if len(binding.Values) != 0 {
				values = binding.Values
			}
			for i, id := range binding.Names {
				if i >= len(values) {
					continue
				}
				c.objects[id.Obj] = values[i]
				if packageDecls[group] {
					c.packageNames[id.Name] = append(c.packageNames[id.Name], values[i])
				}
			}
		}
		return true
	})
}

func (c platformStringConstants) stringValue(expr ast.Expr) (string, bool) {
	return c.resolve(expr, map[ast.Expr]bool{})
}

func (c platformStringConstants) resolve(expr ast.Expr, visiting map[ast.Expr]bool) (string, bool) {
	if expr == nil || visiting[expr] {
		return "", false
	}
	visiting[expr] = true
	defer delete(visiting, expr)
	switch n := expr.(type) {
	case *ast.BasicLit:
		if n.Kind == token.STRING {
			value, err := strconv.Unquote(n.Value)
			return value, err == nil
		}
	case *ast.ParenExpr:
		return c.resolve(n.X, visiting)
	case *ast.BinaryExpr:
		if n.Op == token.ADD {
			left, lok := c.resolve(n.X, visiting)
			right, rok := c.resolve(n.Y, visiting)
			return left + right, lok && rok
		}
	case *ast.Ident:
		values := c.bindings(n)
		if len(values) == 1 {
			return c.resolve(values[0], visiting)
		}
	}
	return "", false
}

func (c platformStringConstants) containsHan(expr ast.Expr) bool {
	found := false
	platformInspectValueExpressions(expr, func(child ast.Expr) bool {
		if found {
			return false
		}
		value, static := c.stringValue(child)
		if static && platformHasHan(value) {
			found = true
			return false
		}
		return true
	})
	return found
}

func (c platformStringConstants) bindings(id *ast.Ident) []ast.Expr {
	if id.Obj != nil {
		if value := c.objects[id.Obj]; value != nil {
			return []ast.Expr{value}
		}
		return nil
	}
	return c.packageNames[id.Name]
}

func (c platformStringConstants) ambiguousName(expr ast.Expr) string {
	visiting := map[ast.Expr]bool{}
	var find func(ast.Expr) string
	find = func(expr ast.Expr) string {
		if visiting[expr] {
			return ""
		}
		visiting[expr] = true
		defer delete(visiting, expr)
		name := ""
		platformInspectValueExpressions(expr, func(child ast.Expr) bool {
			if name != "" {
				return false
			}
			if id, ok := child.(*ast.Ident); ok {
				values := c.bindings(id)
				if len(values) > 1 {
					name = id.Name
				} else if len(values) == 1 {
					name = find(values[0])
				}
			}
			return name == ""
		})
		return name
	}
	return find(expr)
}

// Selector names and struct/parameter field declarations are not bare value
// references. In particular, err.Error and input.Message must never resolve to
// unrelated package constants named Error or Message.
func platformInspectValueExpressions(expr ast.Expr, inspect func(ast.Expr) bool) {
	ast.Inspect(expr, func(node ast.Node) bool {
		if _, field := node.(*ast.Field); field {
			return false
		}
		child, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		if !inspect(child) {
			return false
		}
		if selector, ok := child.(*ast.SelectorExpr); ok {
			platformInspectValueExpressions(selector.X, inspect)
			return false
		}
		return true
	})
}

func platformHasHan(value string) bool {
	return strings.ContainsFunc(value, func(r rune) bool { return unicode.Is(unicode.Han, r) })
}

func platformImports(tree *ast.File) map[string]string {
	imports := map[string]string{}
	for _, spec := range tree.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != "errors" && path != "fmt" {
			continue
		}
		name := path
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = path
	}
	return imports
}

func platformConstructor(call *ast.CallExpr, imports map[string]string) (string, int, bool) {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		if fn.Obj != nil && fn.Obj.Kind != ast.Fun {
			return "", 0, false
		}
		switch fn.Name {
		case "invalid", "conflict":
			return fn.Name, 0, false
		case "errf":
			return fn.Name, 0, true
		case "releaseError":
			return fn.Name, 2, false
		case "releaseConflict":
			return fn.Name, 1, false
		case "invalidDiagnostic":
			return fn.Name, 1, false
		}
	case *ast.SelectorExpr:
		if id, ok := fn.X.(*ast.Ident); ok && id.Obj == nil {
			if imports[id.Name] == "errors" && fn.Sel.Name == "New" {
				return "errors.New", 0, false
			}
			if imports[id.Name] == "fmt" && fn.Sel.Name == "Errorf" {
				return "fmt.Errorf", 0, true
			}
		}
	}
	return "", 0, false
}

func platformParameterNames(expr ast.Expr, parameters []Parameter) bool {
	literal, ok := expr.(*ast.CompositeLit)
	if !ok {
		return false
	}
	names := map[string]bool{}
	for _, element := range literal.Elts {
		entry, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, ok := entry.Key.(*ast.BasicLit)
		if !ok || key.Kind != token.STRING {
			return false
		}
		name, err := strconv.Unquote(key.Value)
		if err != nil || names[name] {
			return false
		}
		names[name] = true
	}
	if len(names) != len(parameters) {
		return false
	}
	for _, parameter := range parameters {
		if !names[parameter.Name] {
			return false
		}
	}
	return true
}

func platformSortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func platformSourceDigest(files map[string][]byte) string {
	hash := sha256.New()
	for _, name := range platformSortedNames(files) {
		fmt.Fprintf(hash, "%s\x00%x\n", name, sha256.Sum256(files[name]))
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
