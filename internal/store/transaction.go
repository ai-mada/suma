package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strconv"
	"sync"
	"time"
)

// TxScope owns a transaction and any connection-local settings it borrows.
// SQLite busy_timeout survives rollback, so a transaction which changes it must
// retain the actual connection until the original value has been restored.
type TxScope struct {
	Tx         *sql.Tx
	conn       *sql.Conn
	sqliteBusy int
	once       sync.Once
	err        error
}

func BeginScopedTx(ctx context.Context, db *sql.DB, dialect string, options *sql.TxOptions) (*TxScope, error) {
	scope := &TxScope{}
	var err error
	if dialect == "sqlite" {
		scope.conn, err = db.Conn(ctx)
		if err == nil {
			err = scope.conn.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&scope.sqliteBusy)
		}
		if err == nil {
			scope.Tx, err = scope.conn.BeginTx(ctx, options)
		}
	} else {
		scope.Tx, err = db.BeginTx(ctx, options)
	}
	if err != nil {
		if scope.conn != nil {
			scope.conn.Close()
		}
		return nil, err
	}
	return scope, nil
}

// Close is idempotent after either commit or rollback. Restoration uses an
// independent short context: the request may already have been cancelled.
// A connection which cannot be restored is discarded instead of being returned
// to the pool with another request's settings. No database-wide setting changes.
func (s *TxScope) Close() error {
	s.once.Do(func() {
		if err := s.Tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			s.err = err
		}
		if s.conn != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			// A failed deferred-constraint COMMIT can finish sql.Tx while
			// SQLite keeps its transaction open. Clean only this retained
			// connection. SAVEPOINT also makes the full ROLLBACK valid after
			// a successful commit/rollback, without matching driver errors.
			_, err := s.conn.ExecContext(ctx, `SAVEPOINT suma_scope_cleanup`)
			if err == nil {
				_, err = s.conn.ExecContext(ctx, `ROLLBACK`)
			}
			if err == nil {
				_, err = s.conn.ExecContext(ctx, `PRAGMA busy_timeout=`+strconv.Itoa(s.sqliteBusy))
			}
			cancel()
			if err != nil {
				// database/sql recognizes this sentinel and evicts the physical
				// connection, including when the callback itself performs no IO.
				_ = s.conn.Raw(func(any) error { return driver.ErrBadConn })
				s.err = errors.Join(s.err, err)
			}
			s.err = errors.Join(s.err, s.conn.Close())
		}
	})
	return s.err
}
