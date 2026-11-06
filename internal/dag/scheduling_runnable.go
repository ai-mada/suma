package dag

import (
	"context"
	"database/sql"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// RunnableHook runs only in the scheduler transaction. It performs no network
// I/O and may not acquire additional human authority after project/run locks.
type RunnableHook struct {
	BindAssignmentTx func(context.Context, *sql.Tx, *Assignment) error
	ValidateReplayTx func(context.Context, *sql.Tx, Assignment) error
	// RecordPrivateClaimTx observes a newly recorded private claim, including
	// an empty poll. Receipt replays and shared Pool claims never call it.
	RecordPrivateClaimTx func(context.Context, *sql.Tx) error
}

func runnableHook(hooks []*RunnableHook) (*RunnableHook, error) {
	if len(hooks) > 1 {
		return nil, ErrInvalid
	}
	if len(hooks) == 0 {
		return nil, nil
	}
	return hooks[0], nil
}

func WorkBuddyCloudNode(n Node) bool {
	return n.Type == wc.NodeType && n.Config["plugin"] == wc.Plugin && n.Config["workflow"] == wc.Protocol && len(n.Command) == 0
}

func cloudAssignment(a Assignment) bool {
	return WorkBuddyCloudNode(Node{Type: a.Type, Config: a.Config, Command: a.Command})
}

func (t *transaction) bindRunnable(a *Assignment, hook *RunnableHook) error {
	if a == nil {
		return nil
	}
	before := fingerprint(a)
	if hook != nil && hook.BindAssignmentTx != nil {
		if err := hook.BindAssignmentTx(t.ctx, t.tx, a); err != nil {
			return err
		}
	}
	copy := *a
	copy.WorkBuddyCloud = nil
	if fingerprint(copy) != before {
		return ErrConflict
	}
	if cloudAssignment(*a) {
		if a.WorkBuddyCloud == nil || a.WorkBuddyCloud.Validate() != nil || hook == nil || hook.BindAssignmentTx == nil {
			return ErrConflict
		}
	} else if a.WorkBuddyCloud != nil {
		return ErrConflict
	}
	return nil
}

func (t *transaction) validateRunnableReplay(a *Assignment, hook *RunnableHook) error {
	if a == nil {
		return nil
	}
	if !cloudAssignment(*a) {
		if a.WorkBuddyCloud != nil {
			return ErrConflict
		}
		return nil
	}
	if a.WorkBuddyCloud == nil || a.WorkBuddyCloud.Validate() != nil || hook == nil || hook.ValidateReplayTx == nil {
		return ErrConflict
	}
	stored, err := ReadExternalAssignmentTx(t.ctx, t.tx, t.e.dialect, a.TenantID, a.ProjectID, a.RunID, a.NodeID, a.Attempt)
	if err != nil {
		return err
	}
	if stored.Binding != *a.WorkBuddyCloud || !wc.SameAssignmentIdentity(stored.Assignment, externalAssignmentRef(*a)) {
		return ErrConflict
	}
	return hook.ValidateReplayTx(t.ctx, t.tx, *a)
}
