package platform

import (
	"context"
	"database/sql"
	"strings"

	"github.com/ai-mada/suma/internal/dag"
)

func rolloutWorkflow(r ApplicationReleaseRollout) dag.Workflow {
	w := dag.Workflow{Name: "Application rollout " + r.ID, Nodes: []dag.Node{}}
	for _, m := range r.Members {
		deps := []string{}
		for _, id := range m.DependsOn {
			deps = append(deps, "app-"+id)
		}
		w.Nodes = append(w.Nodes, dag.Node{ID: m.ParentNodeID, Name: m.ApplicationName, Type: "subworkflow", DependsOn: deps, MaxRetries: 0, Idempotent: false, Config: map[string]any{"service_binding_id": m.BindingID}})
	}
	return w
}
func (s *Service) StartApplicationReleaseRollout(ctx context.Context, actor Actor, project, setID string, q ApplicationReleaseRolloutStartRequest, engine *dag.Engine) (out ApplicationReleaseRolloutView, err error) {
	requestCtx := ctx
	if actor.kind != actorBrowser {
		return out, forbidden()
	}
	if engine == nil || !handoffID(setID, false) || !validRolloutPreview(project, q.ApplicationReleaseRolloutPreviewRequest) || !contentDigest.MatchString(q.ExpectedPreviewSHA256) || !planningKey(q.IdempotencyKey) {
		return out, invalid(applicationRolloutInvalid)
	}
	// Recover original operations before stale preview checks, with current
	// browser authority. Stable IDs also close the concurrent first-call race.
	ctx, pre, close, err := s.planningAuthority(ctx, actor, project, setID, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	if err = s.releaseActorAdminTx(ctx, pre.tx, pre.target.tenantID, project, pre.currentPrincipal.ID); err != nil {
		close()
		return out, err
	}
	_, replay, e := s.commandReceipt(ctx, pre, q.IdempotencyKey, map[string]any{"application_release_rollout_start": setID, "request": q})
	if e != nil {
		close()
		return out, e
	}
	id := digest(stringify([]string{"application-release-rollout", pre.target.tenantID, project, pre.currentPrincipal.ID, q.IdempotencyKey}))[:32]
	if replay {
		r, e := s.applicationRolloutReadTx(ctx, pre.tx, pre.currentPrincipal, project, setID, id)
		if e == nil {
			out, e = s.applicationRolloutViewTx(ctx, pre.tx, pre.currentPrincipal, r, true)
		}
		close()
		return out, e
	}
	preview, e := s.applicationRolloutPreviewTx(ctx, pre.tx, pre.currentPrincipal, project, setID, q.ApplicationReleaseRolloutPreviewRequest)
	if e != nil {
		close()
		return out, e
	}
	p := pre.currentPrincipal
	close()
	ctx = requestCtx
	if !preview.CanStart {
		return out, conflict(applicationRolloutBlocked)
	}
	if preview.PreviewSHA256 != q.ExpectedPreviewSHA256 {
		return out, conflict(applicationRolloutChanged)
	}
	r := preview.Rollout
	r.ID = id
	r.ParentRunID = "rollout-" + id
	r.CreatedBy = p.ID
	r.CreatedAt = now()
	r.StartedPreviewSHA256 = preview.PreviewSHA256
	for i := range r.Members {
		r.Members[i].BindingID = digest(stringify([]string{r.ParentRunID, r.Members[i].ApplicationID}))[:32]
	}
	r.SnapshotSHA256 = applicationRolloutDigest(r)
	var auth *authorizedCommand
	var receipt string
	_, err = engine.CreateManagedRunBound(ctx, p.TenantID, project, r.ParentRunID, "rollout-start-"+id, rolloutWorkflow(r), func(tx *sql.Tx) error {
		var e error
		auth, e = s.releaseBrowserTx(ctx, tx, actor, project, true)
		if e != nil {
			return e
		}
		receipt, replay, e = s.commandReceipt(ctx, auth, q.IdempotencyKey, map[string]any{"application_release_rollout_start": setID, "request": q})
		if e != nil {
			return e
		}
		if replay {
			return nil
		}
		current, e := s.applicationRolloutPreviewTx(ctx, tx, auth.currentPrincipal, project, setID, q.ApplicationReleaseRolloutPreviewRequest)
		if e != nil {
			return e
		}
		if !current.CanStart {
			return conflict(applicationRolloutBlocked)
		}
		if current.PreviewSHA256 != q.ExpectedPreviewSHA256 {
			return conflict(applicationRolloutChanged)
		}
		return nil
	}, func(tx *sql.Tx, run string) error {
		if replay || run != r.ParentRunID {
			return conflict(applicationRolloutChanged)
		}
		if _, e := tx.ExecContext(ctx, s.q(`INSERT INTO suma_application_release_rollouts(id,tenant_id,project_id,set_id,group_id,parent_run_id,snapshot_sha256,starter_hash,starter_id,created_at,payload)VALUES(?,?,?,?,?,?,?,?,?,?,?)`), r.ID, p.TenantID, project, setID, r.PlanGroupID, r.ParentRunID, r.SnapshotSHA256, actor.credential.digest, p.ID, r.CreatedAt, stringify(r)); e != nil {
			return e
		}
		for _, m := range r.Members {
			if _, e := tx.ExecContext(ctx, s.q(`INSERT INTO suma_application_release_rollout_members(binding_id,tenant_id,project_id,rollout_id,parent_run_id,parent_node_id,child_run_id,plan_id)VALUES(?,?,?,?,?,?,?,?)`), m.BindingID, p.TenantID, project, r.ID, r.ParentRunID, m.ParentNodeID, m.ChildRunID, m.PlanID); e != nil {
				return e
			}
		}
		if e := s.saveCommandReceipt(ctx, auth, q.IdempotencyKey, receipt, r); e != nil {
			return e
		}
		return s.commandActivity(ctx, auth, "application_release_rollout.started", r.ID, q.Reason)
	})
	if err != nil {
		return out, err
	}
	return s.GetApplicationReleaseRollout(ctx, actor, project, setID, r.ID)
}

func (s *Service) PreviewApplicationReleaseRolloutResume(ctx context.Context, actor Actor, project, setID, id string, q ApplicationReleaseRolloutResumePreviewRequest) (out ApplicationReleaseRolloutResumePreview, err error) {
	if actor.kind != actorBrowser {
		return out, forbidden()
	}
	if q.ProjectID != project || q.ExpectedRunVersion < 1 || !contentDigest.MatchString(q.ExpectedRolloutSHA256) {
		return out, invalid(applicationRolloutInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	if err = s.releaseActorAdminTx(ctx, auth.tx, auth.target.tenantID, project, auth.currentPrincipal.ID); err != nil {
		return out, err
	}
	r, err := s.applicationRolloutReadTx(ctx, auth.tx, auth.currentPrincipal, project, setID, id)
	if err != nil {
		return out, err
	}
	return s.applicationRolloutResumePreviewTx(ctx, auth.tx, auth.currentPrincipal, r, q)
}
func (s *Service) applicationRolloutResumePreviewTx(ctx context.Context, tx *sql.Tx, p Principal, r ApplicationReleaseRollout, q ApplicationReleaseRolloutResumePreviewRequest) (out ApplicationReleaseRolloutResumePreview, err error) {
	view, err := s.applicationRolloutViewTx(ctx, tx, p, r, true)
	if err != nil {
		return out, err
	}
	if r.SnapshotSHA256 != q.ExpectedRolloutSHA256 || view.RunVersion != q.ExpectedRunVersion {
		return out, conflict(applicationRolloutChanged)
	}
	out = ApplicationReleaseRolloutResumePreview{RolloutID: r.ID, RolloutSHA256: r.SnapshotSHA256, RunVersion: view.RunVersion, Blockers: append([]ApplicationReleaseSetBlocker{}, view.Blockers...)}
	if view.State != "paused" {
		out.Blockers = append(out.Blockers, rolloutBlocker("rollout_not_paused", "", r.ID))
	}
	out.CanResume = len(out.Blockers) == 0
	out.PreviewSHA256 = digest(stringify(struct {
		Protocol string
		Rollout  string
		Version  int
		Items    []ApplicationReleaseRolloutItem
		Blockers []ApplicationReleaseSetBlocker
	}{"application-release-rollout-resume-v1", r.SnapshotSHA256, view.RunVersion, view.Items, out.Blockers}))
	return out, nil
}
func (s *Service) CommandApplicationReleaseRollout(ctx context.Context, actor Actor, project, setID, id string, q ApplicationReleaseRolloutCommandRequest, engine *dag.Engine) (out ApplicationReleaseRolloutView, err error) {
	if actor.kind != actorBrowser {
		return out, forbidden()
	}
	if engine == nil || q.ProjectID != project || q.ExpectedRunVersion < 1 || !contentDigest.MatchString(q.ExpectedRolloutSHA256) || !planningKey(q.IdempotencyKey) || strings.TrimSpace(q.Reason) != q.Reason || len(q.Reason) < 1 || len(q.Reason) > 2000 || q.Action != "pause" && q.Action != "resume" && q.Action != "cancel" || q.Action == "resume" && !contentDigest.MatchString(q.ExpectedPreviewSHA256) || q.Action != "resume" && q.ExpectedPreviewSHA256 != "" {
		return out, invalid(applicationRolloutInvalid)
	}
	var r ApplicationReleaseRollout
	var auth *authorizedCommand
	var receipt string
	var replay bool
	var permitTx *sql.Tx
	defer func() {
		if permitTx != nil {
			applicationRolloutCommandPermits.Delete(permitTx)
		}
	}()
	// Identity lookup is read-only; the command's current browser scope is
	// established before any scheduler lock or original receipt recovery.
	read, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	r, e = s.applicationRolloutReadTx(ctx, read, actor.Projection(), project, setID, id)
	read.Rollback()
	if e != nil {
		return out, e
	}
	_, err = engine.CommandWithAuthority(ctx, actor.credential.tenantID, project, r.ParentRunID, q.Action, "rollout-command-"+digest(q.IdempotencyKey)[:32], q.ExpectedRunVersion, map[string]any{"reason": q.Reason}, func(tx *sql.Tx) error {
		var e error
		auth, e = s.releaseBrowserTx(ctx, tx, actor, project, true)
		if e != nil {
			return e
		}
		receipt, replay, e = s.commandReceipt(ctx, auth, q.IdempotencyKey, map[string]any{"application_release_rollout_command": id, "request": q})
		if e != nil {
			return e
		}
		if replay {
			permitTx = tx
			applicationRolloutCommandPermits.Store(tx, applicationRolloutCommandPermit{Service: s, Authority: auth, ID: id, Action: q.Action})
			return nil
		}
		current, e := s.applicationRolloutReadTx(ctx, tx, auth.currentPrincipal, project, setID, id)
		if e != nil {
			return e
		}
		if current.SnapshotSHA256 != q.ExpectedRolloutSHA256 {
			return conflict(applicationRolloutChanged)
		}
		if q.Action == "resume" {
			preview, e := s.applicationRolloutResumePreviewTx(ctx, tx, auth.currentPrincipal, current, ApplicationReleaseRolloutResumePreviewRequest{ProjectID: project, ExpectedRunVersion: q.ExpectedRunVersion, ExpectedRolloutSHA256: q.ExpectedRolloutSHA256})
			if e != nil {
				return e
			}
			if !preview.CanResume {
				return conflict(applicationRolloutBlocked)
			}
			if preview.PreviewSHA256 != q.ExpectedPreviewSHA256 {
				return conflict(applicationRolloutChanged)
			}
			if _, e = tx.ExecContext(ctx, s.q(`UPDATE suma_application_release_rollouts SET starter_hash=?,starter_id=? WHERE tenant_id=? AND project_id=? AND id=?`), actor.credential.digest, auth.currentPrincipal.ID, auth.target.tenantID, project, id); e != nil {
				return e
			}
		}
		permitTx = tx
		applicationRolloutCommandPermits.Store(tx, applicationRolloutCommandPermit{Service: s, Authority: auth, ID: id, Action: q.Action})
		return nil
	}, func(tx *sql.Tx, run dag.Run) error {
		view, e := s.applicationRolloutViewTx(ctx, tx, auth.currentPrincipal, r, true)
		if e != nil {
			return e
		}
		if e = s.saveCommandReceipt(ctx, auth, q.IdempotencyKey, receipt, view); e != nil {
			return e
		}
		return s.commandActivity(ctx, auth, "application_release_rollout."+q.Action, id, q.Reason)
	})
	if err != nil {
		return out, err
	}
	return s.GetApplicationReleaseRollout(ctx, actor, project, setID, id)
}
