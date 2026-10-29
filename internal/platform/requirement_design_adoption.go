package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) designSourceTx(ctx context.Context, auth *authorizedCommand, project, requirement string, lock bool) (Record, error) {
	if e := assertProjectRead(auth, project); e != nil {
		return Record{}, e
	}
	r, e := scanRecord(auth.tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT `+recordColumns+` FROM suma_requirements WHERE tenant_id=? AND project_id=? AND id=?`, lock)), auth.target.tenantID, project, requirement))
	if errors.Is(e, sql.ErrNoRows) {
		return r, missing()
	}
	return r, e
}

// One bounded batch reads complete formal task records. Locking reads are only
// used by an effect-bearing command after the shared project guard; preview is
// genuinely read-only and its hash must later survive a fresh locked read.
func (s *Service) designTasksTx(ctx context.Context, auth *authorizedCommand, project string, links []designTaskLink, lock bool) (map[string]Record, error) {
	if e := assertProjectRead(auth, project); e != nil {
		return nil, e
	}
	out := map[string]Record{}
	ids := []string{}
	for _, l := range links {
		ids = append(ids, l.TaskID)
	}
	sort.Strings(ids)
	for start := 0; start < len(ids); start += 200 {
		end := start + 200
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		args := []any{auth.target.tenantID, project}
		for _, id := range chunk {
			args = append(args, id)
		}
		query := `SELECT ` + recordColumns + ` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id IN (` + strings.TrimRight(strings.Repeat("?,", len(chunk)), ",") + `) ORDER BY id`
		rows, e := auth.tx.QueryContext(ctx, s.q(s.executionReadSQL(query, lock)), args...)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			r, e := scanRecord(rows)
			if e != nil {
				rows.Close()
				return nil, e
			}
			out[r.ID] = r
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	if len(out) != len(links) {
		return nil, conflict("已采用任务记录缺失，需要核对绑定")
	}
	return out, nil
}
func designProposedRecord(t RequirementDesignTask, current Record, requirement string, ids map[string]string) Record {
	current.Title = t.Title
	current.Name = t.Title
	current.Description = t.Description
	current.Priority = t.Priority
	current.OwnerID = t.OwnerID
	current.RequirementID = requirement
	current.AcceptanceCriteria = append([]string{}, t.AcceptanceCriteria...)
	current.DependsOn = []string{}
	for _, key := range t.DependsOn {
		current.DependsOn = append(current.DependsOn, ids[key])
	}
	return current
}
func designTaskEqual(a, b Record) bool {
	return taskBasis(a) == taskBasis(b) && a.Priority == b.Priority
}

type designPlanState struct {
	View   RequirementDesignView
	Source Record
	Links  []designTaskLink
	Tasks  map[string]Record
}

func (s *Service) designPreviewTx(ctx context.Context, auth *authorizedCommand, project, requirement string, request RequirementDesignPreviewRequest, lock bool) (out RequirementDesignPreview, state designPlanState, err error) {
	source, e := s.designSourceTx(ctx, auth, project, requirement, lock)
	if e != nil {
		return out, state, e
	}
	view, e := s.designViewTx(ctx, auth, project, requirement)
	if e != nil {
		return out, state, e
	}
	if view.Version != request.ExpectedVersion || view.Published == nil || view.PublishedRevision != request.Revision || view.Published.SHA256 != request.SHA256 || !designSourceCurrent(source, view.Published.Requirement) {
		return out, state, conflict("需求设计发布或来源已变化，请重新预览")
	}
	if e = validateDesignDraft(view.Published.Content, view.Published.Tasks); e != nil {
		return out, state, e
	}
	if len(designDiagnostics(*view.Published)) > 0 {
		return out, state, conflict("已发布设计校验不完整")
	}
	links, e := s.designLinksTx(ctx, auth, project, requirement)
	if e != nil {
		return out, state, e
	}
	tasks, e := s.designTasksTx(ctx, auth, project, links, lock)
	if e != nil {
		return out, state, e
	}
	state = designPlanState{View: view, Source: source, Links: links, Tasks: tasks}
	byKey := map[string]designTaskLink{}
	ids := map[string]string{}
	for _, l := range links {
		byKey[l.Key] = l
		ids[l.Key] = l.TaskID
	}
	proposals := map[string]RequirementDesignTask{}
	for _, t := range view.Published.Tasks {
		proposals[t.Key] = t
		if ids[t.Key] == "" {
			ids[t.Key] = "pending:" + t.Key
		}
	}
	keys := []string{}
	for key := range proposals {
		keys = append(keys, key)
	}
	for _, l := range links {
		if _, ok := proposals[l.Key]; !ok && l.Active == 1 {
			keys = append(keys, l.Key)
		}
	}
	sort.Strings(keys)
	out = RequirementDesignPreview{ProjectID: project, RequirementID: requirement, Version: view.Version, Revision: view.PublishedRevision, SHA256: view.Published.SHA256, RequirementVersion: source.Version, Changes: []RequirementDesignChange{}}
	for _, key := range keys {
		p, has := proposals[key]
		link, exists := byKey[key]
		c := RequirementDesignChange{Key: key}
		if has {
			copy := p
			c.Proposed = &copy
		}
		if exists {
			r := tasks[link.TaskID]
			c.Current = &r
			c.TaskID = r.ID
			c.ExpectedTaskVersion = r.Version
		}
		switch {
		case !has:
			c.Action = "exclude"
			out.ExcludeCount++
		case !exists:
			c.Action = "create"
			out.CreateCount++
		default:
			r := tasks[link.TaskID]
			desired := designProposedRecord(p, r, requirement, ids)
			same := link.Active == 1 && link.Revision == view.PublishedRevision && link.RequirementVersion == source.Version && link.TaskBasis == taskBasis(r) && r.Status != "archived" && r.Status != "deleted" && designTaskEqual(r, desired)
			if same {
				c.Action = "keep"
				out.KeepCount++
			} else {
				c.Action = "update"
				out.UpdateCount++
			}
		}
		out.Changes = append(out.Changes, c)
	}
	// Bind even inactive links and every complete current record, not just the
	// user-visible changed subset. This makes independent edits explicit conflicts.
	records := []Record{}
	for _, l := range links {
		records = append(records, tasks[l.TaskID])
	}
	out.PlanSHA256 = digest(stringify(struct {
		Version   int
		Source    Record
		Published RequirementDesignRevision
		Links     []designTaskLink
		Tasks     []Record
	}{view.Version, source, *view.Published, links, records}))
	out.HasChanges = out.CreateCount+out.UpdateCount+out.ExcludeCount > 0
	return out, state, nil
}
func validateDesignPreviewRequest(project string, r RequirementDesignPreviewRequest) error {
	if r.ProjectID != project || !designVersion(r.ExpectedVersion) || r.ExpectedVersion < 1 || !designVersion(r.Revision) || r.Revision < 1 || len(r.SHA256) != 64 {
		return invalid("设计预览需要当前项目、版本与发布摘要")
	}
	return nil
}
func (s *Service) PreviewRequirementDesign(ctx context.Context, actor Actor, project, requirement string, request RequirementDesignPreviewRequest) (out RequirementDesignPreview, err error) {
	if err = validateDesignPreviewRequest(project, request); err != nil {
		return
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: requirement}, purposeProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, _, e = s.designPreviewTx(auth.ctx, auth, project, requirement, request, false)
		return e
	})
	return
}

func (s *Service) writeDesignTaskTx(ctx context.Context, auth *authorizedCommand, project string, r Record, oldVersion int, oldOwner string) error {
	if e := assertDesignWrite(auth, project); e != nil {
		return e
	}
	res, e := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_tasks SET version=?,title=?,description=?,status=?,priority=?,owner_id=?,requirement_id=?,depends_on=?,acceptance_criteria=?,updated_at=?,data=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), r.Version, r.Title, r.Description, r.Status, r.Priority, r.OwnerID, r.RequirementID, stringify(r.DependsOn), stringify(r.AcceptanceCriteria), r.UpdatedAt, stringify(r.Data), auth.target.tenantID, project, r.ID, oldVersion)
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return conflict("设计采用时任务已变化，请重新预览")
	}
	if r.OwnerID != "" && r.OwnerID != oldOwner {
		return s.projectTaskAssignmentNotificationTx(ctx, auth, oldOwner, r)
	}
	return nil
}
func (s *Service) writeDesignLinkTx(ctx context.Context, auth *authorizedCommand, project, requirement string, l designTaskLink, exists bool) error {
	if e := assertDesignWrite(auth, project); e != nil {
		return e
	}
	if !exists {
		_, e := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_requirement_design_task_links(tenant_id,project_id,requirement_id,task_key,task_id,active,revision,requirement_version,task_basis,adoption_id)VALUES(?,?,?,?,?,?,?,?,?,?)`), auth.target.tenantID, project, requirement, l.Key, l.TaskID, l.Active, l.Revision, l.RequirementVersion, l.TaskBasis, l.AdoptionID)
		return e
	}
	res, e := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_requirement_design_task_links SET active=?,revision=?,requirement_version=?,task_basis=?,adoption_id=? WHERE tenant_id=? AND project_id=? AND requirement_id=? AND task_key=? AND task_id=?`), l.Active, l.Revision, l.RequirementVersion, l.TaskBasis, l.AdoptionID, auth.target.tenantID, project, requirement, l.Key, l.TaskID)
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return conflict("设计任务绑定已变化")
	}
	return nil
}

// Adopt creates IDs only after explicit approval and a matching complete plan.
// Task writes, graph validation, invalidation, protected bindings, aggregate CAS,
// append-only adoption, audit and receipt all share this single transaction.
func (s *Service) AdoptRequirementDesign(ctx context.Context, actor Actor, project, requirement string, request RequirementDesignAdoptRequest) (out RequirementDesignAdoption, err error) {
	previewRequest := RequirementDesignPreviewRequest{ProjectID: request.ProjectID, ExpectedVersion: request.ExpectedVersion, Revision: request.Revision, SHA256: request.SHA256}
	if e := validateDesignPreviewRequest(project, previewRequest); e != nil {
		return out, e
	}
	if len(request.PlanSHA256) != 64 {
		return out, invalid("采用需要已审阅的任务变更摘要")
	}
	ctx, tx, cleanup, e := s.beginAuthorityTx(ctx)
	if e != nil {
		return out, e
	}
	defer cleanup()
	defer tx.Rollback()
	auth, e := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: requirement}, purposeBrowserProjectApprove)
	if e != nil {
		return out, e
	}
	ctx = auth.ctx
	if e = dag.GuardProjectTx(ctx, tx, s.Dialect, auth.target.tenantID, project); e != nil {
		return out, e
	}
	hash, replay, e := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"design_adopt": requirement, "request": request})
	if e != nil {
		return out, e
	}
	if replay {
		if e = json.Unmarshal([]byte(hash), &out); e != nil {
			return out, e
		}
		return out, tx.Commit()
	}
	plan, state, e := s.designPreviewTx(ctx, auth, project, requirement, previewRequest, true)
	if e != nil {
		return out, e
	}
	if plan.PlanSHA256 != request.PlanSHA256 {
		return out, conflict("任务变更预览已失效，请核对当前记录")
	}
	if !plan.HasChanges {
		if state.View.LastAdoption == nil {
			return out, conflict("设计没有可恢复的采用记录")
		}
		out = *state.View.LastAdoption
		if e = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); e != nil {
			return out, e
		}
		return out, tx.Commit()
	}
	if e = s.validateDesignOwnersTx(ctx, auth, project, state.View.Published.Tasks); e != nil {
		return out, e
	}
	stamp := auth.checkedAt.Format(time.RFC3339Nano)
	out = RequirementDesignAdoption{ID: newID(), ProjectID: project, RequirementID: requirement, Version: plan.Version + 1, Revision: plan.Revision, SHA256: plan.SHA256, RequirementVersion: plan.RequirementVersion, PlanSHA256: plan.PlanSHA256, Tasks: []RequirementDesignAdoptedTask{}, CreatedBy: auth.currentPrincipal.ID, CreatedAt: stamp}
	byKey := map[string]designTaskLink{}
	ids := map[string]string{}
	for _, l := range state.Links {
		byKey[l.Key] = l
		ids[l.Key] = l.TaskID
	}
	for _, c := range plan.Changes {
		if c.Action == "create" {
			ids[c.Key] = newID()
		}
	}
	roots := []string{}
	for _, c := range plan.Changes {
		if c.Action == "update" || c.Action == "exclude" {
			roots = append(roots, c.TaskID)
		}
	}
	if len(roots) > 0 {
		if e = s.InvalidateDependentsTx(ctx, tx, auth, project, roots, true); e != nil {
			return RequirementDesignAdoption{}, e
		}
	}
	// The locked pre-plan rows cannot change externally. The invalidation above
	// may reopen an accepted root, so the subsequent CAS uses its post-effect row.
	current, e := s.designTasksTx(ctx, auth, project, state.Links, true)
	if e != nil {
		return RequirementDesignAdoption{}, e
	}
	var validate *Record
	for _, c := range plan.Changes {
		var r Record
		link, exists := byKey[c.Key]
		if c.Action == "create" {
			r = Record{ID: ids[c.Key], TenantID: auth.target.tenantID, ProjectID: project, Version: 1, Status: "todo", CreatedAt: stamp, UpdatedAt: stamp, Data: map[string]any{}, DependsOn: []string{}, AcceptanceCriteria: []string{}}
			r = designProposedRecord(*c.Proposed, r, requirement, ids)
			if e = s.insertCommandRecord(ctx, tx, auth, r, "tasks"); e != nil {
				return RequirementDesignAdoption{}, e
			}
			copy := r
			validate = &copy
		} else {
			r = current[c.TaskID]
			if c.Action == "keep" {
				out.Tasks = append(out.Tasks, RequirementDesignAdoptedTask{Key: c.Key, TaskID: r.ID, Version: r.Version, Action: c.Action})
				continue
			}
			old, oldOwner := r.Version, r.OwnerID
			r.Version++
			r.UpdatedAt = stamp
			if c.Action == "exclude" {
				r.Status = "archived"
			} else {
				r = designProposedRecord(*c.Proposed, r, requirement, ids)
				r.Status = "todo"
				copy := r
				validate = &copy
			}
			delete(r.Data, "verified_run_id")
			delete(r.Data, "run_task_basis")
			delete(r.Data, "requirement_version")
			delete(r.Data, "dependency_basis")
			r.Data["acceptance_invalidated_reason"] = "需求设计采用依据已变化"
			if e = s.writeDesignTaskTx(ctx, auth, project, r, old, oldOwner); e != nil {
				return RequirementDesignAdoption{}, e
			}
		}
		link = designTaskLink{Key: c.Key, TaskID: r.ID, Active: 1, Revision: plan.Revision, RequirementVersion: plan.RequirementVersion, TaskBasis: taskBasis(r), AdoptionID: out.ID}
		if c.Action == "exclude" {
			link.Active = 0
		}
		if e = s.writeDesignLinkTx(ctx, auth, project, requirement, link, exists); e != nil {
			return RequirementDesignAdoption{}, e
		}
		out.Tasks = append(out.Tasks, RequirementDesignAdoptedTask{Key: c.Key, TaskID: r.ID, Version: r.Version, Action: c.Action})
	}
	// Proposal fields and each owner were checked above; reuse the existing full
	// formal graph validator once for the whole batch, avoiding N graph scans.
	if validate != nil {
		if e = s.validateRecord(ctx, tx, auth, *validate, "tasks"); e != nil {
			return RequirementDesignAdoption{}, e
		}
	}
	if _, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_requirement_design_adoptions(tenant_id,project_id,requirement_id,id,version,snapshot)VALUES(?,?,?,?,?,?)`), auth.target.tenantID, project, requirement, out.ID, out.Version, stringify(out)); e != nil {
		return RequirementDesignAdoption{}, e
	}
	if e = s.indexRequirementDesignAdoptionTx(ctx, auth, project, requirement, out); e != nil {
		return RequirementDesignAdoption{}, e
	}
	res, e := tx.ExecContext(ctx, s.q(`UPDATE suma_requirement_designs SET version=version+1,last_adoption_id=?,updated_at=? WHERE tenant_id=? AND project_id=? AND requirement_id=? AND version=?`), out.ID, stamp, auth.target.tenantID, project, requirement, plan.Version)
	if e != nil {
		return RequirementDesignAdoption{}, e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return RequirementDesignAdoption{}, e
	}
	if n != 1 {
		return RequirementDesignAdoption{}, conflict("需求设计已变化，请重新预览")
	}
	if e = s.commandActivity(ctx, auth, "requirement.design_adopted", requirement, state.Source.Title); e != nil {
		return RequirementDesignAdoption{}, e
	}
	if e = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); e != nil {
		return RequirementDesignAdoption{}, e
	}
	return out, tx.Commit()
}

func (s *Service) resolveTaskRequirementDesignTx(ctx context.Context, auth *authorizedCommand, project, taskID string) (*RequirementDesignContext, error) {
	if e := assertProjectRead(auth, project); e != nil {
		return nil, e
	}
	var requirement string
	var l designTaskLink
	e := auth.tx.QueryRowContext(ctx, s.q(`SELECT requirement_id,task_key,task_id,active,revision,requirement_version,task_basis,adoption_id FROM suma_requirement_design_task_links WHERE tenant_id=? AND project_id=? AND task_id=?`), auth.target.tenantID, project, taskID).Scan(&requirement, &l.Key, &l.TaskID, &l.Active, &l.Revision, &l.RequirementVersion, &l.TaskBasis, &l.AdoptionID)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if l.Active != 1 {
		return nil, conflict("设计任务已移出范围，需要重新审阅采用")
	}
	a, e := s.designAggregateTx(ctx, auth, project, requirement)
	if e != nil {
		return nil, e
	}
	if a.Published != l.Revision || a.Published < 1 {
		return nil, conflict("设计任务尚未采用当前发布")
	}
	rev, e := s.designRevisionTx(ctx, auth, project, requirement, l.Revision)
	if e != nil {
		return nil, e
	}
	source, e := s.getRecordTx(ctx, auth, project, "requirements", requirement)
	if e != nil {
		return nil, conflict("设计需求来源不可用")
	}
	if rev.State != "published" || source.Version != l.RequirementVersion || !designSourceCurrent(source, rev.Requirement) {
		return nil, conflict("设计需求来源已变化，需要重新审阅采用")
	}
	task, e := s.getRecordTx(ctx, auth, project, "tasks", taskID)
	if e != nil {
		return nil, e
	}
	if task.RequirementID != requirement || task.Status == "archived" || task.Status == "deleted" || taskBasis(task) != l.TaskBasis {
		return nil, conflict("正式任务已偏离设计采用依据，请重新预览")
	}
	adoption, e := s.designAdoptionTx(ctx, auth, project, requirement, l.AdoptionID)
	if e != nil {
		return nil, e
	}
	found := false
	for _, t := range adoption.Tasks {
		if t.Key == l.Key && t.TaskID == taskID && t.Action != "exclude" {
			found = true
		}
	}
	if !found || adoption.Revision != l.Revision || adoption.RequirementVersion != l.RequirementVersion || adoption.SHA256 != rev.SHA256 {
		return nil, conflict("设计任务采用凭证不一致")
	}
	return &RequirementDesignContext{RequirementID: requirement, RequirementVersion: l.RequirementVersion, Revision: l.Revision, SHA256: rev.SHA256, TaskKey: l.Key, AdoptionID: l.AdoptionID, Content: rev.Content}, nil
}
