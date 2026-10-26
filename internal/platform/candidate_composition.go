package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/dag"
	"time"
)

func compositionRequestValid(project string, r CandidateCompositionCreateRequest) bool {
	return r.ProjectID == project && handoffID(project, false) && handoffID(r.TaskID, false) && r.ExpectedTaskVersion > 0 && diagnosisID(r.WorkflowID, 100) && r.WorkflowVersion > 0 && diagnosisID(r.ProducerNodeID, 150) && len(r.Sources) >= 2 && len(r.Sources) <= 16 && planningKey(r.IdempotencyKey)
}
func (s *Service) CreateCandidateComposition(ctx context.Context, engine *dag.Engine, actor Actor, project string, req CandidateCompositionCreateRequest) (out CandidateCompositionPlan, err error) {
	if engine == nil || !compositionRequestValid(project, req) {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, req.TaskID, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"composition_create": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		r, e := s.candidateCompositionTx(ctx, auth.tx, auth.target.tenantID, project, out.ID)
		if e != nil {
			return out, e
		}
		return s.candidateCompositionProjectionTx(ctx, auth, r.Plan)
	}
	out, err = s.compositionSnapshotTx(ctx, engine, auth, req)
	if err != nil {
		return out, err
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	out.ID = newID()
	out.CreatedAt = automationStamp(clock)
	out.ExpiresAt = automationStamp(clock.Add(30 * time.Minute))
	out.FixedTimestamp = clock.UTC().Format(time.RFC3339Nano)
	out.OperationID = "composition-" + out.ID
	r := candidateCompositionRecord{Plan: out, Creator: expertCredentialSnapshot(actor)}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_candidate_compositions(id,tenant_id,project_id,task_id,version,state,created_at,payload)VALUES(?,?,?,?,?,?,?,?)`), out.ID, auth.target.tenantID, project, out.TaskID, 1, out.State, out.CreatedAt, stringify(r)); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "candidate.composition_preview", out.ID, out.Context.Task.Title); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if out, err = s.candidateCompositionProjectionTx(ctx, auth, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
func (s *Service) validateCandidateCompositionTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, p CandidateCompositionPlan) error {
	if p.State != "ready" || p.RunID != "" {
		return compositionError(409, "changed", compositionChanged)
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return err
	}
	if automationStamp(clock) >= p.ExpiresAt {
		return compositionError(409, "expired", compositionExpired)
	}
	req := CandidateCompositionCreateRequest{ProjectID: p.ProjectID, TaskID: p.TaskID, ExpectedTaskVersion: p.TaskVersion, WorkflowID: p.WorkflowID, WorkflowVersion: p.WorkflowVersion, ProducerNodeID: p.ProducerNodeID, Sources: []CandidateCompositionSourceRequest{}}
	for _, b := range p.Sources {
		req.Sources = append(req.Sources, CandidateCompositionSourceRequest{b.TaskID, b.TaskVersion, b.SourceNodeID})
	}
	current, err := s.compositionSnapshotTx(ctx, engine, auth, req)
	if err != nil {
		return err
	}
	if current.TaskBasisSHA256 != p.TaskBasisSHA256 || current.SourcesSHA256 != p.SourcesSHA256 || stringify(current.Workflow) != stringify(p.Workflow) || current.ExecutionRoute != p.ExecutionRoute {
		return compositionError(409, "changed", compositionChanged)
	}
	return nil
}
func (s *Service) StartCandidateComposition(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req CandidateCompositionStartRequest) (out CandidateCompositionPlan, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(id, false) || req.ExpectedVersion < 1 || !planningKey(req.IdempotencyKey) {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	if !req.Acknowledge {
		return out, compositionError(400, "ack", compositionAck)
	}
	read, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	r, err := s.candidateCompositionTx(read, auth.tx, auth.target.tenantID, project, id)
	body := map[string]any{"composition_start": id, "request": req}
	if err == nil {
		_, _, err = s.commandReceipt(read, auth, req.IdempotencyKey, body)
	}
	close()
	if err != nil {
		return out, err
	}
	if err = dag.Compile(r.Plan.Workflow); err != nil {
		return out, err
	}
	run := "composition-" + id
	a := s.engineAuthority(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectWrite)
	defer a.close()
	var hash string
	_, err = engine.CreateManagedRunBound(a.ctx, actor.credential.tenantID, project, run, "composition-start-"+digest(actor.credential.subjectID+":"+req.IdempotencyKey), r.Plan.Workflow, func(tx *sql.Tx) error {
		if e := a.before(tx); e != nil {
			return e
		}
		var replay bool
		var e error
		hash, replay, e = s.commandReceipt(a.ctx, a.proof, req.IdempotencyKey, body)
		if e != nil || replay {
			return e
		}
		r, e = s.candidateCompositionTx(a.ctx, tx, a.proof.target.tenantID, project, id)
		if e != nil {
			return e
		}
		if r.Plan.Version != req.ExpectedVersion {
			return compositionError(409, "changed", compositionChanged)
		}
		return s.validateCandidateCompositionTx(a.ctx, engine, a.proof, r.Plan)
	}, func(tx *sql.Tx, actual string) error {
		bound, e := dag.ReadRunExecutionRouteTx(a.ctx, tx, s.Dialect, a.proof.target.tenantID, project, actual)
		if e != nil {
			return e
		}
		if bound != r.Plan.ExecutionRoute {
			return compositionError(409, "changed", compositionChanged)
		}
		old := r.Plan.Version
		r.Plan.Version++
		r.Plan.RunID = actual
		r.Plan.State = "queued"
		r.Creator = expertCredentialSnapshot(actor)
		r.Execution = &ccmp.CandidateCompositionExecution{Protocol: ccmp.CandidateCompositionProtocol, PlanID: id, PlanVersion: r.Plan.Version, ProjectID: project, BaseCommit: r.Plan.AdminBaseCommit, Sources: compositionSources(r.Plan), SourcesSHA256: r.Plan.SourcesSHA256, FixedTimestamp: r.Plan.FixedTimestamp, OperationID: r.Plan.OperationID}
		if e = ccmp.ValidateCandidateCompositionExecution(*r.Execution); e != nil {
			return e
		}
		if e = s.saveCandidateCompositionTx(a.ctx, a.proof, old, r); e != nil {
			return e
		}
		if _, e = tx.ExecContext(a.ctx, s.q(`INSERT INTO suma_candidate_composition_executions(run_id,tenant_id,project_id,plan_id,node_id,input_kind,payload)VALUES(?,?,?,?,?,'composition',?)`), actual, a.proof.target.tenantID, project, id, r.Plan.ProducerNodeID, stringify(r)); e != nil {
			return e
		}
		if e = s.BindRunTx(a.ctx, tx, a.proof, project, r.Plan.TaskID, actual, r.Plan.TaskVersion); e != nil {
			return e
		}
		if e = s.commandActivity(a.ctx, a.proof, "candidate.composition_started", id, r.Plan.Context.Task.Title); e != nil {
			return e
		}
		return s.saveCommandReceipt(a.ctx, a.proof, req.IdempotencyKey, hash, r.Plan)
	})
	if err != nil {
		return out, err
	}
	a.close()
	return s.GetCandidateComposition(ctx, engine, actor, project, id)
}
func (s *Service) CommandCandidateComposition(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req CandidateCompositionCommandRequest) (out CandidateCompositionPlan, err error) {
	if req.ProjectID != project || req.ExpectedVersion < 1 || req.Action != "dismiss" || !planningText(req.Reason, 2000) || !planningKey(req.IdempotencyKey) {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"composition_command": id, "request": req})
	if err != nil {
		return out, err
	}
	r, err := s.candidateCompositionTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if replay {
		return s.candidateCompositionProjectionTx(ctx, auth, r.Plan)
	}
	if r.Plan.Version != req.ExpectedVersion || r.Plan.RunID != "" || r.Plan.State != "ready" {
		return out, compositionError(409, "changed", compositionChanged)
	}
	old := r.Plan.Version
	r.Plan.Version++
	r.Plan.State = "dismissed"
	if err = s.saveCandidateCompositionTx(ctx, auth, old, r); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "candidate.composition_dismissed", id, req.Reason); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, r.Plan); err != nil {
		return out, err
	}
	out, err = s.candidateCompositionProjectionTx(ctx, auth, r.Plan)
	if err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
