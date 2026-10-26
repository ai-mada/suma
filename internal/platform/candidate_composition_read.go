package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) compositionCurrentPreviewTx(ctx context.Context, auth *authorizedCommand, p CandidateCompositionPlan) error {
	if p.ConflictBasis != nil {
		return s.validateCandidateCompositionRepairTx(ctx, nil, auth, p)
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return err
	}
	if automationStamp(clock) >= p.ExpiresAt {
		return compositionError(409, "expired", compositionExpired)
	}
	context, err := s.taskContextForDispatchTx(ctx, auth, p.ProjectID, p.TaskID, nil)
	if err != nil {
		return err
	}
	if digest(stringify(context)) != p.TaskBasisSHA256 {
		return compositionError(409, "changed", compositionChanged)
	}
	if err = s.checkCompositionSourcesTx(ctx, auth.tx, auth.currentPrincipal, p); err != nil {
		return err
	}
	var raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), auth.target.tenantID, p.ProjectID, p.WorkflowID, p.WorkflowVersion).Scan(&raw)
	if err != nil {
		return err
	}
	var w dag.Workflow
	if err = json.Unmarshal([]byte(raw), &w); err != nil {
		return err
	}
	w, err = buildCandidateCompositionWorkflow(w, p.ProducerNodeID)
	if err != nil {
		return err
	}
	if stringify(w) != stringify(p.Workflow) {
		return compositionError(409, "changed", compositionChanged)
	}
	route, err := dag.ReadProjectExecutionRouteTx(ctx, auth.tx, s.Dialect, auth.target.tenantID, p.ProjectID)
	if err != nil {
		return err
	}
	if route != p.ExecutionRoute {
		return compositionError(409, "changed", compositionChanged)
	}
	return nil
}
func (s *Service) candidateCompositionProjectionTx(ctx context.Context, auth *authorizedCommand, out CandidateCompositionPlan) (CandidateCompositionPlan, error) {
	out.Valid = true
	out.Blockers = []string{}
	out.AllowedActions = []string{}
	out.Conflicts = []string{}
	out.Candidate = nil
	write, _, _, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	if out.RunID == "" {
		if out.State == "ready" {
			if write {
				out.AllowedActions = append(out.AllowedActions, "dismiss")
			}
			if err = s.compositionCurrentPreviewTx(ctx, auth, out); err != nil {
				if !repositoryKnownError(err) {
					return out, err
				}
				out.Valid = false
				out.Blockers = append(out.Blockers, err.Error())
				if api, ok := err.(*APIError); ok && api.Code == "candidate_composition_expired" && write && out.ConflictBasis != nil {
					if _, e := s.refreshedCandidateCompositionRepairTx(ctx, auth, out); e == nil {
						out.AllowedActions = append(out.AllowedActions, "refresh")
					} else if !repositoryKnownError(e) {
						return out, e
					}
				}
			} else if write {
				out.AllowedActions = append(out.AllowedActions, "start")
				if out.ConflictBasis != nil {
					out.AllowedActions = append(out.AllowedActions, "refresh")
				}
			}
		}
		return out, nil
	}
	if err = s.checkCompositionSourcesTx(ctx, auth.tx, auth.currentPrincipal, out); err != nil {
		if !repositoryKnownError(err) {
			return out, err
		}
		out.Valid = false
		out.Blockers = append(out.Blockers, compositionSource)
	}
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, out.ProjectID, out.RunID).Scan(&out.RunState)
	if err != nil {
		return out, err
	}
	var taskRun string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT status,run_id FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, out.ProjectID, out.TaskID).Scan(&out.TaskState, &taskRun)
	if err != nil {
		return out, err
	}
	switch out.RunState {
	case "queued":
		out.State = "queued"
	case "running":
		out.State = "executing"
		var began int
		if e := auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_nodes WHERE run_id=? AND attempt>0`), out.RunID).Scan(&began); e != nil {
			return out, e
		}
		if began == 0 {
			out.State = "queued"
		}
	case "paused":
		out.State = "executing"
		out.Blockers = append(out.Blockers, compositionUnknown)
	case "reconciling":
		out.State = "reconciling"
	case "cancelled":
		out.State = "cancelled"
	case "succeeded":
		out.State = "awaiting_review"
	default:
		out.State = "failed"
	}
	var binding string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT c.binding FROM suma_candidate_transfers c JOIN dag_nodes n ON n.run_id=c.run_id AND n.id=c.node_id AND n.attempt=c.attempt AND n.fence=c.fence AND n.worker_id=c.worker_id WHERE c.tenant_id=? AND c.project_id=? AND c.run_id=? AND c.node_id=?`), auth.target.tenantID, out.ProjectID, out.RunID, out.ProducerNodeID).Scan(&binding)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if err == nil {
		var b cc.Binding
		if json.Unmarshal([]byte(binding), &b) == nil && b.ValidSealed() {
			out.Candidate = &b
		}
	}
	if out.RunState == "succeeded" && taskRun == out.RunID && out.TaskState == "accepted" {
		b, e := s.repositoryBasisReadTx(ctx, auth.tx, auth.currentPrincipal, out.ProjectID, out.TaskID, out.ProducerNodeID)
		if e == nil && out.Candidate != nil && b.Source == *out.Candidate && out.Valid {
			out.State = "accepted"
		} else if e != nil && !repositoryKnownError(e) {
			return out, e
		}
	}
	// Runtime conflict paths are typed workspace facts, never parsed from prose.
	var raw *string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT a.result FROM dag_nodes n LEFT JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=n.attempt AND a.fence=n.fence AND a.worker_id=n.worker_id WHERE n.run_id=? AND n.id=?`), out.RunID, out.ProducerNodeID).Scan(&raw)
	if err != nil {
		return out, err
	}
	if raw != nil {
		var r dag.Result
		if json.Unmarshal([]byte(*raw), &r) == nil {
			if fact, ok := r.WorkspaceEvidence["candidate_composition"].(map[string]any); ok && fact["state"] == "conflicted" {
				out.State = "conflicted"
				out.Conflicts = stringsAt(fact, "conflicts")
			}
		}
	}
	return out, nil
}
func (s *Service) GetCandidateComposition(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (out CandidateCompositionPlan, err error) {
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	r, err := s.candidateCompositionTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	return s.candidateCompositionProjectionTx(ctx, auth, r.Plan)
}

type compositionCursor struct{ Tenant, Project, Task, Created, ID string }

func (s *Service) ListCandidateCompositions(ctx context.Context, engine *dag.Engine, actor Actor, project, task string, limit int, before string) (out CandidateCompositionPage, err error) {
	out.Items = []CandidateCompositionPlan{}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(task, true) || limit < 1 || limit > 100 || len(before) > 4096 {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	where := `tenant_id=? AND project_id=?`
	args := []any{auth.target.tenantID, project}
	if task != "" {
		where += ` AND task_id=?`
		args = append(args, task)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_candidate_compositions WHERE `+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if before != "" {
		var c compositionCursor
		raw, e := base64.RawURLEncoding.Strict().DecodeString(before)
		if e != nil || json.Unmarshal(raw, &c) != nil || c.Tenant != auth.target.tenantID || c.Project != project || c.Task != task || !validReleaseTime(c.Created) || !handoffID(c.ID, false) || base64.RawURLEncoding.EncodeToString([]byte(stringify(c))) != before {
			return out, compositionError(400, "invalid", compositionInvalid)
		}
		where += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, c.Created, c.Created, c.ID)
	}
	args = append(args, limit+1)
	rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT payload FROM suma_candidate_compositions WHERE `+where+` ORDER BY created_at DESC,id DESC LIMIT ?`), args...)
	if e != nil {
		return out, e
	}
	plans := []CandidateCompositionPlan{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			break
		}
		var r candidateCompositionRecord
		if e = json.Unmarshal([]byte(raw), &r); e != nil {
			break
		}
		plans = append(plans, r.Plan)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(plans) > limit {
		p := plans[limit-1]
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(compositionCursor{auth.target.tenantID, project, task, p.CreatedAt, p.ID})))
		plans = plans[:limit]
	}
	for _, p := range plans {
		p, e = s.candidateCompositionProjectionTx(ctx, auth, p)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	return out, nil
}
func (s *Service) CandidateCompositionOptions(ctx context.Context, engine *dag.Engine, actor Actor, project, task string) (out CandidateCompositionOptions, err error) {
	out = CandidateCompositionOptions{ProjectID: project, TaskID: task, Sources: []CandidateCompositionSourceChoice{}, Blockers: []string{}}
	if !handoffID(project, false) || !handoffID(task, false) {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	var run, status string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version,run_id,status FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, task).Scan(&out.TaskVersion, &run, &status)
	if err != nil {
		return out, err
	}
	write, _, _, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	out.CanCreate = write && run == "" && status != "accepted"
	if e := s.rejectReservedCandidateRepairTaskTx(ctx, auth.tx, auth.target.tenantID, project, task, ""); e != nil {
		if !repositoryKnownError(e) {
			return out, e
		}
		out.CanCreate = false
		out.Blockers = append(out.Blockers, compositionRepairTask)
	}
	if !out.CanCreate {
		out.Blockers = append(out.Blockers, compositionStarted)
	}
	baseline, _, err := s.taskCodeBaselineTx(ctx, auth.tx, auth.target.tenantID, project, task)
	if err != nil {
		return out, err
	}
	if baseline != nil {
		out.CanCreate = false
		out.Blockers = append(out.Blockers, compositionBaseline)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT t.id,t.version,t.title,c.node_id,c.binding FROM suma_tasks t JOIN suma_candidate_transfers c ON c.tenant_id=t.tenant_id AND c.project_id=t.project_id AND c.run_id=t.run_id WHERE t.tenant_id=? AND t.project_id=? AND t.status='accepted' AND t.id<>? ORDER BY t.updated_at DESC,t.id,c.node_id LIMIT 200`), auth.target.tenantID, project, task)
	if err != nil {
		return out, err
	}
	choices := []CandidateCompositionSourceChoice{}
	for rows.Next() {
		c := CandidateCompositionSourceChoice{Blockers: []string{}}
		var raw string
		if err = rows.Scan(&c.TaskID, &c.TaskVersion, &c.Title, &c.SourceNodeID, &raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &c.Source); err != nil {
			break
		}
		choices = append(choices, c)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, c := range choices {
		_, e := s.repositoryBasisReadTx(ctx, auth.tx, auth.currentPrincipal, project, c.TaskID, c.SourceNodeID)
		c.Eligible = e == nil
		if e != nil {
			if !repositoryKnownError(e) {
				return out, e
			}
			c.Blockers = append(c.Blockers, compositionSource)
		}
		out.Sources = append(out.Sources, c)
	}
	return out, nil
}
