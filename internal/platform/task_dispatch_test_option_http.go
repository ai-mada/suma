package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func (s *Server) taskDispatchTestOptionHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, taskID string) {
	w.Header().Set("Cache-Control", "no-store")
	if actor.kind != actorBrowser {
		writeError(w, forbidden())
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return
	}
	if len(r.URL.RawQuery) > 4096 {
		writeError(w, invalid(taskDispatchTestOptionInvalid))
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	required := []string{
		"project_id", "expected_task_version", "workflow_id", "workflow_version", "agent_node_id",
		"development_tool_profile_id", "development_tool_profile_version", "test_node_id", "test_tool_profile_id", "test_tool_profile_version",
	}
	if err != nil || len(q) != len(required) || q.Get("project_id") != project {
		writeError(w, invalid(taskDispatchTestOptionInvalid))
		return
	}
	for _, key := range required {
		if len(q[key]) != 1 || q.Get(key) == "" {
			writeError(w, invalid(taskDispatchTestOptionInvalid))
			return
		}
	}

	req := TaskDispatchTestOptionRequest{
		ProjectID: project, TaskID: taskID,
		WorkflowID: q.Get("workflow_id"), AgentNodeID: q.Get("agent_node_id"),
		DevelopmentToolProfileID: q.Get("development_tool_profile_id"),
		TestNodeID:               q.Get("test_node_id"), TestToolProfileID: q.Get("test_tool_profile_id"),
	}
	for _, field := range []struct {
		key   string
		value *int
	}{
		{"expected_task_version", &req.ExpectedTaskVersion},
		{"workflow_version", &req.WorkflowVersion},
		{"development_tool_profile_version", &req.DevelopmentToolProfileVersion},
		{"test_tool_profile_version", &req.TestToolProfileVersion},
	} {
		raw := q.Get(field.key)
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > taskDispatchTestOptionMaxVersion || strconv.Itoa(n) != raw {
			writeError(w, invalid(taskDispatchTestOptionInvalid))
			return
		}
		*field.value = n
	}
	out, err := s.Service.GetTaskDispatchTestOption(r.Context(), s.Engine, actor, project, taskID, req)
	respond(w, http.StatusOK, out, err)
}
