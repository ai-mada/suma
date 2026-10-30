package platform

import (
	"context"
	"database/sql"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// validateCloudDispatchBindingTx is shared by central execute and exact fact
// lookup. It reads the selected grant from the original scheduler snapshot,
// never from a replacement client field or latest-delegation selection.
func (w *WorkBuddyConnections) validateCloudDispatchBindingTx(ctx context.Context, tx *sql.Tx, tenant, project string, d wc.CloudDelegation, a cc.AssignmentRef) error {
	snapshot, err := dag.ReadExternalAssignmentTx(ctx, tx, w.s.Dialect, tenant, project, a.RunID, a.NodeID, a.Attempt)
	if err != nil {
		return err
	}
	binding, err := d.AssignmentBinding()
	if err != nil || snapshot.Binding.DelegationID != d.ID || snapshot.Binding.DelegationSHA256 != binding.DelegationSHA256 || snapshot.Binding.DelegationVersion > d.Version || !wc.SameAssignmentIdentity(snapshot.Assignment, a) {
		return cloudConflict()
	}
	return nil
}
