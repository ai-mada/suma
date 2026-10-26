package platform

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// The ordinary current-project transport supplies actor and project. Query
// values cannot replace that view; model output never becomes a command DTO.
func (s *Server) knowledgeIntelligenceHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, m map[string]any) bool {
	if len(parts) < 2 || parts[0] != "knowledge-intelligence" || parts[1] != "analyses" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(intelligenceInvalid))
		return true
	}
	for key, items := range values {
		if len(items) != 1 || key != "project_id" && !(r.Method == http.MethodGet && len(parts) == 2 && (key == "limit" || key == "before")) {
			writeError(w, invalid(intelligenceInvalid))
			return true
		}
	}
	if v, ok := values["project_id"]; ok && (v[0] == "" || v[0] != project) || r.Method == http.MethodGet && values.Get("project_id") == "" {
		writeError(w, invalid(intelligenceInvalid))
		return true
	}
	switch {
	case len(parts) == 2 && r.Method == http.MethodGet:
		limit := 20
		if raw, ok := values["limit"]; ok {
			if raw[0] == "" || raw[0][0] == '0' {
				writeError(w, invalid(intelligenceInvalid))
				return true
			}
			for _, c := range raw[0] {
				if c < '0' || c > '9' {
					writeError(w, invalid(intelligenceInvalid))
					return true
				}
			}
			limit, err = strconv.Atoi(raw[0])
			if err != nil || limit < 1 || limit > 100 {
				writeError(w, invalid(intelligenceInvalid))
				return true
			}
		}
		out, err := s.Service.ListKnowledgeIntelligence(r.Context(), actor, project, limit, values.Get("before"))
		respond(w, 200, out, err)
	case len(parts) == 2 && r.Method == http.MethodPost:
		var req KnowledgeIntelligenceCreateRequest
		if err := projectKnowledgeRequired(m, "project_id", "kind", "goal", "locale", "sources", "tool_profile_id", "expected_tool_version", "acknowledge_model_use", "idempotency_key"); err != nil {
			writeError(w, invalid(intelligenceInvalid))
			return true
		}
		for field, maximum := range map[string]int64{"max_steps": 1, "max_output_tokens": 8192} {
			if raw, exists := m[field]; exists {
				n, ok := raw.(json.Number)
				if !ok {
					writeError(w, invalid(intelligenceInvalid))
					return true
				}
				value, e := n.Int64()
				if e != nil || value < 1 || value > maximum {
					writeError(w, invalid(intelligenceInvalid))
					return true
				}
			}
		}
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateKnowledgeIntelligence(r.Context(), s.Engine, actor, project, req)
		respond(w, 201, out, err)
	case len(parts) == 3 && r.Method == http.MethodGet:
		out, err := s.Service.GetKnowledgeIntelligence(r.Context(), actor, project, parts[2])
		respond(w, 200, out, err)
	case len(parts) == 4 && parts[3] == "commands" && r.Method == http.MethodPost:
		var req KnowledgeIntelligenceCommandRequest
		if err := projectKnowledgeRequired(m, "project_id", "expected_version", "action", "reason", "idempotency_key"); err != nil {
			writeError(w, invalid(intelligenceInvalid))
			return true
		}
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommandKnowledgeIntelligence(r.Context(), actor, project, parts[2], req)
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
