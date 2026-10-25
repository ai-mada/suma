package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func designRequiredFields(m map[string]any, names ...string) error {
	for _, name := range names {
		if value, exists := m[name]; !exists || value == nil {
			return invalid("需求设计请求缺少完整且有效的字段")
		}
	}
	return nil
}

// JSON null must not silently turn a full replacement into zero values. Strict
// DTO decoding separately rejects unknown fields, wrong types and number lexemes.
func designDraftEnvelope(m map[string]any) error {
	if err := designRequiredFields(m, "project_id", "expected_version", "expected_requirement_version", "content", "tasks", "idempotency_key"); err != nil {
		return err
	}
	content, ok := m["content"].(map[string]any)
	if !ok {
		return invalid("需求设计正文必须是完整的产品、体验与技术设计对象")
	}
	if err := designRequiredFields(content, "product", "experience", "technical"); err != nil {
		return err
	}
	tasks, ok := m["tasks"].([]any)
	if !ok {
		return invalid("需求设计任务必须是完整数组")
	}
	for _, raw := range tasks {
		task, ok := raw.(map[string]any)
		if !ok {
			return invalid("需求设计任务必须是对象")
		}
		if err := designRequiredFields(task, "key", "title", "description", "priority", "owner_id", "depends_on", "acceptance_criteria", "covers"); err != nil {
			return err
		}
		for _, field := range []string{"depends_on", "acceptance_criteria", "covers"} {
			items, ok := task[field].([]any)
			if !ok {
				return invalid("需求设计任务的依赖、验收与覆盖必须是数组")
			}
			for _, item := range items {
				if item == nil {
					return invalid("需求设计任务数组不能含null")
				}
			}
		}
	}
	return nil
}

func designReadQuery(r *http.Request, project string, history bool) (limit int, cursor string, err error) {
	if len(r.URL.RawQuery) > 8192 {
		return 0, "", invalid("需求设计筛选或分页参数无效")
	}
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return 0, "", invalid("需求设计筛选或分页参数无效")
	}
	for key, items := range values {
		if len(items) != 1 || key != "project_id" && (!history || key != "limit" && key != "cursor") {
			return 0, "", invalid("需求设计筛选或分页参数无效")
		}
	}
	if values.Get("project_id") == "" || values.Get("project_id") != project {
		return 0, "", invalid("需求设计项目与当前范围不一致")
	}
	cursor = values.Get("cursor")
	if len(cursor) > 4096 {
		return 0, "", invalid("需求设计筛选或分页参数无效")
	}
	if raw, exists := values["limit"]; exists {
		if raw[0] == "" || raw[0][0] == '0' {
			return 0, "", invalid("需求设计筛选或分页参数无效")
		}
		for _, char := range raw[0] {
			if char < '0' || char > '9' {
				return 0, "", invalid("需求设计筛选或分页参数无效")
			}
		}
		limit, e = strconv.Atoi(raw[0])
		if e != nil || limit < 1 || limit > 100 {
			return 0, "", invalid("需求设计筛选或分页参数无效")
		}
	}
	return limit, cursor, nil
}

func (s *Server) requirementDesignHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, parts []string, m map[string]any) bool {
	if len(parts) < 3 || parts[0] != "requirements" || parts[2] != "design" {
		return false
	}
	id := parts[1]
	switch {
	case len(parts) == 3 && r.Method == http.MethodGet:
		if _, _, err := designReadQuery(r, project, false); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.GetRequirementDesign(r.Context(), actor, project, id)
		respond(w, 200, out, err)
	case len(parts) == 3 && r.Method == http.MethodPut:
		if err := designDraftEnvelope(m); err != nil {
			writeError(w, err)
			return true
		}
		var request RequirementDesignSaveRequest
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.SaveRequirementDesign(r.Context(), actor, project, id, request)
		respond(w, 200, out, err)
	case len(parts) == 4 && parts[3] == "commands" && r.Method == http.MethodPost:
		if err := designRequiredFields(m, "project_id", "action", "expected_version", "expected_requirement_version", "idempotency_key"); err != nil {
			writeError(w, err)
			return true
		}
		var request RequirementDesignCommandRequest
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommandRequirementDesign(r.Context(), actor, project, id, request)
		respond(w, 200, out, err)
	case len(parts) == 4 && parts[3] == "preview" && r.Method == http.MethodPost:
		if err := designRequiredFields(m, "project_id", "expected_version", "revision", "sha256"); err != nil {
			writeError(w, err)
			return true
		}
		var request RequirementDesignPreviewRequest
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.PreviewRequirementDesign(r.Context(), actor, project, id, request)
		respond(w, 200, out, err)
	case len(parts) == 4 && parts[3] == "adopt" && r.Method == http.MethodPost:
		if err := designRequiredFields(m, "project_id", "expected_version", "revision", "sha256", "plan_sha256", "idempotency_key"); err != nil {
			writeError(w, err)
			return true
		}
		var request RequirementDesignAdoptRequest
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.AdoptRequirementDesign(r.Context(), actor, project, id, request)
		respond(w, 200, out, err)
	case len(parts) == 4 && r.Method == http.MethodGet && (parts[3] == "revisions" || parts[3] == "adoptions"):
		limit, cursor, err := designReadQuery(r, project, true)
		if err != nil {
			writeError(w, err)
			return true
		}
		if parts[3] == "revisions" {
			out, err := s.Service.RequirementDesignRevisions(r.Context(), actor, project, id, limit, cursor)
			respond(w, 200, out, err)
		} else {
			out, err := s.Service.RequirementDesignAdoptions(r.Context(), actor, project, id, limit, cursor)
			respond(w, 200, out, err)
		}
	default:
		writeError(w, missing())
	}
	return true
}
