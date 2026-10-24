package platform

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func (s *Service) Teams(ctx context.Context, p Principal) ([]map[string]any, error) {
	if _, err := s.liveOrganization(ctx, s.DB, p); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT id,version,name,description,status,created_at,updated_at FROM suma_teams WHERE tenant_id=? ORDER BY name,id`), p.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		item, err := scanTeam(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanTeam(row scanner) (map[string]any, error) {
	var id, name, description, status, created, updated string
	var version int
	err := row.Scan(&id, &version, &name, &description, &status, &created, &updated)
	return map[string]any{"id": id, "version": version, "name": name, "description": description, "status": status, "created_at": created, "updated_at": updated}, err
}

func (s *Service) SaveTeam(ctx context.Context, p Principal, id string, m map[string]any) (map[string]any, error) {
	return s.governanceWrite(ctx, p, "organization.team.save", id, m, requireOrgAdmin, func(tx *sql.Tx, p Principal) (map[string]any, error) {
		name, description, status := strings.TrimSpace(str(m, "name")), str(m, "description"), str(m, "status")
		version, created := 0, now()
		if id != "" {
			old, err := scanTeam(tx.QueryRowContext(ctx, s.q(`SELECT id,version,name,description,status,created_at,updated_at FROM suma_teams WHERE tenant_id=? AND id=?`), p.TenantID, id))
			if errors.Is(err, sql.ErrNoRows) {
				return nil, missing()
			}
			if err != nil {
				return nil, err
			}
			version = old["version"].(int)
			created = old["created_at"].(string)
			if number(m, "expected_version") != version {
				return nil, conflict("团队已更新，请刷新后重试")
			}
			if _, ok := m["name"]; !ok {
				name = old["name"].(string)
			}
			if _, ok := m["description"]; !ok {
				description = old["description"].(string)
			}
			if _, ok := m["status"]; !ok {
				status = old["status"].(string)
			}
		} else {
			if number(m, "expected_version") != 0 {
				return nil, conflict("新团队的预期版本应为 0")
			}
			id = newID()
			if status == "" {
				status = "active"
			}
		}
		if name == "" || len(name) > 200 || len(description) > 20000 {
			return nil, invalid("团队名称应为 1–200 字节，描述最多 20000 字节")
		}
		if status != "active" && status != "archived" {
			return nil, invalid("团队状态应为 active 或 archived")
		}
		updated := now()
		if version == 0 {
			if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_teams(id,tenant_id,version,name,description,status,created_at,updated_at) VALUES(?,?,1,?,?,?,?,?)`), id, p.TenantID, name, description, status, created, updated); err != nil {
				return nil, err
			}
		} else {
			r, err := tx.ExecContext(ctx, s.q(`UPDATE suma_teams SET name=?,description=?,status=?,version=version+1,updated_at=? WHERE tenant_id=? AND id=? AND version=?`), name, description, status, updated, p.TenantID, id, version)
			if err != nil {
				return nil, err
			}
			n, _ := r.RowsAffected()
			if n != 1 {
				return nil, conflict("团队已更新")
			}
		}
		return map[string]any{"id": id, "version": version + 1, "name": name, "description": description, "status": status, "created_at": created, "updated_at": updated}, nil
	})
}

func (s *Service) teamIdentity(ctx context.Context, q governanceQuery, p Principal, kind, id, project string) (string, error) {
	var name string
	if kind == "human" {
		if project != "" {
			return "", invalid("人类团队成员使用组织身份，不需要 project_id")
		}
		err := q.QueryRowContext(ctx, s.q(`SELECT u.name FROM suma_users u JOIN suma_org_members m ON m.user_id=u.id WHERE m.tenant_id=? AND u.id=?`), p.TenantID, id).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return "", invalid("用户不是当前组织的真实成员")
		}
		return name, err
	}
	if (kind != "agent" && kind != "tool") || project == "" {
		return "", invalid("代理或工具成员必须指定资源类型和项目")
	}
	err := q.QueryRowContext(ctx, s.q(`SELECT m.title FROM suma_members m JOIN suma_projects p ON p.id=m.project_id AND p.tenant_id=m.tenant_id WHERE m.tenant_id=? AND m.project_id=? AND m.id=? AND m.kind=?`), p.TenantID, project, id, kind).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", invalid("代理或工具资源不存在、类型不符或不属于当前组织项目")
	}
	return name, err
}

// BindHumanResourceTx validates the canonical account behind a project human
// resource. Resource registration never creates an account or a project grant.
func (s *Service) BindHumanResourceTx(ctx context.Context, tx *sql.Tx, p Principal, project, user string) error {
	if user == "" || len(user) > 64 {
		return invalid("人类资源必须绑定真实 user_id")
	}
	if err := s.lockOrganization(ctx, tx, p.TenantID); err != nil {
		return err
	}
	var err error
	p, err = s.liveOrganization(ctx, tx, p)
	if err != nil {
		return err
	}
	if !orgAdmin(p) {
		var actorRole string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`), project, p.ID).Scan(&actorRole); err != nil || actorRole == "viewer" || !validProjectRole(actorRole) {
			return forbidden()
		}
	}
	var organizationRole string
	var projectRole sql.NullString
	err = tx.QueryRowContext(ctx, s.q(`SELECT om.role,pm.role FROM suma_org_members om JOIN suma_users u ON u.id=om.user_id JOIN suma_projects p ON p.tenant_id=om.tenant_id AND p.id=? LEFT JOIN suma_project_members pm ON pm.project_id=p.id AND pm.user_id=om.user_id WHERE om.tenant_id=? AND om.user_id=?`), project, p.TenantID, user).Scan(&organizationRole, &projectRole)
	if errors.Is(err, sql.ErrNoRows) {
		return invalid("人类资源用户或项目不属于当前组织")
	}
	if err != nil {
		return err
	}
	if organizationRole == "owner" || organizationRole == "admin" || (projectRole.Valid && validProjectRole(projectRole.String)) {
		return nil
	}
	return invalid("人类资源用户尚未获得当前项目权限")
}

func (s *Service) TeamMembers(ctx context.Context, p Principal, team string) ([]map[string]any, error) {
	if _, err := s.liveOrganization(ctx, s.DB, p); err != nil {
		return nil, err
	}
	var id string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT id FROM suma_teams WHERE tenant_id=? AND id=?`), p.TenantID, team).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return nil, missing()
	} else if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT tm.id,tm.resource_kind,tm.resource_id,tm.project_id,tm.role,tm.version,tm.status,COALESCE(u.name,m.title,'') FROM suma_team_members tm LEFT JOIN suma_users u ON tm.resource_kind='human' AND u.id=tm.resource_id LEFT JOIN suma_members m ON tm.resource_kind<>'human' AND m.tenant_id=tm.tenant_id AND m.project_id=tm.project_id AND m.id=tm.resource_id AND m.kind=tm.resource_kind WHERE tm.tenant_id=? AND tm.team_id=? ORDER BY tm.resource_kind,tm.resource_id,tm.project_id`), p.TenantID, team)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, kind, resource, project, role, status, name string
		var version int
		if err = rows.Scan(&id, &kind, &resource, &project, &role, &version, &status, &name); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "team_id": team, "resource_kind": kind, "resource_id": resource, "project_id": project, "role": role, "version": version, "status": status, "name": name})
	}
	return out, rows.Err()
}

func (s *Service) SaveTeamMember(ctx context.Context, p Principal, team string, m map[string]any) (map[string]any, error) {
	if _, ok := m["expected_version"]; !ok {
		return nil, invalid("请提供 expected_version；首次添加使用 0")
	}
	return s.governanceWrite(ctx, p, "organization.team.member.save", team, m, requireOrgAdmin, func(tx *sql.Tx, p Principal) (map[string]any, error) {
		kind, resource, project, role, action := str(m, "resource_kind"), str(m, "resource_id"), str(m, "project_id"), str(m, "role"), str(m, "action")
		if action != "" && action != "remove" {
			return nil, invalid("无效团队成员操作")
		}
		var teamStatus string
		err := tx.QueryRowContext(ctx, s.q(`SELECT status FROM suma_teams WHERE tenant_id=? AND id=?`), p.TenantID, team).Scan(&teamStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, missing()
		}
		if err != nil {
			return nil, err
		}
		if teamStatus != "active" && action != "remove" {
			return nil, conflict("归档团队不允许新增或修改成员")
		}
		var id, oldStatus string
		var version int
		err = tx.QueryRowContext(ctx, s.q(`SELECT id,version,status FROM suma_team_members WHERE tenant_id=? AND team_id=? AND resource_kind=? AND resource_id=? AND project_id=?`), p.TenantID, team, kind, resource, project).Scan(&id, &version, &oldStatus)
		exists := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if number(m, "expected_version") != version {
			return nil, conflict("团队成员已更新，请刷新后重试")
		}
		if action == "remove" && (!exists || oldStatus != "active") {
			return nil, conflict("此团队成员已移除或不存在")
		}
		name := ""
		if action != "remove" {
			name, err = s.teamIdentity(ctx, tx, p, kind, resource, project)
			if err != nil {
				return nil, err
			}
			if role != "lead" && role != "member" && role != "observer" {
				return nil, invalid("团队角色应为 lead、member 或 observer")
			}
		}
		status := "active"
		if action == "remove" {
			status = "removed"
		}
		if !exists {
			id = newID()
			_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_team_members(id,tenant_id,team_id,resource_kind,resource_id,project_id,role,version,status) VALUES(?,?,?,?,?,?,?,1,?)`), id, p.TenantID, team, kind, resource, project, role, status)
		} else {
			r, updateErr := tx.ExecContext(ctx, s.q(`UPDATE suma_team_members SET role=?,status=?,version=version+1 WHERE tenant_id=? AND id=? AND version=?`), role, status, p.TenantID, id, version)
			err = updateErr
			if err == nil {
				n, _ := r.RowsAffected()
				if n != 1 {
					return nil, conflict("团队成员已更新")
				}
			}
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": id, "team_id": team, "resource_kind": kind, "resource_id": resource, "project_id": project, "name": name, "role": role, "status": status, "version": version + 1}, nil
	})
}

func (s *Service) projectGovernance(ctx context.Context, q governanceQuery, p Principal, project string) error {
	if project == "" {
		return invalid("请选择项目")
	}
	var tenant string
	err := q.QueryRowContext(ctx, s.q(`SELECT tenant_id FROM suma_projects WHERE id=?`), project).Scan(&tenant)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && tenant != p.TenantID) {
		return missing()
	}
	if err != nil {
		return err
	}
	if orgAdmin(p) {
		return nil
	}
	var role string
	if err = q.QueryRowContext(ctx, s.q(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`), project, p.ID).Scan(&role); errors.Is(err, sql.ErrNoRows) {
		return forbidden()
	} else if err != nil {
		return err
	}
	if role != "admin" {
		return forbidden()
	}
	return nil
}

func (s *Service) ProjectAccess(ctx context.Context, p Principal, project string) ([]map[string]any, error) {
	p, err := s.liveOrganization(ctx, s.DB, p)
	if err != nil {
		return nil, err
	}
	if err = s.projectGovernance(ctx, s.DB, p, project); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT u.id,u.name,u.email,pm.role,COALESCE(v.version,1),'active' FROM suma_project_members pm JOIN suma_users u ON u.id=pm.user_id JOIN suma_org_members om ON om.user_id=pm.user_id AND om.tenant_id=? LEFT JOIN suma_project_grant_versions v ON v.tenant_id=om.tenant_id AND v.project_id=pm.project_id AND v.user_id=pm.user_id WHERE pm.project_id=? UNION ALL SELECT u.id,u.name,u.email,'',v.version,'removed' FROM suma_project_grant_versions v JOIN suma_users u ON u.id=v.user_id JOIN suma_org_members om ON om.tenant_id=v.tenant_id AND om.user_id=v.user_id WHERE v.tenant_id=? AND v.project_id=? AND NOT EXISTS(SELECT 1 FROM suma_project_members pm WHERE pm.project_id=v.project_id AND pm.user_id=v.user_id) ORDER BY 2,1`), p.TenantID, project, p.TenantID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var user, name, email, role, status string
		var version int
		if err = rows.Scan(&user, &name, &email, &role, &version, &status); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"project_id": project, "user_id": user, "name": name, "email": email, "role": role, "version": version, "status": status})
	}
	return out, rows.Err()
}

func (s *Service) projectGrantVersion(ctx context.Context, tx *sql.Tx, tenant, project, user string) (int, error) {
	var version int
	err := tx.QueryRowContext(ctx, s.q(`SELECT version FROM suma_project_grant_versions WHERE tenant_id=? AND project_id=? AND user_id=?`), tenant, project, user).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		var n int
		err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_project_members WHERE project_id=? AND user_id=?`), project, user).Scan(&n)
		return n, err
	}
	return version, err
}

func (s *Service) setProjectGrantVersion(ctx context.Context, tx *sql.Tx, tenant, project, user string, version int) error {
	r, err := tx.ExecContext(ctx, s.q(`UPDATE suma_project_grant_versions SET version=? WHERE tenant_id=? AND project_id=? AND user_id=?`), version, tenant, project, user)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_project_grant_versions(tenant_id,project_id,user_id,version) VALUES(?,?,?,?)`), tenant, project, user, version)
	}
	return err
}

func (s *Service) SaveProjectAccess(ctx context.Context, p Principal, m map[string]any) (map[string]any, error) {
	if _, ok := m["expected_version"]; !ok {
		return nil, invalid("请提供 expected_version；首次授权使用 0")
	}
	project, user := str(m, "project_id"), str(m, "user_id")
	return s.governanceWrite(ctx, p, "organization.project.grant", project+":"+user, m, func(tx *sql.Tx, p Principal) error { return s.projectGovernance(ctx, tx, p, project) }, func(tx *sql.Tx, p Principal) (map[string]any, error) {
		role, action := str(m, "role"), str(m, "action")
		if action != "" && action != "remove" {
			return nil, invalid("无效项目授权操作")
		}
		if action != "remove" && !validProjectRole(role) {
			return nil, invalid("项目角色应为 admin、reviewer、developer 或 viewer")
		}
		var name, email string
		err := tx.QueryRowContext(ctx, s.q(`SELECT u.name,u.email FROM suma_users u JOIN suma_org_members m ON m.user_id=u.id WHERE m.tenant_id=? AND u.id=?`), p.TenantID, user).Scan(&name, &email)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, invalid("只能授权当前组织的真实成员")
		}
		if err != nil {
			return nil, err
		}
		// Project admins have no authority over organization owners/admins.
		var targetRole string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT role FROM suma_org_members WHERE tenant_id=? AND user_id=?`), p.TenantID, user).Scan(&targetRole); err != nil {
			return nil, err
		}
		if !orgAdmin(p) && (targetRole == "owner" || targetRole == "admin") {
			return nil, forbidden()
		}
		version, err := s.projectGrantVersion(ctx, tx, p.TenantID, project, user)
		if err != nil {
			return nil, err
		}
		if number(m, "expected_version") != version {
			return nil, conflict("项目授权已更新，请刷新后重试")
		}
		var active int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_project_members WHERE project_id=? AND user_id=?`), project, user).Scan(&active); err != nil {
			return nil, err
		}
		status := "active"
		if action == "remove" {
			if active != 1 {
				return nil, conflict("项目授权已移除或不存在")
			}
			status = "removed"
			if err = s.revokeProjectCredentials(ctx, tx, p.TenantID, project, user); err != nil {
				return nil, err
			}
			_, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`), project, user)
		} else if active == 1 {
			_, err = tx.ExecContext(ctx, s.q(`UPDATE suma_project_members SET role=? WHERE project_id=? AND user_id=?`), role, project, user)
		} else {
			_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_project_members(project_id,user_id,role) VALUES(?,?,?)`), project, user, role)
		}
		if err != nil {
			return nil, err
		}
		if err = s.setProjectGrantVersion(ctx, tx, p.TenantID, project, user, version+1); err != nil {
			return nil, err
		}
		return map[string]any{"project_id": project, "user_id": user, "name": name, "email": email, "role": role, "version": version + 1, "status": status}, nil
	})
}

func (s *Service) revokeProjectCredentials(ctx context.Context, tx *sql.Tx, tenant, project, user string) error {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT t.token_hash FROM suma_sessions t JOIN suma_token_scopes ts ON ts.token_hash=t.token_hash WHERE t.tenant_id=? AND t.user_id=? AND ts.project_id=?`), tenant, user, project)
	if err != nil {
		return err
	}
	hashes := []string{}
	for rows.Next() {
		var hash string
		if err = rows.Scan(&hash); err != nil {
			rows.Close()
			return err
		}
		hashes = append(hashes, hash)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, hash := range hashes {
		if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_token_scopes WHERE token_hash=?`), hash); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_sessions WHERE token_hash=?`), hash); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) revokeOrganizationProjectGrants(ctx context.Context, tx *sql.Tx, tenant, user string) error {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT pm.project_id FROM suma_project_members pm JOIN suma_projects p ON p.id=pm.project_id WHERE p.tenant_id=? AND pm.user_id=? UNION SELECT project_id FROM suma_project_grant_versions WHERE tenant_id=? AND user_id=?`), tenant, user, tenant, user)
	if err != nil {
		return err
	}
	projects := []string{}
	for rows.Next() {
		var project string
		if err = rows.Scan(&project); err != nil {
			rows.Close()
			return err
		}
		projects = append(projects, project)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, project := range projects {
		version, err := s.projectGrantVersion(ctx, tx, tenant, project, user)
		if err != nil {
			return err
		}
		if err = s.setProjectGrantVersion(ctx, tx, tenant, project, user, version+1); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_project_members WHERE user_id=? AND project_id IN (SELECT id FROM suma_projects WHERE tenant_id=?)`), user, tenant)
	return err
}
