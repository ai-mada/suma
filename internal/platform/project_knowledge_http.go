package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func projectKnowledgeRequired(m map[string]any, fields ...string) error {
	for _, field := range fields {
		if value, ok := m[field]; !ok || value == nil {
			return invalid("知识请求缺少完整且有效的字段")
		}
	}
	for _, field := range []string{"title", "summary"} {
		if value, exists := m[field]; exists && value == nil {
			return invalid("知识文本字段不能为null")
		}
	}
	if sources, ok := m["sources"]; ok {
		items, ok := sources.([]any)
		if !ok {
			return invalid("知识来源必须是数组")
		}
		for _, item := range items {
			source, ok := item.(map[string]any)
			if !ok {
				return invalid("知识来源必须是对象")
			}
			if quote, exists := source["quote"]; exists && quote == nil {
				return invalid("知识引用片段必须为字符串")
			}
		}
	}
	return nil
}
func projectKnowledgeQuery(r *http.Request) (ProjectKnowledgeListQuery, error) {
	var out ProjectKnowledgeListQuery
	if len(r.URL.RawQuery) > 8192 {
		return out, invalid("知识筛选或分页参数无效")
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return out, invalid("知识筛选或分页参数无效")
	}
	for key, items := range values {
		if key != "project_id" && key != "kind" && key != "state" && key != "q" && key != "limit" && key != "cursor" || len(items) != 1 {
			return out, invalid("知识筛选或分页参数无效")
		}
	}
	out.Kind, out.State, out.Query, out.Cursor = values.Get("kind"), values.Get("state"), values.Get("q"), values.Get("cursor")
	if raw, ok := values["limit"]; ok {
		if len(raw[0]) == 0 || raw[0][0] == '0' {
			return out, invalid("知识筛选或分页参数无效")
		}
		for _, c := range raw[0] {
			if c < '0' || c > '9' {
				return out, invalid("知识筛选或分页参数无效")
			}
		}
		out.Limit, err = strconv.Atoi(raw[0])
		if err != nil {
			return out, invalid("知识筛选或分页参数无效")
		}
	}
	return out, nil
}
func (s *Server) projectKnowledgeHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, parts []string, m map[string]any) bool {
	if s.knowledgeProjectionHTTP(w, r, actor, project, parts) {
		return true
	}
	if s.knowledgeDiscoveryHTTP(w, r, actor, project, parts) {
		return true
	}
	if len(parts) == 0 || parts[0] != "project-knowledge" {
		return false
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		query, err := projectKnowledgeQuery(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ListProjectKnowledge(r.Context(), actor, project, query)
		respond(w, 200, out, err)
	case len(parts) == 1 && r.Method == http.MethodPost:
		var request ProjectKnowledgeWriteRequest
		if err := projectKnowledgeRequired(m, "project_id", "kind", "title", "summary", "content", "sources", "expected_version", "idempotency_key"); err != nil {
			writeError(w, err)
			return true
		}
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateProjectKnowledge(r.Context(), actor, project, request)
		respond(w, 201, out, err)
	case len(parts) == 2 && parts[1] == "extractions" && r.Method == http.MethodPost:
		var request ProjectKnowledgeExtractionRequest
		if err := projectKnowledgeRequired(m, "project_id", "kind", "sources", "idempotency_key"); err != nil {
			writeError(w, err)
			return true
		}
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ExtractProjectKnowledge(r.Context(), actor, project, request)
		respond(w, 201, out, err)
	case len(parts) == 2 && r.Method == http.MethodGet:
		values, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(values) != 1 || len(values["project_id"]) != 1 {
			writeError(w, invalid("知识详情只接受唯一项目参数"))
			return true
		}
		out, err := s.Service.GetProjectKnowledge(r.Context(), actor, project, parts[1])
		respond(w, 200, out, err)
	case len(parts) == 2 && r.Method == http.MethodPatch:
		var request ProjectKnowledgeWriteRequest
		if err := projectKnowledgeRequired(m, "project_id", "kind", "title", "summary", "content", "sources", "expected_version", "idempotency_key"); err != nil {
			writeError(w, err)
			return true
		}
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.UpdateProjectKnowledge(r.Context(), actor, project, parts[1], request)
		respond(w, 200, out, err)
	case len(parts) == 4 && parts[2] == "revisions" && r.Method == http.MethodGet:
		w.Header().Set("Cache-Control", "no-store")
		values, err := url.ParseQuery(r.URL.RawQuery)
		revision, parseErr := strconv.Atoi(parts[3])
		if err != nil || len(values) != 1 || len(values["project_id"]) != 1 || values.Get("project_id") != project || project == "" || parseErr != nil || revision < 1 || revision > 2147483647 || strconv.Itoa(revision) != parts[3] {
			writeError(w, invalid(projectKnowledgeRevisionInvalid))
			return true
		}
		out, err := s.Service.GetProjectKnowledgeRevision(r.Context(), actor, project, parts[1], revision)
		respond(w, http.StatusOK, out, err)
	case len(parts) == 3 && parts[2] == "revisions" && r.Method == http.MethodGet:
		query, err := projectKnowledgeQuery(r)
		if err != nil || query.Kind != "" || query.State != "" || query.Query != "" {
			if err == nil {
				err = invalid("知识历史只接受项目、分页大小与游标")
			}
			writeError(w, err)
			return true
		}
		values, _ := url.ParseQuery(r.URL.RawQuery)
		for key := range values {
			if key != "project_id" && key != "limit" && key != "cursor" {
				writeError(w, invalid("知识历史只接受项目、分页大小与游标"))
				return true
			}
		}
		out, err := s.Service.ProjectKnowledgeRevisions(r.Context(), actor, project, parts[1], query.Limit, query.Cursor)
		respond(w, 200, out, err)

	case len(parts) == 3 && parts[2] == "commands" && r.Method == http.MethodPost:
		var request ProjectKnowledgeCommandRequest
		if err := projectKnowledgeRequired(m, "project_id", "action", "expected_version", "idempotency_key"); err != nil {
			writeError(w, err)
			return true
		}
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommandProjectKnowledge(r.Context(), actor, project, parts[1], request)
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
