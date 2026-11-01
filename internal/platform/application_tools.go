package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// Declarations describe available application effects. Every returned nested
// schema and scope slice is newly allocated; callers cannot edit later lists.
func DeclaredApplicationTools() []ApplicationToolDefinition {
	makeDefinition := func(name, description, key string, parameters map[string]any, readonly bool, scopes ...string) ApplicationToolDefinition {
		return ApplicationToolDefinition{Name: name, MCPName: "suma_" + strings.ReplaceAll(name, ".", "_"), Description: description, DescriptionKey: key, Parameters: parameters, ReadOnly: readonly, CredentialScopes: append([]string{}, scopes...)}
	}
	knowledge := applicationObject(map[string]any{"references": applicationArray(applicationObject(map[string]any{"id": applicationString(64), "revision": applicationInteger(1, 2147483647), "sha256": map[string]any{"type": "string", "minLength": 64, "maxLength": 64}}, "id", "revision", "sha256"), 16)}, "references")
	knowledge["properties"].(map[string]any)["references"].(map[string]any)["minItems"] = 1
	listKnowledge := applicationObject(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"memory", "spec", "skill"}}, "state": map[string]any{"type": "string", "enum": []string{"draft", "published", "retired"}}, "query": applicationString(200), "limit": applicationInteger(1, 100), "cursor": applicationString(4096)})
	definitions := []ApplicationToolDefinition{
		makeDefinition("project.overview", "读取当前授权项目的真实状态与活动", "agent.tool_overview", applicationObject(map[string]any{}), true, "agent", "api"),
		makeDefinition("project.tasks", "读取当前项目任务", "agent.tool_tasks", applicationTaskPageSchema(), true, "agent", "api"),
		makeDefinition("project.search", "检索当前项目资料并返回来源", "agent.tool_search", applicationObject(map[string]any{"query": applicationString(500)}, "query"), true, "agent", "api"),
		makeDefinition("project.knowledge", "读取明确发布的项目知识固定修订并核验当前来源", "knowledge.agent_read", knowledge, true, "agent", "api"),
		makeDefinition("project.knowledge.search", "发现当前可采用的已发布记忆、规范和技能，不返回草稿或自动选入上下文", "knowledge.agent_discovery", knowledgeDiscoveryParameters(), true, "agent", "api"),
		makeDefinition("project.knowledge.index_search", "通过已配置索引发现当前可采用的已发布知识并核验准确引用，不自动采用", "knowledge.agent_index_search", knowledgeIndexParameters(), true, "agent", "api"),
		makeDefinition("project.requirements", "分页读取当前项目需求", "application_tool_project_requirements", applicationPageSchema(), true, "api"),
		makeDefinition("project.requirement", "读取当前项目的单个正式需求", "application_tool_project_requirement", applicationObject(map[string]any{"id": applicationString(64)}, "id"), true, "api"),
		makeDefinition("project.task", "读取当前项目的单个正式任务", "application_tool_project_task", applicationObject(map[string]any{"id": applicationString(64)}, "id"), true, "api"),
		makeDefinition("project.requirement_design", "读取当前需求设计与发布资格", "application_tool_project_requirement_design", applicationObject(map[string]any{"requirement_id": applicationString(64)}, "requirement_id"), true, "api"),
		makeDefinition("project.task_design", "读取任务原采用设计依据与当前资格", "application_tool_project_task_design", applicationObject(map[string]any{"task_id": applicationString(64)}, "task_id"), true, "api"),
		makeDefinition("project.knowledge_list", "分页读取当前项目知识元数据与草稿状态", "application_tool_project_knowledge_list", listKnowledge, true, "api"),
		makeDefinition("project.requirement_draft_create", "创建当前项目需求草稿，不批准或启动执行", "application_tool_project_requirement_draft_create", applicationDraftSchema(false), false, "api"),
		makeDefinition("project.task_draft_create", "创建当前项目待办任务，不验收或启动执行", "application_tool_project_task_draft_create", applicationDraftSchema(true), false, "api"),
		makeDefinition("project.requirement_design_save", "保存版本化需求设计草稿，不发布或采用任务", "application_tool_project_requirement_design_save", applicationDesignSchema(), false, "api"),
	}
	definitions = append(definitions, applicationSkillDefinitions(makeDefinition)...)
	return append(definitions, applicationQualityDefinitions(makeDefinition)...)
}

func nativeReadToolAllowed(name string) bool {
	switch name {
	case "project.overview", "project.tasks", "project.search", "project.knowledge", "project.knowledge.search", "project.knowledge.index_search":
		return true
	}
	return false
}
func applicationDefinition(name string) (ApplicationToolDefinition, bool) {
	for _, definition := range DeclaredApplicationTools() {
		if definition.Name == name {
			return definition, true
		}
	}
	return ApplicationToolDefinition{}, false
}
func applicationActorAllows(actor Actor, definition ApplicationToolDefinition) bool {
	if actor.kind == actorProjectAPI {
		return true
	}
	return (actor.kind == actorAgentRead || actor.kind == actorBrowser) && nativeReadToolAllowed(definition.Name) && definition.ReadOnly
}

func (s *Service) ApplicationTools(ctx context.Context, actor Actor, projectID string) (out ApplicationToolList, err error) {
	out.Items = []ApplicationToolDefinition{}
	if projectID == "" {
		return out, forbidden()
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: projectID}, knowledgeReadPurpose(actor), func(auth *authorizedCommand) error {
		canWrite := false
		if actor.kind == actorProjectAPI {
			_, e := s.authorizeTx(auth.ctx, auth.tx, actor, auth.target, purposeProjectWrite)
			if e != nil {
				var api *APIError
				if !errors.As(e, &api) || api.Status != 403 {
					return e
				}
			} else {
				canWrite = true
			}
		}
		for _, definition := range DeclaredApplicationTools() {
			if applicationActorAllows(actor, definition) && (definition.ReadOnly || canWrite) {
				out.Items = append(out.Items, definition)
			}
		}
		return nil
	})
	if err != nil {
		return ApplicationToolList{}, err
	}
	return
}

// InvokeApplicationTool never treats a previously returned catalogue as proof.
// Reads resolve their complete result under one current-authority transaction;
// draft effects call the existing services' own authority/receipt transaction.
func (s *Service) InvokeApplicationTool(ctx context.Context, actor Actor, call ApplicationToolCall) (out any, err error) {
	definition, exists := applicationDefinition(call.Tool)
	if !exists {
		return nil, &APIError{422, "tool_not_allowed", "该工具尚未授权或未实现", nil}
	}
	if !applicationActorAllows(actor, definition) {
		return nil, forbidden()
	}
	if !definition.ReadOnly {
		return s.invokeApplicationDraft(ctx, actor, call)
	}
	if call.Tool == "project.knowledge.index_search" {
		return s.searchProjectKnowledgeIndex(ctx, actor, call.ProjectID, func() (ProjectKnowledgeIndexQuery, error) {
			var input ProjectKnowledgeIndexQuery
			if e := strictApplicationInput(call.Arguments, &input, "q"); e != nil {
				return input, e
			}
			if applicationHasField(call.Arguments, "limit") && input.Limit == 0 || applicationHasField(call.Arguments, "kind") && input.Kind == "" {
				return input, knowledgeIndexInvalid()
			}
			return input, nil
		})
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: call.ProjectID}, knowledgeReadPurpose(actor), func(auth *authorizedCommand) error {
		var e error
		out, e = s.invokeApplicationRead(auth.ctx, auth, call)
		if e != nil {
			return e
		}
		raw, e := json.Marshal(out)
		if e != nil {
			return e
		}
		if len(raw) > applicationToolOutputLimit {
			return &APIError{422, "tool_output_limit", "工具读取结果超过64KiB，请缩小页面或引用范围", nil}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) invokeApplicationRead(ctx context.Context, auth *authorizedCommand, call ApplicationToolCall) (any, error) {
	project, args := call.ProjectID, call.Arguments
	switch call.Tool {
	case "project.task_delivery", "project.quality_execution":
		return s.invokeApplicationQualityRead(ctx, auth, call)
	case "project.skill_catalog_list", "project.skill_catalog_get":
		return s.invokeApplicationSkillRead(ctx, auth, call)
	case "project.overview":
		var input struct{}
		if e := strictApplicationInput(args, &input); e != nil {
			return nil, e
		}
		return s.overviewTx(ctx, auth, project)
	case "project.tasks", "project.requirements":
		var input ApplicationToolPageInput
		if e := strictApplicationInput(args, &input); e != nil {
			return nil, e
		}
		if e := applicationPage(input); e != nil {
			return nil, e
		}
		if call.Tool != "project.tasks" && applicationHasField(args, "limit") && input.Limit == 0 {
			return nil, applicationInputError()
		}
		kind := "tasks"
		if call.Tool == "project.requirements" {
			kind = "requirements"
		}
		items, total, next, e := s.listPageTx(ctx, auth, project, kind, input.Limit, input.Cursor)
		return map[string]any{"items": items, "total": total, "next_cursor": next}, e
	case "project.search":
		var input ApplicationToolSearchInput
		if e := strictApplicationInput(args, &input, "query"); e != nil {
			return nil, e
		}
		return s.searchKeywordsTx(ctx, auth, project, input.Query)
	case "project.knowledge":
		var input ApplicationToolKnowledgeInput
		if e := strictApplicationInput(args, &input, "references"); e != nil {
			return nil, e
		}
		if len(input.References) == 0 {
			return nil, invalid("请选择明确的已发布项目知识修订")
		}
		for _, ref := range input.References {
			if ref.Revision > 2147483647 {
				return nil, applicationInputError()
			}
		}
		items, e := s.resolvePublishedKnowledgeTx(ctx, auth, project, input.References)
		return readListEnvelope(items), e
	case "project.knowledge.search":
		var input ProjectKnowledgeDiscoveryQuery
		if e := strictApplicationInput(args, &input); e != nil {
			return nil, e
		}
		if applicationHasField(args, "limit") && input.Limit == 0 || applicationHasField(args, "kind") && input.Kind == "" {
			return nil, knowledgeDiscoveryInvalid()
		}
		return s.discoverProjectKnowledgeTx(ctx, auth, project, input)
	case "project.requirement", "project.task":
		var input ApplicationToolIDInput
		if e := strictApplicationInput(args, &input, "id"); e != nil {
			return nil, e
		}
		if !applicationID(input.ID) {
			return nil, applicationInputError()
		}
		kind := "tasks"
		if call.Tool == "project.requirement" {
			kind = "requirements"
		}
		return s.getRecordTx(ctx, auth, project, kind, input.ID)
	case "project.requirement_design":
		var input ApplicationToolRequirementInput
		if e := strictApplicationInput(args, &input, "requirement_id"); e != nil {
			return nil, e
		}
		if !applicationID(input.RequirementID) {
			return nil, applicationInputError()
		}
		return s.designViewTx(ctx, auth, project, input.RequirementID)
	case "project.task_design":
		var input ApplicationToolTaskInput
		if e := strictApplicationInput(args, &input, "task_id"); e != nil {
			return nil, e
		}
		if !applicationID(input.TaskID) {
			return nil, applicationInputError()
		}
		return s.taskDesignContextViewTx(ctx, auth, project, input.TaskID)
	case "project.knowledge_list":
		var input ApplicationToolKnowledgeListInput
		if e := strictApplicationInput(args, &input); e != nil {
			return nil, e
		}
		if input.Limit < 0 || applicationHasField(args, "limit") && input.Limit == 0 {
			return nil, applicationInputError()
		}
		if input.Kind == "" && applicationHasField(args, "kind") || input.State == "" && applicationHasField(args, "state") {
			return nil, applicationInputError()
		}
		return s.listProjectKnowledgeTx(ctx, auth, project, ProjectKnowledgeListQuery{Kind: input.Kind, State: input.State, Query: input.Query, Limit: input.Limit, Cursor: input.Cursor})
	}
	return nil, &APIError{422, "tool_not_allowed", "该工具尚未授权或未实现", nil}
}

func applicationDraftBody(title, description, priority, owner, key string, criteria []string) map[string]any {
	return map[string]any{"title": title, "description": description, "priority": priority, "owner_id": owner, "acceptance_criteria": applicationStringValues(criteria), "idempotency_key": key}
}
func applicationStringValues(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
func applicationDraftResult(record Record) ApplicationToolDraftResult {
	return ApplicationToolDraftResult{ID: record.ID, ProjectID: record.ProjectID, Version: record.Version, Status: record.Status}
}

func (s *Service) invokeApplicationDraft(ctx context.Context, actor Actor, call ApplicationToolCall) (any, error) {
	// This explicit closed kind check also protects the native semantic-read
	// endpoint from gaining a browser command without its original-view guard.
	if actor.kind != actorProjectAPI {
		return nil, forbidden()
	}
	switch call.Tool {
	case "project.requirement_draft_create":
		var input ApplicationRequirementDraftInput
		if e := strictApplicationInput(call.Arguments, &input, "title", "idempotency_key"); e != nil {
			return nil, e
		}
		if e := applicationDraftFields(input.Title, input.Description, input.Priority, input.OwnerID, input.IdempotencyKey, input.AcceptanceCriteria); e != nil {
			return nil, e
		}
		if input.Priority == "" && applicationHasField(call.Arguments, "priority") {
			return nil, applicationInputError()
		}
		body := applicationDraftBody(input.Title, input.Description, input.Priority, input.OwnerID, input.IdempotencyKey, input.AcceptanceCriteria)
		body["status"] = "draft"
		record, e := s.Create(ctx, actor, call.ProjectID, "requirements", body)
		if e != nil {
			return nil, e
		}
		return applicationDraftResult(record), nil
	case "project.task_draft_create":
		var input ApplicationTaskDraftInput
		if e := strictApplicationInput(call.Arguments, &input, "title", "idempotency_key"); e != nil {
			return nil, e
		}
		if e := applicationDraftFields(input.Title, input.Description, input.Priority, input.OwnerID, input.IdempotencyKey, input.AcceptanceCriteria); e != nil {
			return nil, e
		}
		if input.Priority == "" && applicationHasField(call.Arguments, "priority") {
			return nil, applicationInputError()
		}
		if len(input.RequirementID) > 64 || len(input.DependsOn) > 200 {
			return nil, applicationInputError()
		}
		for _, id := range input.DependsOn {
			if !applicationID(id) {
				return nil, applicationInputError()
			}
		}
		body := applicationDraftBody(input.Title, input.Description, input.Priority, input.OwnerID, input.IdempotencyKey, input.AcceptanceCriteria)
		body["status"] = "todo"
		body["requirement_id"] = input.RequirementID
		body["depends_on"] = applicationStringValues(input.DependsOn)
		record, e := s.Create(ctx, actor, call.ProjectID, "tasks", body)
		if e != nil {
			return nil, e
		}
		return applicationDraftResult(record), nil
	case "project.requirement_design_save":
		var input ApplicationRequirementDesignSaveInput
		if e := strictApplicationInput(call.Arguments, &input, "requirement_id", "expected_version", "expected_requirement_version", "content", "tasks", "idempotency_key"); e != nil {
			return nil, e
		}
		if !applicationID(input.RequirementID) {
			return nil, applicationInputError()
		}
		// Reuse the original full-replacement envelope checks, including all
		// mandatory content and proposal fields, without accepting project input.
		var envelope map[string]any
		d := json.NewDecoder(strings.NewReader(string(call.Arguments)))
		d.UseNumber()
		if d.Decode(&envelope) != nil {
			return nil, applicationInputError()
		}
		delete(envelope, "requirement_id")
		envelope["project_id"] = call.ProjectID
		if e := designDraftEnvelope(envelope); e != nil {
			return nil, e
		}
		view, e := s.SaveRequirementDesign(ctx, actor, call.ProjectID, input.RequirementID, RequirementDesignSaveRequest{ProjectID: call.ProjectID, ExpectedVersion: input.ExpectedVersion, ExpectedRequirementVersion: input.ExpectedRequirementVersion, Content: input.Content, Tasks: input.Tasks, IdempotencyKey: input.IdempotencyKey})
		if e != nil {
			return nil, e
		}
		return ApplicationToolDesignDraftResult{ProjectID: view.ProjectID, RequirementID: view.RequirementID, Version: view.Version, LatestRevision: view.LatestRevision, PublishedRevision: view.PublishedRevision, HasDraft: view.HasDraft}, nil
	}
	return nil, &APIError{422, "tool_not_allowed", "该工具尚未授权或未实现", nil}
}
