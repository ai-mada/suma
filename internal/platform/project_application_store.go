package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/store"
)

const (
	applicationInvalid      = "Invalid project application request"
	applicationChanged      = "The application or reviewed code version has changed; reload before deciding"
	applicationScope        = "Application code scopes require current governed targets and explicit shared ownership"
	applicationTeam         = "The application owner must be a current project-qualified team"
	applicationSource       = "The application source requires an applied repository receipt and current independent accepted evidence"
	applicationInspection   = "The fixed source inspection is incomplete or does not match the reviewed commit"
	applicationDependencies = "Unresolved or partial dependency observations require explicit acknowledgement"
)

// This is current-domain initialization only. It neither imports generic release
// records nor changes existing business data or an older database declaration.
func (s *Service) InitializeProjectApplications(ctx context.Context) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS suma_project_applications(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,app_key VARCHAR(64) NOT NULL,name VARCHAR(200) NOT NULL,state VARCHAR(20) NOT NULL,parent_id VARCHAR(64) NOT NULL,owner_team_id VARCHAR(64) NOT NULL,current_version_id VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,app_key))`,
		`CREATE TABLE IF NOT EXISTS suma_application_versions(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,application_id VARCHAR(64) NOT NULL,sequence INTEGER NOT NULL,label VARCHAR(64) NOT NULL,label_key VARCHAR(64) NOT NULL,snapshot_sha256 VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,application_id,sequence),UNIQUE(tenant_id,project_id,application_id,label_key))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_application_page", "suma_project_applications", "tenant_id", "project_id", "created_at", "id"); err != nil {
		return err
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_application_version_page", "suma_application_versions", "tenant_id", "project_id", "application_id", "created_at", "id"); err != nil {
		return err
	}
	return s.InitializeApplicationGraph(ctx)
}

func decodeProjectApplication(raw, project, id string, version int, key, state, parent, team, current string) (out ProjectApplication, err error) {
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return
	}
	if out.ID != id || out.ProjectID != project || out.Version != version || out.Key != key || out.State != state || out.ParentApplicationID != parent || out.OwnerTeamID != team || out.CurrentRegisteredVersionID != current || out.CodeScopes == nil {
		err = conflict(applicationChanged)
	}
	return
}
func applicationVersionDigest(out ApplicationCodeVersion) string {
	out.SnapshotSHA256 = ""
	return digest(stringify(out))
}
func decodeApplicationVersion(raw, project, app, id string, sequence int, label, sha string) (out ApplicationCodeVersion, err error) {
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return
	}
	if out.ID != id || out.ProjectID != project || out.ApplicationID != app || out.Sequence != sequence || out.Label != label || out.SnapshotSHA256 != sha || !contentDigest.MatchString(sha) || applicationVersionDigest(out) != sha || out.Sources == nil || out.Dependencies == nil || out.Observations == nil {
		err = conflict(applicationChanged)
	}
	return
}
func (s *Service) projectApplicationTx(ctx context.Context, auth *authorizedCommand, project, id string) (out ProjectApplication, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return
	}
	var raw, key, state, parent, team, current string
	var version int
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version,app_key,state,parent_id,owner_team_id,current_version_id,payload FROM suma_project_applications WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, id).Scan(&version, &key, &state, &parent, &team, &current, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return
	}
	return decodeProjectApplication(raw, project, id, version, key, state, parent, team, current)
}
func (s *Service) applicationVersionTx(ctx context.Context, auth *authorizedCommand, project, app, id string) (out ApplicationCodeVersion, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return
	}
	var raw, label, sha string
	var sequence int
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT sequence,label,snapshot_sha256,payload FROM suma_application_versions WHERE tenant_id=? AND project_id=? AND application_id=? AND id=?`), auth.target.tenantID, project, app, id).Scan(&sequence, &label, &sha, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return
	}
	return decodeApplicationVersion(raw, project, app, id, sequence, label, sha)
}
func (s *Service) saveProjectApplicationTx(ctx context.Context, auth *authorizedCommand, out ProjectApplication, previous int) error {
	if err := assertProjectCommand(auth.tx, auth, out.ProjectID); err != nil {
		return err
	}
	if previous == 0 {
		_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_project_applications(id,tenant_id,project_id,version,app_key,name,state,parent_id,owner_team_id,current_version_id,created_at,updated_at,payload)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`), out.ID, auth.target.tenantID, out.ProjectID, out.Version, out.Key, out.Name, out.State, out.ParentApplicationID, out.OwnerTeamID, out.CurrentRegisteredVersionID, out.CreatedAt, out.UpdatedAt, stringify(out))
		return err
	}
	res, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_project_applications SET version=?,app_key=?,name=?,state=?,parent_id=?,owner_team_id=?,current_version_id=?,updated_at=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), out.Version, out.Key, out.Name, out.State, out.ParentApplicationID, out.OwnerTeamID, out.CurrentRegisteredVersionID, out.UpdatedAt, stringify(out), auth.target.tenantID, out.ProjectID, out.ID, previous)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return conflict(applicationChanged)
	}
	return err
}
