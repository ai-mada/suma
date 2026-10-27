package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

// Model options are observations under the same current project/tool authority
// as dispatch. Reading them never probes a provider or changes configuration.
func (s *Server) modelExecutionHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string) bool {
	if len(parts) != 2 || parts[0] != "model-execution" || parts[1] != "options" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return true
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 4096 {
		writeError(w, invalid(modelExecutionInvalid))
		return true
	}
	for key, values := range q {
		if (key != "project_id" && key != "tool_profile_id" && key != "expected_tool_version") || len(values) != 1 || values[0] == "" {
			writeError(w, invalid(modelExecutionInvalid))
			return true
		}
	}
	version, err := strconv.Atoi(q.Get("expected_tool_version"))
	if err != nil || version < 1 || version > 2147483646 || strconv.Itoa(version) != q.Get("expected_tool_version") || q.Get("project_id") != project {
		writeError(w, invalid(modelExecutionInvalid))
		return true
	}
	value, err := s.Service.GetModelExecutionOptions(r.Context(), actor, project, ModelExecutionOptionsRequest{ProjectID: project, ToolProfileID: q.Get("tool_profile_id"), ExpectedToolVersion: version})
	respond(w, http.StatusOK, value, err)
	return true
}
