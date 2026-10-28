package platform

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
)

func projectRiskRating(v *int) bool { return v == nil || *v >= 1 && *v <= 5 }
func projectRiskLinkKind(kind string) string {
	return map[string]string{"task": "tasks", "requirement": "requirements", "issue": "issues"}[kind]
}
func projectRiskFieldsValid(fields ProjectRiskFields) bool {
	if !testPlanText(fields.Title, 500, true) || !testPlanText(fields.Detail, 8000, true) || !handoffID(fields.OwnerID, true) || !validDate(fields.ReviewDate) || !testPlanText(fields.Plan, 8000, false) || !projectRiskRating(fields.Probability) || !projectRiskRating(fields.Impact) || fields.Links == nil || len(fields.Links) > 32 {
		return false
	}
	switch fields.Strategy {
	case "", "mitigate", "avoid", "transfer", "accept":
	default:
		return false
	}
	seen := map[string]bool{}
	for _, link := range fields.Links {
		key := link.Kind + ":" + link.ID
		if projectRiskLinkKind(link.Kind) == "" || !handoffID(link.ID, false) || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func projectRiskReviewValid(input ProjectRiskReviewInput) bool {
	if !projectRiskRating(input.ResidualProbability) || !projectRiskRating(input.ResidualImpact) || !testPlanText(input.Rationale, 4000, true) || !testPlanText(input.Evidence, 8000, true) {
		return false
	}
	switch input.Disposition {
	case "mitigated", "accepted", "avoided", "transferred", "not_applicable":
		return true
	}
	return false
}

func (s *Service) projectRiskOwnerTx(ctx context.Context, auth *authorizedCommand, project, id string) (Record, string, error) {
	if id == "" {
		return Record{}, "", invalid(projectRiskOwner)
	}
	resource, err := s.getRecordTx(ctx, auth, project, "members", id)
	if err != nil {
		return resource, "", err
	}
	user, _ := resource.Data["user_id"].(string)
	if resource.Kind != "human" || resource.Status == "archived" || resource.Status == "deleted" || resource.Status != "active" || user == "" {
		return resource, user, invalid(projectRiskOwner)
	}
	if _, err = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, user, project, false, false); err != nil {
		return resource, user, err
	}
	return resource, user, nil
}

func (s *Service) projectRiskLinksTx(ctx context.Context, auth *authorizedCommand, risk ProjectRisk, requireAccepted bool) ([]ProjectRiskLinkBasis, []ProjectRiskBlocker, error) {
	links := []ProjectRiskLinkBasis{}
	blockers := []ProjectRiskBlocker{}
	tasks := 0
	for _, ref := range risk.Links {
		r, err := s.getRecordTx(ctx, auth, risk.ProjectID, projectRiskLinkKind(ref.Kind), ref.ID)
		if err != nil {
			return nil, blockers, err
		}
		if r.Status == "archived" || r.Status == "deleted" {
			blockers = append(blockers, ProjectRiskBlocker{Code: "risk_links_changed", RefID: r.ID})
		}
		basis := ProjectRiskLinkBasis{Kind: ref.Kind, ID: ref.ID, Version: r.Version}
		if ref.Kind == "task" {
			tasks++
			if r.Status == "accepted" {
				basis.VerifiedRunID, _ = r.Data["verified_run_id"].(string)
				err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id=? AND task_version=? AND decision='accepted' ORDER BY created_at DESC,id DESC LIMIT 1`), risk.TenantID, risk.ProjectID, r.ID, basis.VerifiedRunID, r.Version-1).Scan(&basis.ReviewID)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return nil, blockers, err
				}
			}
			if requireAccepted {
				if r.Status != "accepted" || basis.VerifiedRunID == "" || basis.ReviewID == "" {
					blockers = append(blockers, ProjectRiskBlocker{Code: "risk_task_not_accepted", RefID: r.ID})
				} else if err = s.checkCurrentTaskEvidenceTx(ctx, auth.tx, auth.currentPrincipal, risk.ProjectID, r.ID, map[string]bool{}, map[string]bool{}); err != nil {
					var api *APIError
					if !errors.As(err, &api) {
						return nil, blockers, err
					}
					blockers = append(blockers, ProjectRiskBlocker{Code: "risk_task_evidence_stale", RefID: r.ID})
				} else {
					ok, code, e := s.projectRiskCodeEvidenceTx(ctx, auth, risk.ProjectID, basis.VerifiedRunID)
					if e != nil {
						return nil, blockers, e
					}
					if !ok {
						blockers = append(blockers, ProjectRiskBlocker{Code: code, RefID: r.ID})
					}
				}
			}
		}
		links = append(links, basis)
	}
	if requireAccepted && tasks == 0 {
		blockers = append(blockers, ProjectRiskBlocker{Code: "risk_task_not_accepted"})
	}
	sort.Slice(links, func(i, j int) bool { return links[i].Kind+":"+links[i].ID < links[j].Kind+":"+links[j].ID })
	return links, blockers, nil
}

func projectRiskAssessmentHash(r ProjectRisk) string {
	fields := r.ProjectRiskFields
	fields.Links = append([]ProjectRiskReference{}, fields.Links...)
	sort.Slice(fields.Links, func(i, j int) bool {
		return fields.Links[i].Kind+":"+fields.Links[i].ID < fields.Links[j].Kind+":"+fields.Links[j].ID
	})
	return digest(stringify([]any{r.ID, r.Cycle, r.AssessmentRevision, fields}))
}

func projectRiskAssessmentComplete(r ProjectRisk) bool {
	return r.OwnerID != "" && r.ReviewDate != "" && r.Strategy != "" && strings.TrimSpace(r.Plan) != ""
}
