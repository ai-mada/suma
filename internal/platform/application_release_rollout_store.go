package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	rc "github.com/ai-mada/suma/internal/releasecontract"
)

const applicationRolloutInvalid = "Invalid application release rollout request"
const applicationRolloutChanged = "The application release rollout basis has changed"
const applicationRolloutBlocked = "The application release rollout is blocked"

func (s *Service) InitializeApplicationReleaseRollouts(ctx context.Context) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS suma_application_release_rollouts(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,set_id VARCHAR(64) NOT NULL,group_id VARCHAR(64) NOT NULL,parent_run_id VARCHAR(100) NOT NULL UNIQUE,snapshot_sha256 VARCHAR(64) NOT NULL,starter_hash VARCHAR(64) NOT NULL,starter_id VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_application_release_rollout_members(binding_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,rollout_id VARCHAR(64) NOT NULL,parent_run_id VARCHAR(100) NOT NULL,parent_node_id VARCHAR(150) NOT NULL,child_run_id VARCHAR(100) NOT NULL UNIQUE,plan_id VARCHAR(64) NOT NULL UNIQUE,UNIQUE(parent_run_id,parent_node_id))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	return nil
}
func applicationRolloutDigest(r ApplicationReleaseRollout) string {
	r.SnapshotSHA256 = ""
	h, _ := rc.CanonicalDigest(r)
	return h
}
func applicationRolloutPreviewDigest(r ApplicationReleaseRollout) string {
	r.ID, r.ParentRunID, r.SnapshotSHA256, r.StartedPreviewSHA256, r.CreatedBy, r.CreatedAt = "", "", "", "", "", ""
	r.Members = append([]ApplicationReleaseRolloutMember{}, r.Members...)
	for i := range r.Members {
		r.Members[i].BindingID = ""
	}
	h, _ := rc.CanonicalDigest(map[string]any{"protocol": "application-release-rollout-preview-v1", "rollout": r})
	return h
}
func (s *Service) applicationRolloutReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, setID, id string) (out ApplicationReleaseRollout, err error) {
	var raw, run, sha string
	err = tx.QueryRowContext(ctx, s.q(`SELECT parent_run_id,snapshot_sha256,payload FROM suma_application_release_rollouts WHERE tenant_id=? AND project_id=? AND set_id=? AND id=?`), p.TenantID, project, setID, id).Scan(&run, &sha, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(raw), &out) != nil || out.ID != id || out.ProjectID != project || out.ReleaseSetID != setID || out.ParentRunID != run || out.SnapshotSHA256 != sha || applicationRolloutDigest(out) != sha || len(out.Members) < 1 || len(out.Members) > 16 {
		return out, fmt.Errorf("application release rollout snapshot is inconsistent")
	}
	return out, nil
}
func (s *Service) applicationRolloutForBindingTx(ctx context.Context, tx *sql.Tx, tenant, project, binding string) (ApplicationReleaseRollout, ApplicationReleaseRolloutMember, string, string, error) {
	var set, id, hash, starter string
	err := tx.QueryRowContext(ctx, s.q(`SELECT r.set_id,r.id,r.starter_hash,r.starter_id FROM suma_application_release_rollout_members m JOIN suma_application_release_rollouts r ON r.id=m.rollout_id AND r.tenant_id=m.tenant_id AND r.project_id=m.project_id WHERE m.tenant_id=? AND m.project_id=? AND m.binding_id=?`), tenant, project, binding).Scan(&set, &id, &hash, &starter)
	if errors.Is(err, sql.ErrNoRows) {
		return ApplicationReleaseRollout{}, ApplicationReleaseRolloutMember{}, "", "", forbidden()
	}
	if err != nil {
		return ApplicationReleaseRollout{}, ApplicationReleaseRolloutMember{}, "", "", err
	}
	r, err := s.applicationRolloutReadTx(ctx, tx, Principal{TenantID: tenant}, project, set, id)
	if err != nil {
		return r, ApplicationReleaseRolloutMember{}, "", "", err
	}
	for _, m := range r.Members {
		if m.BindingID == binding {
			return r, m, hash, starter, nil
		}
	}
	return r, ApplicationReleaseRolloutMember{}, "", "", conflict(applicationRolloutChanged)
}
