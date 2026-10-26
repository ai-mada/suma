package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/store"
)

const (
	handoffBasisChanged     = "Task handoff basis has changed; create a new proposal"
	handoffRunUnsafe        = "The linked execution is active or has an unknown outcome"
	handoffRecipientInvalid = "The handoff recipient is not an active human with project write access"
	handoffTeamInvalid      = "The recipient is not an active member or lead of the selected team"
	handoffPending          = "This task already has a pending handoff"
	handoffPermission       = "Only the designated recipient or original sender may perform this handoff action"
	handoffVersion          = "The handoff has changed; reload before deciding"
	handoffAcknowledge      = "Confirm that changing the owner invalidates previous execution and acceptance evidence"
	handoffSameOwner        = "The recipient is already the task owner"
)

func (s *Service) MigrateTaskHandoffs(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS suma_task_handoffs(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_task_handoff_pending(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,handoff_id VARCHAR(64) NOT NULL UNIQUE,PRIMARY KEY(tenant_id,project_id,task_id))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return err
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_task_handoff_history", "suma_task_handoffs", "tenant_id", "project_id", "task_id", "created_at", "id")
}

func handoffID(value string, empty bool) bool {
	if value == "" {
		return empty
	}
	if len(value) > 64 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 33 || r == '/' || r == '\\' || r == 127 {
			return false
		}
	}
	return true
}

func handoffText(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 4000 && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func (s *Service) taskHandoffTx(ctx context.Context, auth *authorizedCommand, project, id string) (TaskHandoff, error) {
	var out TaskHandoff
	if err := assertProjectRead(auth, project); err != nil {
		return out, err
	}
	var raw, state string
	var version int
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT version,state,payload FROM suma_task_handoffs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, id).Scan(&version, &state, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if out.ID != id || out.ProjectID != project || out.Version != version || out.State != state {
		return out, conflict(handoffVersion)
	}
	return out, nil
}

func (s *Service) handoffWriteUserTx(ctx context.Context, auth *authorizedCommand, project, user string) (bool, error) {
	var role string
	var projectRole sql.NullString
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT om.role,pm.role FROM suma_users u JOIN suma_org_members om ON om.user_id=u.id LEFT JOIN suma_project_members pm ON pm.user_id=u.id AND pm.project_id=? WHERE om.tenant_id=? AND u.id=?`), project, auth.target.tenantID, user).Scan(&role, &projectRole)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return role == "owner" || role == "admin" || projectRole.Valid && (projectRole.String == "admin" || projectRole.String == "reviewer" || projectRole.String == "developer"), nil
}

func (s *Service) handoffRecipientTx(ctx context.Context, auth *authorizedCommand, project, resource string) (TaskHandoffRecipient, error) {
	out := TaskHandoffRecipient{Teams: []TaskHandoffTeam{}}
	r, err := scanRecord(auth.tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_members WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, resource))
	if errors.Is(err, sql.ErrNoRows) {
		return out, invalid(handoffRecipientInvalid)
	}
	if err != nil {
		return out, err
	}
	user, _ := r.Data["user_id"].(string)
	if r.Kind != "human" || r.Status != "active" || !handoffID(user, false) {
		return out, invalid(handoffRecipientInvalid)
	}
	allowed, err := s.handoffWriteUserTx(ctx, auth, project, user)
	if err != nil {
		return out, err
	}
	if !allowed {
		return out, invalid(handoffRecipientInvalid)
	}
	var canonical string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT resource_id FROM suma_human_resources WHERE tenant_id=? AND project_id=? AND user_id=?`), auth.target.tenantID, project, user).Scan(&canonical)
	if errors.Is(err, sql.ErrNoRows) || err == nil && canonical != resource {
		return out, invalid(handoffRecipientInvalid)
	}
	if err != nil {
		return out, err
	}
	out.ResourceID, out.ResourceVersion, out.UserID, out.Name = r.ID, r.Version, user, r.Title
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT t.id,t.name,t.version,m.version FROM suma_teams t JOIN suma_team_members m ON m.tenant_id=t.tenant_id AND m.team_id=t.id WHERE t.tenant_id=? AND t.status='active' AND m.resource_kind='human' AND m.resource_id=? AND m.project_id='' AND m.status='active' AND (m.role='member' OR m.role='lead') ORDER BY t.name,t.id`), auth.target.tenantID, user)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var team TaskHandoffTeam
		if err = rows.Scan(&team.TeamID, &team.Name, &team.TeamVersion, &team.MembershipVersion); err != nil {
			return out, err
		}
		out.Teams = append(out.Teams, team)
	}
	return out, rows.Err()
}

func handoffSelectedTeam(recipient TaskHandoffRecipient, team string) (TaskHandoffTeam, bool) {
	if team == "" {
		return TaskHandoffTeam{}, true
	}
	for _, item := range recipient.Teams {
		if item.TeamID == team {
			return item, true
		}
	}
	return TaskHandoffTeam{}, false
}

func (s *Service) handoffPendingTx(ctx context.Context, auth *authorizedCommand, project, task string) (bool, error) {
	var id string
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT handoff_id FROM suma_task_handoff_pending WHERE tenant_id=? AND project_id=? AND task_id=?`), auth.target.tenantID, project, task).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
