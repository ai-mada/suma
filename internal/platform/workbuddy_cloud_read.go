package platform

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// GetCloudNode reopens the exact existing node; it does not contact WorkBuddy.
// Shared task facts are project-readable. The owner's delegation stays private.
func (w *WorkBuddyConnections) GetCloudNode(ctx context.Context, actor Actor, project, run, node string) (wc.CloudNodeView, error) {
	if w == nil || w.s == nil || !cloudReadID(project, 64) || !cloudReadID(run, 100) || !cloudReadID(node, 150) {
		return wc.CloudNodeView{}, cloudInvalid()
	}
	ctx, auth, close, err := w.s.planningAuthority(ctx, actor, project, run, purposeProjectRead)
	if err != nil {
		return wc.CloudNodeView{}, err
	}
	defer close()
	write, err := w.currentWrite(auth)
	if err != nil {
		return wc.CloudNodeView{}, err
	}
	n, err := w.cloudNodeTx(ctx, auth.tx, auth.target.tenantID, project, run, node)
	if err != nil {
		return wc.CloudNodeView{}, err
	}
	out := wc.CloudNodeView{ProjectID: project, RunID: run, NodeID: node, RunVersion: n.runVersion, NodeSpecSHA256: n.specHash, RequestSHA256: n.requestHash, Binding: n.binding, AllowedActions: []string{}}
	var delegationID string
	err = auth.tx.QueryRowContext(ctx, w.s.q(`SELECT id FROM suma_workbuddy_cloud_delegations WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`), auth.target.tenantID, project, run, node).Scan(&delegationID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return wc.CloudNodeView{}, err
	}
	var owned *cloudDelegationRow
	if err == nil {
		d, e := w.cloudDelegationTx(ctx, auth.tx, auth.target.tenantID, project, delegationID, true)
		if e != nil {
			return wc.CloudNodeView{}, e
		}
		if d.OwnerID == auth.currentPrincipal.ID {
			owned = &d
			out.Delegation = &d.CloudDelegation
		}
	}
	var submissionID string
	err = auth.tx.QueryRowContext(ctx, w.s.q(`SELECT id FROM suma_workbuddy_cloud_submissions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`), auth.target.tenantID, project, run, node).Scan(&submissionID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return wc.CloudNodeView{}, err
	}
	if err == nil {
		s, e := w.cloudSubmissionTx(ctx, auth.tx, auth.target.tenantID, project, submissionID, true)
		if e != nil {
			return wc.CloudNodeView{}, e
		}
		out.Submission = &s.CloudSubmission
	}
	if write {
		if delegationID == "" && out.Submission == nil && n.attempt == 0 && n.runVersion < wc.MaxVersion && (n.runState == "running" || n.runState == "paused") {
			connection, _, e := w.cloudGrantTx(ctx, auth.tx, auth.target.tenantID, project, auth.currentPrincipal.ID, n.binding.ConnectionID, n.binding.GrantGeneration, "")
			if e == nil {
				e = w.cloudBudgetTx(ctx, auth.tx, auth.target.tenantID, project)
			}
			allowed, fatal := cloudActionEligibility(e)
			if fatal != nil {
				return wc.CloudNodeView{}, fatal
			}
			if allowed && connection.Version < wc.MaxVersion {
				out.AllowedActions = append(out.AllowedActions, "delegate")
			}
		}
		if owned != nil {
			if owned.State == "active" && owned.Version < wc.MaxVersion {
				out.AllowedActions = append(out.AllowedActions, "revoke")
			}
			if out.Submission != nil && out.Submission.State == "task_known" && out.Submission.Version < wc.MaxVersion {
				_, _, e := w.cloudGrantTx(ctx, auth.tx, auth.target.tenantID, project, owned.OwnerID, owned.ConnectionID, owned.GrantGeneration, owned.ClientBindingSHA256)
				allowed, fatal := cloudActionEligibility(e)
				if fatal != nil {
					return wc.CloudNodeView{}, fatal
				}
				if allowed {
					out.AllowedActions = append(out.AllowedActions, "observe")
				}
			}
		}
	}
	if out.Validate() != nil {
		return wc.CloudNodeView{}, cloudConflict()
	}
	return out, nil
}

func cloudReadID(value string, max int) bool {
	if value == "" || len(value) > max || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.ContainsAny(value, "/\\") {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func cloudActionEligibility(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	var api *APIError
	if errors.As(err, &api) && api.Status >= 400 && api.Status < 500 {
		return false, nil
	}
	return false, err
}
