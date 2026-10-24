// suma-contracts emits response schemas and TypeScript wire types from the Go domain types.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/platform"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

var models = map[string]reflect.Type{}

func schema(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Pointer {
		return map[string]any{"anyOf": []any{schema(t.Elem()), map[string]any{"type": "null"}}}
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64, reflect.Int32:
		return map[string]any{"type": "integer"}
	case reflect.Float64, reflect.Float32:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": schema(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schema(t.Elem())}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Struct:
		models[t.Name()] = t
		return map[string]any{"$ref": "#/components/schemas/" + t.Name()}
	}
	return map[string]any{}
}
func structSchema(t reflect.Type) map[string]any {
	properties := map[string]any{}
	required := []string{}
	for _, f := range wireFields(t) {
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			name = f.Name
		}
		properties[name] = schema(f.Type)
		if !strings.Contains(tag, "omitempty") {
			required = append(required, name)
		}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

// encoding/json promotes untagged embedded struct fields into the outer object.
// PlannedTask embeds Record and BaselineDetail embeds BaselineSummary.
func wireFields(t reflect.Type) []reflect.StructField {
	var fields []reflect.StructField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" || f.Tag.Get("json") == "-" {
			continue
		}
		if f.Anonymous && strings.Split(f.Tag.Get("json"), ",")[0] == "" && f.Type.Kind() == reflect.Struct {
			fields = append(fields, wireFields(f.Type)...)
		} else {
			fields = append(fields, f)
		}
	}
	return fields
}
func ts(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		return ts(t.Elem()) + " | null"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Interface:
		return "unknown"
	case reflect.Slice, reflect.Array:
		return "(" + ts(t.Elem()) + ")[]"
	case reflect.Map:
		return "{ [key: string]: " + ts(t.Elem()) + " }"
	case reflect.Struct:
		return t.Name()
	}
	return "unknown"
}
func generate() (map[string]any, string) {
	models = map[string]reflect.Type{}
	for _, v := range append(append(append([]any{dag.Workflow{}, dag.Node{}, dag.Edge{}, dag.Run{}, dag.NodeRun{}, dag.Assignment{}, dag.Result{}, dag.Event{}, platform.Record{}, platform.Principal{}}, extraModels()...), releaseModels()...), append(automationModels(), automationIngressModels()...)...) {
		schema(reflect.TypeOf(v))
	}
	for _, v := range toolExecutionModels() {
		schema(reflect.TypeOf(v))
	}
	schemas := map[string]any{}
	for len(schemas) < len(models) {
		for _, name := range modelNames() {
			if _, exists := schemas[name]; !exists {
				schemas[name] = structSchema(models[name])
			}
		}
	}
	enhanceSchemas(schemas)
	enhanceReleaseSchemas(schemas)
	enhanceAutomationSchemas(schemas)
	enhanceAutomationIngressSchemas(schemas)
	enhanceToolExecutionSchemas(schemas)
	schemas["Error"] = map[string]any{"type": "object", "required": []string{"error"}, "properties": map[string]any{"error": map[string]any{"type": "object", "required": []string{"code", "message"}, "properties": map[string]any{"code": map[string]string{"type": "string"}, "message": map[string]string{"type": "string"}, "details": map[string]any{}}}}}
	paths := map[string]any{}
	add := func(path, method, model string, list, body bool) map[string]any {
		responseSchema := map[string]any{}
		if model != "" {
			responseSchema = map[string]any{"$ref": "#/components/schemas/" + model}
		}
		if list {
			responseSchema = map[string]any{"type": "object", "required": []string{"items", "total"}, "properties": map[string]any{"items": map[string]any{"type": "array", "items": responseSchema}, "total": map[string]string{"type": "integer"}, "next_cursor": map[string]string{"type": "string"}}}
		}
		if path == "/runs/{id}/events" {
			responseSchema = map[string]any{"type": "object", "required": []string{"items", "next_cursor"}, "properties": map[string]any{"items": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Event"}}, "next_cursor": map[string]string{"type": "integer"}}}
		}
		parameters := []any{}
		if list && method == "get" && (model == "Record" && path != "/projects" || model == "Run") {
			parameters = append(parameters, map[string]any{"in": "query", "name": "limit", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 100}}, map[string]any{"in": "query", "name": "cursor", "schema": map[string]string{"type": "string"}})
		}
		if path == "/runs/{id}/events" {
			parameters = append(parameters, map[string]any{"in": "query", "name": "after", "schema": map[string]any{"type": "integer", "minimum": 0}})
		}
		if projectScoped(path) {
			parameters = append(parameters, map[string]any{"in": "query", "name": "project_id", "required": !body, "schema": map[string]string{"type": "string"}})
		}
		for _, part := range strings.Split(path, "/") {
			if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
				parameters = append(parameters, map[string]any{"in": "path", "name": strings.Trim(part, "{}"), "required": true, "schema": map[string]string{"type": "string"}})
			}
		}
		op := map[string]any{"operationId": method + strings.NewReplacer("/", "_", "{", "", "}", "").Replace(path), "parameters": parameters, "responses": map[string]any{"200": map[string]any{"description": "Successful response; creates may return 201", "content": map[string]any{"application/json": map[string]any{"schema": responseSchema}}}, "201": map[string]any{"description": "Created", "content": map[string]any{"application/json": map[string]any{"schema": responseSchema}}}, "400": map[string]any{"description": "Invalid request"}, "401": map[string]any{"description": "Unauthenticated"}, "403": map[string]any{"description": "Forbidden"}, "409": map[string]any{"description": "Version or operation conflict"}}, "security": []any{map[string]any{"cookieAuth": []string{}}, map[string]any{"bearerAuth": []string{}}}}
		if body {
			op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"project_id": map[string]string{"type": "string"}, "idempotency_key": map[string]string{"type": "string"}, "expected_version": map[string]string{"type": "integer"}}, "additionalProperties": true}}}}
		}
		if path == "/auth/setup" || path == "/auth/login" {
			op["security"] = []any{}
		}
		op["x-request-schema-coverage"] = "legacy generic envelope; field validation is specified by the handler and API guide"
		entry, ok := paths[path].(map[string]any)
		if !ok {
			entry = map[string]any{}
			paths[path] = entry
		}
		entry[method] = op
		return op
	}
	for _, path := range []string{"/auth/setup", "/auth/login", "/auth/logout", "/auth/tokens"} {
		add(path, "post", "", false, true)
	}
	add("/auth/session", "get", "", false, false)
	for _, kind := range []string{"projects", "requirements", "tasks", "members", "issues", "documents", "releases", "tests", "evidence"} {
		add("/"+kind, "get", "Record", true, false)
		add("/"+kind, "post", "Record", false, true)
		if kind != "projects" {
			add("/"+kind+"/{id}", "get", "Record", false, false)
			add("/"+kind+"/{id}", "patch", "Record", false, true)
		}
	}
	add("/overview", "get", "", false, false)
	add("/capabilities", "get", "", false, false)
	add("/search", "post", "", false, true)
	add("/requirements/{id}/decompose", "post", "Record", true, true)
	add("/tasks/{id}/review", "post", "Record", false, true)
	add("/issues/{id}/intake", "post", "Record", false, true)
	add("/workflows", "get", "Workflow", true, false)
	add("/workflows", "post", "Workflow", false, true)
	add("/workflows/{id}", "get", "Workflow", false, false)
	add("/workflows/validate", "post", "", false, true)
	add("/runs", "get", "Run", true, false)
	add("/runs", "post", "Run", false, true)
	add("/runs/{id}", "get", "Run", false, false)
	add("/runs/{id}/commands", "post", "Run", false, true)
	add("/runs/{id}/events", "get", "Event", true, false)
	for _, route := range []string{"claim", "heartbeat", "results"} {
		add("/runner/"+route, "post", "", false, true)
	}
	add("/workspace", "get", "", false, false)
	add("/workspace/binding", "post", "", false, true)
	add("/workspace/file", "get", "", false, false)
	add("/workspace/file", "post", "", false, true)
	add("/agent/tools", "get", "", true, false)
	add("/agent/tools/invoke", "post", "", false, true)
	extendContracts(add, paths)
	automationContracts(add, paths)
	automationIngressContracts(add)
	toolExecutionContracts(add, paths)
	releaseContracts(add)
	api := map[string]any{"openapi": "3.1.0", "info": map[string]string{"title": "速码 suma API", "version": "0.1.0", "description": "Domain response fields are reflected from Go types; map-backed envelopes and new request fields are audited DTOs. Legacy requests retain partial generic envelopes. Cross-field validation, authorization, idempotency and invitation secrecy remain server-enforced; see docs/api/README.md."}, "servers": []any{map[string]string{"url": "/api/v1"}}, "paths": paths, "components": map[string]any{"schemas": schemas, "securitySchemes": map[string]any{"cookieAuth": map[string]string{"type": "apiKey", "in": "cookie", "name": "suma_session", "description": "Browser session. Organization governance rejects every scoped API, Runner and Agent project token."}, "bearerAuth": map[string]string{"type": "http", "scheme": "bearer", "description": "Project-scoped API credential; applicable routes enforce the bound project and scope."}, "runnerAuth": map[string]string{"type": "http", "scheme": "bearer", "description": "Runner-scoped credential bound to this project; browser sessions, Agent and API tokens are rejected."}}}}
	var out strings.Builder
	automationIngressSecurity(api)
	api["components"].(map[string]any)["securitySchemes"].(map[string]any)["providerAuth"] = map[string]string{"type": "http", "scheme": "bearer", "description": "Independent provider credential configured by the administrator. Not a browser, project API, Agent or Runner token; binds provider identity and authorized operation."}
	out.WriteString("// Generated by go run ./cmd/suma-contracts. Do not edit.\n")
	for _, name := range modelNames() {
		fmt.Fprintf(&out, "export interface %s {\n", name)
		t := models[name]
		for _, f := range wireFields(t) {
			tag := f.Tag.Get("json")
			key := strings.Split(tag, ",")[0]
			if key == "-" {
				continue
			}
			if key == "" {
				key = f.Name
			}
			optional := ""
			if strings.Contains(tag, "omitempty") {
				optional = "?"
			}
			fmt.Fprintf(&out, "  %s%s: %s;\n", key, optional, ts(f.Type))
		}
		out.WriteString("}\n")
	}
	return api, out.String()
}

func modelNames() []string {
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func emit(path string, data []byte, check bool) error {
	if check {
		current, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, data) {
			return fmt.Errorf("generated contract drift: %s; run go run ./cmd/suma-contracts", path)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func main() {
	check := flag.Bool("check", false, "verify generated artifacts without rewriting them")
	flag.Parse()
	api, types := generate()
	b, err := json.MarshalIndent(api, "", "  ")
	if err == nil {
		err = emit("docs/api/openapi.json", append(b, '\n'), *check)
	}
	if err == nil {
		err = emit("packages/contracts/src/index.ts", []byte(types), *check)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	verb := "Generated"
	if *check {
		verb = "Verified"
	}
	fmt.Printf("%s %d model schemas, %d API paths and TypeScript wire types.\n", verb, len(models), len(api["paths"].(map[string]any)))
}
