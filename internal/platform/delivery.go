package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/ai-mada/suma/internal/dag"
	"strings"
)

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func (s *Service) Decompose(ctx context.Context, p Principal, project, id string, m map[string]any) ([]Record, error) {
	if err := s.Authorize(ctx, p, project, true, false); err != nil {
		return nil, err
	}
	if err := exactVersion(m, "expected_version", 1); err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	key := str(m, "idempotency_key")
	hash, replay, err := s.receipt(ctx, tx, p, project, key, map[string]any{"requirement": id, "body": m})
	if err != nil {
		return nil, err
	}
	if replay {
		var out []Record
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	req, err := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_requirements WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id))
	if err != nil {
		return nil, missing()
	}
	if req.Version != number(m, "expected_version") {
		return nil, conflict("需求已变更，请检查拆解草稿")
	}
	drafts, ok := m["drafts"].([]any)
	if !ok || len(drafts) == 0 || len(drafts) > 100 {
		return nil, invalid("请选择 1–100 项任务")
	}
	out := []Record{}
	keys := map[string]string{}
	for _, item := range drafts {
		d, ok := item.(map[string]any)
		if !ok {
			return nil, invalid("任务草稿格式错误")
		}
		r := recordFrom(p, project, d)
		r.RequirementID = id
		r.Status = "todo"
		if err = requireTitle(r.Title); err != nil {
			return nil, err
		}
		k := str(d, "key")
		if k != "" {
			if _, exists := keys[k]; exists {
				return nil, invalid("任务草稿 key 重复")
			}
			keys[k] = r.ID
		}
		out = append(out, r)
	}
	for i := range out {
		for j, d := range out[i].DependsOn {
			if mapped, ok := keys[d]; ok {
				out[i].DependsOn[j] = mapped
			}
		}
		if err = s.insertRecord(ctx, tx, out[i], "tasks"); err != nil {
			return nil, err
		}
	}
	for _, r := range out {
		if err = s.validateRecord(ctx, tx, p, r, "tasks"); err != nil {
			return nil, err
		}
	}
	if err = s.activity(ctx, tx, p, project, "requirement.decomposed", id, req.Title); err != nil {
		return nil, err
	}
	if err = s.saveReceipt(ctx, tx, p, project, key, hash, out); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

func (s *Service) AttachRun(ctx context.Context, p Principal, project, taskID, runID string) error {
	if taskID == "" {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.BindRunTx(ctx, tx, p, project, taskID, runID, 0); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) BindRunTx(ctx context.Context, tx *sql.Tx, p Principal, project, taskID, runID string, expected int) error {
	if taskID == "" {
		return nil
	}
	r, err := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, taskID))
	if err != nil {
		return missing()
	}
	if r.RunID == runID {
		return nil
	}
	if expected != 0 && r.Version != expected {
		return conflict("任务已变化，需要重新派发预览")
	}
	dependencyBasis := map[string]string{}
	for _, dep := range r.DependsOn {
		if e := s.checkTaskEvidenceTx(ctx, tx, p, project, dep, map[string]bool{}, map[string]bool{}); e != nil {
			return e
		}
		upstream, e := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, dep))
		if e != nil || upstream.Status != "accepted" {
			return invalid("前置任务尚未通过验收")
		}
		verifiedRun, _ := upstream.Data["verified_run_id"].(string)
		if e = s.checkEvidenceTx(ctx, tx, p.TenantID, project, verifiedRun, 0, map[string]bool{}); e != nil {
			return e
		}
		dependencyBasis[dep] = digest(stringify(map[string]any{"version": upstream.Version, "verified_run_id": upstream.Data["verified_run_id"]}))
	}
	r.Data["dependency_basis"] = dependencyBasis
	r.Data["run_task_basis"] = taskBasis(r)
	if r.RequirementID != "" {
		var v int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT version FROM suma_requirements WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, r.RequirementID).Scan(&v); err != nil {
			return err
		}
		r.Data["requirement_version"] = v
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_tasks SET run_id=?,status='in_progress',version=version+1,updated_at=?,data=? WHERE id=? AND tenant_id=? AND project_id=? AND version=?`), runID, now(), stringify(r.Data), r.ID, p.TenantID, project, r.Version)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return conflict("任务已变更")
	}
	return s.ReopenPlanningTx(ctx, tx, p, project)
}

func (s *Service) Review(ctx context.Context, p Principal, project, id string, m map[string]any, engine *dag.Engine) (Record, error) {
	if err := s.Authorize(ctx, p, project, true, true); err != nil {
		return Record{}, err
	}
	if err := exactVersion(m, "expected_version", 1); err != nil {
		return Record{}, err
	}
	reason := str(m, "reason")
	decision := str(m, "decision")
	if reason == "" || len(reason) > 5000 {
		return Record{}, invalid("请填写验收意见")
	}
	if decision == "approved" {
		decision = "accepted"
	}
	if decision != "accepted" && decision != "rejected" {
		return Record{}, invalid("验收结论应为 accepted 或 rejected")
	}
	key := str(m, "idempotency_key")
	request := map[string]any{"review": id, "body": m}
	if saved, replay, e := s.reviewReceipt(ctx, p, project, key, request); e != nil {
		return Record{}, e
	} else if replay {
		var result Record
		e = json.Unmarshal([]byte(saved), &result)
		return result, e
	}
	runID := str(m, "run_id")
	if runID == "" {
		r, err := s.Get(ctx, p, project, "tasks", id)
		if err != nil {
			return r, err
		}
		runID = r.RunID
	}
	run, err := engine.GetRun(ctx, p.TenantID, project, runID)
	if err != nil {
		return Record{}, invalid("尚无可验收的执行记录")
	}
	evidenceVersions := map[string]int{}
	if decision == "accepted" {
		if run.State != "succeeded" {
			return Record{}, invalid("执行尚未成功，不能通过验收")
		}
		tests, e := countVerifiedTests(ctx, engine, p.TenantID, project, run, 0, evidenceVersions)
		if e != nil {
			return Record{}, e
		}
		if tests == 0 {
			return Record{}, invalid("缺少明确测试节点的执行证据")
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	if err = s.authorizeProjectTx(ctx, tx, p, project, true, true); err != nil {
		return Record{}, err
	}
	hash, replay, err := s.receipt(ctx, tx, p, project, key, request)
	if err != nil {
		return Record{}, err
	}
	if replay {
		var result Record
		err = json.Unmarshal([]byte(hash), &result)
		return result, err
	}
	if decision == "accepted" {
		if _, err = tx.ExecContext(ctx, `UPDATE dag_lock SET seq=seq WHERE id=1`); err != nil {
			return Record{}, err
		}
		for runID, version := range evidenceVersions {
			var current int
			var state string
			if err = tx.QueryRowContext(ctx, s.q(`SELECT version,state FROM dag_runs WHERE id=? AND tenant_id=? AND project_id=?`), runID, p.TenantID, project).Scan(&current, &state); err != nil || current != version || state != "succeeded" {
				return Record{}, conflict("执行证据已变化，请重新审查")
			}
			var count int
			if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_compensations WHERE original_run_id=?`), runID).Scan(&count); err != nil {
				return Record{}, err
			}
			if count != 0 {
				return Record{}, conflict("执行已进入补偿，不能通过验收")
			}
		}
	}
	r, err := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id))
	if err != nil {
		return r, missing()
	}
	if r.Version != number(m, "expected_version") {
		return r, conflict("任务已变更，请重新审查")
	}
	if r.RunID != runID || r.Data["run_task_basis"] != taskBasis(r) {
		return r, conflict("执行依据已失效，需要重新派发")
	}
	if decision == "accepted" {
		basis, _ := r.Data["dependency_basis"].(map[string]any)
		for _, dep := range r.DependsOn {
			if e := s.checkTaskEvidenceTx(ctx, tx, p, project, dep, map[string]bool{}, map[string]bool{}); e != nil {
				return r, e
			}
			upstream, e := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, dep))
			if e != nil || upstream.Status != "accepted" || basis[dep] != digest(stringify(map[string]any{"version": upstream.Version, "verified_run_id": upstream.Data["verified_run_id"]})) {
				return r, conflict("前置任务的验收依据已变化，需要重新派发")
			}
		}
	}
	if r.RequirementID != "" {
		var v int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT version FROM suma_requirements WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, r.RequirementID).Scan(&v); err != nil {
			return r, err
		}
		stored, _ := r.Data["requirement_version"].(float64)
		if int(stored) != v {
			return r, conflict("需求已变更，旧执行不能通过验收")
		}
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_reviews(id,tenant_id,project_id,task_id,task_version,run_id,reviewer_id,decision,reason,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`), newID(), p.TenantID, project, id, r.Version, runID, p.ID, decision, reason, now()); err != nil {
		return r, err
	}
	old := r.Version
	r.Version++
	r.Status = "blocked"
	if decision == "accepted" {
		r.Status = "accepted"
		r.Data["verified_run_id"] = runID
	}
	r.UpdatedAt = now()
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_tasks SET status=?,version=?,updated_at=?,data=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), r.Status, r.Version, r.UpdatedAt, stringify(r.Data), p.TenantID, project, id, old)
	if err != nil {
		return r, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return r, conflict("验收保存冲突")
	}
	if err = s.ReopenPlanningTx(ctx, tx, p, project); err != nil {
		return r, err
	}
	if err = s.activity(ctx, tx, p, project, "task.reviewed", id, r.Title); err != nil {
		return r, err
	}
	if err = s.saveReceipt(ctx, tx, p, project, key, hash, r); err != nil {
		return r, err
	}
	return r, tx.Commit()
}

func countVerifiedTests(ctx context.Context, engine *dag.Engine, tenant, project string, run dag.Run, depth int, seen map[string]int) (int, error) {
	if _, exists := seen[run.ID]; depth > 32 || exists {
		return 0, invalid("子流程证据链无效")
	}
	seen[run.ID] = run.Version
	if run.State != "succeeded" || run.CompensationRunID != "" {
		return 0, conflict("运行或子运行已失败、正在补偿或已补偿，不能复用测试证据")
	}
	if err := validateCodeTestEvidence(run); err != nil {
		return 0, err
	}
	count := 0
	for _, node := range run.Nodes {
		if node.Type == "test" {
			if node.State == "skipped" {
				continue
			}
			if node.State != "succeeded" {
				return 0, invalid("本次已选测试节点必须通过")
			}
			count++
		}
		if node.ChildRunID != "" {
			child, e := engine.GetRun(ctx, tenant, project, node.ChildRunID)
			if e != nil {
				return 0, e
			}
			n, e := countVerifiedTests(ctx, engine, tenant, project, child, depth+1, seen)
			if e != nil {
				return 0, e
			}
			count += n
		}
	}
	return count, nil
}

func (s *Service) FailureIssue(ctx context.Context, tenant, project string, run dag.Run) error {
	if run.State != "failed" {
		return nil
	}
	if handled, err := s.projectManagedReleaseFailure(ctx, tenant, project, run.ID); handled || err != nil {
		return err
	}
	p := Principal{ID: "system", TenantID: tenant, Role: "owner", internal: true}
	m := map[string]any{"title": "执行失败：" + run.WorkflowName, "description": "运行 " + run.ID + " 失败，请查看节点证据并分诊。", "status": "open", "priority": "high", "idempotency_key": "failure-" + run.ID, "data": map[string]any{"run_id": run.ID, "origin": "execution"}}
	_, err := s.Create(ctx, p, project, "issues", m)
	return err
}

func (s *Service) IssueToRequirement(ctx context.Context, p Principal, project, id string, m map[string]any) (Record, error) {
	issue, err := s.Get(ctx, p, project, "issues", id)
	if err != nil {
		return Record{}, err
	}
	title := str(m, "title")
	if title == "" {
		title = issue.Title
	}
	return s.Create(ctx, p, project, "requirements", map[string]any{"title": title, "description": issue.Description, "status": "draft", "priority": issue.Priority, "idempotency_key": "issue-intake-" + id, "data": map[string]any{"source_issue_id": id, "source": issue.Data}})
}

func (s *Service) Overview(ctx context.Context, p Principal, project string) (map[string]any, error) {
	if err := s.Authorize(ctx, p, project, false, false); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for kind := range tables {
		var n int
		if err := s.DB.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_`+kind+` WHERE tenant_id=? AND project_id=?`), p.TenantID, project).Scan(&n); err != nil {
			return nil, err
		}
		counts[kind] = n
	}
	for _, state := range []string{"accepted", "blocked", "in_progress"} {
		var n int
		if err := s.DB.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_tasks WHERE tenant_id=? AND project_id=? AND status=?`), p.TenantID, project, state).Scan(&n); err != nil {
			return nil, err
		}
		counts[state] = n
	}
	var name, description string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT title,description FROM suma_projects WHERE tenant_id=? AND id=?`), p.TenantID, project).Scan(&name, &description); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT id,kind,object_id,title,created_at FROM suma_activity WHERE tenant_id=? AND project_id=? ORDER BY created_at DESC,id DESC LIMIT 30`), p.TenantID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	activity := []map[string]any{}
	for rows.Next() {
		var id, kind, object, title, at string
		if err = rows.Scan(&id, &kind, &object, &title, &at); err != nil {
			return nil, err
		}
		activity = append(activity, map[string]any{"id": id, "type": kind, "kind": kind, "object_id": object, "title": title, "created_at": at, "at": at})
	}
	return map[string]any{"project": map[string]any{"id": project, "title": name, "name": name, "description": description}, "counts": counts, "activity": activity, "as_of": now()}, rows.Err()
}

func (s *Service) Search(ctx context.Context, p Principal, project, query string) (map[string]any, error) {
	return s.SearchKeywords(ctx, p, project, query)
}
