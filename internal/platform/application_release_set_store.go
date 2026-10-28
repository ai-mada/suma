package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/store"
)

const (
	applicationReleaseSetInvalid = "Invalid application release set request"
	applicationReleaseSetChanged = "The application release set or reviewed deployment basis has changed"
	applicationReleaseSetBlocked = "Complete the exact application release set evidence before preparing deployment"
)

func (s *Service) InitializeApplicationReleaseSets(ctx context.Context) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS suma_application_release_sets(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,label VARCHAR(64) NOT NULL,label_key VARCHAR(64) NOT NULL,snapshot_sha256 VARCHAR(64) NOT NULL,member_count INTEGER NOT NULL,created_by VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,label_key))`,
		`CREATE TABLE IF NOT EXISTS suma_application_release_set_plan_groups(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,set_id VARCHAR(64) NOT NULL,set_sha256 VARCHAR(64) NOT NULL,snapshot_sha256 VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_application_release_set_plan_bindings(plan_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,group_id VARCHAR(64) NOT NULL,set_id VARCHAR(64) NOT NULL,set_sha256 VARCHAR(64) NOT NULL,application_id VARCHAR(64) NOT NULL,application_version_id VARCHAR(64) NOT NULL,application_version_sha256 VARCHAR(64) NOT NULL,candidate_id VARCHAR(64) NOT NULL,artifact_id VARCHAR(64) NOT NULL,artifact_sha256 VARCHAR(64) NOT NULL,plan_sha256 VARCHAR(64) NOT NULL)`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_app_release_set_page", "suma_application_release_sets", "tenant_id", "project_id", "created_at", "id"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_app_release_groups", "suma_application_release_set_plan_groups", "tenant_id", "project_id", "set_id", "created_at", "id")
}
func applicationReleaseSetDigest(set ApplicationReleaseSet) string {
	set.SnapshotSHA256 = ""
	return digest(stringify(set))
}
func applicationReleaseSetDraftDigest(set ApplicationReleaseSet) string {
	set.ID = ""
	set.CreatedBy = ""
	set.CreatedAt = ""
	set.RegisteredPreviewSHA256 = ""
	return applicationReleaseSetDigest(set)
}
func applicationReleaseSetGroupDigest(group ApplicationReleaseSetPlanGroup) string {
	group.SnapshotSHA256 = ""
	return digest(stringify(group))
}

// These read-fact functions require the caller's current project authority and
// governance locks. They never construct an Actor or grant permission themselves.
func (s *Service) applicationReleaseSetReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string) (out ApplicationReleaseSet, err error) {
	var raw, sha, label string
	var count int
	err = tx.QueryRowContext(ctx, s.q(`SELECT label,snapshot_sha256,member_count,payload FROM suma_application_release_sets WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id).Scan(&label, &sha, &count, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return
	}
	if out.ID != id || out.ProjectID != project || out.Label != label || len(out.Members) != count || count < 1 || count > 16 || out.Members == nil || out.Dependencies == nil || out.Cycles == nil || sha != out.SnapshotSHA256 || !contentDigest.MatchString(sha) || applicationReleaseSetDigest(out) != sha {
		return out, conflict(applicationReleaseSetChanged)
	}
	return
}
func (s *Service) applicationReleaseSetGroupReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, setID, id string) (out ApplicationReleaseSetPlanGroup, err error) {
	var raw, sha, setSHA string
	err = tx.QueryRowContext(ctx, s.q(`SELECT set_sha256,snapshot_sha256,payload FROM suma_application_release_set_plan_groups WHERE tenant_id=? AND project_id=? AND set_id=? AND id=?`), p.TenantID, project, setID, id).Scan(&setSHA, &sha, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return
	}
	if out.ID != id || out.ProjectID != project || out.ReleaseSetID != setID || out.ReleaseSetSHA256 != setSHA || out.SnapshotSHA256 != sha || applicationReleaseSetGroupDigest(out) != sha || out.Bindings == nil || len(out.Bindings) < 1 || len(out.Bindings) > 16 {
		return out, conflict(applicationReleaseSetChanged)
	}
	return
}
