package platform

import (
	"context"
	"encoding/json"
)

func projectRiskReadActor(actor Actor) bool {
	return actor.kind == actorBrowser || actor.kind == actorProjectAPI
}

func (s *Service) GetProjectRisk(ctx context.Context, actor Actor, project, id string) (ProjectRiskView, error) {
	if !projectRiskReadActor(actor) {
		return ProjectRiskView{}, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) {
		return ProjectRiskView{}, invalid(projectRiskInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return ProjectRiskView{}, err
	}
	defer close()
	risk, err := s.projectRiskTx(ctx, auth, project, id)
	if err != nil {
		return ProjectRiskView{}, err
	}
	return s.projectRiskViewTx(ctx, auth, risk)
}

func (s *Service) ListProjectRisks(ctx context.Context, actor Actor, project, state, owner string, limit int, before string) (page ProjectRiskPage, err error) {
	page.Items = []ProjectRisk{}
	if !projectRiskReadActor(actor) {
		return page, forbidden()
	}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(owner, true) || !handoffID(before, true) || limit < 1 || limit > 100 || state != "" && state != "open" && state != "mitigating" && state != "in_review" && state != "closed" {
		return page, invalid(projectRiskInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return page, err
	}
	defer close()
	filter := ` FROM suma_risks WHERE tenant_id=? AND project_id=?`
	args := []any{auth.target.tenantID, project}
	if state != "" {
		filter += ` AND state=?`
		args = append(args, state)
	}
	if owner != "" {
		filter += ` AND owner_id=?`
		args = append(args, owner)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+filter), args...).Scan(&page.Total); err != nil {
		return page, err
	}
	query := `SELECT payload` + filter
	if before != "" {
		stamp, e := s.planningCursorTx(ctx, auth.tx, auth.target.tenantID, project, "suma_risks", before)
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
	for rows.Next() {
		var raw string
		var risk ProjectRisk
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &risk); err != nil {
			break
		}
		if risk.TenantID != auth.target.tenantID || risk.ProjectID != project {
			err = conflict(projectRiskVersion)
			break
		}
		page.Items = append(page.Items, risk)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = page.Items[len(page.Items)-1].ID
	}
	if actor.kind == actorBrowser {
		_, e := s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, false)
		if e != nil && !projectRiskExpectedError(e) {
			return page, e
		}
		page.CanCreate = e == nil
	}
	return page, nil
}

// Counts cover the complete authorized project. Matrix cells describe only
// active ordinal assessments; missing ratings never become zero probability.
func (s *Service) GetProjectRiskSummary(ctx context.Context, actor Actor, project string) (out ProjectRiskSummary, err error) {
	out = ProjectRiskSummary{ProjectID: project, AsOf: now(), Matrix: []ProjectRiskMatrixCell{}, MatrixScope: "active"}
	if !projectRiskReadActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) {
		return out, invalid(projectRiskInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT state,COUNT(*) FROM suma_risks WHERE tenant_id=? AND project_id=? GROUP BY state`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var state string
		var count int
		if err = rows.Scan(&state, &count); err != nil {
			break
		}
		out.Total += count
		switch state {
		case "open":
			out.OpenCount = count
		case "mitigating":
			out.MitigatingCount = count
		case "in_review":
			out.InReviewCount = count
		case "closed":
			out.ClosedCount = count
		default:
			err = conflict(projectRiskVersion)
		}
		if err != nil {
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	out.ActiveCount = out.Total - out.ClosedCount
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_risks WHERE tenant_id=? AND project_id=? AND state<>'closed' AND (probability IS NULL OR impact IS NULL)`), auth.target.tenantID, project).Scan(&out.UnassessedCount); err != nil {
		return out, err
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_risks WHERE tenant_id=? AND project_id=? AND state<>'closed' AND review_date<>'' AND review_date<?`), auth.target.tenantID, project, out.AsOf[:10]).Scan(&out.OverdueCount); err != nil {
		return out, err
	}
	rows, err = auth.tx.QueryContext(ctx, s.q(`SELECT probability,impact,COUNT(*) FROM suma_risks WHERE tenant_id=? AND project_id=? AND state<>'closed' AND probability IS NOT NULL AND impact IS NOT NULL GROUP BY probability,impact`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	counts := map[[2]int]int{}
	for rows.Next() {
		var p, i, n int
		if err = rows.Scan(&p, &i, &n); err != nil {
			break
		}
		if p < 1 || p > 5 || i < 1 || i > 5 {
			err = conflict(projectRiskVersion)
			break
		}
		counts[[2]int{p, i}] = n
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	for p := 1; p <= 5; p++ {
		for i := 1; i <= 5; i++ {
			out.Matrix = append(out.Matrix, ProjectRiskMatrixCell{Probability: p, Impact: i, Count: counts[[2]int{p, i}]})
		}
	}
	return out, nil
}
