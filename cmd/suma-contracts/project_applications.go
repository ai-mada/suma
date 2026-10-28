package main

import (
	"encoding/json"
	"github.com/ai-mada/suma/internal/platform"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func projectApplicationModels() []any {
	return []any{platform.ProjectApplication{}, platform.ProjectApplicationView{}, platform.ProjectApplicationPage{}, platform.ApplicationWriteRequest{}, platform.ApplicationCodeScope{}, platform.ApplicationCodeScopeInput{}, platform.ApplicationOptions{}, platform.ApplicationTeamOption{}, platform.ApplicationCodeVersion{}, platform.ApplicationVersionPreview{}, platform.ApplicationVersionPreviewRequest{}, platform.ApplicationVersionRegisterRequest{}, platform.ApplicationVersionView{}, platform.ApplicationVersionPage{}, platform.ApplicationVersionOptions{}, platform.ApplicationDependencyGraph{}, platform.ApplicationImpactRequest{}, platform.ApplicationImpact{}, ri.RepositorySourceInspectionRequest{}, ri.ProviderSourceInspection{}, ri.SourceDependencyReport{}}
}

func projectApplicationContracts(add operationAdder, api map[string]any) {
	for _, r := range []struct{ path, method, input, output, status string }{
		{"/applications", "get", "", "ProjectApplicationPage", "200"},
		{"/applications", "post", "ApplicationWriteRequest", "ProjectApplicationView", "201"},
		{"/applications/options", "get", "", "ApplicationOptions", "200"},
		{"/applications/graph", "get", "", "ApplicationDependencyGraph", "200"},
		{"/applications/impact", "post", "ApplicationImpactRequest", "ApplicationImpact", "200"},
		{"/applications/{id}", "get", "", "ProjectApplicationView", "200"},
		{"/applications/{id}", "patch", "ApplicationWriteRequest", "ProjectApplicationView", "200"},
		{"/applications/{id}/versions/options", "get", "", "ApplicationVersionOptions", "200"},
		{"/applications/{id}/versions/preview", "post", "ApplicationVersionPreviewRequest", "ApplicationVersionPreview", "200"},
		{"/applications/{id}/versions", "post", "ApplicationVersionRegisterRequest", "ApplicationVersionView", "201"},
		{"/applications/{id}/versions", "get", "", "ApplicationVersionPage", "200"},
		{"/applications/{id}/versions/{version_id}", "get", "", "ApplicationVersionView", "200"},
	} {
		read := r.input == "" || r.input == "ApplicationVersionPreviewRequest" || r.input == "ApplicationImpactRequest"
		op := add(r.path, r.method, r.output, false, r.input != "" && !read)
		successStatus(op, r.status)
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current project application catalogue and immutable registered code versions. Source scopes bind governed repository targets; version sources require actual applied integration receipts, the exact successful attempt and current independently tested accepted code. Source inspection reads fixed Git objects without installing or executing repository code. Original observations and reasoned manual dependency pins are distinct. Registration does not approve a deployment or create execution. All reads and original receipt recovery reauthorize the current project reader."
		if r.input != "" {
			request(op, r.input)
		}
		if read {
			op["x-read-only"] = true
		} else {
			browserOnly(op)
		}
		if r.path == "/applications" && r.method == "get" {
			appendQuery(op, queryParameter("state", "Optional exact catalogue lifecycle.", map[string]any{"type": "string", "enum": []string{"active", "archived"}}))
		}
		if r.method == "get" && (r.path == "/applications" || r.path == "/applications/{id}/versions") {
			appendQuery(op, queryParameter("limit", "Default20, maximum100.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("before", "Original current-project page cursor.", map[string]any{"type": "string"}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ProjectApplication", "ProjectApplicationView", "ProjectApplicationPage", "ApplicationWriteRequest", "ApplicationCodeScope", "ApplicationCodeScopeInput", "ApplicationOptions", "ApplicationTeamOption", "ApplicationCodeVersion", "ApplicationVersionPreview", "ApplicationVersionPreviewRequest", "ApplicationVersionRegisterRequest", "ApplicationVersionView", "ApplicationVersionPage", "ApplicationVersionOptions", "ApplicationVersionOption", "ApplicationIntegrationOption", "ApplicationVersionSource", "ApplicationVersionSourceInput", "ApplicationSourceInspection", "ApplicationSourceCoverage", "ApplicationManifestObservation", "ApplicationManifestDependency", "ApplicationDependencyObservation", "ApplicationDependencyInput", "ApplicationDependency", "ApplicationDependencyGraph", "ApplicationGraphNode", "ApplicationGraphEdge", "ApplicationImpactRequest", "ApplicationImpact", "ApplicationImpactItem", "RepositorySourceInspectionRequest", "ProviderSourceInspection", "RepositorySourceInspectionScope", "RepositorySourceManifest", "RepositorySourceInspectionCoverage", "RepositorySourceInspectionLimits", "SourceDependencyReport", "SourceDependencyPackage", "SourceDependencyDeclaration", "SourceDependencyFile", "SourceDependencyCoverage", "SourceDependencyDiagnostic"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	field := func(model, name string, attrs map[string]any) {
		v := toolContractMap(schemas, model, "properties", name)
		for k, a := range attrs {
			v[k] = a
		}
	}
	for _, name := range []string{"ProjectApplication", "ApplicationWriteRequest"} {
		field(name, "key", map[string]any{"pattern": "^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$", "maxLength": 64})
		field(name, "name", map[string]any{"minLength": 1, "maxLength": 200, "x-max-utf8-bytes": 200})
		field(name, "kind", map[string]any{"enum": []string{"application", "microapp", "service", "library"}})
		field(name, "state", map[string]any{"enum": []string{"active", "archived"}})
		field(name, "code_scopes", map[string]any{"minItems": 1, "maxItems": 8})
	}
	for _, name := range []string{"ApplicationVersionPreviewRequest", "ApplicationVersionRegisterRequest", "ApplicationCodeVersion"} {
		field(name, "label", map[string]any{"minLength": 1, "maxLength": 64, "x-max-utf8-bytes": 64})
		field(name, "sources", map[string]any{"minItems": 1, "maxItems": 8})
		field(name, "dependencies", map[string]any{"maxItems": 64})
	}
	for _, name := range []string{"ApplicationWriteRequest", "ApplicationVersionRegisterRequest"} {
		field(name, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	field("ApplicationWriteRequest", "expected_version", map[string]any{"minimum": 0, "description": "Create requires0; update requires the current positive version."})
	field("ApplicationCodeScopeInput", "expected_target_version", map[string]any{"minimum": 1, "description": "Current command CAS only. It is not a permanent code-source identity."})
	field("ApplicationCodeVersion", "registered_preview_sha256", map[string]any{"readOnly": true, "pattern": "^[a-f0-9]{64}$", "description": "The exact human-reviewed preview digest, separate from the final immutable version digest."})
	field("ApplicationDependencyGraph", "nodes", map[string]any{"maxItems": 1000})
	field("ApplicationDependencyGraph", "edges", map[string]any{"maxItems": 5000})
	field("ApplicationDependencyGraph", "cycles", map[string]any{"description": "Strongly connected declared groups, sorted IDs rather than fabricated ordered paths; incomplete graphs cannot prove absence of other groups."})
	field("ApplicationDependencyGraph", "edge_total_exact", map[string]any{"description": "False when application truncation makes edge_total only a loaded-scope lower bound."})
	field("ApplicationImpactItem", "path", map[string]any{"maxItems": 65, "description": "Selected changed application first, followed by dependent applications along a shortest reverse-dependency path."})
	field("SourceDependencyReport", "parser_id", map[string]any{"const": ri.SourceDependencyParserID})
	field("SourceDependencyReport", "parser_version", map[string]any{"const": ri.SourceDependencyParserVersion})
	field("SourceDependencyDeclaration", "range_state", map[string]any{"enum": []string{"valid", "invalid", "unsupported", "unknown"}})
	field("SourceDependencyDeclaration", "compatibility", map[string]any{"enum": []string{"compatible", "incompatible", "unknown"}})
	field("RepositorySourceManifest", "content_base64", map[string]any{"description": "Private provider/inspector protocol only. Never returned in a project application/version response."})
}

func sourceInspectorRuntimeContract() ([]byte, error) {
	value := map[string]any{"schema_version": 1, "protocol": ri.RepositorySourceInspectionProtocol, "parser_id": ri.SourceDependencyParserID, "parser_version": ri.SourceDependencyParserVersion, "max_transport_bytes": ri.RepositorySourceMaxTransportBytes, "limits": ri.RepositorySourceLimits()}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return []byte("// Generated from Go source inspection contracts. Do not edit.\nexport const sourceInspectorContract = Object.freeze(" + string(raw) + ");\n"), nil
}
