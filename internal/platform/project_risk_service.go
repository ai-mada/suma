package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func (s *Service) projectRiskCommand(ctx context.Context, actor Actor, project, id, key string, purpose commandPurpose, request any, change func(context.Context, *authorizedCommand) (ProjectRisk, *bool, error)) (out ProjectRiskView, err error) {
	if actor.kind != actorBrowser {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, true) || !planningKey(key) {
		return out, invalid(projectRiskInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purpose)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, key, request)
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	risk, created, err := change(ctx, auth)
	if err != nil {
		return out, err
	}
	out, err = s.projectRiskViewTx(ctx, auth, risk)
	if err != nil {
		return out, err
	}
	out.Created = created
	if err = s.saveCommandReceipt(ctx, auth, key, hash, out); err != nil {
		return ProjectRiskView{}, err
	}
	return out, auth.tx.Commit()
}

func projectRiskReason(project, requested, reason string) bool {
	return project == requested && testPlanText(reason, 2000, true)
}

func (s *Service) CreateProjectRisk(ctx context.Context, actor Actor, project string, request ProjectRiskCreateRequest) (ProjectRiskView, error) {
	if !projectRiskReason(project, request.ProjectID, request.Reason) || !projectRiskFieldsValid(request.ProjectRiskFields) || !handoffID(request.SourceAlertID, true) || request.SourceAlertID == "" && request.ExpectedAlertVersion != 0 || request.SourceAlertID != "" && request.ExpectedAlertVersion < 1 {
		return ProjectRiskView{}, invalid(projectRiskInvalid)
	}
	return s.projectRiskCommand(ctx, actor, project, "", request.IdempotencyKey, purposeBrowserProjectWrite, map[string]any{"project_risk_create": request}, func(ctx context.Context, auth *authorizedCommand) (ProjectRisk, *bool, error) {
		if request.SourceAlertID != "" {
			var existing string
			err := auth.tx.QueryRowContext(ctx, s.q(`SELECT risk_id FROM suma_risk_sources WHERE tenant_id=? AND project_id=? AND alert_id=?`), auth.target.tenantID, project, request.SourceAlertID).Scan(&existing)
			if err == nil {
				risk, e := s.projectRiskTx(ctx, auth, project, existing)
				created := false
				if e == nil && (risk.Source == nil || risk.Source.AlertID != request.SourceAlertID) {
					e = conflict(projectRiskSource)
				}
				return risk, &created, e
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return ProjectRisk{}, nil, err
			}
		}
		stamp := now()
		risk := ProjectRisk{ID: newID(), TenantID: auth.target.tenantID, ProjectID: project, Version: 1, Cycle: 1, AssessmentRevision: 1, State: "open", ProjectRiskFields: request.ProjectRiskFields, Score: projectRiskScore(request.Probability, request.Impact), CreatedBy: auth.currentPrincipal.ID, CreatedAt: stamp, UpdatedAt: stamp}
		if risk.OwnerID != "" {
			if _, _, err := s.projectRiskOwnerTx(ctx, auth, project, risk.OwnerID); err != nil {
				return risk, nil, err
			}
		}
		if _, _, err := s.projectRiskLinksTx(ctx, auth, risk, false); err != nil {
			return risk, nil, err
		}
		if request.SourceAlertID != "" {
			source, err := s.projectRiskSuggestionTx(ctx, auth, project, request.SourceAlertID, request.ExpectedAlertVersion)
			if err != nil {
				return risk, nil, err
			}
			risk.Source = &source
		}
		if err := s.saveProjectRiskTx(ctx, auth, risk, 0); err != nil {
			return risk, nil, err
		}
		if risk.Source != nil {
			if _, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_risk_sources(tenant_id,project_id,alert_id,risk_id)VALUES(?,?,?,?)`), risk.TenantID, project, risk.Source.AlertID, risk.ID); err != nil {
				return risk, nil, err
			}
		}
		if err := s.commandActivity(ctx, auth, "risk.created", risk.ID, request.Reason); err != nil {
			return risk, nil, err
		}
		created := true
		return risk, &created, nil
	})
}

// Adopting a suggestion preserves its original model/source proof. It neither
// changes the suggestion's resolve semantics nor treats a model as an approver.
func (s *Service) projectRiskSuggestionTx(ctx context.Context, auth *authorizedCommand, project, id string, version int) (out ProjectRiskSource, err error) {
	alert, err := s.planningAlertTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if alert.ID != id || alert.ProjectID != project || alert.Version != version {
		return out, conflict(projectRiskSource)
	}
	record, err := s.planningRecordTx(ctx, auth.tx, auth.target.tenantID, project, alert.AnalysisID)
	if err != nil {
		return out, err
	}
	a := record.Analysis
	if a.RunID != alert.RunID || a.OriginalOutputSHA256 == "" || digest(a.OriginalOutput) != a.OriginalOutputSHA256 {
		return out, conflict(projectRiskSource)
	}
	original, err := parsePlanningOutput(a.OriginalOutput, a.Source)
	if err != nil {
		return out, conflict(projectRiskSource)
	}
	found := false
	for _, risk := range original.Risks {
		if risk.Key == alert.RiskKey && risk.Title == alert.Title && risk.Detail == alert.Detail && risk.Severity == alert.Severity && stringify(risk.TaskIDs) == stringify(alert.TaskIDs) {
			found = true
			break
		}
	}
	if !found {
		return out, conflict(projectRiskSource)
	}
	return ProjectRiskSource{AlertID: id, AlertVersion: version, AnalysisID: a.ID, RunID: a.RunID, RiskKey: alert.RiskKey, SourceSHA256: a.Source.SHA256, OriginalOutputSHA256: a.OriginalOutputSHA256, AdoptedBy: auth.currentPrincipal.ID, AdoptedAt: now()}, nil
}

func (s *Service) SaveProjectRisk(ctx context.Context, actor Actor, project, id string, request ProjectRiskSaveRequest) (ProjectRiskView, error) {
	if !projectRiskReason(project, request.ProjectID, request.Reason) || request.ExpectedVersion < 1 || !projectRiskFieldsValid(request.ProjectRiskFields) {
		return ProjectRiskView{}, invalid(projectRiskInvalid)
	}
	return s.projectRiskCommand(ctx, actor, project, id, request.IdempotencyKey, purposeBrowserProjectWrite, map[string]any{"project_risk_save": id, "request": request}, func(ctx context.Context, auth *authorizedCommand) (ProjectRisk, *bool, error) {
		risk, err := s.projectRiskTx(ctx, auth, project, id)
		if err != nil {
			return risk, nil, err
		}
		if risk.Version != request.ExpectedVersion {
			return risk, nil, conflict(projectRiskVersion)
		}
		if risk.State != "open" && risk.State != "mitigating" {
			return risk, nil, conflict(projectRiskState)
		}
		risk.ProjectRiskFields = request.ProjectRiskFields
		if risk.OwnerID != "" {
			if _, _, err = s.projectRiskOwnerTx(ctx, auth, project, risk.OwnerID); err != nil {
				return risk, nil, err
			}
		}
		if _, _, err = s.projectRiskLinksTx(ctx, auth, risk, false); err != nil {
			return risk, nil, err
		}
		previous := risk.Version
		risk.Version++
		risk.AssessmentRevision++
		risk.UpdatedAt = now()
		risk.Score = projectRiskScore(risk.Probability, risk.Impact)
		if err = s.saveProjectRiskTx(ctx, auth, risk, previous); err != nil {
			return risk, nil, err
		}
		err = s.commandActivity(ctx, auth, "risk.assessment_saved", id, request.Reason)
		return risk, nil, err
	})
}

func (s *Service) CommandProjectRisk(ctx context.Context, actor Actor, project, id string, request ProjectRiskCommandRequest) (ProjectRiskView, error) {
	if !projectRiskReason(project, request.ProjectID, request.Reason) || request.ExpectedVersion < 1 || !handoffID(request.ExpectedReviewID, true) {
		return ProjectRiskView{}, invalid(projectRiskInvalid)
	}
	purpose := purposeBrowserProjectWrite
	switch request.Action {
	case "start", "submit_review":
	case "return", "close", "reopen":
		purpose = purposeBrowserProjectApprove
	default:
		return ProjectRiskView{}, invalid(projectRiskInvalid)
	}
	if request.Action == "submit_review" {
		if request.Review == nil || !projectRiskReviewValid(*request.Review) {
			return ProjectRiskView{}, invalid(projectRiskInvalid)
		}
	} else if request.Review != nil {
		return ProjectRiskView{}, invalid(projectRiskInvalid)
	}
	if (request.Action == "close" || request.Action == "return") && request.ExpectedReviewID == "" {
		return ProjectRiskView{}, invalid(projectRiskInvalid)
	}
	return s.projectRiskCommand(ctx, actor, project, id, request.IdempotencyKey, purpose, map[string]any{"project_risk_command": id, "request": request}, func(ctx context.Context, auth *authorizedCommand) (ProjectRisk, *bool, error) {
		risk, err := s.projectRiskTx(ctx, auth, project, id)
		if err != nil {
			return risk, nil, err
		}
		if risk.Version != request.ExpectedVersion {
			return risk, nil, conflict(projectRiskVersion)
		}
		previous := risk.Version
		switch request.Action {
		case "start":
			if risk.State != "open" {
				return risk, nil, conflict(projectRiskState)
			}
			if !projectRiskAssessmentComplete(risk) {
				return risk, nil, conflict(projectRiskBasis)
			}
			if _, _, err = s.projectRiskOwnerTx(ctx, auth, project, risk.OwnerID); err != nil {
				return risk, nil, err
			}
			risk.State = "mitigating"
		case "submit_review":
			if risk.State != "mitigating" || !projectRiskAssessmentComplete(risk) {
				return risk, nil, conflict(projectRiskState)
			}
			review, e := s.prepareProjectRiskReviewTx(ctx, auth, risk, *request.Review)
			if e != nil {
				return risk, nil, e
			}
			if err = s.saveProjectRiskReviewTx(ctx, auth, risk, review, true); err != nil {
				return risk, nil, err
			}
			risk.CurrentReviewID, risk.State = review.ID, "in_review"
		case "return", "close":
			if risk.State != "in_review" || risk.CurrentReviewID != request.ExpectedReviewID {
				return risk, nil, conflict(projectRiskState)
			}
			review, e := s.projectRiskReviewTx(ctx, auth, risk, risk.CurrentReviewID)
			if e != nil {
				return risk, nil, e
			}
			if review.Decision != "pending" {
				return risk, nil, conflict(projectRiskState)
			}
			if request.Action == "close" {
				if err = s.checkProjectRiskReviewTx(ctx, auth, risk, review); err != nil {
					return risk, nil, err
				}
				review.Decision, risk.State = "closed", "closed"
			} else {
				review.Decision, risk.State = "returned", "mitigating"
				risk.CurrentReviewID = ""
			}
			review.DecidedBy, review.DecidedAt, review.DecisionReason = auth.currentPrincipal.ID, now(), request.Reason
			if err = s.saveProjectRiskReviewTx(ctx, auth, risk, review, false); err != nil {
				return risk, nil, err
			}
		case "reopen":
			if risk.State != "closed" {
				return risk, nil, conflict(projectRiskState)
			}
			risk.Cycle++
			risk.AssessmentRevision++
			risk.CurrentReviewID = ""
			risk.State = "open"
		}
		risk.Version++
		risk.UpdatedAt = now()
		if err = s.saveProjectRiskTx(ctx, auth, risk, previous); err != nil {
			return risk, nil, err
		}
		err = s.commandActivity(ctx, auth, "risk."+request.Action, id, request.Reason)
		return risk, nil, err
	})
}
