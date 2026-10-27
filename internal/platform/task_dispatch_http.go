package platform

import "net/http"

func (s *Server) taskDispatchHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, path string, parts []string, project string, m map[string]any) bool {
	if parts[0] != "task-dispatch-plans" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	switch {
	case len(parts) == 1 && r.Method == "POST":
		var request TaskDispatchPreviewRequest
		if err := validateModelSelectionFields(m); err != nil {
			writeError(w, err)
			return true
		}
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateTaskDispatchPlan(r.Context(), s.Engine, actor, project, request)
		respond(w, 201, out, err)
	case len(parts) == 2 && r.Method == "GET":
		out, err := s.Service.GetTaskDispatchPlan(r.Context(), s.Engine, actor, project, parts[1])
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[2] == "start" && r.Method == "POST":
		var request TaskDispatchStartRequest
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.StartTaskDispatchPlan(r.Context(), s.Engine, actor, project, parts[1], request)
		respond(w, 201, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
