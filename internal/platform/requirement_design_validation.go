package platform

import (
	"context"
	"encoding/json"
	"strings"
)

func designVersion(v int) bool { return v >= 0 && v < 2147483647 }
func designSnapshot(r Record) TaskContextRecord {
	return TaskContextRecord{ID: r.ID, Version: r.Version, Title: r.Title, Description: r.Description, AcceptanceCriteria: append([]string{}, r.AcceptanceCriteria...)}
}
func designSourceCurrent(r Record, fixed TaskContextRecord) bool {
	return r.Status != "archived" && r.Status != "deleted" && stringify(designSnapshot(r)) == stringify(fixed)
}
func designRevisionHash(r RequirementDesignRevision) string {
	return digest(stringify(struct {
		Requirement TaskContextRecord
		Content     RequirementDesignContent
		Tasks       []RequirementDesignTask
	}{r.Requirement, r.Content, r.Tasks}))
}
func designKey(key string) bool {
	if len(key) < 1 || len(key) > 64 {
		return false
	}
	for _, c := range key {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// Hard safety and shape limits apply even to an intentionally incomplete draft.
// Completeness and graph diagnostics remain editable until explicit publication.
func validateDesignDraft(content RequirementDesignContent, tasks []RequirementDesignTask) error {
	if !knowledgeText(content.Product, 16000) || !knowledgeText(content.Experience, 16000) || !knowledgeText(content.Technical, 16000) || len(content.Product)+len(content.Experience)+len(content.Technical) > 16000 {
		return invalid("设计正文合计不得超过16000字节")
	}
	raw, e := json.Marshal(tasks)
	if e != nil || len(raw) > 256*1024 || tasks == nil || len(tasks) > 100 {
		return invalid("设计任务提案超出限制或集合无效")
	}
	keys := map[string]bool{}
	for _, t := range tasks {
		if !designKey(t.Key) || keys[t.Key] || !knowledgeText(t.Title, 200) || !knowledgeText(t.Description, 4000) || !knowledgeText(t.OwnerID, 64) || len(t.DependsOn) > 100 || len(t.AcceptanceCriteria) > 20 || len(t.Covers) > 100 || t.DependsOn == nil || t.AcceptanceCriteria == nil || t.Covers == nil {
			return invalid("设计任务的标识、文本、集合或大小无效")
		}
		keys[t.Key] = true
		if t.Priority != "urgent" && t.Priority != "high" && t.Priority != "medium" && t.Priority != "low" {
			return invalid("设计任务优先级无效")
		}
		seen := map[string]bool{}
		for _, d := range t.DependsOn {
			if !designKey(d) || seen[d] {
				return invalid("设计任务依赖标识重复或无效")
			}
			seen[d] = true
		}
		for _, c := range t.AcceptanceCriteria {
			if !knowledgeText(c, 1000) {
				return invalid("设计任务验收文本超出限制")
			}
		}
	}
	return nil
}
func designDiagnostics(r RequirementDesignRevision) []RequirementDesignDiagnostic {
	out := []RequirementDesignDiagnostic{}
	add := func(code, field, key string, index int) {
		out = append(out, RequirementDesignDiagnostic{Code: code, Field: field, TaskKey: key, CriterionIndex: index})
	}
	for _, s := range []struct{ field, text string }{{"product", r.Content.Product}, {"experience", r.Content.Experience}, {"technical", r.Content.Technical}} {
		if strings.TrimSpace(s.text) == "" {
			add(s.field+"_required", s.field, "", 0)
		}
	}
	if len(r.Requirement.AcceptanceCriteria) == 0 {
		add("requirement_criteria_required", "tasks", "", 0)
	}
	for _, c := range r.Requirement.AcceptanceCriteria {
		if strings.TrimSpace(c) == "" {
			add("requirement_criteria_required", "tasks", "", 0)
			break
		}
	}
	if len(r.Tasks) == 0 {
		add("tasks_required", "tasks", "", 0)
	}
	keys := map[string]bool{}
	graph := map[string][]string{}
	covered := map[int]bool{}
	for _, t := range r.Tasks {
		keys[t.Key] = true
		graph[t.Key] = t.DependsOn
	}
	for _, t := range r.Tasks {
		if strings.TrimSpace(t.Title) == "" {
			add("task_title_required", "tasks", t.Key, 0)
		}
		valid := len(t.AcceptanceCriteria) > 0
		for _, c := range t.AcceptanceCriteria {
			if strings.TrimSpace(c) == "" {
				valid = false
			}
		}
		if !valid {
			add("task_criteria_required", "tasks", t.Key, 0)
		}
		for _, d := range t.DependsOn {
			if !keys[d] {
				add("dependency_missing", "tasks", t.Key, 0)
			}
		}
		indices := map[int]bool{}
		for _, c := range t.Covers {
			if c < 1 || c > len(r.Requirement.AcceptanceCriteria) || indices[c] {
				add("criterion_invalid", "tasks", t.Key, c)
			} else {
				covered[c] = true
			}
			indices[c] = true
		}
	}
	state := map[string]int{}
	var visit func(string) bool
	visit = func(k string) bool {
		if state[k] == 1 {
			return true
		}
		if state[k] == 2 || !keys[k] {
			return false
		}
		state[k] = 1
		for _, d := range graph[k] {
			if visit(d) {
				return true
			}
		}
		state[k] = 2
		return false
	}
	for _, t := range r.Tasks {
		if visit(t.Key) {
			add("dependency_cycle", "tasks", t.Key, 0)
			break
		}
	}
	for i := range r.Requirement.AcceptanceCriteria {
		if !covered[i+1] {
			add("criterion_uncovered", "tasks", "", i+1)
		}
	}
	return out
}
func (s *Service) validateDesignOwnersTx(ctx context.Context, auth *authorizedCommand, project string, tasks []RequirementDesignTask) error {
	seen := map[string]bool{}
	for _, t := range tasks {
		if t.OwnerID == "" || seen[t.OwnerID] {
			continue
		}
		seen[t.OwnerID] = true
		owner, e := s.getRecordTx(ctx, auth, project, "members", t.OwnerID)
		if e != nil {
			return invalid("负责人不属于当前项目")
		}
		if owner.Status == "archived" || owner.Status == "deleted" {
			return invalid("负责人资源已停用")
		}
		if owner.Kind == "human" {
			user, _ := owner.Data["user_id"].(string)
			if e = s.BindHumanResourceTx(ctx, auth.tx, auth, project, user); e != nil {
				return e
			}
		}
	}
	return nil
}
