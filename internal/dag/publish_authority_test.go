package dag

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func publicationAuthorityFixture(t *testing.T, e *Engine) {
	t.Helper()
	for _, statement := range []string{
		`CREATE TABLE publish_authority_marker (id TEXT PRIMARY KEY)`,
		`CREATE TABLE publish_authority_outbox (workflow_id TEXT NOT NULL, version INTEGER NOT NULL)`,
		`CREATE TRIGGER publish_authority_scheduler BEFORE UPDATE ON dag_scheduler_projects
		 WHEN NOT EXISTS (SELECT 1 FROM publish_authority_marker WHERE id='accepted')
		 BEGIN SELECT RAISE(ABORT, 'scheduler lock preceded authority'); END`,
		`CREATE TRIGGER publish_authority_workflow BEFORE INSERT ON dag_workflows
		 WHEN NOT EXISTS (SELECT 1 FROM publish_authority_marker WHERE id='accepted')
		 BEGIN SELECT RAISE(ABORT, 'publication did not share authority transaction'); END`,
		`CREATE TRIGGER publish_authority_emit AFTER INSERT ON dag_workflows
		 BEGIN INSERT INTO publish_authority_outbox(workflow_id,version) VALUES(NEW.id,NEW.version); END`,
	} {
		if _, err := e.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
}

func publicationAuthorityAccept(tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO publish_authority_marker(id) VALUES('accepted')`)
	return err
}

func publicationCounts(t *testing.T, e *Engine) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range []string{"dag_workflows", "dag_operations", "dag_events", "dag_scheduler_projects", "publish_authority_marker", "publish_authority_outbox"} {
		var count int
		if err := e.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		counts[table] = count
	}
	return counts
}

func publicationStored(t *testing.T, e *Engine) [3]string {
	t.Helper()
	var result [3]string
	if err := e.db.QueryRowContext(ctx, `SELECT spec FROM dag_workflows WHERE tenant_id='t' AND project_id='p' ORDER BY version LIMIT 1`).Scan(&result[0]); err != nil {
		t.Fatal(err)
	}
	if err := e.db.QueryRowContext(ctx, `SELECT fingerprint,result_id FROM dag_operations WHERE tenant_id='t' AND project_id='p' AND scope='publish_workflow' AND operation_id='publish'`).Scan(&result[1], &result[2]); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPublishWithAuthorityAllowsReceiptReplay(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "publish-authority.db"))
	publicationAuthorityFixture(t, e)
	w := Workflow{Name: "authorized", Nodes: []Node{{ID: "work", Type: "task", Command: []string{"original"}}}}
	first, err := e.PublishWithAuthority(ctx, "t", "p", w, "publish", publicationAuthorityAccept)
	if err != nil || first.ID == "" || first.Version != 1 {
		t.Fatalf("authorized publication failed: %+v %v", first, err)
	}
	stored := publicationStored(t, e)
	if _, err := e.db.ExecContext(ctx, `DELETE FROM publish_authority_marker`); err != nil {
		t.Fatal(err)
	}
	second, err := e.PublishWithAuthority(ctx, "t", "p", w, "publish", publicationAuthorityAccept)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("authorized replay changed immutable workflow: first=%+v replay=%+v err=%v", first, second, err)
	}
	if after := publicationStored(t, e); after != stored {
		t.Fatalf("replay changed workflow bytes or receipt: before=%q after=%q", stored, after)
	}
	want := map[string]int{"dag_workflows": 1, "dag_operations": 1, "dag_events": 0, "dag_scheduler_projects": 1, "publish_authority_marker": 1, "publish_authority_outbox": 1}
	if got := publicationCounts(t, e); !reflect.DeepEqual(got, want) {
		t.Fatalf("replay repeated publication effects: got=%v want=%v", got, want)
	}
}

func TestPublishWithAuthorityRejectsFreshWithoutEffects(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "publish-denied.db"))
	publicationAuthorityFixture(t, e)
	before := publicationCounts(t, e)
	denied := errors.New("current credential denied")
	w := Workflow{Name: "rejected", Nodes: []Node{{ID: "work", Type: "task"}}}
	out, err := e.PublishWithAuthority(ctx, "t", "p", w, "publish", func(*sql.Tx) error {
		return fmt.Errorf("publish authorization: %w", denied)
	})
	if !errors.Is(err, denied) {
		t.Fatalf("fresh publication bypassed authority: got=%+v err=%v", out, err)
	}
	if !reflect.DeepEqual(out, Workflow{}) {
		t.Fatalf("denied publication returned workflow: %+v", out)
	}
	if after := publicationCounts(t, e); !reflect.DeepEqual(after, before) {
		t.Fatalf("denied publication changed persistent state: before=%v after=%v", before, after)
	}
}

func TestPublishWithAuthorityRejectsReplayBeforeReceipt(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "publish-replay-denied.db"))
	publicationAuthorityFixture(t, e)
	w := Workflow{Name: "retained", Nodes: []Node{{ID: "work", Type: "task", Command: []string{"original"}}}}
	first, err := e.PublishWithAuthority(ctx, "t", "p", w, "publish", publicationAuthorityAccept)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.ExecContext(ctx, `DELETE FROM publish_authority_marker`); err != nil {
		t.Fatal(err)
	}
	before, stored := publicationCounts(t, e), publicationStored(t, e)
	denied := errors.New("credential revoked")
	for _, name := range []string{w.Name, "conflicting request"} {
		request := w
		request.Name = name
		out, err := e.PublishWithAuthority(ctx, "t", "p", request, "publish", func(*sql.Tx) error { return denied })
		if !errors.Is(err, denied) || !reflect.DeepEqual(out, Workflow{}) {
			t.Fatalf("denied replay returned prior success or inspected receipt first: got=%+v err=%v", out, err)
		}
	}
	if after := publicationStored(t, e); after != stored {
		t.Fatalf("denied replay changed original workflow or receipt: before=%q after=%q", stored, after)
	}
	if after := publicationCounts(t, e); !reflect.DeepEqual(after, before) {
		t.Fatalf("denied replay changed persistent state: before=%v after=%v", before, after)
	}
	if first.Version != 1 || stored[2] != first.ID+":1" {
		t.Fatalf("original receipt was not retained: %+v %q", first, stored[2])
	}
}

func TestPublishWithAuthorityRollsBackCallbackError(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "publish-callback-error.db"))
	publicationAuthorityFixture(t, e)
	before := publicationCounts(t, e)
	callbackErr := errors.New("authority proof failed")
	w := Workflow{Name: "rollback", Nodes: []Node{{ID: "work", Type: "task"}}}
	_, err := e.PublishWithAuthority(ctx, "t", "p", w, "publish", func(tx *sql.Tx) error {
		if err := publicationAuthorityAccept(tx); err != nil {
			return err
		}
		return fmt.Errorf("current proof: %w", callbackErr)
	})
	if !errors.Is(err, callbackErr) {
		t.Fatalf("callback error identity lost: %v", err)
	}
	if after := publicationCounts(t, e); !reflect.DeepEqual(after, before) {
		t.Fatalf("callback failure did not roll back its transaction: before=%v after=%v", before, after)
	}
}

func TestPublishWithAuthorityRollsBackCallbackOnVersionConflict(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "publish-conflict.db"))
	w := Workflow{ID: "fixed", Name: "immutable", Nodes: []Node{{ID: "work", Type: "task"}}}
	if _, err := e.PublishWithOperation(ctx, "t", "p", w, "publish"); err != nil {
		t.Fatal(err)
	}
	publicationAuthorityFixture(t, e)
	before, stored := publicationCounts(t, e), publicationStored(t, e)
	w.Version = 1 // Compiles, then conflicts with the required next immutable version.
	callbackRan := false
	_, err := e.PublishWithAuthority(ctx, "t", "p", w, "conflict", func(tx *sql.Tx) error {
		callbackRan = true
		return publicationAuthorityAccept(tx)
	})
	if !errors.Is(err, ErrConflict) || !callbackRan {
		t.Fatalf("expected a post-authority immutable-version conflict: callback=%v err=%v", callbackRan, err)
	}
	if after := publicationCounts(t, e); !reflect.DeepEqual(after, before) {
		t.Fatalf("later publication failure committed callback effects: before=%v after=%v", before, after)
	}
	if after := publicationStored(t, e); after != stored {
		t.Fatalf("conflict changed fixed workflow or receipt: before=%q after=%q", stored, after)
	}
}

func TestPublishWithAuthorityNilKeepsKernelReceipt(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "publish-nil-authority.db"))
	w := Workflow{ID: "kernel", Name: "kernel", Nodes: []Node{{ID: "work", Type: "task"}}}
	first, err := e.PublishWithAuthority(ctx, "t", "p", w, "publish", nil)
	if err != nil || first.Version != 1 {
		t.Fatalf("nil callback publication failed: %+v %v", first, err)
	}
	replayed, err := e.PublishWithOperation(ctx, "t", "p", w, "publish")
	if err != nil || !reflect.DeepEqual(replayed, first) {
		t.Fatalf("authority variant changed existing kernel receipt: first=%+v replay=%+v err=%v", first, replayed, err)
	}
	next, err := e.Publish(ctx, "t", "p", w)
	if err != nil || next.Version != 2 {
		t.Fatalf("kernel publication no longer advances versions: %+v %v", next, err)
	}
	if _, err := e.PublishWithAuthority(ctx, "t", "p", w, "", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("authority variant dropped operation ID requirement: %v", err)
	}
}
