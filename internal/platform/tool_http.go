package platform

import (
	"encoding/json"
	"net/http"

	"github.com/ai-mada/suma/internal/strictjson"
)

func (s *Server) toolProfilesHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, path string, parts []string, project string, m map[string]any) bool {
	if s.toolProfileReplacementHTTP(w, r, actor, scope, parts, project, m) {
		return true
	}
	if s.toolRecoveriesHTTP(w, r, actor, scope, parts, project, m) {
		return true
	}
	if s.toolInspectionsHTTP(w, r, actor, scope, parts, project, m) {
		return true
	}
	if s.toolDiscoveryHTTP(w, r, actor, scope, project, parts) {
		return true
	}
	if len(parts) == 0 || parts[0] != "tool-profiles" {
		return false
	}
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	switch {
	case len(parts) == 2 && parts[1] == "matches" && r.Method == http.MethodPost:
		var req ToolProfileMatchRequest
		raw, err := json.Marshal(m)
		if err != nil || strictjson.Decode(raw, &req, 2<<20) != nil {
			writeError(w, invalid("工具名称、执行节点或项目无效"))
			return true
		}
		out, err := s.Service.MatchToolProfiles(r.Context(), actor, project, req)
		respond(w, http.StatusOK, out, err)
	case len(parts) == 1 && r.Method == http.MethodPost:
		var req ToolProfileCreateRequest
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateToolProfile(r.Context(), actor, project, req)
		respond(w, 201, out, err)
	case len(parts) == 1 && r.Method == http.MethodGet:
		query, err := toolProfileListHTTPQuery(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ListToolProfiles(r.Context(), actor, project, query)
		respond(w, 200, out, err)
	case len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.GetToolProfile(r.Context(), actor, project, parts[1])
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[2] == "diagnostics" && r.Method == http.MethodGet:
		out, err := s.Service.GetToolProfileDiagnostics(r.Context(), actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
	case len(parts) == 3 && parts[2] == "commands" && r.Method == http.MethodPost:
		var req ToolProfileCommandRequest
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommandToolProfile(r.Context(), actor, project, parts[1], req)
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
