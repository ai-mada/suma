package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/ai-mada/suma/internal/dag"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

const taskDeliveryReferenceLimit = 20

func emptyTaskDeliveryNodeSummary() TaskDeliveryNodeSummary {
	return TaskDeliveryNodeSummary{Limit: taskDeliveryReferenceLimit, Complete: true, Items: []TaskDeliveryNode{}}
}

func appendTaskDeliveryNode(summary *TaskDeliveryNodeSummary, node TaskDeliveryNode) {
	summary.Total++
	switch node.State {
	case "pending", "ready", "waiting", "waiting_approval", "waiting_external", "waiting_timer", "waiting_retry", "paused":
		summary.Waiting++
	case "running", "cancelling":
		summary.Running++
	case "succeeded":
		summary.Succeeded++
	case "failed":
		summary.Failed++
	case "skipped":
		summary.Skipped++
	case "reconciling":
		summary.Reconciling++
	default:
		summary.Other++
	}
	if len(summary.Items) < summary.Limit {
		summary.Items = append(summary.Items, node)
	}
	summary.Complete = summary.Total <= summary.Limit
}

// Only compact node metadata is read. Output, logs, commands, environment,
// credentials and evidence payloads are never returned by this summary.
func (s *Service) taskDeliveryNodesTx(ctx context.Context, auth *authorizedCommand, run string) (nodes, tests TaskDeliveryNodeSummary, err error) {
	nodes, tests = emptyTaskDeliveryNodeSummary(), emptyTaskDeliveryNodeSummary()
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT n.id,n.state,n.attempt,n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND r.id=? ORDER BY n.id`), auth.target.tenantID, auth.target.projectID, run)
	if err != nil {
		return nodes, tests, err
	}
	defer rows.Close()
	for rows.Next() {
		var node TaskDeliveryNode
		var raw string
		if err = rows.Scan(&node.ID, &node.State, &node.Attempt, &raw); err != nil {
			return nodes, tests, err
		}
		var spec struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		if err = json.Unmarshal([]byte(raw), &spec); err != nil {
			return nodes, tests, err
		}
		node.Name, node.Type = spec.Name, spec.Type
		appendTaskDeliveryNode(&nodes, node)
		if node.Type == "test" {
			appendTaskDeliveryNode(&tests, node)
		}
	}
	return nodes, tests, rows.Err()
}

// Select a target independently of the bounded generic list, using only local
// run state and the original assignment retained by the DAG.
func (s *Service) taskDeliveryWorkBuddyTargetTx(ctx context.Context, auth *authorizedCommand, run string) (*TaskDeliveryRunTarget, error) {
	tenant, project := auth.target.tenantID, auth.target.projectID
	const waiting, reason = "waiting", "cloud.delivery_required"
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT n.id,n.state,n.reason,n.attempt,n.fence,n.worker_id,n.spec
		FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id
		WHERE r.tenant_id=? AND r.project_id=? AND r.id=? AND n.state=? AND n.reason=? AND n.attempt>0 ORDER BY n.id`), tenant, project, run, waiting, reason)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type candidate struct {
		id, worker string
		attempt    int
		fence      int64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var state, marker, raw string
		if err = rows.Scan(&c.id, &state, &marker, &c.attempt, &c.fence, &c.worker, &raw); err != nil {
			return nil, err
		}
		var spec dag.Node
		if state != waiting || marker != reason || json.Unmarshal([]byte(raw), &spec) != nil || !dag.WorkBuddyCloudNode(spec) {
			continue
		}
		if _, err = wc.ParseNodeBinding(spec.Config["workbuddy_cloud"]); err == nil {
			candidates = append(candidates, c)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for _, c := range candidates {
		assignment, err := dag.ReadExternalAssignmentTx(ctx, auth.tx, s.Dialect, tenant, project, run, c.id, c.attempt)
		if errors.Is(err, dag.ErrNotFound) || errors.Is(err, dag.ErrConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if assignment.Fence == c.fence && assignment.WorkerID == c.worker {
			return &TaskDeliveryRunTarget{RunID: run, NodeID: c.id, Attempt: c.attempt}, nil
		}
	}
	return nil, nil
}

func (s *Service) taskDeliveryExpertPlansTx(ctx context.Context, auth *authorizedCommand, task Record) (out TaskDeliveryExpertPlans, err error) {
	out = TaskDeliveryExpertPlans{Scope: "current_run", Limit: taskDeliveryReferenceLimit, Complete: true, Items: []TaskDeliveryExpertPlan{}}
	if task.RunID == "" {
		return out, nil
	}
	from := ` FROM suma_expert_team_plans p WHERE p.tenant_id=? AND p.project_id=? AND EXISTS(SELECT 1 FROM suma_expert_team_plan_items i WHERE i.tenant_id=p.tenant_id AND i.project_id=p.project_id AND i.expert_plan_id=p.id AND i.run_id=?)`
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+from), auth.target.tenantID, task.ProjectID, task.RunID).Scan(&out.Total); err != nil {
		return out, err
	}
	out.Complete = out.Total <= out.Limit
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT p.payload`+from+` ORDER BY p.id LIMIT ?`), auth.target.tenantID, task.ProjectID, task.RunID, out.Limit)
	if err != nil {
		return out, err
	}
	plans := []ExpertTeamPlan{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return out, err
		}
		var plan ExpertTeamPlan
		if err = json.Unmarshal([]byte(raw), &plan); err != nil {
			rows.Close()
			return out, err
		}
		plans = append(plans, plan)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, plan := range plans {
		if plan.ProjectID != task.ProjectID {
			return out, forbidden()
		}
		current, e := s.expertTeamProjectFactsTx(ctx, auth.tx, auth.target.tenantID, plan)
		if e != nil {
			return out, e
		}
		found := false
		for _, item := range current.Items {
			if item.TaskID == task.ID && item.RunID == task.RunID {
				out.Items = append(out.Items, TaskDeliveryExpertPlan{PlanID: current.ID, Version: current.Version, Revision: current.Revision, TeamID: current.TeamID, TeamName: current.TeamName, State: current.State, ItemState: item.State})
				found = true
				break
			}
		}
		if !found {
			return out, conflict(expertPlanChanged)
		}
	}
	return out, nil
}

// GetTaskDelivery uses one current authorization/project transaction for the
// task, exact run, acceptance, bounded team references and trusted issue lineage.
// Reading never advances a controller, generates an issue or performs intake.
func (s *Service) GetTaskDelivery(ctx context.Context, actor Actor, project, taskID string) (out TaskDelivery, err error) {
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, taskID, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	return s.taskDeliveryTx(ctx, auth, project, taskID)
}

// Reuse the exact current project-read transaction for HTTP and application/MCP
// reads. The original caller owns its lifetime; no second connection is opened.
func (s *Service) taskDeliveryTx(ctx context.Context, auth *authorizedCommand, project, taskID string) (out TaskDelivery, err error) {
	if auth == nil || auth.target.projectID != project || auth.purpose != purposeProjectRead {
		return out, forbidden()
	}
	if err = auth.assertTx(auth.tx, auth.target, purposeProjectRead); err != nil {
		return out, err
	}
	task, err := s.getRecordTx(ctx, auth, project, "tasks", taskID)
	if err != nil {
		return out, err
	}
	if task.ID != taskID || task.TenantID != auth.target.tenantID || task.ProjectID != project {
		return out, missing()
	}
	out = TaskDelivery{ProjectID: project, TaskID: task.ID, TaskVersion: task.Version, TaskStatus: task.Status, RunBasisState: "not_dispatched", ExpertPlans: TaskDeliveryExpertPlans{Scope: "current_run", Limit: taskDeliveryReferenceLimit, Complete: true, Items: []TaskDeliveryExpertPlan{}}, Issues: TaskDeliveryIssues{Scope: "current_run", Limit: taskDeliveryReferenceLimit, Complete: true, Items: []TaskDeliveryIssue{}}}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_test_plans WHERE tenant_id=? AND project_id=? AND task_id=?`), auth.target.tenantID, project, taskID).Scan(&out.TestPlanID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if out.Repair, err = s.taskDeliveryRepairTx(ctx, auth, taskID, out.TestPlanID); err != nil {
		return out, err
	}
	if task.RunID == "" {
		return out, nil
	}
	var run TaskDeliveryRun
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id,version,state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, task.RunID).Scan(&run.RunID, &run.Version, &run.State)
	if errors.Is(err, sql.ErrNoRows) {
		out.RunBasisState = "unknown"
		return out, nil
	}
	if err != nil {
		return out, err
	}
	basis, ok := task.Data["run_task_basis"].(string)
	switch {
	case !ok || basis == "":
		out.RunBasisState = "unknown"
	case basis != taskBasis(task):
		out.RunBasisState = "changed"
	default:
		out.RunBasisState = "current"
	}
	if run.Nodes, run.Tests, err = s.taskDeliveryNodesTx(ctx, auth, task.RunID); err != nil {
		return out, err
	}
	if run.WorkBuddyDelivery, err = s.taskDeliveryWorkBuddyTargetTx(ctx, auth, run.RunID); err != nil {
		return out, err
	}
	if run.WorkBuddyOrigin, err = s.taskDeliveryWorkBuddyOriginTx(ctx, auth, task); err != nil {
		return out, err
	}
	out.Run = &run
	if out.FormalAcceptance, _, err = s.expertTeamFormalAcceptanceTx(ctx, auth.tx, auth.target.tenantID, project, task.ID, task.RunID); err != nil {
		return out, err
	}
	if out.ExpertPlans, err = s.taskDeliveryExpertPlansTx(ctx, auth, task); err != nil {
		return out, err
	}
	if out.Issues, err = s.taskDeliveryIssuesTx(ctx, auth, task); err != nil {
		return out, err
	}
	if err = s.appendTaskDeliveryQualityDefectsTx(ctx, auth, task, &out.Issues); err != nil {
		return out, err
	}
	if err = auth.assertTx(auth.tx, auth.target, auth.purpose); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Server) taskDeliveryHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string) bool {
	if len(parts) == 5 && parts[0] == "tasks" && parts[2] == "delivery" && parts[3] == "repository-source" && parts[4] == "options" {
		s.taskDeliveryRepositorySourceOptionsHTTP(w, r, actor, project, parts[1])
		return true
	}
	if len(parts) == 4 && parts[0] == "tasks" && parts[2] == "delivery" && parts[3] == "repository-source" {
		if scope != "" && scope != "api" {
			writeError(w, forbidden())
			return true
		}
		s.taskDeliveryRepositorySourceHTTP(w, r, actor, project, parts[1])
		return true
	}
	if len(parts) != 3 || parts[0] != "tasks" || parts[2] != "delivery" {
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
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 || len(values) != 1 || len(values["project_id"]) != 1 || values.Get("project_id") == "" || values.Get("project_id") != project {
		writeError(w, invalid("Invalid task delivery query"))
		return true
	}
	out, err := s.Service.GetTaskDelivery(r.Context(), actor, project, parts[1])
	respond(w, http.StatusOK, out, err)
	return true
}
