package platform

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
	"modernc.org/sqlite"
)

// This observes real driver boundaries; the two connections execute real SQL.
// Channels identify the exact waiting/acquisition points without timed sleeps.
type authorityBarrierDriver struct {
	attempted, acquired chan struct{}
	start, finish       sync.Once
}
type authorityBarrierConn struct {
	driver.Conn
	barrier *authorityBarrierDriver
}

func (d *authorityBarrierDriver) Open(name string) (driver.Conn, error) {
	c, err := (&sqlite.Driver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return &authorityBarrierConn{Conn: c, barrier: d}, nil
}
func (c *authorityBarrierConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	guard := strings.HasPrefix(query, "UPDATE suma_authority_sqlite_guard ")
	if guard {
		c.barrier.start.Do(func() { close(c.barrier.attempted) })
	}
	result, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
	if guard && err == nil {
		c.barrier.finish.Do(func() { close(c.barrier.acquired) })
	}
	return result, err
}
func (c *authorityBarrierConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}
func (c *authorityBarrierConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}

func authorityDatabasePath(t *testing.T, s *Service) string {
	t.Helper()
	var path string
	if err := s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAuthorityLogoutBothOrdersTwoConnections(t *testing.T) {
	for _, order := range []string{"command-first", "logout-first", "expiry-first"} {
		t.Run(order, func(t *testing.T) {
			s, _, token, project := authorityFixture(t)
			ctx := context.Background()
			actor, err := s.AuthenticateActor(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			path := authorityDatabasePath(t, s)
			if order != "command-first" {
				db, e := store.Open("sqlite", path)
				if e != nil {
					t.Fatal(e)
				}
				defer db.Close()
				other := &Service{DB: db, Dialect: "sqlite"}
				if order == "logout-first" {
					if e = logoutCredentialForTest(ctx, other, token); e != nil {
						t.Fatal(e)
					}
				} else {
					at, e := other.authorityNow(ctx, db)
					if e != nil {
						t.Fatal(e)
					}
					tx, e := db.BeginTx(ctx, nil)
					if e != nil {
						t.Fatal(e)
					}
					for _, q := range []struct{ sql, key string }{{`UPDATE suma_sessions SET expires_at=? WHERE token_hash=?`, digest(token)}, {`UPDATE suma_session_families SET expires_at=? WHERE id=?`, actor.credential.familyID}, {`UPDATE suma_credential_slots SET expires_at=? WHERE id=?`, actor.credential.slotID}} {
						if _, e = tx.Exec(q.sql, at.Format(time.RFC3339Nano), q.key); e != nil {
							tx.Rollback()
							t.Fatal(e)
						}
					}
					if e = tx.Commit(); e != nil {
						t.Fatal(e)
					}
				}
				tx, e := s.DB.BeginTx(ctx, nil)
				if e != nil {
					t.Fatal(e)
				}
				defer tx.Rollback()
				defer s.endAuthorityTx(tx)
				_, e = s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.projection.TenantID, projectID: project}, purposeProjectWrite)
				requireAuthorityStatus(t, e, 401)
				var count int
				if e = tx.QueryRow(`SELECT COUNT(*) FROM suma_operations WHERE operation_id='rejected-operation'`).Scan(&count); e != nil || count != 0 {
					t.Fatal("rejected source produced operation")
				}
				return
			}
			barrier := &authorityBarrierDriver{attempted: make(chan struct{}), acquired: make(chan struct{})}
			name := "authority_barrier_" + newID()
			sql.Register(name, barrier)
			db, err := sql.Open(name, path)
			if err != nil {
				t.Fatal(err)
			}
			db.SetMaxOpenConns(1)
			defer db.Close()
			other := &Service{DB: db, Dialect: "sqlite"}
			target, err := other.ResolveLogoutTarget(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			bounded, tx, done, err := s.beginAuthorityTx(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer done()
			defer tx.Rollback()
			a, err := s.authorizeTx(bounded, tx, actor, commandTarget{tenantID: actor.projection.TenantID, projectID: project}, purposeProjectWrite)
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { result <- other.Logout(ctx, target) }()
			select {
			case <-barrier.attempted:
			case <-bounded.Done():
				t.Fatal("logout never attempted writer coordination")
			}
			select {
			case <-barrier.acquired:
				t.Fatal("logout acquired writer while guarded command held it")
			default:
			}
			hash, _, err := s.commandReceipt(bounded, a, "accepted-operation", map[string]any{"accepted": true})
			if err != nil {
				t.Fatal(err)
			}
			if err = s.saveCommandReceipt(bounded, a, "accepted-operation", hash, map[string]any{"ok": true}); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			done()
			select {
			case err = <-result:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(6 * time.Second):
				t.Fatal("logout did not complete after command commit")
			}
			select {
			case <-barrier.acquired:
			default:
				t.Fatal("no actual logout writer acquisition")
			}
			_, err = s.AuthenticateActor(ctx, token)
			requireAuthorityStatus(t, err, 401)
			var count int
			if err = s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations WHERE operation_id='accepted-operation'`).Scan(&count); err != nil || count != 1 {
				t.Fatal("accepted command missing after ordered logout")
			}
		})
	}
}

func TestAuthorityFirstBudgetAndBlockedDeadline(t *testing.T) {
	s, _, token, project := authorityFixture(t)
	ctx := context.Background()
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	path := authorityDatabasePath(t, s)
	bounded, tx, done, err := s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	defer tx.Rollback()
	target := commandTarget{tenantID: actor.projection.TenantID, projectID: project}
	first, err := s.authorizeTx(bounded, tx, actor, target, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.authorizeTx(bounded, tx, actor, target, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	if first.deadline != second.deadline {
		t.Fatal("second proof renewed transaction budget")
	}
	otherDB, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	other := &Service{DB: otherDB, Dialect: "sqlite"}
	var previousBusy int
	if err = otherDB.QueryRow(`PRAGMA busy_timeout`).Scan(&previousBusy); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 75*time.Millisecond)
	defer cancel()
	started := time.Now()
	otherCtx, otherTx, otherDone, err := other.beginAuthorityTx(short)
	if err != nil {
		t.Fatal(err)
	}
	defer otherDone()
	defer otherTx.Rollback()
	_, err = other.authorizeTx(otherCtx, otherTx, actor, target, purposeProjectWrite)
	if err == nil {
		t.Fatal("blocked deadline accepted a command")
	}
	if time.Since(started) > time.Second {
		t.Fatal("lock wait exceeded short parent's remaining budget")
	}
	otherTx.Rollback()
	otherDone()
	var restoredBusy int
	if err = otherDB.QueryRow(`PRAGMA busy_timeout`).Scan(&restoredBusy); err != nil || restoredBusy != previousBusy {
		t.Fatalf("connection wait setting leaked after canceled transaction: %d -> %d (%v)", previousBusy, restoredBusy, err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	done()
	if _, ok := s.authorityBudgets.Load(tx); ok {
		t.Fatal("finished transaction retained budget timer")
	}
}
