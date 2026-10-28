package platform

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

// Formal task acceptance is the common authority. For code-bearing tasks we
// additionally retain the original candidate-to-independent-test relationship;
// a successful test-only result cannot substitute a missing code producer.
func (s *Service) projectRiskCodeEvidenceTx(ctx context.Context, auth *authorizedCommand, project, run string) (bool, string, error) {
	return s.projectRiskCodeEvidenceTreeTx(ctx, auth, project, run, 0, map[string]bool{}, new(int))
}

// A successful wrapper is not evidence that its children were non-code. Only
// current scheduler-owned child links may extend this proof; arbitrary output
// references and older subworkflow attempts cannot introduce a child result.
func (s *Service) projectRiskCodeEvidenceTreeTx(ctx context.Context, auth *authorizedCommand, project, run string, depth int, visiting map[string]bool, total *int) (bool, string, error) {
	unsupported := "risk_execution_source_unsupported"
	if run == "" || depth > 32 || visiting[run] {
		return false, unsupported, nil
	}
	visiting[run] = true
	defer delete(visiting, run)
	var state string
	if err := auth.tx.QueryRowContext(ctx, s.q(`SELECT state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, run).Scan(&state); err != nil {
		return false, "", err
	}
	if state != "succeeded" {
		return false, "risk_task_evidence_stale", nil
	}
	nodes, err := s.testPlanNodesTx(ctx, auth.tx, auth.target.tenantID, project, run)
	if err != nil {
		return false, "", err
	}
	*total += len(nodes)
	if *total > 10000 {
		return false, unsupported, nil
	}
	for _, node := range nodes {
		if node.State == "skipped" {
			continue
		}
		if node.Spec.Type == "map" || node.Spec.Type == "external_workflow" {
			return false, unsupported, nil
		}
		if node.Spec.Type == "test" {
			if _, ok := node.Spec.Config["test_from"]; ok {
				return false, unsupported, nil
			}
			if _, ok := node.Spec.Config["candidate_composition"]; ok {
				return false, unsupported, nil
			}
		}
		if head, _ := node.Workspace["head_commit"].(string); head != "" && node.Spec.Type != "agent" && node.Spec.Type != "command" && node.Spec.Type != "test" {
			return false, unsupported, nil
		}
	}
	for _, producer := range nodes {
		if producer.Spec.Type != "agent" && producer.Spec.Type != "command" {
			continue
		}
		head, _ := producer.Workspace["head_commit"].(string)
		if head == "" {
			continue
		} // Non-code actions retain ordinary formal task evidence.
		covered := false
		for _, test := range nodes {
			source, _ := test.Spec.Config["candidate_from"].(string)
			if test.Spec.Type != "test" || source != producer.ID {
				continue
			}
			result, found, e := s.issueRepairNodeResultTx(ctx, auth, project, run, test.ID)
			if e != nil {
				return false, "", e
			}
			if !found || result.Result.ToolEvidence == nil {
				continue
			}
			profileID, profileSHA, bound, e := dag.ToolBinding(test.Spec)
			if e != nil || !bound {
				continue
			}
			proof := result.Result.ToolEvidence
			if toolcontract.ValidateEvidence(*proof) != nil || proof.Terminal != "completed" || proof.Profile.ProfileSHA256 != profileSHA {
				continue
			}
			profile, e := s.toolProfileTx(ctx, auth.tx, auth.currentPrincipal, project, profileID)
			if e != nil {
				var api *APIError
				if errors.As(e, &api) {
					continue
				}
				return false, "", e
			}
			if profile.WorkerID != test.Worker || !toolcontract.EqualProfile(profile.Profile, proof.Profile) {
				continue
			}
			ok, e := s.issueRepairCaseCandidateTx(ctx, auth, project, run, TestCaseEvidence{NodeID: test.ID, Attempt: test.Attempt, Fence: test.Fence, WorkerID: test.Worker, SourceCommit: head})
			if e != nil {
				return false, "", e
			}
			if ok {
				covered = true
				break
			}
		}
		if !covered {
			return false, "risk_task_evidence_stale", nil
		}
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT x.node_id,x.child_run_id,COALESCE(c.tenant_id,''),COALESCE(c.project_id,''),COALESCE(c.state,'') FROM dag_subruns x JOIN dag_nodes n ON n.run_id=x.parent_run_id AND n.id=x.node_id AND n.attempt=x.attempt JOIN dag_runs p ON p.id=x.parent_run_id LEFT JOIN dag_runs c ON c.id=x.child_run_id WHERE p.tenant_id=? AND p.project_id=? AND x.parent_run_id=? ORDER BY x.node_id`), auth.target.tenantID, project, run)
	if err != nil {
		return false, "", err
	}
	type childProof struct{ node, id, tenant, project, state string }
	children := []childProof{}
	for rows.Next() {
		var child childProof
		if err = rows.Scan(&child.node, &child.id, &child.tenant, &child.project, &child.state); err != nil {
			break
		}
		children = append(children, child)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return false, "", err
	}
	linked := map[string]bool{}
	for _, child := range children {
		node, ok := nodes[child.node]
		if !ok || node.Spec.Type != "subworkflow" || node.State != "succeeded" || node.Attempt < 1 || linked[child.node] || child.tenant != auth.target.tenantID || child.project != project || child.state != "succeeded" {
			return false, unsupported, nil
		}
		linked[child.node] = true
		ok, code, e := s.projectRiskCodeEvidenceTreeTx(ctx, auth, project, child.id, depth+1, visiting, total)
		if e != nil || !ok {
			return ok, code, e
		}
	}
	for _, node := range nodes {
		if node.Spec.Type == "subworkflow" && node.State != "skipped" && !linked[node.ID] {
			return false, unsupported, nil
		}
	}
	return true, "", nil
}

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
