package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
)

func (s *Service) checkDispatchRecommendationBasisTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, record dispatchRecommendationRecord) error {
	a := record.Recommendation
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return err
	}
	if automationStamp(clock) >= a.ExpiresAt {
		return conflict(dispatchRecommendationSourceChanged)
	}
	current, err := s.dispatchRecommendationSourceTx(ctx, engine, auth, a.ProjectID, record.Request)
	if err != nil {
		return err
	}
	if current.SHA256 != a.Source.SHA256 {
		return conflict(dispatchRecommendationSourceChanged)
	}
	profile, err := s.toolProfileTx(ctx, auth.tx, auth.currentPrincipal, a.ProjectID, a.AnalysisToolProfileID)
	if err != nil {
		return err
	}
	if profile.Version != a.AnalysisToolProfileVersion || profile.Profile.ProfileSHA256 != a.AnalysisToolProfileSHA256 || !dispatchRecommendationProfile(profile) || profile.WorkerID != record.WorkerID || a.AnalysisModelExecution == nil {
		return conflict(dispatchRecommendationProfileMessage)
	}
	profile, err = s.toolProfileObservationTx(ctx, auth.tx, auth.currentPrincipal, a.ProjectID, profile)
	if err != nil {
		return err
	}
	if !profile.Available {
		return conflict(dispatchRecommendationProfileMessage)
	}
	if err = s.checkDispatchProfileRouteTx(ctx, auth.tx, auth.target.tenantID, a.ProjectID, profile, current.ExecutionRoute); err != nil {
		return err
	}
	b := a.AnalysisModelExecution
	selection := mc.Selection{DeploymentID: b.Deployment.ID, DeploymentRevision: b.Deployment.Revision, DeploymentSHA256: b.Deployment.SHA256, Parameters: b.Parameters, Budget: b.Budget}
	bound, err := s.buildModelExecutionTx(ctx, auth.tx, auth.target.tenantID, a.ProjectID, profile, &selection)
	if err != nil {
		return err
	}
	if stringify(bound) != stringify(b) {
		return conflict(dispatchRecommendationSourceChanged)
	}
	return nil
}

// Read authorization precedes freshness checks. A changed task version must
// never short-circuit authorization of a frozen knowledge body or model output
// derived from it. Old published revisions remain readable only through a
// currently accessible aggregate in this exact authorized project; retirement
// or a newer revision alone is freshness, not a new permission grant/revocation.
func (s *Service) dispatchRecommendationContextReadTx(ctx context.Context, auth *authorizedCommand, project string, fixed TaskContext) error {
	if err := assertProjectRead(auth, project); err != nil {
		return err
	}
	readRecord := func(kind, id string) error {
		_, err := s.getRecordTx(ctx, auth, project, kind, id)
		if err != nil {
			var api *APIError
			if errors.As(err, &api) {
				return forbidden()
			}
		}
		return err
	}
	if err := readRecord("tasks", fixed.Task.ID); err != nil {
		return err
	}
	if fixed.Requirement != nil {
		if err := readRecord("requirements", fixed.Requirement.ID); err != nil {
			return err
		}
	}
	if fixed.Design != nil {
		if err := readRecord("requirements", fixed.Design.RequirementID); err != nil {
			return err
		}
	}
	if len(fixed.Knowledge) > 16 {
		return forbidden()
	}
	for _, item := range fixed.Knowledge {
		if !handoffID(item.ID, false) || item.Revision < 1 || !contentDigest.MatchString(item.SHA256) {
			return forbidden()
		}
		if _, err := s.knowledgeDetailTx(ctx, auth, project, item.ID); err != nil {
			var api *APIError
			if errors.As(err, &api) {
				return forbidden()
			}
			return err
		}
		var raw string
		err := auth.tx.QueryRowContext(ctx, s.q(`SELECT snapshot FROM suma_project_knowledge_revisions WHERE tenant_id=? AND project_id=? AND knowledge_id=? AND revision=?`), auth.target.tenantID, project, item.ID, item.Revision).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return forbidden()
		}
		if err != nil {
			return err
		}
		var original ProjectKnowledgeRevision
		if json.Unmarshal([]byte(raw), &original) != nil || original.State != "published" || original.Revision != item.Revision || original.SHA256 != item.SHA256 || knowledgeRevisionHash(original) != item.SHA256 || original.Kind != item.Kind || original.Title != item.Title || original.Summary != item.Summary || original.Content != item.Content || stringify(knowledgeContextSources(original.Sources)) != stringify(item.Sources) {
			return forbidden()
		}
		for _, source := range original.Sources {
			if !knowledgeSourceKind(source.Kind) {
				return forbidden()
			}
			if err := readRecord(source.Kind, source.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) dispatchRecommendationProjectionTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, record dispatchRecommendationRecord) (out DispatchRecommendation, err error) {
	if err = s.dispatchRecommendationContextReadTx(ctx, auth, record.Recommendation.ProjectID, record.Recommendation.Source.Context); err != nil {
		return DispatchRecommendation{}, err
	}
	out = record.Recommendation
	out.AllowedActions = []string{}
	out.BasisCurrent = true
	if err = s.checkDispatchRecommendationBasisTx(ctx, engine, auth, record); err != nil {
		var api *APIError
		if !errors.As(err, &api) {
			return out, err
		}
		out.BasisCurrent = false
		if out.Reason == "" {
			out.Reason = dispatchRecommendationSourceChanged
		}
	}
	if auth.actor.kind == actorBrowser && out.State == "ready" && out.BasisCurrent && out.Proposal != nil {
		_, e := s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, out.ProjectID, true, false)
		if e != nil {
			var api *APIError
			if !errors.As(e, &api) {
				return out, e
			}
		} else {
			out.AllowedActions = append(out.AllowedActions, "adopt")
		}
	}
	return out, nil
}

func (s *Service) GetDispatchRecommendation(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (DispatchRecommendation, error) {
	if !testPlanActor(actor) {
		return DispatchRecommendation{}, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) {
		return DispatchRecommendation{}, invalid(dispatchRecommendationInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return DispatchRecommendation{}, err
	}
	defer close()
	record, err := s.dispatchRecommendationTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return DispatchRecommendation{}, err
	}
	return s.dispatchRecommendationProjectionTx(ctx, engine, auth, record)
}

func (s *Service) ListDispatchRecommendations(ctx context.Context, engine *dag.Engine, actor Actor, project, task string, limit int, before string) (page DispatchRecommendationPage, err error) {
	page.Items = []DispatchRecommendation{}
	if !testPlanActor(actor) {
		return page, forbidden()
	}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(task, true) || !handoffID(before, true) || limit < 1 || limit > 100 {
		return page, invalid(dispatchRecommendationInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return page, err
	}
	defer close()
	filter := ` FROM suma_dispatch_recommendations WHERE tenant_id=? AND project_id=?`
	args := []any{auth.target.tenantID, project}
	if task != "" {
		if _, err = s.getRecordTx(ctx, auth, project, "tasks", task); err != nil {
			return page, err
		}
		filter += ` AND task_id=?`
		args = append(args, task)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+filter), args...).Scan(&page.Total); err != nil {
		return page, err
	}
	query := `SELECT payload` + filter
	if before != "" {
		stamp, e := s.planningCursorTx(ctx, auth.tx, auth.target.tenantID, project, "suma_dispatch_recommendations", before)
		if e != nil {
			return page, e
		}
		query += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, stamp, stamp, before)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return page, err
	}
	records := []dispatchRecommendationRecord{}
	for rows.Next() {
		var raw string
		var record dispatchRecommendationRecord
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &record); err != nil {
			break
		}
		records = append(records, record)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return page, err
	}
	if len(records) > limit {
		records = records[:limit]
		page.NextCursor = records[len(records)-1].Recommendation.ID
	}
	for _, record := range records {
		item, e := s.dispatchRecommendationProjectionTx(ctx, engine, auth, record)
		if e != nil {
			return page, e
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}
