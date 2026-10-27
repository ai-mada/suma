package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/toolcontract"
)

func (s *Service) usageManagementTx(ctx context.Context, a *authorizedCommand, project string) (bool, bool, error) {
	organization := orgAdmin(a.currentPrincipal)
	if organization {
		return true, true, nil
	}
	var role string
	e := a.tx.QueryRowContext(ctx, s.q(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`), project, a.currentPrincipal.ID).Scan(&role)
	if errors.Is(e, sql.ErrNoRows) {
		return false, false, nil
	}
	return role == "admin", false, e
}
func (s *Service) WriteUsagePolicy(ctx context.Context, actor Actor, project string, req UsagePolicyWriteRequest) (out UsagePolicy, err error) {
	if req.ProjectID != project || !handoffID(project, false) || req.Scope != "project" && req.Scope != "organization" || req.ExpectedVersion < 0 || req.Mode != "observe" && req.Mode != "enforce" || !usageCurrency(req.Currency) || !usageLimit(req.TokenLimit) || !usageLimit(req.CostLimitMicros) || !planningText(req.Reason, 4000) || !planningKey(req.IdempotencyKey) {
		return out, invalid(usageInvalid)
	}
	if req.Mode == "enforce" && !req.AcknowledgeEnforcement {
		return out, invalid(usageEnforceConfirmation)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	projectAdmin, orgAdmin, err := s.usageManagementTx(ctx, auth, project)
	if err != nil {
		return out, err
	}
	if req.Scope == "organization" && !orgAdmin || req.Scope == "project" && !projectAdmin {
		return out, forbidden()
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"usage_policy": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if err = s.usageLockTx(ctx, auth.tx, auth.target.tenantID); err != nil {
		return out, err
	}
	out, err = s.usagePolicyTx(ctx, auth.tx, auth.target.tenantID, project, req.Scope)
	if err != nil {
		return out, err
	}
	if out.Version != req.ExpectedVersion {
		return out, conflict(usageVersion)
	}
	out.Version++
	out.Mode = req.Mode
	out.Currency = req.Currency
	out.TokenLimit = req.TokenLimit
	out.CostLimitMicros = req.CostLimitMicros
	out.UpdatedBy = auth.currentPrincipal.ID
	out.UpdatedAt = now()
	out.AllowedActions = []string{"write"}
	if req.ExpectedVersion == 0 {
		_, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_usage_policies(tenant_id,scope,project_id,version,payload)VALUES(?,?,?,?,?)`), auth.target.tenantID, req.Scope, out.ProjectID, out.Version, stringify(out))
	} else {
		var result sql.Result
		result, err = auth.tx.ExecContext(ctx, s.q(`UPDATE suma_usage_policies SET version=?,payload=? WHERE tenant_id=? AND scope=? AND project_id=? AND version=?`), out.Version, stringify(out), auth.target.tenantID, req.Scope, out.ProjectID, req.ExpectedVersion)
		if err == nil {
			var n int64
			n, err = result.RowsAffected()
			if n != 1 && err == nil {
				err = conflict(usageVersion)
			}
		}
	}
	if err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "usage.policy_updated", out.ID, req.Reason); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
func (s *Service) WriteUsageRate(ctx context.Context, actor Actor, project string, req UsageRateWriteRequest) (out UsageRateRevision, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(req.ToolProfileID, false) || req.ExpectedToolVersion < 1 || req.ExpectedVersion < 0 || !usageCurrency(req.Currency) || !usageInteger(req.InputRateMicrosPerMillion) || !usageInteger(req.OutputRateMicrosPerMillion) || !usageInteger(req.InputTokenReservation) || !usageInteger(req.OutputTokenReservation) || req.InputTokenReservation > usageSafe-req.OutputTokenReservation || !planningText(req.Reason, 4000) || !planningKey(req.IdempotencyKey) {
		return out, invalid(usageInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, req.ToolProfileID, purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	admin, organizationAdmin, err := s.usageManagementTx(ctx, auth, project)
	if err != nil {
		return out, err
	}
	if !admin || !organizationAdmin {
		return out, forbidden()
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"usage_rate": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if err = s.usageLockTx(ctx, auth.tx, auth.target.tenantID); err != nil {
		return out, err
	}
	profile, err := s.toolProfileTx(ctx, auth.tx, auth.currentPrincipal, project, req.ToolProfileID)
	if err != nil {
		return out, err
	}
	model, origin, priced := usagePricedModel(profile)
	if supportsModelExecution(profile) {
		_, deployments, e := s.currentModelDeploymentsTx(ctx, auth.tx, auth.target.tenantID, project, profile)
		if e != nil {
			return out, e
		}
		for _, d := range deployments {
			if d.ID == req.ModelDeploymentID && d.Revision == req.ModelDeploymentRevision && d.SHA256 == req.ModelDeploymentSHA256 {
				model, origin, priced = d.Model, d.EndpointOrigin, true
				break
			}
		}
	} else if req.ModelDeploymentID != "" || req.ModelDeploymentRevision != 0 || req.ModelDeploymentSHA256 != "" {
		return out, invalid(usageInvalid)
	}
	if !priced {
		return out, invalid(usageRateProfile)
	}
	// Pricing is configuration, not permission to dispatch a process. Bind the
	// current approved profile generation without requiring a live worker report.
	if profile.Version != req.ExpectedToolVersion {
		return out, conflict(usageVersion)
	}
	current, err := s.usageRateTx(ctx, auth.tx, auth.target.tenantID, project, profile.ID)
	if err != nil {
		return out, err
	}
	version := 0
	if current != nil {
		version = current.Version
	}
	if version != req.ExpectedVersion {
		return out, conflict(usageVersion)
	}
	out = UsageRateRevision{ID: newID(), ProjectID: project, ToolProfileID: profile.ID, Version: version + 1, ProfileSHA256: profile.Profile.ProfileSHA256, Model: model, EndpointOrigin: origin, Currency: req.Currency, InputRateMicrosPerMillion: req.InputRateMicrosPerMillion, OutputRateMicrosPerMillion: req.OutputRateMicrosPerMillion, InputTokenReservation: req.InputTokenReservation, OutputTokenReservation: req.OutputTokenReservation, Reason: req.Reason, CreatedBy: auth.currentPrincipal.ID, CreatedAt: now()}
	out.ModelDeploymentID, out.ModelDeploymentRevision, out.ModelDeploymentSHA256 = req.ModelDeploymentID, req.ModelDeploymentRevision, req.ModelDeploymentSHA256
	if usagePublic(usageCost(out.InputTokenReservation, out.OutputTokenReservation, out)) == nil {
		return out, invalid(usageInvalid)
	}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_usage_rates(id,tenant_id,project_id,tool_profile_id,version,created_at,payload)VALUES(?,?,?,?,?,?,?)`), out.ID, auth.target.tenantID, project, profile.ID, out.Version, out.CreatedAt, stringify(out)); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "usage.rate_created", out.ID, req.Reason); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

func usageCountersAtLeast(next toolcontract.Usage, old *toolcontract.Usage) bool {
	if next.InputTokens == nil || next.OutputTokens == nil {
		return false
	}
	if old == nil {
		return true
	}
	for _, pair := range [][2]*int64{{next.InputTokens, old.InputTokens}, {next.OutputTokens, old.OutputTokens}, {next.CachedInputTokens, old.CachedInputTokens}, {next.CacheWriteInputTokens, old.CacheWriteInputTokens}, {next.ReasoningOutputTokens, old.ReasoningOutputTokens}} {
		if pair[1] != nil && (pair[0] == nil || *pair[0] < *pair[1]) {
			return false
		}
	}
	return true
}

func (s *Service) ConfirmUsageAttempt(ctx context.Context, actor Actor, project, id string, req UsageConfirmRequest) (out UsageAttempt, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || !planningText(req.SourceRef, 2000) || !planningText(req.Reason, 4000) || !planningKey(req.IdempotencyKey) {
		return out, invalid(usageInvalid)
	}
	if !req.AcknowledgeUsage {
		return out, invalid(usageConfirmRequired)
	}
	raw := stringify(req.Usage)
	var parsed toolcontract.Usage
	if e := json.Unmarshal([]byte(raw), &parsed); e != nil || parsed.InputTokens == nil || parsed.OutputTokens == nil {
		return out, invalid(usageInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	defer close()
	if !orgAdmin(auth.currentPrincipal) {
		return out, forbidden()
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"usage_confirm": id, "body": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if err = s.usageLockTx(ctx, auth.tx, auth.target.tenantID); err != nil {
		return out, err
	}
	record, err := s.usageRecordTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	out = record.Attempt
	if out.Version != req.ExpectedVersion || !record.Unresolved {
		return out, conflict(usageVersion)
	}
	projection, e := s.usageProjectionTx(ctx, auth, record)
	if e != nil {
		return out, e
	}
	if len(projection.AllowedActions) == 0 {
		return out, conflict(usageVersion)
	}
	if !usageCountersAtLeast(req.Usage, out.Usage) {
		return out, invalid(usageCannotReduce)
	}
	old := out.Version
	record.Attempt.Usage = &req.Usage
	record.Attempt.UsageState = toolcontract.UsageComplete
	record.Attempt.UsageSource = "manual_confirmation"
	record.Attempt.DecisionBy = auth.currentPrincipal.ID
	record.Attempt.DecisionAt = now()
	record.Attempt.DecisionReason = req.Reason
	record.Attempt.SourceRef = req.SourceRef
	if err = s.settleUsageTx(ctx, auth.tx, auth.target.tenantID, &record, true, "confirmed"); err != nil {
		return out, err
	}
	record.Attempt.Version++
	record.Attempt.UpdatedAt = now()
	if err = s.saveUsageTx(ctx, auth.tx, auth.target.tenantID, record, old); err != nil {
		return out, err
	}
	if err = s.usageFactTx(ctx, auth.tx, auth.target.tenantID, project, id, "confirmation", digest(stringify(req)), req); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "usage.manually_confirmed", id, req.Reason); err != nil {
		return out, err
	}
	out = record.Attempt
	out.AllowedActions = []string{}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
