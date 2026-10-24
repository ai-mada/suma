package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

var tables = map[string]bool{"requirements": true, "tasks": true, "members": true, "issues": true, "documents": true, "releases": true, "tests": true, "evidence": true}

const recordColumns = `id,tenant_id,project_id,version,title,description,status,priority,owner_id,requirement_id,run_id,depends_on,acceptance_criteria,kind,created_at,updated_at,data`

type scanner interface{ Scan(...any) error }

func scanRecord(row scanner) (Record, error) {
	var r Record
	var deps, criteria, data string
	err := row.Scan(&r.ID, &r.TenantID, &r.ProjectID, &r.Version, &r.Title, &r.Description, &r.Status, &r.Priority, &r.OwnerID, &r.RequirementID, &r.RunID, &deps, &criteria, &r.Kind, &r.CreatedAt, &r.UpdatedAt, &data)
	r.Name = r.Title
	r.DependsOn = []string{}
	r.AcceptanceCriteria = []string{}
	r.Data = map[string]any{}
	if err == nil {
		for _, p := range []struct {
			s string
			v any
		}{{deps, &r.DependsOn}, {criteria, &r.AcceptanceCriteria}, {data, &r.Data}} {
			if e := json.Unmarshal([]byte(p.s), p.v); e != nil {
				return r, e
			}
		}
	}
	return r, err
}
func (s *Service) List(ctx context.Context, p Principal, project, kind string) ([]Record, error) {
	if !tables[kind] {
		return nil, missing()
	}
	if err := s.Authorize(ctx, p, project, false, false); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_`+kind+` WHERE tenant_id=? AND project_id=? ORDER BY updated_at DESC,id LIMIT 1000`), p.TenantID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Service) Get(ctx context.Context, p Principal, project, kind, id string) (Record, error) {
	if !tables[kind] {
		return Record{}, missing()
	}
	if err := s.Authorize(ctx, p, project, false, false); err != nil {
		return Record{}, err
	}
	r, err := scanRecord(s.DB.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_`+kind+` WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, missing()
	}
	return r, err
}
func (s *Service) receipt(ctx context.Context, tx interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, p Principal, project, key string, body any) (string, bool, error) {
	if len(key) < 8 || len(key) > 128 {
		return "", false, invalid("请提供 8–128 字符的 idempotency_key")
	}
	hash := digest(p.ID + stringify(body))
	var stored, response string
	err := tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), p.TenantID, project, key).Scan(&stored, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return hash, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if stored != hash {
		return "", false, &APIError{409, "idempotency_conflict", "该操作标识已用于其他内容", nil}
	}
	return response, true, nil
}
func (s *Service) saveReceipt(ctx context.Context, tx *sql.Tx, p Principal, project, key, hash string, response any) error {
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_operations(tenant_id,project_id,operation_id,request_hash,response) VALUES(?,?,?,?,?)`), p.TenantID, project, key, hash, stringify(response))
	return err
}
func (s *Service) insertRecord(ctx context.Context, tx *sql.Tx, r Record, kind string) error {
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_`+kind+` (`+recordColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), r.ID, r.TenantID, r.ProjectID, r.Version, r.Title, r.Description, r.Status, r.Priority, r.OwnerID, r.RequirementID, r.RunID, stringify(r.DependsOn), stringify(r.AcceptanceCriteria), r.Kind, r.CreatedAt, r.UpdatedAt, stringify(r.Data))
	return err
}
func recordFrom(p Principal, project string, m map[string]any) Record {
	r := Record{ID: newID(), TenantID: p.TenantID, ProjectID: project, Version: 1, Title: strings.TrimSpace(str(m, "title")), Description: str(m, "description"), Status: str(m, "status"), Priority: str(m, "priority"), OwnerID: str(m, "owner_id"), RequirementID: str(m, "requirement_id"), RunID: str(m, "run_id"), DependsOn: stringsAt(m, "depends_on"), AcceptanceCriteria: stringsAt(m, "acceptance_criteria"), Kind: str(m, "kind"), CreatedAt: now(), UpdatedAt: now(), Data: map[string]any{}}
	if r.Title == "" {
		r.Title = str(m, "name")
	}
	if r.Status == "" {
		r.Status = "todo"
	}
	if r.Priority == "" {
		r.Priority = "medium"
	}
	if d, ok := m["data"].(map[string]any); ok {
		r.Data = d
	}
	return r
}
func (s *Service) validateRecord(ctx context.Context, tx *sql.Tx, p Principal, r Record, kind string) error {
	if err := requireTitle(r.Title); err != nil {
		return err
	}
	if len(r.Description) > 200000 || len(r.DependsOn) > 200 || len(r.AcceptanceCriteria) > 100 {
		return invalid("内容超出单项限制")
	}
	if kind == "documents" {
		body, _ := r.Data["content"].(string)
		size := len(r.Description)
		if body != "" {
			size += 1 + len(body)
		}
		if size > 1<<20 {
			return invalid("文档正文与描述合计不得超过 1 MiB UTF-8 字节")
		}
	}
	if kind == "tasks" && (r.Status == "accepted" || r.Status == "completed" || r.Status == "done") {
		return invalid("任务完成须经过测试证据与人工验收")
	}
	if r.OwnerID != "" {
		var n int
		if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_members WHERE tenant_id=? AND project_id=? AND id=?`), r.TenantID, r.ProjectID, r.OwnerID).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return invalid("负责人不属于当前项目")
		}
		if kind == "tasks" {
			owner, e := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_members WHERE tenant_id=? AND project_id=? AND id=?`), r.TenantID, r.ProjectID, r.OwnerID))
			if e != nil {
				return e
			}
			if owner.Status == "archived" || owner.Status == "deleted" {
				return invalid("负责人资源已停用")
			}
			if owner.Kind == "human" {
				user, _ := owner.Data["user_id"].(string)
				if e = s.BindHumanResourceTx(ctx, tx, p, r.ProjectID, user); e != nil {
					return e
				}
			}
		}
	}
	if r.RequirementID != "" {
		var n int
		if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_requirements WHERE tenant_id=? AND project_id=? AND id=?`), r.TenantID, r.ProjectID, r.RequirementID).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return invalid("关联需求不属于当前项目")
		}
	}
	if kind == "members" && r.Kind != "human" && r.Kind != "agent" && r.Kind != "tool" {
		return invalid("资源类型应为 human、agent 或 tool")
	}
	if kind == "members" && r.Kind == "human" {
		user, _ := r.Data["user_id"].(string)
		if e := s.BindHumanResourceTx(ctx, tx, p, r.ProjectID, user); e != nil {
			return e
		}
		var resource string
		er := tx.QueryRowContext(ctx, s.q(`SELECT resource_id FROM suma_human_resources WHERE tenant_id=? AND project_id=? AND user_id=?`), r.TenantID, r.ProjectID, user).Scan(&resource)
		if er != nil && er != sql.ErrNoRows {
			return er
		}
		if er == nil && resource != r.ID {
			return conflict("此账号已在项目中登记，请更新已有资源")
		}
		if er == sql.ErrNoRows {
			if _, er = tx.ExecContext(ctx, s.q(`INSERT INTO suma_human_resources(tenant_id,project_id,user_id,resource_id)VALUES(?,?,?,?)`), r.TenantID, r.ProjectID, user, r.ID); er != nil {
				return er
			}
		}
	}
	if kind == "releases" && (r.Status == "published" || r.Status == "deployed") {
		return invalid("发布登记不能替代实际部署验证")
	}
	if kind == "tasks" {
		rows, err := tx.QueryContext(ctx, s.q(`SELECT id,depends_on FROM suma_tasks WHERE tenant_id=? AND project_id=?`), r.TenantID, r.ProjectID)
		if err != nil {
			return err
		}
		graph := map[string][]string{}
		for rows.Next() {
			var id, raw string
			if err = rows.Scan(&id, &raw); err != nil {
				rows.Close()
				return err
			}
			var deps []string
			if err = json.Unmarshal([]byte(raw), &deps); err != nil {
				rows.Close()
				return err
			}
			graph[id] = deps
		}
		rows.Close()
		graph[r.ID] = r.DependsOn
		state := map[string]int{}
		var visit func(string) error
		visit = func(id string) error {
			if state[id] == 1 {
				return invalid("任务依赖存在环路")
			}
			if state[id] == 2 {
				return nil
			}
			deps, ok := graph[id]
			if !ok {
				return invalid("前置任务不存在或不属于当前项目")
			}
			state[id] = 1
			for _, d := range deps {
				if err := visit(d); err != nil {
					return err
				}
			}
			state[id] = 2
			return nil
		}
		for id := range graph {
			if err := visit(id); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Service) Create(ctx context.Context, p Principal, project, kind string, m map[string]any) (Record, error) {
	if !tables[kind] {
		return Record{}, missing()
	}
	if err := s.Authorize(ctx, p, project, true, false); err != nil {
		return Record{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	if err = s.authorizeProjectTx(ctx, tx, p, project, true, false); err != nil {
		return Record{}, err
	}
	key := str(m, "idempotency_key")
	hash, replay, err := s.receipt(ctx, tx, p, project, key, map[string]any{"kind": kind, "body": m})
	if err != nil {
		return Record{}, err
	}
	if replay {
		var r Record
		err = json.Unmarshal([]byte(hash), &r)
		return r, err
	}
	r := recordFrom(p, project, m)
	if kind == "tasks" {
		r.RunID = ""
	}
	if str(m, "status") == "" {
		switch kind {
		case "requirements", "releases":
			r.Status = "draft"
		case "members", "documents":
			r.Status = "active"
		case "issues":
			r.Status = "open"
		case "tests":
			r.Status = "planned"
		case "evidence":
			r.Status = "unverified"
		}
	}
	if err = s.validateRecord(ctx, tx, p, r, kind); err != nil {
		return r, err
	}
	if err = s.insertRecord(ctx, tx, r, kind); err != nil {
		return r, err
	}
	if err = s.activity(ctx, tx, p, project, kind+".created", r.ID, r.Title); err != nil {
		return r, err
	}
	if kind == "documents" {
		if err = s.indexEvent(ctx, tx, r, "upsert"); err != nil {
			return r, err
		}
	}
	if err = s.saveReceipt(ctx, tx, p, project, key, hash, r); err != nil {
		return r, err
	}
	return r, tx.Commit()
}
func (s *Service) Update(ctx context.Context, p Principal, project, kind, id string, m map[string]any) (Record, error) {
	if err := exactVersion(m, "expected_version", 1); err != nil {
		return Record{}, err
	}
	if !tables[kind] {
		return Record{}, missing()
	}
	if err := s.Authorize(ctx, p, project, true, false); err != nil {
		return Record{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	if err = s.authorizeProjectTx(ctx, tx, p, project, true, false); err != nil {
		return Record{}, err
	}
	key := str(m, "idempotency_key")
	hash, replay, err := s.receipt(ctx, tx, p, project, key, map[string]any{"kind": kind, "id": id, "body": m})
	if err != nil {
		return Record{}, err
	}
	if replay {
		var r Record
		err = json.Unmarshal([]byte(hash), &r)
		return r, err
	}
	r, err := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_`+kind+` WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, missing()
	}
	if err != nil {
		return r, err
	}
	if number(m, "expected_version") != r.Version {
		return r, conflict("记录已变化，请保留草稿并重新加载")
	}
	oldBasis := taskBasis(r)
	originalKind := r.Kind
	originalUser, _ := r.Data["user_id"].(string)
	oldVersion := r.Version
	for k, v := range m {
		switch k {
		case "title", "name":
			if a, ok := v.(string); ok {
				r.Title = a
			}
		case "description":
			r.Description = str(m, k)
		case "status":
			r.Status = str(m, k)
		case "priority":
			r.Priority = str(m, k)
		case "owner_id":
			r.OwnerID = str(m, k)
		case "requirement_id":
			r.RequirementID = str(m, k)
		case "run_id":
			if kind == "tests" || kind == "evidence" {
				r.RunID = str(m, k)
			}
		case "depends_on":
			r.DependsOn = stringsAt(m, k)
		case "acceptance_criteria":
			r.AcceptanceCriteria = stringsAt(m, k)
		case "kind":
			r.Kind = str(m, k)
		case "data":
			if data, ok := v.(map[string]any); ok {
				for dk, dv := range data {
					if dk != "run_task_basis" && dk != "requirement_version" && dk != "verified_run_id" && dk != "dependency_basis" {
						r.Data[dk] = dv
					}
				}
			}
		}
	}
	if kind == "tasks" && taskBasis(r) != oldBasis {
		r.Status = "in_progress"
		delete(r.Data, "verified_run_id")
	}
	if kind == "members" {
		nextUser, _ := r.Data["user_id"].(string)
		if r.Kind != originalKind || originalKind == "human" && originalUser != "" && nextUser != originalUser {
			return r, invalid("资源身份与类型不可替换，请登记独立资源")
		}
	}
	if kind == "tasks" && r.Status == "accepted" && taskBasis(r) == oldBasis {
		r.Status = "in_progress"
		delete(r.Data, "verified_run_id")
	}
	if err = s.validateRecord(ctx, tx, p, r, kind); err != nil {
		return r, err
	}
	r.Version++
	r.Name = r.Title
	r.UpdatedAt = now()
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_`+kind+` SET version=?,title=?,description=?,status=?,priority=?,owner_id=?,requirement_id=?,depends_on=?,acceptance_criteria=?,kind=?,updated_at=?,data=? WHERE id=? AND tenant_id=? AND project_id=? AND version=?`), r.Version, r.Title, r.Description, r.Status, r.Priority, r.OwnerID, r.RequirementID, stringify(r.DependsOn), stringify(r.AcceptanceCriteria), r.Kind, r.UpdatedAt, stringify(r.Data), id, p.TenantID, project, oldVersion)
	if err != nil {
		return r, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return r, conflict("保存冲突")
	}
	if kind == "requirements" {
		rows, e := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tasks WHERE tenant_id=? AND project_id=? AND requirement_id=?`), p.TenantID, project, id)
		if e != nil {
			return r, e
		}
		ids := []string{}
		for rows.Next() {
			var taskID string
			if e = rows.Scan(&taskID); e != nil {
				rows.Close()
				return r, e
			}
			ids = append(ids, taskID)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return r, e
		}
		if err = s.InvalidateDependentsTx(ctx, tx, p, project, ids, true); err != nil {
			return r, err
		}
	}
	if kind == "tasks" {
		if err = s.InvalidateDependentsTx(ctx, tx, p, project, []string{id}, false); err != nil {
			return r, err
		}
	}
	if kind == "documents" {
		if err = s.indexEvent(ctx, tx, r, "upsert"); err != nil {
			return r, err
		}
	}
	if err = s.activity(ctx, tx, p, project, kind+".updated", id, r.Title); err != nil {
		return r, err
	}
	if err = s.saveReceipt(ctx, tx, p, project, key, hash, r); err != nil {
		return r, err
	}
	return r, tx.Commit()
}
func taskBasis(r Record) string {
	return digest(stringify(map[string]any{"title": r.Title, "description": r.Description, "owner_id": r.OwnerID, "requirement_id": r.RequirementID, "depends_on": r.DependsOn, "acceptance_criteria": r.AcceptanceCriteria}))
}
func (s *Service) indexEvent(ctx context.Context, tx *sql.Tx, r Record, action string) error {
	if r.Status == "archived" || r.Status == "deleted" {
		action = "delete"
	}
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_index_outbox(id,tenant_id,project_id,source_id,revision,action,payload,state,created_at) VALUES(?,?,?,?,?,?,?,?,?)`), newID(), r.TenantID, r.ProjectID, r.ID, r.Version, action, stringify(r), "pending", now())
	return err
}

func (s *Service) Projects(ctx context.Context, p Principal) ([]Record, error) {
	var liveRole string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT role FROM suma_org_members WHERE tenant_id=? AND user_id=?`), p.TenantID, p.ID).Scan(&liveRole); err != nil {
		return nil, forbidden()
	}
	p.Role = liveRole
	query := `SELECT p.id,p.tenant_id,p.version,p.title,p.description,p.status,p.created_at,p.updated_at FROM suma_projects p WHERE p.tenant_id=?`
	args := []any{p.TenantID}
	if p.Role != "owner" && p.Role != "admin" {
		query += ` AND EXISTS(SELECT 1 FROM suma_project_members m WHERE m.project_id=p.id AND m.user_id=?)`
		args = append(args, p.ID)
	}
	query += ` ORDER BY p.created_at,p.id`
	rows, err := s.DB.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var r Record
		if err = rows.Scan(&r.ID, &r.TenantID, &r.Version, &r.Title, &r.Description, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Name = r.Title
		r.ProjectID = r.ID
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Service) CreateProject(ctx context.Context, p Principal, m map[string]any) (Record, error) {
	var liveRole string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT role FROM suma_org_members WHERE tenant_id=? AND user_id=?`), p.TenantID, p.ID).Scan(&liveRole); err != nil || liveRole != "owner" && liveRole != "admin" {
		return Record{}, forbidden()
	}
	title := str(m, "name")
	if title == "" {
		title = str(m, "title")
	}
	if err := requireTitle(title); err != nil {
		return Record{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, p.TenantID); err != nil {
		return Record{}, err
	}
	p, err = s.liveOrganization(ctx, tx, p)
	if err != nil || !orgAdmin(p) {
		return Record{}, forbidden()
	}
	key := str(m, "idempotency_key")
	hash, replay, err := s.receipt(ctx, tx, p, "", key, m)
	if err != nil {
		return Record{}, err
	}
	if replay {
		var r Record
		err = json.Unmarshal([]byte(hash), &r)
		return r, err
	}
	r := Record{ID: newID(), TenantID: p.TenantID, Version: 1, Title: title, Name: title, Description: str(m, "description"), Status: "active", CreatedAt: now(), UpdatedAt: now()}
	r.ProjectID = r.ID
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_projects(id,tenant_id,version,title,description,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`), r.ID, r.TenantID, r.Version, r.Title, r.Description, r.Status, r.CreatedAt, r.UpdatedAt); err != nil {
		return r, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_project_members(project_id,user_id,role) VALUES(?,?,?)`), r.ID, p.ID, "admin"); err != nil {
		return r, err
	}
	if err = s.saveReceipt(ctx, tx, p, "", key, hash, r); err != nil {
		return r, err
	}
	return r, tx.Commit()
}
