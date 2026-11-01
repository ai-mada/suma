package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	dc "github.com/ai-mada/suma/internal/developmentcontract"
)

const continuationChanged = "The development continuation source changed; refresh the task and review its candidate"
const continuationInvalid = "Invalid development continuation request"

func continuationError() error { return conflict(continuationChanged) }

func continuationTerminal(state string) bool {
	return state == "succeeded" || state == "failed" || state == "cancelled"
}

// A previous task pointer is insufficient: the original protected binding
// survives later Start operations and identifies this exact task/version.
func (s *Service) continuationProvenanceTx(ctx context.Context, tx *sql.Tx, tenant, project, task, run string) (int, error) {
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_task_run_provenance WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id=?`), tenant, project, task, run).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, continuationError()
	}
	if err != nil {
		return 0, err
	}
	var p TaskRunProvenance
	if json.Unmarshal([]byte(raw), &p) != nil || p.TaskID != task || p.RunID != run || p.TaskVersion == nil || *p.TaskVersion < 1 || p.BindingKind != "bound" || p.BoundAt == nil {
		return 0, continuationError()
	}
	plan, version, err := s.taskRunSourceTx(ctx, tx, tenant, project, task, run)
	if err != nil {
		return 0, err
	}
	if version != nil && (*version != *p.TaskVersion || plan != p.PlanID) {
		return 0, continuationError()
	}
	return *p.TaskVersion, nil
}

func (s *Service) resolveDevelopmentContinuationTx(ctx context.Context, auth *authorizedCommand, project, task, id string, version int, source dc.DevelopmentContinuationSource) (*dc.DevelopmentContinuationBinding, error) {
	if dc.ValidateSource(source) != nil {
		return nil, invalid(continuationInvalid)
	}
	t, err := s.getRecordTx(ctx, auth, project, "tasks", task)
	if err != nil {
		return nil, err
	}
	if t.Version != version || t.RunID != source.RunID || t.Status == "accepted" {
		return nil, continuationError()
	}
	var state string
	var runVersion int
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT state,version FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, source.RunID).Scan(&state, &runVersion); err != nil {
		return nil, err
	}
	if !continuationTerminal(state) || runVersion != source.ExpectedRunVersion {
		return nil, continuationError()
	}
	v, err := s.continuationProvenanceTx(ctx, auth.tx, auth.target.tenantID, project, task, source.RunID)
	if err != nil {
		return nil, err
	}
	var transfer string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`), auth.target.tenantID, project, source.RunID, source.NodeID, source.Attempt, source.Fence).Scan(&transfer)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, continuationError()
	}
	if err != nil {
		return nil, err
	}
	subject := candidateSubject{tenant: auth.target.tenantID, project: project}
	candidate, err := s.candidateTransferTx(ctx, auth.tx, subject, transfer)
	if err != nil {
		return nil, err
	}
	if err = s.candidateSourceTx(ctx, auth.tx, subject, candidate.binding); err != nil {
		return nil, err
	}
	b := dc.DevelopmentContinuationBinding{Protocol: dc.ContinuationProtocol, ID: id, ProjectID: project, TaskID: task, TaskVersion: version, SourceTaskRunID: source.RunID, SourceTaskVersion: v, SourceRunVersion: runVersion, Source: candidate.binding}
	b.BindingSHA256, err = dc.BindingDigest(b)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// The persisted dispatch payload is the only input grant. Published node JSON
// and a transfer ID cannot synthesize this relation.
func (s *Service) readDevelopmentContinuationPlanTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string) (*TaskDispatchPlan, error) {
	var id, raw, state string
	err := tx.QueryRowContext(ctx, s.q(`SELECT id,payload,state FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&id, &raw, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p TaskDispatchPlan
	if json.Unmarshal([]byte(raw), &p) != nil {
		return nil, continuationError()
	}
	if p.DevelopmentContinuation == nil {
		return nil, nil
	}
	if p.AgentNodeID != node {
		return nil, nil
	}
	b := p.DevelopmentContinuation
	if state != "started" || p.ID != id || p.ProjectID != project || p.RunID != run || p.AgentNodeID != node || p.TaskID != b.TaskID || p.TaskVersion != b.TaskVersion || b.ID != id || b.ProjectID != project || p.CodeBaseline != nil || dc.ValidateBinding(*b) != nil {
		return nil, continuationError()
	}
	var specRaw string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, run, node).Scan(&specRaw); err != nil {
		return nil, err
	}
	var actual dag.Node
	if json.Unmarshal([]byte(specRaw), &actual) != nil {
		return nil, continuationError()
	}
	found := false
	for _, frozen := range p.Workflow.Nodes {
		if frozen.ID == node {
			found = stringify(frozen) == stringify(actual)
		}
	}
	if !found || !developmentContinuationNode(actual, b.ID) {
		return nil, continuationError()
	}
	return &p, nil
}

func developmentContinuationNode(n dag.Node, id string) bool {
	_, purpose := n.Config["purpose"]
	_, fullBinding := n.Config["development_continuation"]
	return n.Type == "agent" && n.Config["development_continuation_id"] == id && !fullBinding && n.Config["code_baseline"] == nil && n.Config["candidate_from"] == nil && !purpose
}

func (s *Service) readDevelopmentContinuationExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string) (*dc.DevelopmentContinuationBinding, error) {
	p, err := s.readDevelopmentContinuationPlanTx(ctx, tx, tenant, project, run, node)
	if p == nil || err != nil {
		return nil, err
	}
	return p.DevelopmentContinuation, nil
}

func (s *Service) checkDevelopmentContinuationSourceTx(ctx context.Context, tx *sql.Tx, tenant, project string, b dc.DevelopmentContinuationBinding) error {
	if dc.ValidateBinding(b) != nil || b.ProjectID != project {
		return continuationError()
	}
	v, err := s.continuationProvenanceTx(ctx, tx, tenant, project, b.TaskID, b.SourceTaskRunID)
	if err != nil {
		return err
	}
	var state string
	var version int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT state,version FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, b.SourceTaskRunID).Scan(&state, &version); err != nil {
		return err
	}
	if v != b.SourceTaskVersion || version != b.SourceRunVersion || !continuationTerminal(state) {
		return continuationError()
	}
	subject := candidateSubject{tenant: tenant, project: project}
	stored, err := s.candidateTransferTx(ctx, tx, subject, b.Source.TransferID)
	if err != nil {
		return err
	}
	if stored.binding != b.Source {
		return continuationError()
	}
	return s.candidateSourceTx(ctx, tx, subject, b.Source)
}

func (s *Service) checkDevelopmentContinuationExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, fresh bool) error {
	p, err := s.readDevelopmentContinuationPlanTx(ctx, tx, tenant, project, run, node)
	if err != nil || p == nil {
		return err
	}
	b := p.DevelopmentContinuation
	var taskRun, status string
	var version int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT run_id,version,status FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, p.TaskID).Scan(&taskRun, &version, &status); err != nil {
		return err
	}
	if taskRun != run || version < b.TaskVersion+1 || fresh && (version != b.TaskVersion+1 || status == "accepted") {
		return continuationError()
	}
	if fresh {
		if p.Context.CodeBaseline != nil {
			if err = s.checkTaskCodeBaselineTx(ctx, tx, tenant, project, *p.Context.CodeBaseline); err != nil {
				return err
			}
		}
		return s.checkDevelopmentContinuationSourceTx(ctx, tx, tenant, project, *b)
	}
	return nil
}

func (s *Service) developmentContinuationInputBindingTx(ctx context.Context, tx *sql.Tx, a *candidateAuthority, req ccmp.CodeInputRequest) (out candidateTransfer, err error) {
	b, err := s.readDevelopmentContinuationExecutionTx(ctx, tx, a.subject.tenant, a.subject.project, a.assignment.RunID, a.assignment.NodeID)
	if err != nil {
		return out, err
	}
	if b == nil || b.ID != req.BindingID || b.Source.TransferID != req.TransferID || !developmentContinuationNode(a.spec, b.ID) {
		return out, forbidden()
	}
	if err = s.checkDevelopmentContinuationExecutionTx(ctx, tx, a.subject.tenant, a.subject.project, a.assignment.RunID, a.assignment.NodeID, true); err != nil {
		return out, err
	}
	return s.candidateTransferTx(ctx, tx, a.subject, b.Source.TransferID)
}

func (s *Service) validateDevelopmentContinuationResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	b, err := s.readDevelopmentContinuationExecutionTx(ctx, tx, tenant, project, result.RunID, result.NodeID)
	if err != nil {
		return err
	}
	fact, present := result.WorkspaceEvidence["development_continuation"]
	if b == nil {
		if present {
			return forbidden()
		}
		return nil
	}
	if present {
		var actual dc.DevelopmentContinuationBinding
		if json.Unmarshal([]byte(stringify(fact)), &actual) != nil || actual != *b {
			return continuationError()
		}
	}
	if result.OutcomeUnknown || result.Stopped || result.ExitCode != 0 || result.Error != "" {
		return nil
	}
	proof := result.WorkspaceEvidence
	if !present || proof["base_commit"] != b.Source.HeadCommit || proof["admin_base_commit"] != b.Source.BundleBase() || proof["state"] != "completed" || proof["clean"] != true || proof["outcome_unknown"] != false {
		return continuationError()
	}
	var reported cc.Binding
	if json.Unmarshal([]byte(stringify(proof["candidate_bundle"])), &reported) != nil || !reported.ValidSealed() || reported.HeadCommit != proof["head_commit"] || reported.BaseCommit != b.Source.HeadCommit || reported.BundleBase() != b.Source.BundleBase() || reported.EvidenceSHA256 != proof["evidence_sha256"] || reported.Source.RunID != result.RunID || reported.Source.NodeID != result.NodeID || reported.Source.Attempt != result.Attempt || reported.Source.Fence != result.Fence || reported.Source.WorkerID != result.WorkerID {
		return continuationError()
	}
	stored, err := s.candidateTransferTx(ctx, tx, candidateSubject{tenant: tenant, project: project}, reported.TransferID)
	if err != nil {
		return err
	}
	if stored.binding != reported {
		return continuationError()
	}
	return nil
}
