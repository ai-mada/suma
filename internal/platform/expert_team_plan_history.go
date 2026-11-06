package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/ai-mada/suma/internal/dag"
)

var expertTeamPlanFilterID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func (s *Service) ListExpertTeamPlans(ctx context.Context, engine *dag.Engine, actor Actor, project, team string, limit int, before string) (out ExpertTeamPlanPage, err error) {
	out.Items = []ExpertTeamPlan{}
	if limit == 0 {
		limit = 20
	}
	if engine == nil || !handoffID(project, false) || team != "" && !expertTeamPlanFilterID.MatchString(team) || limit < 1 || limit > 100 {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	collection := "expert_team_plans"
	from := ` FROM suma_expert_team_plans p WHERE p.tenant_id=? AND p.project_id=? AND EXISTS(SELECT 1 FROM suma_expert_team_plan_revisions r WHERE r.tenant_id=p.tenant_id AND r.project_id=p.project_id AND r.plan_id=p.id AND r.revision>0 AND LENGTH(r.graph_sha256)=64)`
	args := []any{auth.target.tenantID, project}
	if team != "" {
		collection += ":team:" + digest(stringify([2]string{auth.currentPrincipal.ID, team}))
		// IDs are case-sensitive even under a case-insensitive database collation.
		switch s.Dialect {
		case "oceanbase":
			from += ` AND CAST(p.team_id AS BINARY)=CAST(? AS BINARY)`
		case "postgres":
			from += ` AND p.team_id COLLATE "C"=?`
		default:
			from += ` AND p.team_id=?`
		}
		args = append(args, team)
	}
	position, err := automationDecodeCursor(auth.currentPrincipal, project, collection, before)
	if err != nil {
		return out, err
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+from), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT p.id`+from+` AND p.id>? ORDER BY p.id LIMIT ?`), append(args, position, limit+1)...)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(ids) > limit {
		ids = ids[:limit]
		out.NextCursor = automationEncodeCursor(auth.currentPrincipal, project, collection, ids[len(ids)-1])
	}
	for _, id := range ids {
		p, e := s.expertTeamPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		if team != "" && p.TeamID != team {
			return out, expertError("expert_team_changed", expertPlanChanged)
		}
		p, e = s.expertTeamPlanProjectionTx(ctx, engine, auth, p)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	return out, nil
}

func (s *Service) ExpertTeamPlanRevisions(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, limit int, before string) (out ExpertTeamPlanPage, err error) {
	out.Items = []ExpertTeamPlan{}
	if limit == 0 {
		limit = 20
	}
	if engine == nil || !handoffID(project, false) || !handoffID(id, false) || limit < 1 || limit > 100 {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	if _, err = s.expertTeamPlanTx(ctx, auth.tx, auth.target.tenantID, project, id); err != nil {
		return out, err
	}
	position, err := automationDecodeCursor(auth.currentPrincipal, project, "expert_team_plan_revisions:"+id, before)
	if err != nil {
		return out, err
	}
	last := 0
	if position != "" {
		last, err = strconv.Atoi(position)
		if err != nil || last < 1 || position != fmt.Sprintf("%010d", last) {
			return out, expertError("expert_team_invalid", expertInvalid)
		}
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_expert_team_plan_revisions WHERE tenant_id=? AND project_id=? AND plan_id=?`), auth.target.tenantID, project, id).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT payload,created_at FROM suma_expert_team_plan_revisions WHERE tenant_id=? AND project_id=? AND plan_id=? AND revision>? ORDER BY revision LIMIT ?`), auth.target.tenantID, project, id, last, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var raw, createdAt string
		var p ExpertTeamPlan
		if err = rows.Scan(&raw, &createdAt); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &p); err != nil {
			break
		}
		clearExpertTeamExecution(&p)
		p.CanStart = false
		p.AllowedActions = []string{}
		p.CreatedAt = createdAt
		out.Items = append(out.Items, p)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = automationEncodeCursor(auth.currentPrincipal, project, "expert_team_plan_revisions:"+id, fmt.Sprintf("%010d", out.Items[len(out.Items)-1].Revision))
	}
	return out, nil
}

func (s *Service) GetExpertTeamPlanRevision(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, revision int) (out ExpertTeamPlan, err error) {
	if engine == nil || !handoffID(project, false) || !handoffID(id, false) || revision < 1 {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	if _, err = s.expertTeamPlanTx(ctx, auth.tx, auth.target.tenantID, project, id); err != nil {
		return out, err
	}
	var raw, createdAt string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT payload,created_at FROM suma_expert_team_plan_revisions WHERE tenant_id=? AND project_id=? AND plan_id=? AND revision=?`), auth.target.tenantID, project, id, revision).Scan(&raw, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(raw), &out)
	clearExpertTeamExecution(&out)
	out.CanStart = false
	out.AllowedActions = []string{}
	out.CreatedAt = createdAt
	return out, err
}
