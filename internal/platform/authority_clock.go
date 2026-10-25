package platform

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/ai-mada/suma/internal/store"
)

const authorityTransactionBudget = 5 * time.Second

func authoritySetRetry(err error) bool {
	var api *APIError
	return errors.As(err, &api) && api.Code == "authority_retry"
}

type authorityTxBudget struct {
	deadline time.Time
	timer    *time.Timer
	ctx      context.Context
	cancel   context.CancelFunc
}

func (s *Service) txAuthorityBudget(ctx context.Context, tx *sql.Tx) *authorityTxBudget {
	s.authorityBudgetMu.Lock()
	defer s.authorityBudgetMu.Unlock()
	deadline := time.Now().Add(authorityTransactionBudget)
	if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
		deadline = parent
	}
	budgetCtx, budgetCancel := context.WithDeadline(ctx, deadline)
	candidate := &authorityTxBudget{deadline: deadline, ctx: budgetCtx, cancel: budgetCancel}
	actual, loaded := s.authorityBudgets.LoadOrStore(tx, candidate)
	budget := actual.(*authorityTxBudget)
	if loaded {
		budgetCancel()
	}
	if !loaded {
		budget.timer = time.AfterFunc(time.Until(deadline), func() { tx.Rollback(); s.endAuthorityTx(tx) })
	}
	return budget
}

// External transaction owners (notably DAG callbacks) defer this at the owner
// boundary, after commit/rollback. Repeated proofs never restart the budget.
func (s *Service) endAuthorityTx(tx *sql.Tx) {
	s.authorityBudgetMu.Lock()
	value, ok := s.authorityBudgets.LoadAndDelete(tx)
	s.authorityBudgetMu.Unlock()
	if ok {
		budget := value.(*authorityTxBudget)
		if budget.timer != nil {
			budget.timer.Stop()
		}
		budget.cancel()
	}
}

func (s *Service) authorityNow(ctx context.Context, q authorityQuery) (time.Time, error) {
	query := `SELECT CAST(strftime('%s','now') AS TEXT)`
	if s.Dialect == "postgres" {
		query = `SELECT CAST(FLOOR(EXTRACT(EPOCH FROM clock_timestamp()))::bigint AS TEXT)`
	}
	if s.Dialect == "oceanbase" {
		query = `SELECT CAST(UNIX_TIMESTAMP() AS CHAR)`
	}
	var raw string
	if err := q.QueryRowContext(ctx, query).Scan(&raw); err != nil {
		return time.Time{}, err
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	return time.Unix(seconds, 0).UTC(), err
}

// Start every interactive short transaction here. The deadline governs lock
// waiting, queries and commit; cancellation causes database/sql to roll back.
func (s *Service) beginAuthorityTx(ctx context.Context) (context.Context, *sql.Tx, context.CancelFunc, error) {
	bounded, cancel := context.WithTimeout(ctx, authorityTransactionBudget)
	var options *sql.TxOptions
	if s.Dialect == "postgres" || s.Dialect == "oceanbase" {
		options = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	scope, err := store.BeginScopedTx(bounded, s.DB, s.Dialect, options)
	if err != nil {
		cancel()
		return bounded, nil, func() {}, err
	}
	tx := scope.Tx
	s.txAuthorityBudget(bounded, tx)
	cleanup := func() { s.endAuthorityTx(tx); scope.Close(); cancel() }
	if s.Dialect == "postgres" {
		_, err = tx.ExecContext(bounded, `SET LOCAL lock_timeout='5s'`)
		if err == nil {
			_, err = tx.ExecContext(bounded, `SET LOCAL statement_timeout='5s'`)
		}
	}
	if err != nil {
		cleanup()
		return bounded, nil, func() {}, err
	}
	return bounded, tx, cleanup, nil
}

// SQLite cannot upgrade a stale read snapshot into a writer. Establish its
// existing single-writer serialization before collecting candidate lock sets.
func (s *Service) authorityWriter(ctx context.Context, tx *sql.Tx) error {
	if s.Dialect != "sqlite" {
		return nil
	}
	wait := authorityTransactionBudget
	if deadline, ok := ctx.Deadline(); ok {
		wait = time.Until(deadline)
	}
	if wait <= 0 {
		return context.DeadlineExceeded
	}
	if wait > authorityTransactionBudget {
		wait = authorityTransactionBudget
	}
	var previousBusy int
	if err := tx.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&previousBusy); err != nil {
		return err
	}
	millis := wait.Milliseconds()
	if millis < 1 {
		millis = 1
	}
	// modernc's SQLite busy handler may outlive context cancellation. Bound
	// its own connection-local wait to the *remaining* first transaction budget.
	if _, err := tx.ExecContext(ctx, `PRAGMA busy_timeout=`+strconv.FormatInt(millis, 10)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE suma_authority_sqlite_guard SET sequence=sequence+1 WHERE id=1`)
	if err != nil {
		return err
	}
	// Once this transaction holds SQLite's writer, subsequent statements do
	// not wait on another writer. Restore on the same live transaction now;
	// owned-connection cleanup also covers timeout/cancellation before this point.
	_, err = tx.ExecContext(ctx, `PRAGMA busy_timeout=`+strconv.Itoa(previousBusy))
	return err
}
