package platform

import (
	"context"
	"encoding/json"
	"errors"
)

func (s *Service) prepareProjectRiskReviewTx(ctx context.Context, auth *authorizedCommand, risk ProjectRisk, input ProjectRiskReviewInput) (ProjectRiskReview, error) {
	owner, user, err := s.projectRiskOwnerTx(ctx, auth, risk.ProjectID, risk.OwnerID)
	if err != nil {
		return ProjectRiskReview{}, err
	}
	links, blockers, err := s.projectRiskLinksTx(ctx, auth, risk, input.Disposition == "mitigated")
	if err != nil {
		return ProjectRiskReview{}, err
	}
	if len(blockers) > 0 {
		if input.Disposition == "mitigated" {
			return ProjectRiskReview{}, conflict(projectRiskMitigation)
		}
		return ProjectRiskReview{}, conflict(projectRiskBasis)
	}
	return ProjectRiskReview{ID: newID(), Cycle: risk.Cycle, AssessmentRevision: risk.AssessmentRevision, AssessmentSHA256: projectRiskAssessmentHash(risk), OwnerID: owner.ID, OwnerVersion: owner.Version, OwnerUserID: user, Links: links, ProjectRiskReviewInput: input, ResidualScore: projectRiskScore(input.ResidualProbability, input.ResidualImpact), SubmittedBy: auth.currentPrincipal.ID, SubmittedAt: now(), Decision: "pending"}, nil
}

func (s *Service) checkProjectRiskReviewTx(ctx context.Context, auth *authorizedCommand, risk ProjectRisk, review ProjectRiskReview) error {
	if review.Decision != "pending" || review.Cycle != risk.Cycle || review.AssessmentRevision != risk.AssessmentRevision || review.AssessmentSHA256 != projectRiskAssessmentHash(risk) || review.OwnerID != risk.OwnerID {
		return conflict(projectRiskVersion)
	}
	owner, user, err := s.projectRiskOwnerTx(ctx, auth, risk.ProjectID, risk.OwnerID)
	if err != nil {
		return err
	}
	if owner.Version != review.OwnerVersion || user != review.OwnerUserID {
		return conflict(projectRiskVersion)
	}
	links, blockers, err := s.projectRiskLinksTx(ctx, auth, risk, review.Disposition == "mitigated")
	if err != nil {
		return err
	}
	if len(blockers) > 0 || stringify(links) != stringify(review.Links) {
		return conflict(projectRiskVersion)
	}
	return nil
}

func projectRiskExpectedError(err error) bool { var api *APIError; return errors.As(err, &api) }

func (s *Service) projectRiskViewTx(ctx context.Context, auth *authorizedCommand, risk ProjectRisk) (out ProjectRiskView, err error) {
	out = ProjectRiskView{Risk: risk, History: []ProjectRiskReview{}, AllowedActions: []string{}, Readiness: ProjectRiskReadiness{Blockers: []ProjectRiskBlocker{}}}
	if risk.CurrentReviewID != "" {
		review, e := s.projectRiskReviewTx(ctx, auth, risk, risk.CurrentReviewID)
		if e != nil {
			return out, e
		}
		out.CurrentReview = &review
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_risk_reviews WHERE tenant_id=? AND project_id=? AND risk_id=? AND id<>?`), risk.TenantID, risk.ProjectID, risk.ID, risk.CurrentReviewID).Scan(&out.HistoryTotal); err != nil {
		return out, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT snapshot FROM suma_risk_reviews WHERE tenant_id=? AND project_id=? AND risk_id=? AND id<>? ORDER BY created_at DESC,id DESC LIMIT ?`), risk.TenantID, risk.ProjectID, risk.ID, risk.CurrentReviewID, projectRiskHistoryLimit)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var raw string
		var review ProjectRiskReview
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &review); err != nil {
			break
		}
		out.History = append(out.History, review)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	out.HistoryComplete = len(out.History) == out.HistoryTotal
	add := func(code, ref string) {
		for _, b := range out.Readiness.Blockers {
			if b.Code == code && b.RefID == ref {
				return
			}
		}
		out.Readiness.Blockers = append(out.Readiness.Blockers, ProjectRiskBlocker{Code: code, RefID: ref})
	}
	complete := projectRiskAssessmentComplete(risk)
	if !complete {
		add("risk_assessment_incomplete", "")
	}
	ownerOK := false
	if risk.OwnerID == "" {
		add("risk_owner_required", "")
	} else {
		_, _, e := s.projectRiskOwnerTx(ctx, auth, risk.ProjectID, risk.OwnerID)
		if e != nil {
			if !projectRiskExpectedError(e) {
				return out, e
			}
			add("risk_owner_unavailable", risk.OwnerID)
		} else {
			ownerOK = true
		}
	}
	_, linkBlockers, e := s.projectRiskLinksTx(ctx, auth, risk, false)
	linksOK := e == nil && len(linkBlockers) == 0
	if e != nil {
		if !projectRiskExpectedError(e) {
			return out, e
		}
		add("risk_links_changed", "")
	}
	for _, b := range linkBlockers {
		add(b.Code, b.RefID)
	}
	out.Readiness.AssessmentReady = complete && ownerOK && linksOK
	_, mitigationBlockers, e := s.projectRiskLinksTx(ctx, auth, risk, true)
	if e != nil {
		if !projectRiskExpectedError(e) {
			return out, e
		}
		add("risk_task_evidence_stale", "")
	}
	for _, b := range mitigationBlockers {
		add(b.Code, b.RefID)
	}
	out.Readiness.MitigationReady = out.Readiness.AssessmentReady && e == nil && len(mitigationBlockers) == 0
	if risk.State == "in_review" {
		if out.CurrentReview == nil {
			add("risk_review_missing", "")
		} else if e = s.checkProjectRiskReviewTx(ctx, auth, risk, *out.CurrentReview); e != nil {
			if !projectRiskExpectedError(e) {
				return out, e
			}
			add("risk_review_changed", out.CurrentReview.ID)
		} else {
			out.Readiness.CloseReady = true
		}
	}
	if risk.State == "closed" {
		add("risk_closed", "")
	}
	if auth.actor.kind != actorBrowser {
		return out, nil
	}
	_, writeErr := s.currentProjectMembershipLockedTx(ctx, auth.tx, risk.TenantID, auth.currentPrincipal.ID, risk.ProjectID, true, false)
	if writeErr != nil && !projectRiskExpectedError(writeErr) {
		return out, writeErr
	}
	_, approveErr := s.currentProjectMembershipLockedTx(ctx, auth.tx, risk.TenantID, auth.currentPrincipal.ID, risk.ProjectID, true, true)
	if approveErr != nil && !projectRiskExpectedError(approveErr) {
		return out, approveErr
	}
	if writeErr == nil {
		if risk.State == "open" || risk.State == "mitigating" {
			out.AllowedActions = append(out.AllowedActions, "save")
		}
		if risk.State == "open" && out.Readiness.AssessmentReady {
			out.AllowedActions = append(out.AllowedActions, "start")
		}
		if risk.State == "mitigating" && out.Readiness.AssessmentReady {
			out.AllowedActions = append(out.AllowedActions, "submit_review")
		}
	}
	if approveErr == nil {
		if risk.State == "in_review" {
			out.AllowedActions = append(out.AllowedActions, "return")
			if out.Readiness.CloseReady {
				out.AllowedActions = append(out.AllowedActions, "close")
			}
		}
		if risk.State == "closed" {
			out.AllowedActions = append(out.AllowedActions, "reopen")
		}
	}
	return out, nil
}
