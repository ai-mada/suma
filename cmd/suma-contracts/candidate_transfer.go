package main

import cc "github.com/ai-mada/suma/internal/candidatecontract"

func candidateTransferModels() []any {
	return []any{cc.AssignmentRef{}, cc.StartRequest{}, cc.CommandRequest{}, cc.Binding{}, cc.ImportReceipt{}}
}

func candidateTransferContracts(add operationAdder, api map[string]any) {
	for _, namespace := range []string{"runner", "pool-runner"} {
		base := "/" + namespace + "/candidate-transfers"
		for _, route := range []struct{ path, method, request, response string }{
			{base, "post", "CandidateTransferStartRequest", "CandidateBinding"},
			{base + "/{id}/seal", "post", "CandidateTransferCommandRequest", "CandidateBinding"},
			{base + "/resolve", "post", "CandidateTransferCommandRequest", "CandidateBinding"},
			{base + "/{id}/bundle", "put", "", ""},
			{base + "/{id}/bundle", "get", "", ""},
		} {
			op := add(route.path, route.method, route.response, false, route.method != "get")
			op["security"] = []any{map[string]any{"bearerAuth": []string{}}}
			op["description"] = "Exact current attempt/lease authority before receipt replay. Pool identities retain the full frozen machine dispatch and current approved bilateral grant; they never become human roles. Sealed code is not test or acceptance evidence."
			op["x-request-schema-coverage"] = "strict versioned candidate transfer contract"
			if route.request != "" {
				op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + route.request}}}}
			}
			if route.method == "post" {
				status := "200"
				if route.path == base {
					status = "201"
				}
				successStatus(op, status)
			}
			if route.method == "put" || route.method == "get" {
				parameters := op["parameters"].([]any)
				found := false
				for _, raw := range parameters {
					p := raw.(map[string]any)
					if p["name"] == "project_id" {
						p["required"] = true
						found = true
					}
				}
				if !found {
					parameters = append(parameters, map[string]any{"in": "query", "name": "project_id", "required": true, "schema": map[string]any{"type": "string"}})
				}
				parameters = append(parameters, map[string]any{"in": "header", "name": "X-Suma-Assignment", "required": true, "description": "Base64url JSON CandidateAssignmentRef including complete pool dispatch identity when applicable", "schema": map[string]any{"type": "string"}})
				if route.method == "put" {
					op["x-max-request-body-bytes"] = cc.MaxBundleBytes
					toolContractMap(op, "responses", "200")["content"] = map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "required": []string{"acknowledged"}, "additionalProperties": false, "properties": map[string]any{"acknowledged": map[string]any{"type": "boolean"}}}}}
					parameters = append(parameters, map[string]any{"in": "header", "name": "X-Suma-Operation-ID", "required": true, "schema": map[string]any{"type": "string", "maxLength": 128}})
					op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary", "maxLength": cc.MaxBundleBytes}}}}
				} else {
					toolContractMap(op, "responses", "200")["content"] = map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}
				}
				op["parameters"] = parameters
				successStatus(op, "200")
			}
			if route.method != "get" {
				toolContractMap(op, "responses")["423"] = map[string]any{"description": "Maintenance: command not started"}
			}
		}
	}
	for _, name := range []string{"CandidateAssignmentRef", "CandidateTransferStartRequest", "CandidateTransferCommandRequest", "CandidateBinding", "CandidateImportReceipt"} {
		schema := toolContractMap(api, "components", "schemas", name)
		schema["additionalProperties"] = false
	}
	for _, name := range []string{"CandidateTransferStartRequest", "CandidateBinding"} {
		props := toolContractMap(api, "components", "schemas", name, "properties")
		toolContractMap(props, "size_bytes")["minimum"] = 1
		toolContractMap(props, "size_bytes")["maximum"] = cc.MaxBundleBytes
		for _, field := range []string{"bundle_sha256", "evidence_sha256"} {
			toolContractMap(props, field)["pattern"] = "^[a-f0-9]{64}$"
		}
	}
	toolContractMap(api, "components", "schemas", "CandidateBinding", "properties", "state")["enum"] = []string{"uploading", "sealed"}
}
