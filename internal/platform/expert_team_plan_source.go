package platform

import (
	"context"
	"sort"

	"github.com/ai-mada/suma/internal/dag"
)

func expertTeamRole(c ExpertTeamConfiguration, key string) (ExpertTeamRole, bool) {
	for _, r := range c.Roles {
		if r.Key == key {
			return r, true
		}
	}
	return ExpertTeamRole{}, false
}

func (s *Service) expertTeamSourceItemTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, configuration ExpertTeamConfiguration, request ExpertTeamPlanItemRequest, revision int, source *TaskDispatchPlan) (out ExpertTeamPlanItem, err error) {
	role, ok := expertTeamRole(configuration, request.RoleKey)
	if !ok {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	task, err := s.getRecordTx(ctx, auth, auth.target.projectID, "tasks", request.TaskID)
	if err != nil {
		return out, err
	}
	if task.Version != request.ExpectedTaskVersion {
		return out, expertError("expert_team_changed", expertSource)
	}
	_, blockers, err := s.handoffBasisTx(ctx, auth, task)
	if err != nil {
		return out, err
	}
	if len(blockers) != 0 {
		return out, expertError("expert_team_ineligible", expertTask)
	}
	snapshot, err := s.buildTaskDispatchSnapshotWithSourceTx(ctx, engine, auth, auth.target.projectID, TaskDispatchPreviewRequest{ProjectID: auth.target.projectID, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkflowID: role.WorkflowID, WorkflowVersion: role.WorkflowVersion, AgentNodeID: role.AgentNodeID, ToolProfileID: role.ToolProfileID, ExpectedToolVersion: role.ToolProfileVersion, Knowledge: role.Knowledge, ModelExecutionSelection: request.ModelExecutionSelection, CodexExecutionSelection: request.CodexExecutionSelection}, source)
	if err != nil {
		return out, err
	}
	return ExpertTeamPlanItem{TaskID: task.ID, TaskVersion: task.Version, TaskBasisSHA256: taskBasis(task), SourceRevision: revision, DependsOn: append([]string{}, task.DependsOn...), Dependencies: []ExpertTeamDependencyFact{}, State: "ready", Blockers: []string{}, RoleKey: role.Key, RoleTitle: role.Title, ResourceID: role.ResourceID, ResourceName: role.ResourceName, Plan: snapshot}, nil
}

// Dependencies come only from the actual project tasks. No request edge can
// create authority or implicitly add another task to the reviewed scope.
func (s *Service) compileExpertTeamGraphTx(ctx context.Context, auth *authorizedCommand, plan *ExpertTeamPlan) error {
	selected := map[string]int{}
	for i, item := range plan.Items {
		if item.TaskID == "" {
			return expertError("expert_team_invalid", expertGraph)
		}
		if _, exists := selected[item.TaskID]; exists {
			return expertError("expert_team_invalid", expertGraph)
		}
		selected[item.TaskID] = i
	}
	marks := map[string]int{}
	order := []int{}
	var visit func(string) error
	visit = func(id string) error {
		if marks[id] == 1 {
			return expertError("expert_team_invalid", expertGraph)
		}
		if marks[id] == 2 {
			return nil
		}
		marks[id] = 1
		item := &plan.Items[selected[id]]
		item.Dependencies = []ExpertTeamDependencyFact{}
		seen := map[string]bool{}
		for _, dep := range item.DependsOn {
			if dep == id || seen[dep] {
				return expertError("expert_team_invalid", expertGraph)
			}
			seen[dep] = true
			task, err := s.getRecordTx(ctx, auth, plan.ProjectID, "tasks", dep)
			if err != nil {
				return err
			}
			_, inside := selected[dep]
			fact := ExpertTeamDependencyFact{TaskID: dep, Selected: inside, TaskVersion: task.Version, Accepted: task.Status == "accepted", RunID: task.RunID}
			if inside {
				if err = visit(dep); err != nil {
					return err
				}
			} else {
				if task.Status != "accepted" {
					return expertError("expert_team_ineligible", expertWaiting)
				}
				if err = s.checkCurrentTaskEvidenceTx(ctx, auth.tx, auth.currentPrincipal, plan.ProjectID, dep, map[string]bool{}, map[string]bool{}); err != nil {
					return err
				}
			}
			item.Dependencies = append(item.Dependencies, fact)
		}
		marks[id] = 2
		order = append(order, selected[id])
		return nil
	}
	for _, item := range plan.Items {
		if err := visit(item.TaskID); err != nil {
			return err
		}
	}
	ordered := make([]ExpertTeamPlanItem, 0, len(order))
	for _, i := range order {
		ordered = append(ordered, plan.Items[i])
	}
	plan.Items = ordered
	for i := range plan.Items {
		item := &plan.Items[i]
		if item.RunID != "" {
			continue
		}
		item.State = "ready"
		item.Blockers = []string{}
		for _, fact := range item.Dependencies {
			if fact.Selected && !fact.Accepted {
				item.State = "waiting_dependencies"
				item.Blockers = []string{expertWaiting}
				break
			}
		}
	}
	plan.GraphSHA256 = expertTeamGraphDigest(*plan)
	return nil
}

func expertTeamSourceDigest(item ExpertTeamPlanItem) string {
	snapshot := item.Plan
	snapshot.ID, snapshot.RunID, snapshot.CreatedAt, snapshot.ExpiresAt, snapshot.Reason = "", "", "", "", ""
	snapshot.Version, snapshot.State, snapshot.Valid = 1, "ready", true
	return digest(stringify(map[string]any{"task": item.TaskID, "basis": item.TaskBasisSHA256, "role": item.RoleKey, "resource": item.ResourceID, "source_revision": item.SourceRevision, "dependencies": item.DependsOn, "snapshot": snapshot}))
}

func expertTeamGraphDigest(plan ExpertTeamPlan) string {
	items := append([]ExpertTeamPlanItem{}, plan.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].TaskID < items[j].TaskID })
	hashes := []string{}
	for _, item := range items {
		hashes = append(hashes, expertTeamSourceDigest(item))
	}
	return digest(stringify(map[string]any{"configuration": plan.Configuration, "items": hashes}))
}

func (s *Service) checkExpertTeamPendingSourceTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, plan ExpertTeamPlan, item ExpertTeamPlanItem, write bool) error {
	if item.RunID != "" {
		return nil
	}
	head, err := s.expertTeamHeadTx(ctx, auth, plan.TeamID, 0)
	if err != nil {
		return err
	}
	if head.Version != plan.ConfigurationVersion || head.State != "published" {
		return expertError("expert_team_changed", expertChanged)
	}
	task, err := s.getRecordTx(ctx, auth, plan.ProjectID, "tasks", item.TaskID)
	if err != nil {
		return err
	}
	if task.Version != item.TaskVersion || taskBasis(task) != item.TaskBasisSHA256 {
		return expertError("expert_team_changed", expertSource)
	}
	_, blockers, err := s.handoffBasisTx(ctx, auth, task)
	if err != nil {
		return err
	}
	if len(blockers) > 0 {
		return expertError("expert_team_ineligible", expertTask)
	}
	if write {
		current, err := s.expertTeamSourceItemTx(ctx, engine, auth, plan.Configuration, ExpertTeamPlanItemRequest{TaskID: item.TaskID, ExpectedTaskVersion: item.TaskVersion, RoleKey: item.RoleKey}, item.SourceRevision, &item.Plan)
		if err != nil {
			return err
		}
		if expertTeamSourceDigest(current) != expertTeamSourceDigest(item) {
			return expertError("expert_team_changed", expertSource)
		}
		return nil
	}
	_, version, err := s.expertTeamIdentityTx(ctx, auth, plan.TeamID)
	if err != nil {
		return err
	}
	if version != plan.Configuration.TeamVersion {
		return expertError("expert_team_changed", expertChanged)
	}
	role, ok := expertTeamRole(plan.Configuration, item.RoleKey)
	if !ok {
		return expertError("expert_team_invalid", expertInvalid)
	}
	currentRole, err := s.expertTeamResolveRoleTx(ctx, auth, plan.TeamID, expertRoleRequest(role), false)
	if err != nil {
		return err
	}
	if digest(stringify(currentRole)) != digest(stringify(role)) {
		return expertError("expert_team_changed", expertResource)
	}
	return s.checkExpertTeamPreviewReadTx(ctx, auth, item.Plan)
}

func (s *Service) validateExpertTeamPlanTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, plan ExpertTeamPlan, write bool) error {
	head, err := s.expertTeamHeadTx(ctx, auth, plan.TeamID, 0)
	if err != nil {
		return err
	}
	if head.Version != plan.ConfigurationVersion {
		return expertError("expert_team_changed", expertChanged)
	}
	if head.State != "published" {
		return expertError("expert_team_ineligible", expertUnpublished)
	}
	if err = s.checkExpertTeamConfigurationTx(ctx, auth, head, write); err != nil {
		return err
	}
	for _, item := range plan.Items {
		if err = s.checkExpertTeamPendingSourceTx(ctx, engine, auth, plan, item, write); err != nil {
			return err
		}
	}
	return nil
}
