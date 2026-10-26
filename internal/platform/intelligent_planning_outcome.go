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

func planningAdmissionError(message string) error {
	return &APIError{Status: 409, Code: "planning_basis_changed", Message: message}
}

func (s *Service) planningExecutionRecordTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (record planningRecord, found bool, err error) {
	var id string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_planning_analyses WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return record, false, nil
	}
	if err != nil {
		return record, false, err
	}
	record, err = s.planningRecordTx(ctx, tx, tenant, project, id)
	return record, true, err
}

// The caller already holds current runner/pool organization authority. Re-read
// the actual creating credential; an ID never becomes a synthetic browser Actor.
func (s *Service) checkPlanningExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, runID string) error {
	record, found, err := s.planningExecutionRecordTx(ctx, tx, tenant, project, runID)
	if err != nil || !found {
		return err
	}
	a := record.Analysis
	if a.State != "running" && a.State != "reconciling" {
		return planningAdmissionError(planningVersionConflict)
	}
	credential, err := s.readCredential(ctx, tx, record.CreatorCredentialHash, true)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return planningAdmissionError(planningSourceUnauthorized)
		}
		return err
	}
	clock, err := s.authorityNow(ctx, tx)
	if err != nil {
		return err
	}
	if credential.kind != actorBrowser || credential.ref.subjectID != a.CreatedBy || credential.ref.tenantID != tenant || !credential.expires.After(clock) {
		return planningAdmissionError(planningSourceUnauthorized)
	}
	if _, err = s.currentProjectMembershipLockedTx(ctx, tx, tenant, a.CreatedBy, project, true, false); err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return planningAdmissionError(planningSourceUnauthorized)
		}
		return err
	}
	if err = s.planningCurrentSourceTx(ctx, tx, tenant, project, a.Source); err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return planningAdmissionError(planningSourceChanged)
		}
		return err
	}
	profile, err := s.toolProfileTx(ctx, tx, Principal{TenantID: tenant}, project, a.ToolProfileID)
	if err != nil {
		return err
	}
	if !planningProfile(profile) || profile.Version != a.ToolProfileVersion || profile.Profile.ProfileSHA256 != a.ToolProfileSHA256 || profile.WorkerID != record.WorkerID {
		return planningAdmissionError(planningProfileInvalid)
	}
	profile, err = s.toolProfileObservationLockedTx(ctx, tx, tenant, project, profile)
	if err != nil {
		return err
	}
	if !profile.Available {
		return planningAdmissionError(planningProfileInvalid)
	}
	route, err := dag.ReadProjectExecutionRouteTx(ctx, tx, s.Dialect, tenant, project)
	if err != nil {
		return err
	}
	bound, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, runID)
	if err != nil {
		return err
	}
	if route != record.Route || bound != record.Route {
		return planningAdmissionError(planningSourceChanged)
	}
	return s.checkDispatchProfileRouteTx(ctx, tx, tenant, project, profile, route)
}

func planningBoundedOutput(raw string) string {
	if len(raw) <= planningOutputLimit && utf8.ValidString(raw) {
		return raw
	}
	if len(raw) > planningOutputLimit {
		raw = raw[:planningOutputLimit]
	}
	for len(raw) > 0 && !utf8.ValidString(raw) {
		raw = raw[:len(raw)-1]
	}
	return raw
}

// Core has already validated the current attempt and tool result. Authority is
// checked before any projection writes; source drift cannot erase process facts.
func (s *Service) recordPlanningOutcomeTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result, authorize func(*sql.Tx) error) error {
	record, found, err := s.planningExecutionRecordTx(ctx, tx, tenant, project, result.RunID)
	if err != nil || !found {
		return err
	}
	if authorize == nil {
		return forbidden()
	}
	if err := authorize(tx); err != nil {
		return err
	}
	a := record.Analysis
	var attempt int
	var fence int64
	var worker, spec string
	err = tx.QueryRowContext(ctx, s.q(`SELECT n.attempt,n.fence,n.worker_id,n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, result.RunID, result.NodeID).Scan(&attempt, &fence, &worker, &spec)
	if err != nil {
		return err
	}
	if result.NodeID != "analyze" || result.Attempt != attempt || result.Fence != fence || result.WorkerID != worker || worker != record.WorkerID {
		return forbidden()
	}
	var node dag.Node
	if json.Unmarshal([]byte(spec), &node) != nil || node.Config["purpose"] != "planning" || node.Config["tool_profile_id"] != a.ToolProfileID || node.Config["tool_profile_sha256"] != a.ToolProfileSHA256 {
		return forbidden()
	}
	resultSHA := digest(stringify(result))
	var previous string
	err = tx.QueryRowContext(ctx, s.q(`SELECT result_sha256 FROM suma_planning_outcomes WHERE analysis_id=? AND attempt=? AND fence=?`), a.ID, attempt, fence).Scan(&previous)
	if err == nil {
		if previous == resultSHA {
			return nil
		}
		return conflict(planningVersionConflict)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if a.State != "running" && a.State != "reconciling" {
		return conflict(planningVersionConflict)
	}
	old := a.Version
	a.Version++
	a.UpdatedAt = now()
	a.OriginalOutputSHA256 = digest(result.Output)
	a.OriginalOutput = planningBoundedOutput(result.Output)
	record.OriginalOutput = a.OriginalOutput
	a.AllowedActions = []string{}
	a.BasisCurrent = true
	switch {
	case result.OutcomeUnknown:
		a.State = "reconciling"
		a.Reason = planningExecutionUnknown
	case result.Stopped || result.ExitCode != 0 || result.Error != "":
		a.State = "failed"
		a.Reason = planningExecutionFailed
	default:
		evidence := result.ToolEvidence
		if evidence == nil || evidence.Terminal != "completed" || evidence.Profile.ProfileSHA256 != a.ToolProfileSHA256 || evidence.Profile.Adapter != "suma" || !evidence.Profile.ReadOnly || evidence.Profile.Planning == nil || evidence.Profile.Planning.Protocol != toolcontract.PlanningProtocol {
			a.State = "failed"
			a.Reason = planningExecutionFailed
			break
		}
		proposal, e := parsePlanningOutput(result.Output, a.Source)
		if e != nil {
			a.State = "failed"
			a.Reason = planningInvalidOutput
			break
		}
		// Existing execution is durable even when its admission basis expired.
		if e = s.checkPlanningExecutionTx(ctx, tx, tenant, project, result.RunID); e != nil {
			var api *APIError
			if !errors.As(e, &api) {
				return e
			}
			a.State = "failed"
			a.Reason = api.Message
			a.BasisCurrent = false
			break
		}
		a.State = "ready"
		a.Proposal = &proposal
		a.Reason = ""
		for _, risk := range proposal.Risks {
			alert := PlanningAlert{ID: digest(a.ID + ":" + risk.Key)[:32], ProjectID: project, Version: 1, AnalysisID: a.ID, RunID: a.RunID, RiskKey: risk.Key, Severity: risk.Severity, Title: risk.Title, Detail: risk.Detail, TaskIDs: append([]string{}, risk.TaskIDs...), State: "open", CreatedAt: a.UpdatedAt, UpdatedAt: a.UpdatedAt}
			if _, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_planning_alerts(id,tenant_id,project_id,version,analysis_id,risk_key,severity,state,created_at,payload) VALUES(?,?,?,?,?,?,?,?,?,?)`), alert.ID, tenant, project, 1, a.ID, risk.Key, risk.Severity, "open", alert.CreatedAt, stringify(alert)); e != nil {
				return e
			}
		}
	}
	record.Analysis = a
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_planning_outcomes(tenant_id,project_id,analysis_id,run_id,node_id,attempt,fence,worker_id,result_sha256) VALUES(?,?,?,?,?,?,?,?,?)`), tenant, project, a.ID, result.RunID, result.NodeID, attempt, fence, worker, resultSHA); err != nil {
		return err
	}
	return s.savePlanningRecordTx(ctx, tx, tenant, project, record, old)
}
