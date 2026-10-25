package platform

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"modernc.org/sqlite"
)

// The real SQLite transaction commits; only its acknowledgement is replaced.
// No production hook, query result or credential authority is substituted.
type candidateCommitFaultDriver struct{ armed atomic.Bool }
type candidateCommitFaultConn struct {
	driver.Conn
	fault  *candidateCommitFaultDriver
	active *candidateCommitFaultTx
}
type candidateCommitFaultTx struct {
	driver.Tx
	conn      *candidateCommitFaultConn
	published bool
}

var errCandidateCommitACK = errors.New("candidate commit acknowledgement unavailable")

func (d *candidateCommitFaultDriver) Open(name string) (driver.Conn, error) {
	c, err := (&sqlite.Driver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return &candidateCommitFaultConn{Conn: c, fault: d}, nil
}
func (c *candidateCommitFaultConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	result, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args)
	if err == nil && c.active != nil && strings.HasPrefix(q, "UPDATE suma_candidate_transfers SET storage_path=") {
		c.active.published = true
	}
	return result, err
}
func (c *candidateCommitFaultConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
}
func (c *candidateCommitFaultConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	out := &candidateCommitFaultTx{Tx: tx, conn: c}
	c.active = out
	return out, nil
}
func (t *candidateCommitFaultTx) Commit() error {
	err := t.Tx.Commit()
	t.conn.active = nil
	if err == nil && t.published && t.conn.fault.armed.CompareAndSwap(true, false) {
		return errCandidateCommitACK
	}
	return err
}
func (t *candidateCommitFaultTx) Rollback() error { t.conn.active = nil; return t.Tx.Rollback() }

func TestCandidateTransferCommitUnknownRetainsVerifiedBlobAndReceiptRecovery(t *testing.T) {
	f, token, r := candidateTransferFixture(t)
	ctx := context.Background()
	actor := domainCActor(t, f.s, token)
	b, err := f.s.StartCandidateTransfer(ctx, actor, f.project, r)
	if err != nil {
		t.Fatal(err)
	}
	driverName := "candidate_commit_ack_" + newID()
	fault := &candidateCommitFaultDriver{}
	fault.armed.Store(true)
	sql.Register(driverName, fault)
	db, err := sql.Open(driverName, authorityDatabasePath(t, f.s)+"?_pragma=busy_timeout(10000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	f.s.DB = db
	op := newID()
	err = f.s.PutCandidateBundle(ctx, actor, f.project, b.TransferID, r.Assignment, op, r.SizeBytes, strings.NewReader("candidate bytes"))
	if !errors.Is(err, errCandidateCommitACK) {
		t.Fatalf("commit fault not observed: %v", err)
	}
	var path string
	if err = db.QueryRow(`SELECT storage_path FROM suma_candidate_transfers WHERE id=?`, b.TransferID).Scan(&path); err != nil || path == "" {
		t.Fatalf("commit did not publish reference: %s %v", path, err)
	}
	if _, err = os.Stat(filepath.Join(f.s.DataDir, "release-artifacts", filepath.FromSlash(path))); err != nil {
		t.Fatalf("uncertain committed blob was deleted: %v", err)
	}
	if err = f.s.PutCandidateBundle(ctx, actor, f.project, b.TransferID, r.Assignment, op, r.SizeBytes, strings.NewReader("candidate bytes")); err != nil {
		t.Fatalf("same operation could not recover committed acknowledgement: %v", err)
	}
}

func TestCandidateTransferConcurrentIdenticalPutRecoversAfterPathPublication(t *testing.T) {
	f, token, r := candidateTransferFixture(t)
	ctx := context.Background()
	actor := domainCActor(t, f.s, token)
	b, err := f.s.StartCandidateTransfer(ctx, actor, f.project, r)
	if err != nil {
		t.Fatal(err)
	}
	op := newID()
	reader := &candidateMutationReader{Reader: strings.NewReader("candidate bytes"), mutate: func() {
		// Both initial authorization snapshots saw an empty path. The second
		// request commits while the first is outside its authority transaction.
		if err := f.s.PutCandidateBundle(ctx, actor, f.project, b.TransferID, r.Assignment, op, r.SizeBytes, strings.NewReader("candidate bytes")); err != nil {
			t.Fatal(err)
		}
	}}
	if err = f.s.PutCandidateBundle(ctx, actor, f.project, b.TransferID, r.Assignment, op, r.SizeBytes, reader); err != nil {
		t.Fatalf("identical concurrent operation failed recovery: %v", err)
	}
	files, err := os.ReadDir(filepath.Join(f.s.DataDir, "release-artifacts", "blobs"))
	if err != nil || len(files) != 1 {
		t.Fatalf("duplicate bytes retained: %d %v", len(files), err)
	}
}
