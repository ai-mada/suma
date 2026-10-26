package platform

import "context"

// ApplicationSkillCatalogListInput exposes reviewed organization text through
// the credential's current project. Callers cannot select a different tenant.
type ApplicationSkillCatalogListInput struct {
	Domain   string `json:"domain,omitempty"`
	Scenario string `json:"scenario,omitempty"`
	Tag      string `json:"tag,omitempty"`
	State    string `json:"state,omitempty"`
	Query    string `json:"query,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}

// A catalogue result is discovery data. Exact version and digest are required
// before returning its body; a new publication never silently replaces a pin.
type ApplicationSkillCatalogGetInput struct {
	ID           string `json:"id"`
	Version      int    `json:"version"`
	OriginSHA256 string `json:"origin_sha256"`
}

func applicationSkillDefinitions(makeDefinition func(string, string, string, map[string]any, bool, ...string) ApplicationToolDefinition) []ApplicationToolDefinition {
	return []ApplicationToolDefinition{
		makeDefinition("project.skill_catalog_list", "检索当前授权组织的共享技能目录，不安装或启用技能", "application_tool_project_skill_catalog_list", applicationObject(map[string]any{
			"domain": applicationString(80), "scenario": applicationString(80), "tag": applicationString(40),
			"state": map[string]any{"type": "string", "enum": []string{"published", "retired"}},
			"query": applicationString(200), "limit": applicationInteger(1, 100), "cursor": applicationString(4096),
		}), true, "api"),
		makeDefinition("project.skill_catalog_get", "读取共享技能的明确版本和摘要，原来源原件与权限不随文本共享", "application_tool_project_skill_catalog_get", applicationObject(map[string]any{
			"id": applicationString(64), "version": applicationInteger(1, 2147483647),
			"origin_sha256": map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$", "minLength": 64, "maxLength": 64},
		}, "id", "version", "origin_sha256"), true, "api"),
	}
}

// Invoked within the existing current-authority read transaction. No public
// service call or nested transaction can detach discovery from its authority.
func (s *Service) invokeApplicationSkillRead(ctx context.Context, auth *authorizedCommand, call ApplicationToolCall) (any, error) {
	switch call.Tool {
	case "project.skill_catalog_list":
		var input ApplicationSkillCatalogListInput
		if err := strictApplicationInput(call.Arguments, &input); err != nil {
			return nil, err
		}
		if input.Limit < 0 || applicationHasField(call.Arguments, "limit") && input.Limit == 0 || applicationHasField(call.Arguments, "state") && input.State == "" {
			return nil, applicationInputError()
		}
		return s.listSkillCatalogTx(ctx, auth, SkillCatalogListQuery{ProjectID: call.ProjectID, Domain: input.Domain, Scenario: input.Scenario, Tag: input.Tag, State: input.State, Query: input.Query, Limit: input.Limit, Cursor: input.Cursor})
	case "project.skill_catalog_get":
		var input ApplicationSkillCatalogGetInput
		if err := strictApplicationInput(call.Arguments, &input, "id", "version", "origin_sha256"); err != nil {
			return nil, err
		}
		if !applicationID(input.ID) || input.Version < 1 || input.Version > 2147483647 || !applicationSkillDigest(input.OriginSHA256) {
			return nil, applicationInputError()
		}
		detail, err := s.skillCatalogDetailTx(ctx, auth, call.ProjectID, input.ID)
		if err != nil {
			return nil, err
		}
		if detail.Version != input.Version || detail.Origin.SHA256 != input.OriginSHA256 {
			return nil, conflict("共享技能版本已变化，请重新检索并核对")
		}
		return detail, nil
	}
	return nil, &APIError{422, "tool_not_allowed", "该工具尚未授权或未实现", nil}
}

func applicationSkillDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
