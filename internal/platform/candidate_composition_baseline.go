package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/dag"
)

// Acceptance is re-resolved from the integration task and its exact original
// run; a plan projection string or a supplied commit never grants a baseline.
func (s *Service) compositionAcceptedBasisTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string, version int) (RepositoryIntegrationBasis, error) {
	r, err := s.candidateCompositionTx(ctx, tx, p.TenantID, project, id)
	if err != nil {
		return RepositoryIntegrationBasis{}, err
	}
	if r.Plan.Version != version || r.Plan.RunID == "" || !validCandidateCompositionExecutionRecord(r) {
		return RepositoryIntegrationBasis{}, compositionError(409, "baseline_changed", compositionBaseline)
	}
	if err = s.checkCompositionSourcesTx(ctx, tx, p, r.Plan); err != nil {
		return RepositoryIntegrationBasis{}, err
	}
	b, err := s.repositoryBasisReadTx(ctx, tx, p, project, r.Plan.TaskID, r.Plan.ProducerNodeID)
	if err != nil {
		return b, err
	}
	if b.RunID != r.Plan.RunID {
		return b, compositionError(409, "baseline_changed", compositionBaseline)
	}
	return b, nil
}
func (s *Service) checkTaskCodeBaselineTx(ctx context.Context, tx *sql.Tx, tenant, project string, b ccmp.CodeBaselineBinding) error {
	if ccmp.ValidateCodeBaselineBinding(b) != nil || b.ProjectID != project {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	current, _, err := s.taskCodeBaselineTx(ctx, tx, tenant, project, b.TaskID)
	if err != nil {
		return err
	}
	if current == nil || *current != b {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	basis, err := s.compositionAcceptedBasisTx(ctx, tx, Principal{TenantID: tenant}, project, b.CompositionID, b.CompositionVersion)
	if err != nil {
		return err
	}
	if basis.Source != b.Source || basis.BasisSHA256 != b.AcceptedBasisSHA256 {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	return nil
}
func (s *Service) GetTaskCodeBaseline(ctx context.Context, actor Actor, project, task string) (out CandidateCompositionBaseline, err error) {
	out = CandidateCompositionBaseline{ProjectID: project, TaskID: task, Blockers: []string{}}
	if !handoffID(project, false) || !handoffID(task, false) {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	var run string
	var version int
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT run_id,version FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, task).Scan(&run, &version)
	if err != nil {
		return out, err
	}
	out.Binding, out.Version, err = s.taskCodeBaselineTx(ctx, auth.tx, auth.target.tenantID, project, task)
	if err != nil {
		return out, err
	}
	write, _, _, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	out.CanBind = write && run == ""
	if e := s.rejectReservedCandidateRepairTaskTx(ctx, auth.tx, auth.target.tenantID, project, task, ""); e != nil {
		if !repositoryKnownError(e) {
			return out, e
		}
		out.CanBind = false
		out.Blockers = append(out.Blockers, compositionRepairTask)
	}
	if run != "" {
		out.Blockers = append(out.Blockers, compositionStarted)
	}
	if out.Binding != nil {
		e := s.checkTaskCodeBaselineTx(ctx, auth.tx, auth.target.tenantID, project, *out.Binding)
		if e == nil && run == "" && version != out.Binding.TaskVersion {
			e = compositionError(409, "baseline_changed", compositionBaseline)
		}
		if e != nil {
			if !repositoryKnownError(e) {
				return out, e
			}
			out.Blockers = append(out.Blockers, compositionBaseline)
		}
	}
	return out, nil
}
func (s *Service) BindTaskCodeBaseline(ctx context.Context, engine *dag.Engine, actor Actor, project, task string, req TaskCodeBaselineBindRequest) (out Record, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(task, false) || !handoffID(req.CompositionID, false) || req.ExpectedTaskVersion < 1 || req.ExpectedBindingVersion < 0 || req.ExpectedCompositionVersion < 1 || !planningText(req.Reason, 2000) || !planningKey(req.IdempotencyKey) {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	if !req.Acknowledge {
		return out, compositionError(400, "ack", compositionAck)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"composition_baseline": task, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return out, nil
	}
	out, err = scanRecord(auth.tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, task))
	if err != nil {
		return out, err
	}
	if out.Version != req.ExpectedTaskVersion {
		return out, compositionError(409, "changed", compositionChanged)
	}
	if out.RunID != "" || out.Status == "accepted" {
		return out, compositionError(409, "started", compositionStarted)
	}
	if err = s.rejectReservedCandidateRepairTaskTx(ctx, auth.tx, auth.target.tenantID, project, task, ""); err != nil {
		return out, err
	}
	_, oldVersion, err := s.taskCodeBaselineTx(ctx, auth.tx, auth.target.tenantID, project, task)
	if err != nil {
		return out, err
	}
	if oldVersion != req.ExpectedBindingVersion {
		return out, compositionError(409, "baseline_changed", compositionBaseline)
	}
	basis, err := s.compositionAcceptedBasisTx(ctx, auth.tx, auth.currentPrincipal, project, req.CompositionID, req.ExpectedCompositionVersion)
	if err != nil {
		return out, err
	}
	if basis.TaskID == task {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	binding := ccmp.CodeBaselineBinding{Protocol: ccmp.CodeBaselineProtocol, ID: newID(), ProjectID: project, TaskID: task, TaskVersion: out.Version + 1, CompositionID: req.CompositionID, CompositionVersion: req.ExpectedCompositionVersion, AcceptedBasisSHA256: basis.BasisSHA256, Source: basis.Source}
	binding.BindingSHA256, err = ccmp.CodeBaselineBindingDigest(binding)
	if err != nil {
		return out, err
	}
	if err = ccmp.ValidateCodeBaselineBinding(binding); err != nil {
		return out, err
	}
	if oldVersion == 0 {
		_, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_code_baselines(tenant_id,project_id,task_id,version,binding)VALUES(?,?,?,?,?)`), auth.target.tenantID, project, task, 1, stringify(binding))
	} else {
		var res sql.Result
		res, err = auth.tx.ExecContext(ctx, s.q(`UPDATE suma_task_code_baselines SET version=version+1,binding=? WHERE tenant_id=? AND project_id=? AND task_id=? AND version=?`), stringify(binding), auth.target.tenantID, project, task, oldVersion)
		if err == nil {
			if n, _ := res.RowsAffected(); n != 1 {
				err = compositionError(409, "baseline_changed", compositionBaseline)
			}
		}
	}
	if err != nil {
		return out, err
	}
	res, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_tasks SET version=version+1,updated_at=? WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND run_id=''`), now(), auth.target.tenantID, project, task, out.Version)
	if err != nil {
		return out, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return out, compositionError(409, "changed", compositionChanged)
	}
	if err = s.InvalidateDependentsTx(ctx, auth.tx, auth, project, []string{task}, false); err != nil {
		return out, err
	}
	out.Version++
	out.UpdatedAt = now()
	out.CodeBaseline = &binding
	if err = s.commandActivity(ctx, auth, "task.code_baseline_bound", task, req.Reason); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

type codeBaselineExecution struct {
	Binding ccmp.CodeBaselineBinding      `json:"binding"`
	Creator expertTeamCredentialReference `json:"creator"`
	Route   dag.ProjectExecutionRoute     `json:"route"`
}

func codeBaselineConfigMatches(value any, b ccmp.CodeBaselineBinding) bool {
	var actual ccmp.CodeBaselineBinding
	return json.Unmarshal([]byte(stringify(value)), &actual) == nil && actual == b
}

func (s *Service) bindCodeBaselineExecutionTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, run, node string, b ccmp.CodeBaselineBinding) error {
	if auth == nil || assertProjectCommand(tx, auth, project) != nil || !diagnosisID(run, 100) || !diagnosisID(node, 150) {
		return forbidden()
	}
	if err := s.checkTaskCodeBaselineTx(ctx, tx, auth.target.tenantID, project, b); err != nil {
		return err
	}
	var taskRun, raw string
	var version int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT t.run_id,t.version,n.spec FROM suma_tasks t JOIN dag_runs r ON r.tenant_id=t.tenant_id AND r.project_id=t.project_id AND r.id=t.run_id JOIN dag_nodes n ON n.run_id=r.id AND n.id=? WHERE t.tenant_id=? AND t.project_id=? AND t.id=?`), node, auth.target.tenantID, project, b.TaskID).Scan(&taskRun, &version, &raw); err != nil {
		return err
	}
	var spec dag.Node
	if taskRun != run || version != b.TaskVersion+1 || json.Unmarshal([]byte(raw), &spec) != nil || spec.Type != "agent" || spec.Config["candidate_from"] != nil || !codeBaselineConfigMatches(spec.Config["code_baseline"], b) {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	route, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, auth.target.tenantID, project, run)
	if err != nil {
		return err
	}
	r := codeBaselineExecution{Binding: b, Creator: expertCredentialSnapshot(auth.actor), Route: route}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_candidate_composition_executions(run_id,tenant_id,project_id,plan_id,node_id,input_kind,payload)VALUES(?,?,?,?,?,'code_baseline',?)`), run, auth.target.tenantID, project, b.CompositionID, node, stringify(r))
	return err
}
func (s *Service) readCodeBaselineExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string) (out *ccmp.CodeBaselineBinding, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_candidate_composition_executions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND input_kind='code_baseline'`), tenant, project, run, node).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r codeBaselineExecution
	if err = json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, err
	}
	return &r.Binding, ccmp.ValidateCodeBaselineBinding(r.Binding)
}

func (s *Service) checkCodeBaselineExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, requireCurrentSource bool) error {
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_candidate_composition_executions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND input_kind='code_baseline'`), tenant, project, run, node).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var r codeBaselineExecution
	if json.Unmarshal([]byte(raw), &r) != nil || ccmp.ValidateCodeBaselineBinding(r.Binding) != nil || r.Binding.ProjectID != project {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	if _, err = s.compositionCreatorTx(ctx, tx, tenant, project, r.Creator); err != nil {
		return err
	}
	var taskRun, specRaw string
	var taskVersion int
	err = tx.QueryRowContext(ctx, s.q(`SELECT t.run_id,t.version,n.spec FROM suma_tasks t JOIN dag_runs d ON d.tenant_id=t.tenant_id AND d.project_id=t.project_id AND d.id=? JOIN dag_nodes n ON n.run_id=d.id AND n.id=? WHERE t.tenant_id=? AND t.project_id=? AND t.id=?`), run, node, tenant, project, r.Binding.TaskID).Scan(&taskRun, &taskVersion, &specRaw)
	if err != nil {
		return err
	}
	var spec dag.Node
	if taskRun != run || json.Unmarshal([]byte(specRaw), &spec) != nil || spec.Type != "agent" || spec.Config["candidate_from"] != nil || !codeBaselineConfigMatches(spec.Config["code_baseline"], r.Binding) {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	bound, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, run)
	if err != nil {
		return err
	}
	if bound != r.Route {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	if requireCurrentSource {
		if taskVersion != r.Binding.TaskVersion+1 {
			return compositionError(409, "baseline_changed", compositionBaseline)
		}
		if err = s.checkTaskCodeBaselineTx(ctx, tx, tenant, project, r.Binding); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) codeBaselineInputBindingTx(ctx context.Context, tx *sql.Tx, a *candidateAuthority, req ccmp.CodeInputRequest) (out candidateTransfer, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_candidate_composition_executions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND input_kind='code_baseline'`), a.subject.tenant, a.subject.project, a.assignment.RunID, a.assignment.NodeID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, forbidden()
	}
	if err != nil {
		return out, err
	}
	var r codeBaselineExecution
	if err = json.Unmarshal([]byte(raw), &r); err != nil {
		return out, err
	}
	b := r.Binding
	if b.ID != req.BindingID || b.Source.TransferID != req.TransferID || a.spec.Type != "agent" || a.spec.Config["candidate_from"] != nil {
		return out, forbidden()
	}
	if err = s.checkCodeBaselineExecutionTx(ctx, tx, a.subject.tenant, a.subject.project, a.assignment.RunID, a.assignment.NodeID, true); err != nil {
		return out, err
	}
	out, err = s.candidateTransferTx(ctx, tx, a.subject, b.Source.TransferID)
	if err != nil {
		return out, err
	}
	if out.binding != b.Source {
		return out, compositionError(409, "baseline_changed", compositionBaseline)
	}
	return out, s.candidateSourceTx(ctx, tx, a.subject, b.Source)
}
