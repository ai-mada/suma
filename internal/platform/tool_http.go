package platform

import "net/http"

func (s *Server) toolProfilesHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, path string, parts []string, project string, m map[string]any) bool {
	if len(parts) == 0 || parts[0] != "tool-profiles" {
		return false
	}
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodPost:
		var req ToolProfileCreateRequest
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateToolProfile(r.Context(), actor, project, req)
		respond(w, 201, out, err)
	case len(parts) == 1 && r.Method == http.MethodGet:
		limit, cursor, err := automationPage(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ListToolProfiles(r.Context(), actor, project, limit, cursor)
		respond(w, 200, out, err)
	case len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.GetToolProfile(r.Context(), actor, project, parts[1])
		respond(w, 200, out, err)
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
