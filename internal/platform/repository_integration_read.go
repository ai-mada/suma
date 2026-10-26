package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) repositoryPlanProjectionTx(ctx context.Context, auth *authorizedCommand, out RepositoryIntegrationPlan) (RepositoryIntegrationPlan, error) {
	out.CanApprove, out.CanExecute, out.CanReconcile = false, false, false
	out.Blockers = []string{}
	out.Target.CanManage = false
	current, err := s.repositoryPlanTx(ctx, auth.tx, auth.target.tenantID, out.ProjectID, out.ID)
	if err != nil {
		return out, err
	}
	if current.Version != out.Version {
		out.Blockers = append(out.Blockers, repositoryChanged)
		return out, nil
	}
	_, approve, manage, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	if out.OperationID != "" && (out.State == "preparing" || out.State == "applying" || out.State == "reconciling") {
		op, e := s.repositoryOperationTx(ctx, auth.tx, auth.target.tenantID, out.ProjectID, out.OperationID)
		if e != nil {
			return out, e
		}
		out.CanReconcile = approve && op.Assignment != nil
	}
	if out.State == "ready" || out.State == "approved" {
		if e := s.checkRepositoryPlanBasisTx(ctx, auth, out, ""); e != nil {
			if !repositoryKnownError(e) {
				return out, e
			}
			out.Blockers = append(out.Blockers, e.Error())
			return out, nil
		}
		out.CanApprove = manage && out.State == "ready" && out.Preview != nil && out.PreparedSHA256 != ""
		if out.State == "approved" && out.Approval != nil && out.Approval.Decision == "approved" {
			_, e := s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, out.Approval.ActorID, out.ProjectID, false, true)
			if e == nil {
				out.CanExecute = approve
			} else if repositoryKnownError(e) {
				out.Blockers = append(out.Blockers, repositoryApproval)
			} else {
				return out, e
			}
		}
	}
	return out, nil
}
func (s *Service) GetRepositoryIntegrationPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (out RepositoryIntegrationPlan, err error) {
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, repositoryError(400, "invalid", repositoryInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	out, err = s.repositoryPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	return s.repositoryPlanProjectionTx(ctx, auth, out)
}

type repositoryCursor struct{ Tenant, Project, Task, Created, ID string }

func (s *Service) ListRepositoryIntegrationPlans(ctx context.Context, engine *dag.Engine, actor Actor, project, taskID string, limit int, before string) (out RepositoryIntegrationPlanPage, err error) {
	out.Items = []RepositoryIntegrationPlan{}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(taskID, false) || limit < 1 || limit > 100 || len(before) > 4096 {
		return out, repositoryError(400, "invalid", repositoryInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, taskID, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	var cursor repositoryCursor
	if before != "" {
		raw, e := base64.RawURLEncoding.Strict().DecodeString(before)
		if e != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Tenant != auth.target.tenantID || cursor.Project != project || cursor.Task != taskID || !handoffID(cursor.ID, false) || !validReleaseTime(cursor.Created) || base64.RawURLEncoding.EncodeToString([]byte(stringify(cursor))) != before {
			return out, repositoryError(400, "invalid", repositoryInvalid)
		}
	}
	where := ` WHERE tenant_id=? AND project_id=? AND task_id=?`
	args := []any{auth.target.tenantID, project, taskID}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_repository_plans`+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if before != "" {
		where += ` AND (created_at<? OR(created_at=? AND id<?))`
		args = append(args, cursor.Created, cursor.Created, cursor.ID)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT data FROM suma_repository_plans`+where+` ORDER BY created_at DESC,id DESC LIMIT ?`), append(args, limit+1)...)
	if err != nil {
		return out, err
	}
	items := []RepositoryIntegrationPlan{}
	for rows.Next() {
		var raw string
		var plan RepositoryIntegrationPlan
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &plan); err != nil {
			break
		}
		items = append(items, plan)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(repositoryCursor{auth.target.tenantID, project, taskID, last.CreatedAt, last.ID})))
	}
	for _, plan := range items {
		projected, e := s.repositoryPlanProjectionTx(ctx, auth, plan)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, projected)
	}
	return out, nil
}
