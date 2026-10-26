package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
)

func expertRoleRequest(r ExpertTeamRole) ExpertTeamRoleRequest {
	return ExpertTeamRoleRequest{Key: r.Key, Title: r.Title, ResourceID: r.ResourceID, ExpectedResourceVersion: r.ResourceVersion, WorkflowID: r.WorkflowID, WorkflowVersion: r.WorkflowVersion, AgentNodeID: r.AgentNodeID, ToolProfileID: r.ToolProfileID, ExpectedToolVersion: r.ToolProfileVersion, Knowledge: r.Knowledge}
}

func validateExpertRole(r ExpertTeamRoleRequest) error {
	if !expertRoleKey.MatchString(r.Key) || !planningText(r.Title, 120) || !handoffID(r.ResourceID, false) || r.ExpectedResourceVersion < 1 || !handoffID(r.WorkflowID, false) || r.WorkflowVersion < 1 || !handoffID(r.AgentNodeID, false) || !handoffID(r.ToolProfileID, false) || r.ExpectedToolVersion < 1 || len(r.Knowledge) > 8 {
		return expertError("expert_team_invalid", expertInvalid)
	}
	return nil
}

func (s *Service) expertTeamIdentityTx(ctx context.Context, auth *authorizedCommand, team string) (name string, version int, err error) {
	var state string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT name,version,status FROM suma_teams WHERE tenant_id=? AND id=?`), auth.target.tenantID, team).Scan(&name, &version, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return name, version, missing()
	}
	if err == nil && state != "active" {
		err = expertError("expert_team_ineligible", expertInactive)
	}
	return
}

// Role resolution uses only administrator-published workflows and actual current
// resources. The role is never a source of project, machine or tool authority.
func (s *Service) expertTeamResolveRoleTx(ctx context.Context, auth *authorizedCommand, team string, r ExpertTeamRoleRequest, write bool) (out ExpertTeamRole, err error) {
	if err = validateExpertRole(r); err != nil {
		return out, err
	}
	resource, err := s.getRecordTx(ctx, auth, auth.target.projectID, "members", r.ResourceID)
	if err != nil {
		return out, err
	}
	if resource.Version != r.ExpectedResourceVersion || resource.Status != "active" || resource.Kind != "agent" && resource.Kind != "tool" {
		return out, expertError("expert_team_ineligible", expertResource)
	}
	out = ExpertTeamRole{Key: r.Key, Title: r.Title, ResourceID: resource.ID, ResourceVersion: resource.Version, ResourceKind: resource.Kind, ResourceName: resource.Title, WorkflowID: r.WorkflowID, WorkflowVersion: r.WorkflowVersion, AgentNodeID: r.AgentNodeID, ToolProfileID: r.ToolProfileID, Knowledge: append([]ProjectKnowledgeReference{}, r.Knowledge...)}
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id,version FROM suma_team_members WHERE tenant_id=? AND team_id=? AND resource_kind=? AND resource_id=? AND project_id=? AND status='active'`), auth.target.tenantID, team, resource.Kind, resource.ID, auth.target.projectID).Scan(&out.MembershipID, &out.MembershipVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return out, expertError("expert_team_ineligible", expertResource)
	}
	if err != nil {
		return out, err
	}
	profile, err := s.toolProfileTx(ctx, auth.tx, auth.currentPrincipal, auth.target.projectID, r.ToolProfileID)
	if err != nil {
		return out, err
	}
	if profile.Version != r.ExpectedToolVersion {
		return out, expertError("expert_team_changed", expertChanged)
	}
	if write {
		profile, err = s.validateToolDispatchTx(ctx, auth.tx, auth, auth.target.projectID, profile.ID, r.ExpectedToolVersion, profile.Profile.ProfileSHA256)
	} else {
		profile, err = s.toolProfileObservationTx(ctx, auth.tx, auth.currentPrincipal, auth.target.projectID, profile)
	}
	if err != nil {
		return out, err
	}
	if !profile.Available {
		return out, expertError("expert_team_ineligible", expertBinding)
	}
	route, err := dag.ReadProjectExecutionRouteTx(ctx, auth.tx, s.Dialect, auth.target.tenantID, auth.target.projectID)
	if err != nil {
		return out, err
	}
	if err = s.checkDispatchProfileRouteTx(ctx, auth.tx, auth.target.tenantID, auth.target.projectID, profile, route); err != nil {
		return out, err
	}
	var raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT w.spec FROM dag_workflows w WHERE w.tenant_id=? AND w.project_id=? AND w.id=? AND w.version=? AND NOT EXISTS(SELECT 1 FROM dag_managed_workflows m WHERE m.tenant_id=w.tenant_id AND m.project_id=w.project_id AND m.workflow_id=w.id)`), auth.target.tenantID, auth.target.projectID, r.WorkflowID, r.WorkflowVersion).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, expertError("expert_team_ineligible", expertBinding)
	}
	if err != nil {
		return out, err
	}
	var workflow dag.Workflow
	if err = json.Unmarshal([]byte(raw), &workflow); err != nil {
		return out, err
	}
	// Compile the same specialized development/test structure as a task preview.
	if _, err = buildTaskDispatchWorkflow(workflow, r.AgentNodeID, profile.ID, profile.Profile, TaskContext{Task: TaskContextRecord{ID: "template", Version: 1, Title: r.Title, AcceptanceCriteria: []string{}}}); err != nil {
		return out, err
	}
	if _, err = s.resolvePublishedKnowledgeTx(ctx, auth, auth.target.projectID, r.Knowledge); err != nil {
		return out, err
	}
	out.ToolProfileVersion, out.ToolProfileSHA256, out.ToolName = profile.Version, profile.Profile.ProfileSHA256, profile.Name
	return out, nil
}

func (s *Service) checkExpertTeamConfigurationTx(ctx context.Context, auth *authorizedCommand, c ExpertTeamConfiguration, write bool) error {
	_, version, err := s.expertTeamIdentityTx(ctx, auth, c.TeamID)
	if err != nil {
		return err
	}
	if version != c.TeamVersion {
		return expertError("expert_team_changed", expertChanged)
	}
	if len(c.Roles) < 1 || len(c.Roles) > 8 {
		return expertError("expert_team_ineligible", expertBinding)
	}
	for _, role := range c.Roles {
		current, err := s.expertTeamResolveRoleTx(ctx, auth, c.TeamID, expertRoleRequest(role), write)
		if err != nil {
			return err
		}
		if digest(stringify(current)) != digest(stringify(role)) {
			return expertError("expert_team_changed", expertChanged)
		}
	}
	return nil
}

// Use existing independent acceptance and terminal-run facts. Selecting an
// unaccepted predecessor in this batch cannot turn it into execution authority.
func (s *Service) checkExpertTeamTaskTx(ctx context.Context, auth *authorizedCommand, taskID string) error {
	task, err := s.getRecordTx(ctx, auth, auth.target.projectID, "tasks", taskID)
	if err != nil {
		return err
	}
	_, blockers, err := s.handoffBasisTx(ctx, auth, task)
	if err != nil {
		return err
	}
	if len(blockers) > 0 {
		return expertError("expert_team_ineligible", expertTask)
	}
	for _, id := range task.DependsOn {
		upstream, err := s.getRecordTx(ctx, auth, auth.target.projectID, "tasks", id)
		if err != nil {
			return err
		}
		if upstream.Status != "accepted" {
			return expertError("expert_team_ineligible", expertTask)
		}
		if err = s.checkCurrentTaskEvidenceTx(ctx, auth.tx, auth.currentPrincipal, auth.target.projectID, id, map[string]bool{}, map[string]bool{}); err != nil {
			return err
		}
	}
	return nil
}

func expertTeamExpectedError(err error) bool { var api *APIError; return errors.As(err, &api) }
