package platform

import (
	"context"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

// ListOwnedPoolGrants exposes the project-approved shared metadata, never
// project records or logs. Per-row navigation is decided from actual access.
func (s *Service) ListOwnedPoolGrants(ctx context.Context, actor Actor, pool string, limit int, cursor string) (ExecutionPoolGrantPage, error) {
	return s.listPoolGrants(ctx, actor, pool, "", limit, cursor, true)
}

func (s *Service) ListProjectPoolGrants(ctx context.Context, actor Actor, project string, limit int, cursor string) (ExecutionPoolGrantPage, error) {
	return s.listPoolGrants(ctx, actor, "", project, limit, cursor, false)
}

// ListProjectPoolRunners returns only identities delegated by the project's
// exact current route/grant. It exposes no owner registry, token, config or
// another project's inventory. Active means registered, never online.
func (s *Service) ListProjectPoolRunners(ctx context.Context, actor Actor, project string) (ProjectPoolRunnerPage, error) {
	var result0 ProjectPoolRunnerPage
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeBrowserProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readListProjectPoolRunnersTx(bounded, auth, project)
		return e
	})
	return result0, err
}
func (s *Service) readListProjectPoolRunnersTx(ctx context.Context, auth *authorizedCommand, project string) (ProjectPoolRunnerPage, error) {
	if auth == nil {
		return ProjectPoolRunnerPage{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ProjectPoolRunnerPage{}, e
	}
	if auth.target.projectID != project {
		return ProjectPoolRunnerPage{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error

	out := ProjectPoolRunnerPage{Items: []ProjectPoolRunner{}}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out.AsOf = automationStamp(clock)
	rows, err := tx.QueryContext(ctx, s.q(`SELECT r.id,r.worker_id,r.pool_id,r.name,r.status,c.expires_at,r.last_seen_at FROM dag_project_execution_routes d JOIN suma_projects x ON x.tenant_id=d.tenant_id AND x.id=d.project_id AND x.status='active' JOIN suma_execution_pool_grants g ON g.id=d.grant_id AND g.version=d.grant_version AND g.pool_id=d.pool_id AND g.tenant_id=d.tenant_id AND g.project_id=d.project_id JOIN suma_execution_pools p ON p.id=d.pool_id JOIN suma_execution_pool_runners r ON r.pool_id=d.pool_id JOIN suma_execution_pool_credentials c ON c.runner_id=r.id AND c.version=r.credential_version WHERE d.tenant_id=? AND d.project_id=? AND g.status='approved' AND p.status='active' AND r.status='active' AND c.status='active' AND c.expires_at>? ORDER BY r.id LIMIT 1001`), p.TenantID, project, out.AsOf)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ProjectPoolRunner
		if err = rows.Scan(&item.ID, &item.WorkerID, &item.PoolID, &item.Name, &item.Status, &item.CredentialExpiresAt, &item.LastSeenAt); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > 1000 {
		return out, conflict("资源池执行身份超过可配置范围，请联系管理员拆分资源池")
	}
	return out, nil

}

func (s *Service) listPoolGrants(ctx context.Context, actor Actor, pool, project string, limit int, cursor string, owner bool) (ExecutionPoolGrantPage, error) {
	targetProject, purpose := project, purposeBrowserProjectRead
	if owner {
		targetProject = organizationScope
		purpose = purposeOrganizationRead
	}
	var result0 ExecutionPoolGrantPage
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: targetProject}, purpose, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readListPoolGrantsTx(bounded, auth, pool, project, limit, cursor, owner)
		return e
	})
	return result0, err
}
func (s *Service) readListPoolGrantsTx(ctx context.Context, auth *authorizedCommand, pool, project string, limit int, cursor string, owner bool) (ExecutionPoolGrantPage, error) {
	expectedProject := project
	if owner {
		expectedProject = organizationScope
	}
	if auth == nil {
		return ExecutionPoolGrantPage{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ExecutionPoolGrantPage{}, e
	}
	if auth.target.projectID != expectedProject {
		return ExecutionPoolGrantPage{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error
	if owner && !orgAdmin(p) {
		return ExecutionPoolGrantPage{}, forbidden()
	}

	out := ExecutionPoolGrantPage{Items: []ExecutionPoolGrant{}, AllowedActions: []string{}}
	if limit < 1 || limit > 100 {
		return out, invalid("limit必须为1–100")
	}
	collection := "project_pool_grants"
	cursorProject := project
	if owner {
		collection = "owned_pool_grants:" + pool
		cursorProject = organizationScope
	}
	last, err := automationDecodeCursor(p, cursorProject, collection, cursor)
	if err != nil {
		return out, err
	}
	where := `g.tenant_id=? AND g.project_id=?`
	args := []any{p.TenantID, project}
	if owner {
		meta, e := s.executionPoolMetadataTx(ctx, tx, pool)
		if e != nil {
			return out, e
		}
		if meta.OwnerTenantID != p.TenantID {
			return out, missing()
		}
		where = `g.owner_tenant_id=? AND g.pool_id=?`
		args = []any{p.TenantID, pool}
	} else {
		allowed, e := s.currentPoolProjectAccessTx(ctx, tx, p.TenantID, project, p.ID, true)
		if e != nil {
			return out, e
		}
		if allowed {
			out.AllowedActions = []string{"request"}
		}
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_execution_pool_grants g WHERE `+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	queryArgs := append(append([]any{}, args...), last, limit+1)
	rows, err := tx.QueryContext(ctx, s.q(`SELECT `+poolGrantColumns+` FROM suma_execution_pool_grants g JOIN suma_execution_pools p ON p.id=g.pool_id WHERE `+where+` AND g.id>? ORDER BY g.id LIMIT ?`), queryArgs...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		g, e := scanPoolGrant(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = automationEncodeCursor(p, cursorProject, collection, out.Items[len(out.Items)-1].ID)
	}
	for i, g := range out.Items {
		out.Items[i], err = s.poolGrantViewTx(ctx, tx, p, g, owner)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// Registration state and last contact are observations, not a declaration of
// available hardware or tool health. Secret hashes never enter these DTOs.
func (s *Service) ListExecutionPoolRunners(ctx context.Context, actor Actor, pool string, limit int, cursor string) (ExecutionPoolRunnerPage, error) {
	var result0 ExecutionPoolRunnerPage
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readListExecutionPoolRunnersTx(bounded, auth, pool, limit, cursor)
		return e
	})
	return result0, err
}
func (s *Service) readListExecutionPoolRunnersTx(ctx context.Context, auth *authorizedCommand, pool string, limit int, cursor string) (ExecutionPoolRunnerPage, error) {
	if auth == nil {
		return ExecutionPoolRunnerPage{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ExecutionPoolRunnerPage{}, e
	}
	if auth.target.projectID != organizationScope {
		return ExecutionPoolRunnerPage{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error
	if !orgAdmin(p) {
		return ExecutionPoolRunnerPage{}, forbidden()
	}

	out := ExecutionPoolRunnerPage{Items: []ExecutionPoolRunner{}, AllowedActions: []string{}}
	if limit < 1 || limit > 100 {
		return out, invalid("limit必须为1–100")
	}
	collection := "pool_runners:" + pool
	last, err := automationDecodeCursor(p, organizationScope, collection, cursor)
	if err != nil {
		return out, err
	}
	meta, err := s.executionPoolMetadataTx(ctx, tx, pool)
	if err != nil {
		return out, err
	}
	if meta.OwnerTenantID != p.TenantID {
		return out, missing()
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_execution_pool_runners WHERE pool_id=?`), pool).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT `+poolRunnerColumns+` FROM suma_execution_pool_runners r JOIN suma_execution_pool_credentials c ON c.runner_id=r.id WHERE r.pool_id=? AND r.id>? ORDER BY r.id LIMIT ?`), pool, last, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		r, e := scanPoolRunner(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = automationEncodeCursor(p, organizationScope, collection, out.Items[len(out.Items)-1].ID)
	}
	workers := make(map[string]string, len(out.Items))
	for _, runner := range out.Items {
		workers[runner.ID] = runner.WorkerID
	}
	occupancy, err := dag.ReadPoolRunnerSlotCountsTx(ctx, tx, s.Dialect, pool, workers)
	if err != nil {
		return ExecutionPoolRunnerPage{}, err
	}
	observedAt := auth.checkedAt.UTC().Format(time.RFC3339)
	for i := range out.Items {
		runner := &out.Items[i]
		counts := occupancy[runner.ID]
		runner.SlotOccupancy = &ExecutionPoolRunnerSlotOccupancy{
			ObservedAt: observedAt, Held: counts.Held, Running: counts.Running,
			Reconciling: counts.Reconciling, Unknown: counts.Unknown,
		}
	}
	out.AllowedActions = []string{"register"}
	return out, nil
}
