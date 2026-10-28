package platform

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// The current project transport supplies Actor. Neither a prompt, model field,
// resource location nor preview hash can replace current server-side authority.
func (s *Server) intelligentOperationsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, body map[string]any) bool {
	if len(parts) == 0 || parts[0] != "intelligent-operations" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	if !operationActor(actor) {
		writeError(w, forbidden())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(operationInvalid))
		return true
	}
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || key != "project_id" && !(r.Method == http.MethodGet && len(parts) == 2 && (key == "limit" || key == "before")) {
			writeError(w, invalid(operationInvalid))
			return true
		}
	}
	if values, ok := query["project_id"]; ok && values[0] != project || r.Method == http.MethodGet && query.Get("project_id") == "" {
		writeError(w, invalid(operationInvalid))
		return true
	}
	if len(parts) < 2 || parts[1] != "analyses" {
		writeError(w, missing())
		return true
	}
	decode := func(out any, required ...string) bool {
		if projectKnowledgeRequired(body, required...) != nil {
			writeError(w, invalid(operationInvalid))
			return false
		}
		// The outer transport already bounds bytes and decodes JSON numbers;
		// reject explicit nulls and unknown nested members in this closed DTO.
		for _, value := range body {
			if value == nil {
				writeError(w, invalid(operationInvalid))
				return false
			}
		}
		if err := operationStrictJSON(stringify(body), out); err != nil {
			writeError(w, invalid(operationInvalid))
			return false
		}
		return true
	}
	switch {
	case len(parts) == 2 && r.Method == http.MethodGet:
		limit := 20
		if value := query.Get("limit"); value != "" {
			if value[0] == '0' {
				writeError(w, invalid(operationInvalid))
				return true
			}
			for _, c := range value {
				if c < '0' || c > '9' {
					writeError(w, invalid(operationInvalid))
					return true
				}
			}
			limit, err = strconv.Atoi(value)
			if err != nil || limit < 1 || limit > 100 {
				writeError(w, invalid(operationInvalid))
				return true
			}
		}
		out, err := s.Service.ListIntelligentOperations(r.Context(), actor, project, limit, query.Get("before"))
		respond(w, http.StatusOK, out, err)
	case len(parts) == 2 && r.Method == http.MethodPost:
		var req IntelligentOperationCreateRequest
		if !decode(&req, "project_id", "goal", "locale", "sources", "tool_profile_id", "expected_tool_version", "acknowledge_model_use", "idempotency_key") {
			return true
		}
		if value, exists := body["max_output_tokens"]; exists {
			number, ok := value.(json.Number)
			if !ok {
				writeError(w, invalid(operationInvalid))
				return true
			}
			n, e := number.Int64()
			if e != nil || n < 1 || n > 8192 {
				writeError(w, invalid(operationInvalid))
				return true
			}
		}
		out, err := s.Service.CreateIntelligentOperation(r.Context(), s.Engine, actor, project, req)
		respond(w, http.StatusCreated, out, err)
	case len(parts) == 3 && r.Method == http.MethodGet:
		out, err := s.Service.GetIntelligentOperation(r.Context(), actor, project, parts[2])
		respond(w, http.StatusOK, out, err)
	case len(parts) == 4 && parts[3] == "preview" && r.Method == http.MethodPost:
		var req IntelligentOperationPreviewRequest
		if !decode(&req, "project_id", "expected_version") {
			return true
		}
		out, err := s.Service.PreviewIntelligentOperation(r.Context(), actor, project, parts[2], req)
		respond(w, http.StatusOK, out, err)
	case len(parts) == 4 && parts[3] == "commands" && r.Method == http.MethodPost:
		var req IntelligentOperationCommandRequest
		if !decode(&req, "project_id", "expected_version", "action", "reason", "idempotency_key") {
			return true
		}
		out, err := s.Service.CommandIntelligentOperation(r.Context(), actor, project, parts[2], req)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
