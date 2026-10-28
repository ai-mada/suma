package platform

import (
	"context"
	"encoding/json"
)

func (s *Service) PreviewApplicationReleaseSet(ctx context.Context, actor Actor, project string, q ApplicationReleaseSetPreviewRequest) (out ApplicationReleaseSetPreview, err error) {
	if !applicationReleaseSetInputValid(project, q) {
		return out, invalid(applicationReleaseSetInvalid)
	}
	ctx, auth, close, err := s.applicationRead(ctx, actor, project, "")
	if err != nil {
		return out, err
	}
	defer close()
	return s.applicationReleaseSetPreviewTx(ctx, auth.tx, auth.currentPrincipal, project, q)
}
func (s *Service) RegisterApplicationReleaseSet(ctx context.Context, actor Actor, project string, q ApplicationReleaseSetRegisterRequest) (out ApplicationReleaseSetView, err error) {
	if actor.kind != actorBrowser {
		return out, forbidden()
	}
	if !applicationReleaseSetInputValid(project, q.ApplicationReleaseSetPreviewRequest) || !planningKey(q.IdempotencyKey) || !contentDigest.MatchString(q.ExpectedPreviewSHA256) {
		return out, invalid(applicationReleaseSetInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, q.IdempotencyKey, map[string]any{"application_release_set_register": q})
	if err != nil {
		return out, err
	}
	if replay {
		var original ApplicationReleaseSetView
		if err = json.Unmarshal([]byte(hash), &original); err != nil {
			return
		}
		set, e := s.applicationReleaseSetReadTx(ctx, auth.tx, auth.currentPrincipal, project, original.Set.ID)
		if e != nil {
			return out, e
		}
		if original.Set.SnapshotSHA256 != set.SnapshotSHA256 {
			return out, conflict(applicationReleaseSetChanged)
		}
		return s.applicationReleaseSetViewTx(ctx, auth, set)
	}
	preview, err := s.applicationReleaseSetPreviewTx(ctx, auth.tx, auth.currentPrincipal, project, q.ApplicationReleaseSetPreviewRequest)
	if err != nil {
		return out, err
	}
	if !preview.CanRegister {
		return out, conflict(applicationReleaseSetBlocked)
	}
	if preview.PreviewSHA256 != q.ExpectedPreviewSHA256 {
		return out, conflict(applicationReleaseSetChanged)
	}
	var duplicate int
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_application_release_sets WHERE tenant_id=? AND project_id=? AND label_key=?`), auth.target.tenantID, project, digest(q.Label)).Scan(&duplicate); err != nil {
		return
	}
	if duplicate != 0 {
		return out, conflict(applicationReleaseSetChanged)
	}
	set := preview.Set
	set.ID = newID()
	set.CreatedBy = auth.currentPrincipal.ID
	set.CreatedAt = now()
	set.RegisteredPreviewSHA256 = preview.PreviewSHA256
	set.SnapshotSHA256 = applicationReleaseSetDigest(set)
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_application_release_sets(id,tenant_id,project_id,label,label_key,snapshot_sha256,member_count,created_by,created_at,payload)VALUES(?,?,?,?,?,?,?,?,?,?)`), set.ID, auth.target.tenantID, project, set.Label, digest(set.Label), set.SnapshotSHA256, len(set.Members), set.CreatedBy, set.CreatedAt, stringify(set)); err != nil {
		return
	}
	out = ApplicationReleaseSetView{Set: set, BasisCurrent: true, Blockers: []ApplicationReleaseSetBlocker{}, AllowedActions: []string{"prepare_deployment"}, PlanGroups: []ApplicationReleaseSetPlanGroupView{}, PlanGroupsComplete: true}
	if err = s.commandActivity(ctx, auth, "application_release_set.registered", set.ID, q.Reason); err != nil {
		return
	}
	if err = s.saveCommandReceipt(ctx, auth, q.IdempotencyKey, hash, out); err != nil {
		return
	}
	err = auth.tx.Commit()
	return
}
