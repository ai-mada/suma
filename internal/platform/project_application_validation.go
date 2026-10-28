package platform

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"sort"
	"strings"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

var applicationKey = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

func applicationWriteValid(q ApplicationWriteRequest) bool {
	if !handoffID(q.ProjectID, false) || !testPlanText(q.Name, 200, true) || !testPlanText(q.Description, 8000, false) || !testPlanText(q.Reason, 2000, true) || len(q.Key) > 64 || !applicationKey.MatchString(q.Key) || !handoffID(q.ParentApplicationID, true) || !handoffID(q.OwnerTeamID, false) || q.CodeScopes == nil || len(q.CodeScopes) < 1 || len(q.CodeScopes) > 8 {
		return false
	}
	if q.Kind != "application" && q.Kind != "microapp" && q.Kind != "service" && q.Kind != "library" || q.State != "active" && q.State != "archived" {
		return false
	}
	seen := map[string]bool{}
	for _, scope := range q.CodeScopes {
		if !handoffID(scope.ID, true) || !handoffID(scope.TargetID, false) || scope.ExpectedTargetVersion < 1 || !ri.RepositorySourceValidPath(scope.RelativeRoot, true) || !testPlanText(scope.Reason, 2000, scope.Shared) {
			return false
		}
		k := scope.TargetID + ":" + scope.RelativeRoot
		if seen[k] || scope.ID != "" && seen["id:"+scope.ID] {
			return false
		}
		seen[k] = true
		if scope.ID != "" {
			seen["id:"+scope.ID] = true
		}
	}
	return true
}
func applicationRootsOverlap(a, b string) bool {
	return a == "" || b == "" || a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// A team association records accountability only. It grants neither project
// access nor ownership of another team's credentials or execution resources.
func (s *Service) applicationTeamTx(ctx context.Context, auth *authorizedCommand, project, id string) (out ApplicationTeamOption, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return
	}
	return s.applicationTeamReadTx(ctx, auth.tx, auth.currentPrincipal, project, id)
}
func (s *Service) applicationTeamReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string) (out ApplicationTeamOption, err error) {
	var state string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,version,name,status FROM suma_teams WHERE tenant_id=? AND id=?`), p.TenantID, id).Scan(&out.ID, &out.Version, &out.Name, &state)
	if errors.Is(err, sql.ErrNoRows) || err == nil && state != "active" {
		return out, invalid(applicationTeam)
	}
	if err != nil {
		return
	}
	// Filter eligibility in the database before taking one candidate. A large
	// team cannot hide a real project member behind an arbitrary scan cutoff.
	var kind, resource, resourceProject string
	e := tx.QueryRowContext(ctx, s.q(`SELECT tm.resource_kind,tm.resource_id,tm.project_id FROM suma_team_members tm WHERE tm.tenant_id=? AND tm.team_id=? AND tm.status='active' AND ((tm.resource_kind='human' AND tm.project_id='' AND EXISTS(SELECT 1 FROM suma_users u JOIN suma_org_members om ON om.user_id=u.id WHERE u.id=tm.resource_id AND om.tenant_id=tm.tenant_id AND (om.role IN ('owner','admin') OR EXISTS(SELECT 1 FROM suma_project_members pm WHERE pm.project_id=? AND pm.user_id=u.id AND pm.role IN ('admin','reviewer','developer','viewer'))))) OR (tm.resource_kind IN ('agent','tool') AND tm.project_id=? AND EXISTS(SELECT 1 FROM suma_members m WHERE m.tenant_id=tm.tenant_id AND m.project_id=tm.project_id AND m.id=tm.resource_id AND m.kind=tm.resource_kind AND m.status='active'))) ORDER BY tm.id LIMIT 1`), p.TenantID, id, project, project).Scan(&kind, &resource, &resourceProject)
	if errors.Is(e, sql.ErrNoRows) {
		return out, invalid(applicationTeam)
	}
	if e != nil {
		return out, e
	}
	// Tenant/project governance is already locked by the authenticated command.
	// Reuse the ordinary current membership/resource proof for the selected row.
	if kind == "human" {
		_, e = s.currentProjectMembershipLockedTx(ctx, tx, p.TenantID, resource, project, false, false)
		return out, e
	}
	r, e := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_members WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, resource))
	if e != nil {
		return out, e
	}
	if resourceProject != project || r.Kind != kind || r.Status != "active" {
		return out, invalid(applicationTeam)
	}
	return out, nil
}
func (s *Service) applicationFieldsTx(ctx context.Context, auth *authorizedCommand, project string, old ProjectApplication, q ApplicationWriteRequest) ([]ApplicationCodeScope, error) {
	if _, err := s.applicationTeamTx(ctx, auth, project, q.OwnerTeamID); err != nil {
		return nil, err
	}
	if q.ParentApplicationID != "" {
		seen := map[string]bool{old.ID: true}
		id := q.ParentApplicationID
		for depth := 0; id != ""; depth++ {
			if depth >= 1000 || seen[id] {
				return nil, invalid(applicationScope)
			}
			seen[id] = true
			p, err := s.projectApplicationTx(ctx, auth, project, id)
			if err != nil {
				return nil, err
			}
			if p.State != "active" {
				return nil, invalid(applicationScope)
			}
			id = p.ParentApplicationID
		}
	}
	if q.State == "archived" && old.ID != "" {
		var n int
		if err := auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_project_applications WHERE tenant_id=? AND project_id=? AND parent_id=? AND state='active'`), auth.target.tenantID, project, old.ID).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, invalid(applicationScope)
		}
	}
	existing := map[string]bool{}
	for _, x := range old.CodeScopes {
		existing[x.ID] = true
	}
	out := []ApplicationCodeScope{}
	for _, x := range q.CodeScopes {
		if x.ID != "" && !existing[x.ID] {
			return nil, invalid(applicationScope)
		}
		t, err := s.repositoryTargetTx(ctx, auth.tx, auth.target.tenantID, project, x.TargetID)
		if err != nil {
			return nil, err
		}
		if t.ProjectID != project || t.Version != x.ExpectedTargetVersion || t.Status != "active" || !contentDigest.MatchString(t.ConfigSHA256) {
			return nil, conflict(applicationScope)
		}
		if _, err = s.repositoryProvider(auth.target.tenantID, project, t.ProviderID, t.TargetAlias); err != nil {
			return nil, err
		}
		id := x.ID
		if id == "" {
			id = newID()
		}
		scope := ApplicationCodeScope{ID: id, TargetID: t.ID, ProviderID: t.ProviderID, TargetAlias: t.TargetAlias, TargetRef: t.Ref, ConfigSHA256: t.ConfigSHA256, RelativeRoot: x.RelativeRoot, Shared: x.Shared, Reason: x.Reason}
		for _, prior := range out {
			if prior.TargetID == scope.TargetID && applicationRootsOverlap(prior.RelativeRoot, scope.RelativeRoot) {
				return nil, invalid(applicationScope)
			}
		}
		out = append(out, scope)
	}
	if q.State == "active" {
		rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT id,version,app_key,state,parent_id,owner_team_id,current_version_id,payload FROM suma_project_applications WHERE tenant_id=? AND project_id=? AND state='active' AND id<>? ORDER BY id`), auth.target.tenantID, project, old.ID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id, key, state, parent, team, current, raw string
			var version int
			if err = rows.Scan(&id, &version, &key, &state, &parent, &team, &current, &raw); err != nil {
				return nil, err
			}
			a, e := decodeProjectApplication(raw, project, id, version, key, state, parent, team, current)
			if e != nil {
				return nil, e
			}
			for _, other := range a.CodeScopes {
				for _, scope := range out {
					if scope.TargetID == other.TargetID && applicationRootsOverlap(scope.RelativeRoot, other.RelativeRoot) && (!scope.Shared || !other.Shared) {
						return nil, conflict(applicationScope)
					}
				}
			}
		}
		if err = rows.Err(); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
