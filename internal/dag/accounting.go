package dag

import (
	"context"
	"database/sql"
	"encoding/json"
)

// AccountingAttempt is the server-fixed execution identity and immutable node
// specification. Admission passes the next attempt/fence; final facts pass the
// exact attempt/fence already persisted by the scheduler.
type AccountingAttempt struct {
	TenantID, ProjectID, RunID, NodeID, WorkerID string
	Attempt                                      int
	Fence                                        int64
	Spec                                         Node
}

// AttemptAccounting extends the scheduler transaction with service-owned usage
// accounting. Implementations use tx only, never Engine calls or network I/O.
// Check locks and checks without charging; Reserve cannot return a quota deny
// after Check holds those locks. Unknown executions retain financial holds.
type AttemptAccounting interface {
	CheckAttemptTx(context.Context, *sql.Tx, AccountingAttempt) (bool, error)
	ReserveAttemptTx(context.Context, *sql.Tx, AccountingAttempt) error
	RecordResultTx(context.Context, *sql.Tx, AccountingAttempt, Result) error
	RecordResolutionTx(context.Context, *sql.Tx, AccountingAttempt, string) error
}

const accountingBlockedReason = "Model token or cost allowance is unavailable"

func (t *transaction) accountingAttempt(r *runState, n *nodeState, worker string, next bool) (AccountingAttempt, bool, error) {
	if t.e.AttemptAccounting == nil {
		return AccountingAttempt{}, false, nil
	}
	_, _, bound, err := ToolBinding(n.spec)
	if err != nil || !bound {
		return AccountingAttempt{}, false, err
	}
	// The hook receives its own specification snapshot, so a map mutation cannot
	// change the scheduler's fixed workflow or a later accounting invocation.
	var spec Node
	if err = json.Unmarshal([]byte(encoded(n.spec)), &spec); err != nil {
		return AccountingAttempt{}, false, err
	}
	ref := AccountingAttempt{TenantID: r.TenantID, ProjectID: r.ProjectID, RunID: r.ID, NodeID: n.ID, WorkerID: worker, Attempt: n.Attempt, Fence: n.Fence, Spec: spec}
	if next {
		ref.Attempt++
		ref.Fence++
	}
	return ref, true, nil
}

func (t *transaction) checkAttemptAccounting(r *runState, n *nodeState, worker string) (bool, error) {
	ref, bound, err := t.accountingAttempt(r, n, worker, true)
	if err != nil || !bound {
		return err == nil, err
	}
	allowed, err := t.e.AttemptAccounting.CheckAttemptTx(t.ctx, t.tx, ref)
	if err != nil {
		return false, err
	}
	reason, event := "", "accounting_unblocked"
	if !allowed {
		reason, event = accountingBlockedReason, "accounting_blocked"
	}
	if n.Reason != reason && (!allowed || n.Reason == accountingBlockedReason) {
		n.Reason = reason
		if err = t.event(r, n.ID, event, map[string]any{"reason": reason}); err != nil {
			return false, err
		}
		if err = t.save(r); err != nil {
			return false, err
		}
	}
	return allowed, nil
}

func (t *transaction) reserveAttemptAccounting(r *runState, n *nodeState, worker string) error {
	ref, bound, err := t.accountingAttempt(r, n, worker, true)
	if err != nil || !bound {
		return err
	}
	return t.e.AttemptAccounting.ReserveAttemptTx(t.ctx, t.tx, ref)
}

func (t *transaction) recordAttemptAccounting(r *runState, n *nodeState, result Result) error {
	ref, bound, err := t.accountingAttempt(r, n, result.WorkerID, false)
	if err != nil || !bound {
		return err
	}
	return t.e.AttemptAccounting.RecordResultTx(t.ctx, t.tx, ref, result)
}

func (t *transaction) resolveAttemptAccounting(r *runState, n *nodeState, decision string) error {
	ref, bound, err := t.accountingAttempt(r, n, n.WorkerID, false)
	if err != nil || !bound {
		return err
	}
	return t.e.AttemptAccounting.RecordResolutionTx(t.ctx, t.tx, ref, decision)
}
