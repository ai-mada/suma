package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) MigrateCandidateTransfers(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_candidate_transfers(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,subject_hash VARCHAR(64) NOT NULL,binding TEXT NOT NULL,storage_path VARCHAR(200) NOT NULL,created_at VARCHAR(40) NOT NULL,UNIQUE(tenant_id,project_id,run_id,node_id,attempt,fence))`))
	return err
}

// A subject is selected only by an authenticated transport. Machine callbacks
// retain their real pool proof and are never converted into a human Actor.
type candidateSubject struct {
	actor           Actor
	token           string
	pool            bool
	tenant, project string
}
type candidateAuthority struct {
	subject    candidateSubject
	tx         *sql.Tx
	assignment cc.AssignmentRef
	human      *authorizedCommand
	machine    *poolMachineProof
	spec       dag.Node
}
type candidateTransfer struct {
	binding           cc.Binding
	subjectHash, path string
}

func projectCandidateSubject(actor Actor, project string) candidateSubject {
	return candidateSubject{actor: actor, tenant: actor.credential.tenantID, project: project}
}
func candidateDAGAssignment(a cc.AssignmentRef) dag.Assignment {
	return dag.Assignment{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, TenantID: a.TenantID, ProjectID: a.ProjectID, PoolID: a.PoolID, PoolRunnerID: a.PoolRunnerID, PoolSlotID: a.PoolSlotID, PoolGrantID: a.PoolGrantID, PoolGrantVersion: a.PoolGrantVersion, PoolCredentialID: a.PoolCredentialID, PoolCredentialVersion: a.PoolCredentialVersion, DispatchID: a.DispatchID, DispatchState: a.DispatchState, LeaseUntil: a.LeaseUntil}
}
func candidateHasPoolFields(a cc.AssignmentRef) bool {
	return a.TenantID != "" || a.ProjectID != "" || a.PoolID != "" || a.PoolRunnerID != "" || a.PoolSlotID != "" || a.PoolGrantID != "" || a.PoolGrantVersion != 0 || a.PoolCredentialID != "" || a.PoolCredentialVersion != 0 || a.DispatchID != "" || a.DispatchState != "" || a.LeaseUntil != ""
}
func (c candidateSubject) hash() string {
	if c.pool {
		return digest(c.token)
	}
	return c.actor.credential.digest
}
func (a *candidateAuthority) assert() error {
	if a == nil || a.tx == nil {
		return forbidden()
	}
	if a.subject.pool {
		return a.machine.assertAssignment(a.tx, candidateDAGAssignment(a.assignment))
	}
	return a.human.assertTx(a.tx, a.human.target, purposeRunnerWrite)
}
func (s *Service) candidateAuthorityTx(ctx context.Context, tx *sql.Tx, subject candidateSubject, a cc.AssignmentRef) (*candidateAuthority, error) {
	if !a.Valid() || subject.project == "" {
		return nil, invalid("Invalid candidate transfer assignment")
	}
	out := &candidateAuthority{subject: subject, tx: tx, assignment: a}
	var err error
	if subject.pool {
		if subject.token == "" || a.TenantID != subject.tenant || a.ProjectID != subject.project || a.PoolID == "" || a.PoolRunnerID == "" || a.PoolSlotID == "" || a.PoolGrantID == "" || a.PoolGrantVersion < 1 || a.PoolCredentialID == "" || a.PoolCredentialVersion < 1 || a.DispatchID == "" {
			return nil, forbidden()
		}
		out.machine, err = s.PoolRunnerAssignmentAuthorityTx(ctx, tx, subject.token, candidateDAGAssignment(a))
		if err != nil {
			return nil, err
		}
		// Existing fact callbacks may drain a withdrawn grant. Code transfer is a
		// fresh read/write and additionally requires current bilateral approval.
		identity, e := s.poolMachineIdentityTx(ctx, tx, subject.token, a.PoolRunnerID, true)
		if e != nil {
			return nil, e
		}
		if identity.poolStatus != "active" {
			return nil, forbidden()
		}
		scope, e := s.currentPoolActiveScopeTx(ctx, tx, a.PoolID, subject.tenant, subject.project)
		if e != nil {
			return nil, e
		}
		if scope.GrantID != a.PoolGrantID {
			return nil, forbidden()
		}
	} else {
		if candidateHasPoolFields(a) {
			return nil, forbidden()
		}
		out.human, err = s.runnerIdentityTx(ctx, tx, subject.actor, subject.project, a.WorkerID)
		if err != nil {
			return nil, err
		}
		if err = s.projectRunnerExecutionAuthorityTx(ctx, tx, out.human, a.WorkerID, a.RunID); err != nil {
			return nil, err
		}
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, subject.tenant, subject.project); err != nil {
		return nil, err
	}
	var state, runState, worker, raw string
	var attempt int
	var fence, lease int64
	err = tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT n.state,r.state,n.worker_id,n.attempt,n.fence,n.lease_until,n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`, true)), subject.tenant, subject.project, a.RunID, a.NodeID).Scan(&state, &runState, &worker, &attempt, &fence, &lease, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, missing()
	}
	if err != nil {
		return nil, err
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if state != "running" || runState != "running" && runState != "paused" || worker != a.WorkerID || attempt != a.Attempt || fence != a.Fence || lease <= clock {
		return nil, conflict("Candidate transfer lease is no longer current")
	}
	if err = json.Unmarshal([]byte(raw), &out.spec); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) candidateReceipt(ctx context.Context, a *candidateAuthority, op string, body any) (string, bool, error) {
	if err := a.assert(); err != nil {
		return "", false, err
	}
	if len(op) < 8 || len(op) > 128 {
		return "", false, invalid("Invalid candidate transfer operation ID")
	}
	hash := digest(a.subject.hash() + stringify(body))
	var previous, response string
	err := a.tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), a.subject.tenant, a.subject.project, op).Scan(&previous, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return hash, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if hash != previous {
		return "", false, &APIError{409, "idempotency_conflict", "Candidate transfer operation content changed", nil}
	}
	return response, true, nil
}
func (s *Service) saveCandidateReceipt(ctx context.Context, a *candidateAuthority, op, hash string, response any) error {
	if err := a.assert(); err != nil {
		return err
	}
	_, err := a.tx.ExecContext(ctx, s.q(`INSERT INTO suma_operations(tenant_id,project_id,operation_id,request_hash,response)VALUES(?,?,?,?,?)`), a.subject.tenant, a.subject.project, op, hash, stringify(response))
	return err
}
func (s *Service) candidateTransferTx(ctx context.Context, tx *sql.Tx, subject candidateSubject, id string) (candidateTransfer, error) {
	var out candidateTransfer
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT binding,subject_hash,storage_path FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND id=?`), subject.tenant, subject.project, id).Scan(&raw, &out.subjectHash, &out.path)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out.binding); err != nil {
		return out, err
	}
	if out.binding.TransferID != id || out.binding.ProjectID != subject.project {
		return out, conflict("Candidate transfer metadata changed")
	}
	return out, nil
}

func (s *Service) StartCandidateTransfer(ctx context.Context, actor Actor, project string, r cc.StartRequest) (cc.Binding, error) {
	return s.startCandidateTransfer(ctx, projectCandidateSubject(actor, project), r)
}
func (s *Service) startCandidateTransfer(ctx context.Context, subject candidateSubject, r cc.StartRequest) (cc.Binding, error) {
	var out cc.Binding
	if err := r.Validate(); err != nil || r.ProjectID != subject.project {
		return out, invalid("Invalid candidate transfer manifest or assignment")
	}
	ctx, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup()
	defer tx.Rollback()
	auth, err := s.candidateAuthorityTx(ctx, tx, subject, r.Assignment)
	if err != nil {
		return out, err
	}
	if auth.spec.Type != "command" && auth.spec.Type != "agent" {
		return out, invalid("Candidate source must be a code command or agent")
	}
	hash, replay, err := s.candidateReceipt(ctx, auth, r.OperationID, map[string]any{"candidate_start": r})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		current, e := s.candidateTransferTx(ctx, tx, subject, out.TransferID)
		if e != nil {
			return out, e
		}
		expected := cc.Binding{SchemaVersion: cc.SchemaVersion, TransferID: out.TransferID, ProjectID: subject.project, State: out.State,
			Source: r.Assignment, BaseCommit: r.BaseCommit, HeadCommit: r.HeadCommit, EvidenceSHA256: r.EvidenceSHA256, BundleSHA256: r.BundleSHA256, SizeBytes: r.SizeBytes}
		if out != expected || current.subjectHash != subject.hash() || out.State != "uploading" && out.State != "sealed" {
			return out, conflict("Candidate transfer metadata changed")
		}
		// Start recovers the same original operation and immutable producer
		// identity, while projecting its current monotonic transfer state. The
		// original uploading receipt stays unchanged, so a lost seal ACK can
		// recover sealed without retransmitting the entire bundle.
		expected.State = current.binding.State
		if current.binding != expected || current.binding.State != "uploading" && current.binding.State != "sealed" || out.State == "sealed" && current.binding.State != "sealed" {
			return out, conflict("Candidate transfer metadata changed")
		}
		return current.binding, nil
	}
	var existing int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`), subject.tenant, subject.project, r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt, r.Assignment.Fence).Scan(&existing); err != nil {
		return out, err
	}
	if existing != 0 {
		return out, conflict("Candidate source already has a transfer; recover the original operation")
	}
	out = cc.Binding{SchemaVersion: cc.SchemaVersion, TransferID: newID(), ProjectID: subject.project, State: "uploading", Source: r.Assignment, BaseCommit: r.BaseCommit, HeadCommit: r.HeadCommit, EvidenceSHA256: r.EvidenceSHA256, BundleSHA256: r.BundleSHA256, SizeBytes: r.SizeBytes}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_candidate_transfers(id,tenant_id,project_id,run_id,node_id,attempt,fence,worker_id,subject_hash,binding,storage_path,created_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`), out.TransferID, subject.tenant, subject.project, r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt, r.Assignment.Fence, r.Assignment.WorkerID, subject.hash(), stringify(out), "", now())
	if err != nil {
		return out, err
	}
	if err = s.saveCandidateReceipt(ctx, auth, r.OperationID, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) candidateSourceTx(ctx context.Context, tx *sql.Tx, subject candidateSubject, b cc.Binding) error {
	if !b.ValidSealed() {
		return conflict("Candidate transfer is not sealed")
	}
	var nodeState, attemptState, worker, raw, specRaw string
	var attempt int
	var fence int64
	err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT n.state,n.attempt,n.fence,n.worker_id,n.spec,a.state,a.result FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=? AND a.fence=? AND a.worker_id=? WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`, true)), b.Source.Attempt, b.Source.Fence, b.Source.WorkerID, subject.tenant, subject.project, b.Source.RunID, b.Source.NodeID).Scan(&nodeState, &attempt, &fence, &worker, &specRaw, &attemptState, &raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) || nodeState != "succeeded" || attemptState != "succeeded" || attempt != b.Source.Attempt || fence != b.Source.Fence || worker != b.Source.WorkerID {
		return conflict("Candidate source successful attempt changed")
	}
	var result dag.Result
	var spec dag.Node
	if json.Unmarshal([]byte(raw), &result) != nil || json.Unmarshal([]byte(specRaw), &spec) != nil || spec.Type != "command" && spec.Type != "agent" || result.ExitCode != 0 || result.Error != "" || result.Stopped || result.OutcomeUnknown || result.RunID != b.Source.RunID || result.NodeID != b.Source.NodeID || result.Attempt != b.Source.Attempt || result.Fence != b.Source.Fence || result.WorkerID != b.Source.WorkerID {
		return conflict("Candidate source result does not match sealed evidence")
	}
	head, e := workspaceRevision(result.WorkspaceEvidence)
	if e != nil || head != b.HeadCommit || result.WorkspaceEvidence["base_commit"] != b.BaseCommit || result.WorkspaceEvidence["evidence_sha256"] != b.EvidenceSHA256 {
		return conflict("Candidate source result does not match sealed evidence")
	}
	var reported cc.Binding
	if json.Unmarshal([]byte(stringify(result.WorkspaceEvidence["candidate_bundle"])), &reported) != nil || reported != b {
		return conflict("Candidate source result does not match sealed evidence")
	}
	return nil
}

func (s *Service) resolveCandidateTx(ctx context.Context, tx *sql.Tx, subject candidateSubject, auth *candidateAuthority) (candidateTransfer, error) {
	var out candidateTransfer
	if auth.spec.Type != "command" && auth.spec.Type != "agent" && auth.spec.Type != "test" {
		return out, invalid("Candidate consumer must be an immutable command, agent or test")
	}
	var head, source, evidence string
	var attempt int
	var fence int64
	a := auth.assignment
	err := tx.QueryRowContext(ctx, s.q(`SELECT commit_id,source_node,source_attempt,source_fence,evidence_sha256 FROM dag_candidate_revisions WHERE run_id=? AND node_id=? AND attempt=?`), a.RunID, a.NodeID, a.Attempt).Scan(&head, &source, &attempt, &fence, &evidence)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if errors.Is(err, sql.ErrNoRows) || auth.spec.Config["candidate_from"] != source {
		return out, conflict("Consumer has no matching frozen candidate")
	}
	var id string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`), subject.tenant, subject.project, a.RunID, source, attempt, fence).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	out, err = s.candidateTransferTx(ctx, tx, subject, id)
	if err != nil {
		return out, err
	}
	b := out.binding
	if b.HeadCommit != head || b.EvidenceSHA256 != evidence || b.Source.RunID != a.RunID || b.Source.NodeID != source || b.Source.Attempt != attempt || b.Source.Fence != fence {
		return out, conflict("Consumer has no matching frozen candidate")
	}
	if err = s.candidateSourceTx(ctx, tx, subject, b); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Service) ResolveCandidateTransfer(ctx context.Context, actor Actor, project string, r cc.CommandRequest) (cc.Binding, error) {
	return s.resolveCandidateTransfer(ctx, projectCandidateSubject(actor, project), r)
}
func (s *Service) resolveCandidateTransfer(ctx context.Context, subject candidateSubject, r cc.CommandRequest) (cc.Binding, error) {
	var out cc.Binding
	if r.ProjectID != subject.project {
		return out, forbidden()
	}
	ctx, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup()
	defer tx.Rollback()
	auth, err := s.candidateAuthorityTx(ctx, tx, subject, r.Assignment)
	if err != nil {
		return out, err
	}
	transfer, err := s.resolveCandidateTx(ctx, tx, subject, auth)
	if err != nil {
		return out, err
	}
	hash, replay, err := s.candidateReceipt(ctx, auth, r.OperationID, map[string]any{"candidate_resolve": r})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		if out != transfer.binding {
			return out, conflict("Candidate transfer metadata changed")
		}
		return out, nil
	}
	out = transfer.binding
	if err = s.saveCandidateReceipt(ctx, auth, r.OperationID, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
