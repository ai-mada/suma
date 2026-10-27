package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

// Recommendations are scoped observations. This endpoint never prepares or
// starts an execution, enables a profile, or reserves an execution budget.
func (s *Server) taskDispatchOptionsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string) bool {
	if len(parts) != 3 || parts[0] != "tasks" || parts[2] != "dispatch-options" {
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
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(taskDispatchOptionsInvalid))
		return true
	}
	allowed := map[string]bool{"project_id": true, "expected_task_version": true, "workflow_id": true, "workflow_version": true, "agent_node_id": true, "limit": true, "before": true}
	for name, values := range q {
		if !allowed[name] || len(values) != 1 || values[0] == "" {
			writeError(w, invalid(taskDispatchOptionsInvalid))
			return true
		}
	}
	for _, name := range []string{"project_id", "expected_task_version", "workflow_id", "workflow_version", "agent_node_id"} {
		if q.Get(name) == "" {
			writeError(w, invalid(taskDispatchOptionsInvalid))
			return true
		}
	}
	integer := func(name string, fallback, max int) (int, bool) {
		raw := q.Get(name)
		if raw == "" {
			return fallback, true
		}
		n, e := strconv.Atoi(raw)
		return n, e == nil && n > 0 && n <= max && strconv.Itoa(n) == raw
	}
	taskVersion, taskOK := integer("expected_task_version", 0, 2147483646)
	flowVersion, flowOK := integer("workflow_version", 0, 2147483646)
	limit, limitOK := integer("limit", 50, 200)
	if q.Get("project_id") != project || !taskOK || !flowOK || !limitOK {
		writeError(w, invalid(taskDispatchOptionsInvalid))
		return true
	}
	req := TaskDispatchOptionsRequest{ProjectID: project, TaskID: parts[1], ExpectedTaskVersion: taskVersion, WorkflowID: q.Get("workflow_id"), WorkflowVersion: flowVersion, AgentNodeID: q.Get("agent_node_id"), Limit: limit, Before: q.Get("before")}
	out, err := s.Service.GetTaskDispatchOptions(r.Context(), s.Engine, actor, project, parts[1], req)
	respond(w, http.StatusOK, out, err)
	return true
}
