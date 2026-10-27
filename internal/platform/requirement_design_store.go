package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// The aggregate owns CAS pointers; immutable revisions and adoption facts never
// change. Protected task links cannot be supplied through editable task Data.
func (s *Service) InitializeRequirementDesign(ctx context.Context) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS suma_requirement_designs(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,requirement_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,latest_revision INTEGER NOT NULL,published_revision INTEGER NOT NULL,state VARCHAR(20) NOT NULL,last_adoption_id VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,requirement_id))`,
		`CREATE TABLE IF NOT EXISTS suma_requirement_design_revisions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,requirement_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,requirement_id,revision))`,
		`CREATE TABLE IF NOT EXISTS suma_requirement_design_task_links(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,requirement_id VARCHAR(64) NOT NULL,task_key VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,active INTEGER NOT NULL,revision INTEGER NOT NULL,requirement_version INTEGER NOT NULL,task_basis VARCHAR(64) NOT NULL,adoption_id VARCHAR(64) NOT NULL,PRIMARY KEY(tenant_id,project_id,requirement_id,task_key),UNIQUE(tenant_id,project_id,task_id))`,
		`CREATE TABLE IF NOT EXISTS suma_requirement_design_adoptions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,requirement_id VARCHAR(64) NOT NULL,id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,requirement_id,id),UNIQUE(tenant_id,project_id,requirement_id,version))`,
	} {
		if _, e := s.DB.ExecContext(ctx, s.ddl(ddl)); e != nil {
			return e
		}
	}
	return nil
}

type designAggregate struct {
	Version, Latest, Published int
	State, LastAdoption        string
}
type designTaskLink struct {
	Key, TaskID                  string
	Active                       int
	Revision, RequirementVersion int
	TaskBasis, AdoptionID        string
}

func (s *Service) designAggregateTx(ctx context.Context, auth *authorizedCommand, project, requirement string) (out designAggregate, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return
	}
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version,latest_revision,published_revision,state,last_adoption_id FROM suma_requirement_designs WHERE tenant_id=? AND project_id=? AND requirement_id=?`), auth.target.tenantID, project, requirement).Scan(&out.Version, &out.Latest, &out.Published, &out.State, &out.LastAdoption)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}
func (s *Service) designRevisionTx(ctx context.Context, auth *authorizedCommand, project, requirement string, revision int) (out RequirementDesignRevision, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return
	}
	var raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT snapshot FROM suma_requirement_design_revisions WHERE tenant_id=? AND project_id=? AND requirement_id=? AND revision=?`), auth.target.tenantID, project, requirement, revision).Scan(&raw)
	if err != nil {
		return
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return
	}
	if out.Revision != revision || out.Requirement.ID != requirement || out.SHA256 != designRevisionHash(out) {
		err = fmt.Errorf("requirement design revision is inconsistent")
	}
	return
}
func (s *Service) designAdoptionTx(ctx context.Context, auth *authorizedCommand, project, requirement, id string) (out RequirementDesignAdoption, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return
	}
	var raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT snapshot FROM suma_requirement_design_adoptions WHERE tenant_id=? AND project_id=? AND requirement_id=? AND id=?`), auth.target.tenantID, project, requirement, id).Scan(&raw)
	if err != nil {
		return
	}
	err = json.Unmarshal([]byte(raw), &out)
	if err == nil && (out.ID != id || out.ProjectID != project || out.RequirementID != requirement) {
		err = fmt.Errorf("requirement design adoption is inconsistent")
	}
	return
}
func (s *Service) designLinksTx(ctx context.Context, auth *authorizedCommand, project, requirement string) ([]designTaskLink, error) {
	if e := assertProjectRead(auth, project); e != nil {
		return nil, e
	}
	rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT task_key,task_id,active,revision,requirement_version,task_basis,adoption_id FROM suma_requirement_design_task_links WHERE tenant_id=? AND project_id=? AND requirement_id=? ORDER BY task_key`), auth.target.tenantID, project, requirement)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []designTaskLink{}
	for rows.Next() {
		var l designTaskLink
		if e = rows.Scan(&l.Key, &l.TaskID, &l.Active, &l.Revision, &l.RequirementVersion, &l.TaskBasis, &l.AdoptionID); e != nil {
			return nil, e
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
func (s *Service) designViewTx(ctx context.Context, auth *authorizedCommand, project, requirement string) (out RequirementDesignView, err error) {
	r, e := s.getRecordTx(ctx, auth, project, "requirements", requirement)
	if e != nil {
		return out, e
	}
	a, e := s.designAggregateTx(ctx, auth, project, requirement)
	if e != nil {
		return out, e
	}
	out = RequirementDesignView{ProjectID: project, RequirementID: requirement, Version: a.Version, Requirement: designSnapshot(r), LatestRevision: a.Latest, PublishedRevision: a.Published, Eligibility: "none", Validation: []RequirementDesignDiagnostic{}}
	if a.Latest > 0 {
		rev, e := s.designRevisionTx(ctx, auth, project, requirement, a.Latest)
		if e != nil {
			return out, e
		}
		out.Latest = &rev
		out.HasDraft = rev.State == "draft"
		out.Validation = designDiagnostics(rev)
	}
	if a.State == "retired" {
		out.Eligibility = "retired"
	}
	if a.Published > 0 {
		rev, e := s.designRevisionTx(ctx, auth, project, requirement, a.Published)
		if e != nil {
			return out, e
		}
		if rev.State != "published" {
			return out, fmt.Errorf("requirement design publication is inconsistent")
		}
		out.Published = &rev
		out.Eligibility = "current"
		if !designSourceCurrent(r, rev.Requirement) {
			out.Eligibility = "requirement_changed"
		}
	}
	if a.LastAdoption != "" {
		ad, e := s.designAdoptionTx(ctx, auth, project, requirement, a.LastAdoption)
		if e != nil {
			return out, e
		}
		out.LastAdoption = &ad
	}
	return out, nil
}
func (s *Service) GetRequirementDesign(ctx context.Context, actor Actor, project, requirement string) (out RequirementDesignView, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: requirement}, purposeProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.designViewTx(auth.ctx, auth, project, requirement)
		return e
	})
	return
}

type designHistoryCursor struct {
	Tenant, Project, Requirement, Kind string
	Position                           int
}

func designHistoryPosition(auth *authorizedCommand, project, requirement, kind, cursor string, limit int) (int, int, error) {
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return 0, 0, invalid("设计历史分页范围应为1至100")
	}
	if cursor == "" {
		return limit, 0, nil
	}
	var c designHistoryCursor
	raw, e := base64.RawURLEncoding.DecodeString(cursor)
	if len(cursor) > 4096 || e != nil || json.Unmarshal(raw, &c) != nil || c.Tenant != auth.target.tenantID || c.Project != project || c.Requirement != requirement || c.Kind != kind || c.Position < 1 {
		return 0, 0, invalid("设计历史游标无效或不属于当前需求")
	}
	return limit, c.Position, nil
}
func designHistoryNext(auth *authorizedCommand, project, requirement, kind string, position int) string {
	raw, _ := json.Marshal(designHistoryCursor{auth.target.tenantID, project, requirement, kind, position})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func (s *Service) designHistoryTx(ctx context.Context, auth *authorizedCommand, project, requirement, kind string, limit int, cursor string) ([]string, int, string, error) {
	if _, e := s.getRecordTx(ctx, auth, project, "requirements", requirement); e != nil {
		return nil, 0, "", e
	}
	limit, pos, e := designHistoryPosition(auth, project, requirement, kind, cursor, limit)
	if e != nil {
		return nil, 0, "", e
	}
	table, column := "suma_requirement_design_revisions", "revision"
	if kind == "adoptions" {
		table, column = "suma_requirement_design_adoptions", "version"
	}
	args := []any{auth.target.tenantID, project, requirement}
	where := ` WHERE tenant_id=? AND project_id=? AND requirement_id=?`
	var total int
	if e = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM `+table+where), args...).Scan(&total); e != nil {
		return nil, 0, "", e
	}
	if pos > 0 {
		where += ` AND ` + column + `<?`
		args = append(args, pos)
	}
	args = append(args, limit+1)
	rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT `+column+`,snapshot FROM `+table+where+` ORDER BY `+column+` DESC LIMIT ?`), args...)
	if e != nil {
		return nil, 0, "", e
	}
	defer rows.Close()
	out := []string{}
	positions := []int{}
	for rows.Next() {
		var n int
		var raw string
		if e = rows.Scan(&n, &raw); e != nil {
			return nil, 0, "", e
		}
		out = append(out, raw)
		positions = append(positions, n)
	}
	if e = rows.Err(); e != nil {
		return nil, 0, "", e
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = designHistoryNext(auth, project, requirement, kind, positions[limit-1])
	}
	return out, total, next, nil
}
func (s *Service) RequirementDesignRevisions(ctx context.Context, actor Actor, project, requirement string, limit int, cursor string) (out RequirementDesignRevisionList, err error) {
	out.Items = []RequirementDesignRevision{}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: requirement}, purposeProjectRead, func(auth *authorizedCommand) error {
		raw, total, next, e := s.designHistoryTx(auth.ctx, auth, project, requirement, "revisions", limit, cursor)
		if e != nil {
			return e
		}
		out.Total, out.NextCursor = total, next
		for _, item := range raw {
			var rev RequirementDesignRevision
			if e = json.Unmarshal([]byte(item), &rev); e != nil {
				return e
			}
			if rev.Requirement.ID != requirement || rev.SHA256 != designRevisionHash(rev) {
				return fmt.Errorf("requirement design history is inconsistent")
			}
			out.Items = append(out.Items, rev)
		}
		return nil
	})
	return
}
func (s *Service) RequirementDesignAdoptions(ctx context.Context, actor Actor, project, requirement string, limit int, cursor string) (out RequirementDesignAdoptionList, err error) {
	out.Items = []RequirementDesignAdoption{}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: requirement}, purposeProjectRead, func(auth *authorizedCommand) error {
		raw, total, next, e := s.designHistoryTx(auth.ctx, auth, project, requirement, "adoptions", limit, cursor)
		if e != nil {
			return e
		}
		out.Total, out.NextCursor = total, next
		for _, item := range raw {
			var ad RequirementDesignAdoption
			if e = json.Unmarshal([]byte(item), &ad); e != nil {
				return e
			}
			if ad.ProjectID != project || ad.RequirementID != requirement {
				return fmt.Errorf("requirement design adoption history is inconsistent")
			}
			out.Items = append(out.Items, ad)
		}
		return nil
	})
	return
}
