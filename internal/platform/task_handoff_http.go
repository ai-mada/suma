package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

// History is a task-scoped keyset page. A cursor is resolved inside that same
// tenant/project/task by the service, never used as a SQL fragment or offset.
func taskHandoffQuery(r *http.Request) (url.Values, int, error) {
	bad := func() (url.Values, int, error) { return nil, 0, invalid("交接筛选或分页参数无效") }
	if len(r.URL.RawQuery) > 8192 {
		return bad()
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return bad()
	}
	for key, items := range values {
		if len(items) != 1 || key != "project_id" && (r.Method != http.MethodGet || key != "task_id" && key != "limit" && key != "before") {
			return bad()
		}
	}
	limit := 20
	if r.Method == http.MethodGet {
		if values.Get("project_id") == "" || values.Get("task_id") == "" || len(values.Get("task_id")) > 64 || len(values.Get("before")) > 64 {
			return bad()
		}
		if raw, present := values["limit"]; present {
			if raw[0] == "" || raw[0][0] == '0' {
				return bad()
			}
			for _, char := range raw[0] {
				if char < '0' || char > '9' {
					return bad()
				}
			}
			limit, err = strconv.Atoi(raw[0])
			if err != nil || limit < 1 || limit > 100 {
				return bad()
			}
		}
	}
	return values, limit, nil
}

func taskHandoffRequired(m map[string]any, fields ...string) error {
	for _, value := range m {
		if value == nil {
			return invalid("交接请求缺少完整且有效的字段")
		}
	}
	for _, field := range fields {
		if _, exists := m[field]; !exists {
			return invalid("交接请求缺少完整且有效的字段")
		}
	}
	return nil
}

func (s *Server) taskHandoffHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope string, parts []string, project string, m map[string]any) bool {
	if len(parts) == 0 || parts[0] != "task-handoffs" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	values, limit, err := taskHandoffQuery(r)
	if err != nil {
		writeError(w, err)
		return true
	}
	if value, present := values["project_id"]; present && value[0] != project {
		writeError(w, invalid("交接请求的项目范围不一致"))
		return true
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		out, err := s.Service.TaskHandoffs(r.Context(), actor, project, values.Get("task_id"), limit, values.Get("before"))
		respond(w, http.StatusOK, out, err)
	case len(parts) == 1 && r.Method == http.MethodPost:
		var request TaskHandoffCreateRequest
		if err := taskHandoffRequired(m, "project_id", "task_id", "expected_task_version", "to_resource_id", "note", "idempotency_key"); err != nil {
			writeError(w, err)
			return true
		}
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateTaskHandoff(r.Context(), actor, project, request)
		respond(w, http.StatusCreated, out, err)
	case len(parts) == 3 && parts[2] == "commands" && r.Method == http.MethodPost:
		var request TaskHandoffCommandRequest
		if err := taskHandoffRequired(m, "project_id", "expected_version", "action", "reason", "acknowledge_invalidation", "idempotency_key"); err != nil {
			writeError(w, err)
			return true
		}
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommandTaskHandoff(r.Context(), actor, project, parts[1], request)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
