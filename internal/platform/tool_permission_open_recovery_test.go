package platform

import (
	"context"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

// A lost Open ACK is recovered as the same read of the original operation.
// The controlled pause changes no attempt, lease, profile or dispatch authority.
func TestCodexToolPermissionPausedOpenRecoversOriginalWithoutNewGrant(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		name := "private"
		if pooled {
			name = "pool"
		}
		t.Run(name, func(t *testing.T) {
			h, binding := newCodexToolPermissionFixture(t, pooled)
			ctx := context.Background()
			originalRequest := codexToolRequest174(binding, "original-open-unknown-174")
			original, err := h.open(ctx, originalRequest)
			if err != nil {
				t.Fatal(err)
			}
			grantRequest := codexToolRequest174(binding, "original-granted-174")
			grant, err := h.open(ctx, grantRequest)
			if err != nil {
				t.Fatal(err)
			}
			grant = codexToolDecide174(t, h, grant, "accept")
			readBounds := func() (lease, deadline, fence int64) {
				t.Helper()
				if err := h.f.s.DB.QueryRow(`SELECT lease_until,fence FROM dag_nodes WHERE run_id=? AND id=?`, h.assignment.RunID, h.assignment.NodeID).Scan(&lease, &fence); err != nil {
					t.Fatal(err)
				}
				if err := h.f.s.DB.QueryRow(`SELECT deadline_at FROM dag_retry_windows WHERE run_id=? AND node_id=?`, h.assignment.RunID, h.assignment.NodeID).Scan(&deadline); err != nil {
					t.Fatal(err)
				}
				return
			}
			lease, deadline, fence := readBounds()
			if _, err = h.f.s.DB.Exec(`UPDATE dag_runs SET state='paused' WHERE id=?`, h.assignment.RunID); err != nil {
				t.Fatal(err)
			}
			recovered, err := h.open(ctx, originalRequest)
			if err != nil || recovered.ID != original.ID || recovered.Version != original.Version || recovered.State != "pending" || recovered.Decision != nil || stringify(recovered.Request) != stringify(originalRequest) {
				t.Fatalf("paused source blocked recovery of its committed original Open: %v", err)
			}
			changed := originalRequest
			changedNative := *originalRequest.Codex
			changedNative.Reason = "different original body"
			changed.Codex = &changedNative
			if _, err = h.open(ctx, changed); err == nil {
				t.Fatal("same key accepted a different body")
			}
			if _, err = h.open(ctx, codexToolRequest174(binding, "new-paused-open-174")); err == nil {
				t.Fatal("paused run created a new permission")
			}
			recoveredGrant, err := h.open(ctx, grantRequest)
			if err != nil || recoveredGrant.ID != grant.ID || recoveredGrant.Version != grant.Version || recoveredGrant.State != "superseded" || recoveredGrant.Decision == nil || len(recoveredGrant.AllowedActions) != 0 {
				t.Fatalf("receipt recovery renewed a paused grant: %v", err)
			}
			observation := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: originalRequest.Codex.ParamsSHA256, RequestResolved: true, ItemStatus: "declined"}
			var cleaned pc.Permission
			if pooled {
				cleaned, err = h.f.s.ObservePoolToolPermission(ctx, h.token, h.f.project, h.assignment, recovered.ID, newID(), observation)
			} else {
				cleaned, err = h.f.s.ObserveToolPermission(ctx, h.actor, h.f.project, h.assignment, recovered.ID, newID(), observation)
			}
			if err != nil || cleaned.ID != original.ID || cleaned.Observation == nil || cleaned.Observation.DecisionObserved || cleaned.Observation.ReplyStarted || cleaned.Observation.ReplyWritten || !cleaned.Observation.RequestResolved {
				t.Fatalf("original no-human cleanup could not be saved after recovery: %v", err)
			}
			var count int
			if err = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_tool_permissions WHERE run_id=? AND node_id=? AND attempt=?`, h.assignment.RunID, h.assignment.NodeID, h.assignment.Attempt).Scan(&count); err != nil || count != 2 {
				t.Fatal("recovery created a second permission or lost a record", count, err)
			}
			afterLease, afterDeadline, afterFence := readBounds()
			if afterLease != lease || afterDeadline != deadline || afterFence != fence {
				t.Fatal("read recovery renewed original execution bounds")
			}
		})
	}
}
