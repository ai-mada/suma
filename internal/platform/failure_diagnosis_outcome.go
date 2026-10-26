package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func diagnosisAdmission(message string) error {
	return &APIError{Status: 409, Code: "failure_diagnosis_basis_changed", Message: message}
}
func (s *Service) checkFailureDiagnosisExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run string, requireCurrentSource bool) error {
	r, found, e := s.failureDiagnosisExecutionRecordTx(ctx, tx, tenant, project, run)
	if e != nil || !found {
		return e
	}
	a := r.Analysis
	if a.State != "running" && a.State != "reconciling" {
		return diagnosisAdmission(diagnosisVersion)
	}
	c, e := s.readCredential(ctx, tx, r.CreatorCredentialHash, true)
	if e != nil {
		var api *APIError
		if errors.As(e, &api) {
			return diagnosisAdmission(diagnosisAuthority)
		}
		return e
	}
	clock, e := s.authorityNow(ctx, tx)
	if e != nil {
		return e
	}
	if c.kind != actorBrowser || c.ref.tenantID != tenant || c.ref.subjectID != a.CreatedBy || !c.expires.After(clock) {
		return diagnosisAdmission(diagnosisAuthority)
	}
	if _, e = s.currentProjectMembershipLockedTx(ctx, tx, tenant, a.CreatedBy, project, true, false); e != nil {
		var api *APIError
		if errors.As(e, &api) {
			return diagnosisAdmission(diagnosisAuthority)
		}
		return e
	}
	if requireCurrentSource {
		if e = s.checkFailureDiagnosisSourceTx(ctx, tx, tenant, project, a.Source); e != nil {
			var api *APIError
			if errors.As(e, &api) {
				return diagnosisAdmission(diagnosisSource)
			}
			return e
		}
	}
	p, e := s.toolProfileTx(ctx, tx, Principal{TenantID: tenant}, project, a.ToolProfileID)
	if e != nil {
		return e
	}
	if !failureDiagnosisProfile(p) || p.Version != a.ToolProfileVersion || p.Profile.ProfileSHA256 != a.ToolProfileSHA256 || p.WorkerID != r.WorkerID {
		return diagnosisAdmission(diagnosisProfile)
	}
	p, e = s.toolProfileObservationLockedTx(ctx, tx, tenant, project, p)
	if e != nil {
		return e
	}
	if !p.Available {
		return diagnosisAdmission(diagnosisProfile)
	}
	route, e := dag.ReadProjectExecutionRouteTx(ctx, tx, s.Dialect, tenant, project)
	if e != nil {
		return e
	}
	bound, e := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, run)
	if e != nil {
		return e
	}
	if route != r.Route || bound != r.Route {
		return diagnosisAdmission(diagnosisSource)
	}
	return s.checkDispatchProfileRouteTx(ctx, tx, tenant, project, p, route)
}

func (s *Service) recordFailureDiagnosisOutcomeTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result, authorize func(*sql.Tx) error) error {
	r, found, e := s.failureDiagnosisExecutionRecordTx(ctx, tx, tenant, project, result.RunID)
	if e != nil || !found {
		return e
	}
	if authorize == nil {
		return forbidden()
	}
	if e = authorize(tx); e != nil {
		return e
	}
	a := r.Analysis
	var attempt int
	var fence int64
	var worker, raw string
	e = tx.QueryRowContext(ctx, s.q(`SELECT n.attempt,n.fence,n.worker_id,n.spec FROM dag_nodes n JOIN dag_runs d ON d.id=n.run_id WHERE d.tenant_id=? AND d.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, result.RunID, result.NodeID).Scan(&attempt, &fence, &worker, &raw)
	if e != nil {
		return e
	}
	if result.NodeID != "diagnose" || result.Attempt != attempt || result.Fence != fence || result.WorkerID != worker || worker != r.WorkerID {
		return forbidden()
	}
	var node dag.Node
	if json.Unmarshal([]byte(raw), &node) != nil || node.Config["purpose"] != "failure_diagnosis" || stringify(node.Config["max_steps"]) != "1" || node.Config["tool_profile_id"] != a.ToolProfileID || node.Config["tool_profile_sha256"] != a.ToolProfileSHA256 {
		return forbidden()
	}
	sha := digest(stringify(result))
	var count int
	if e = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_failure_diagnosis_executions WHERE analysis_id=? AND attempt=? AND fence=? AND result_sha256=?`), a.ID, attempt, fence, sha).Scan(&count); e != nil {
		return e
	}
	if count > 0 {
		return nil
	}
	if a.State != "running" && a.State != "reconciling" {
		return conflict(diagnosisVersion)
	}
	old := a.Version
	a.Version++
	a.UpdatedAt = now()
	a.AllowedActions = []string{}
	if !result.OutcomeUnknown && a.OriginalOutputSHA256 == "" {
		a.OriginalOutputSHA256 = digest(result.Output)
		if len(result.Output) <= 64<<10 && utf8.ValidString(result.Output) {
			a.OriginalOutput = result.Output
		}
	}
	a.BasisCurrent = true
	if e = s.checkFailureDiagnosisSourceTx(ctx, tx, tenant, project, a.Source); e != nil {
		var api *APIError
		if !errors.As(e, &api) {
			return e
		}
		a.BasisCurrent = false
	}
	switch {
	case result.OutcomeUnknown:
		a.State = "reconciling"
		a.Reason = diagnosisUnknown
	case result.Stopped || result.ExitCode != 0 || result.Error != "":
		a.State = "failed"
		a.Reason = diagnosisFailed
	default:
		proof := result.ToolEvidence
		if proof == nil || proof.Terminal != "completed" || proof.Profile.ProfileSHA256 != a.ToolProfileSHA256 || proof.Profile.Adapter != "suma" || !proof.Profile.ReadOnly || proof.Profile.WorkspaceToolsVersion != "" || proof.Profile.Planning != nil || proof.Profile.KnowledgeExtraction != nil || proof.Profile.FailureDiagnosis == nil || proof.Profile.FailureDiagnosis.Protocol != toolcontract.FailureDiagnosisProtocol {
			a.State = "failed"
			a.Reason = diagnosisFailed
			break
		}
		proposal, err := parseFailureDiagnosisOutput(result.Output, a.Source)
		if err != nil {
			a.State = "failed"
			a.Reason = diagnosisOutput
			break
		}
		a.State = "ready"
		a.Proposal = &proposal
		repair := proposal.Repair
		repair.AcceptanceCriteria = append([]string{}, proposal.Repair.AcceptanceCriteria...)
		a.Repair = &repair
		a.Reason = ""
		if !a.BasisCurrent {
			a.Reason = diagnosisSource
		}
	}
	if _, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_failure_diagnosis_executions(tenant_id,project_id,analysis_id,attempt,fence,result_sha256,created_at,result)VALUES(?,?,?,?,?,?,?,?)`), tenant, project, a.ID, attempt, fence, sha, a.UpdatedAt, stringify(result)); e != nil {
		return e
	}
	r.Analysis = a
	return s.saveFailureDiagnosisTx(ctx, tx, tenant, r, old)
}
