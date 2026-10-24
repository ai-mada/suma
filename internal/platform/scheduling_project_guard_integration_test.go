package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

// A real WAL writer in a different organization must not turn display-only
// candidate reads into write transactions. No deadline or auth data is forged.
func TestCandidateDisplaySnapshotDoesNotWaitForUnrelatedOrganizationWriter(t *testing.T) {
	f := newFixture(t)
	created := f.call("POST", "/organizations", map[string]any{"name": "独立写入组织", "idempotency_key": newID()}, 201)
	var file string
	if err := f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&file); err != nil {
		t.Fatal(err)
	}
	other, err := store.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	for _, kind := range []string{"detail", "list"} {
		t.Run(kind, func(t *testing.T) {
			writer, err := other.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = writer.Exec(`UPDATE suma_organizations SET name=name WHERE id=?`, created["id"]); err != nil {
				writer.Rollback()
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if kind == "detail" {
					_, e := f.s.ReleaseCandidate(ctx, f.user, f.project, "missing-candidate")
					var api *APIError
					if errors.As(e, &api) && api.Status == 404 {
						e = nil
					}
					done <- e
				} else {
					_, _, _, e := f.s.ReleaseCandidates(ctx, f.user, f.project, 20, "")
					done <- e
				}
			}()
			select {
			case err = <-done:
				writer.Rollback()
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(300 * time.Millisecond):
				writer.Rollback()
				<-done
				t.Fatal("display-only candidate read waited for an unrelated SQLite writer")
			}
		})
	}
}

// A concurrent durable execution invalidation must win before an uploader can
// create a new session. This is a controlled lease-loss fault, not a fabricated
// artifact or successful task result.
func TestArtifactAdmissionRechecksAssignmentAfterConcurrentSQLiteWriter(t *testing.T) {
	f, token, ref, _, request := artifactProtocolFixture(t)
	ctx := context.Background()
	var file string
	if err := f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&file); err != nil {
		t.Fatal(err)
	}
	other, err := store.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	writer, err := other.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if err = dag.GuardProjectTx(ctx, writer, "sqlite", f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Exec(`UPDATE dag_nodes SET state='reconciling',lease_until=0 WHERE run_id=? AND id=?`, ref.RunID, ref.NodeID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, e := f.s.StartArtifactUpload(ctx, f.user, f.project, token, request); done <- e }()
	select {
	case e := <-done:
		t.Fatalf("admission escaped the concurrent writer before its execution invalidation committed: %v", e)
	case <-time.After(50 * time.Millisecond):
	}
	if err = writer.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-done:
		releaseWantStatus(t, e, 409)
	case <-time.After(3 * time.Second):
		t.Fatal("admission did not resume after writer committed")
	}
	var count int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_artifact_uploads WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale assignment acquired artifact session: %d %v", count, err)
	}
}

// Two actual connections and separately authorized organizations share only
// event allocation at transaction tail. SQLite still serializes its writers;
// this test does not claim OceanBase/PostgreSQL row-level concurrency.
func TestCrossOrganizationManagedTransactionsKeepGlobalEventCursorContiguous(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	org, err := f.s.CreateOrganization(ctx, f.user, map[string]any{"name": "第二并发组织", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := f.s.SwitchOrganization(ctx, f.user, map[string]any{"organization_id": org["id"], "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	project, err := f.s.CreateProject(ctx, actor, map[string]any{"title": "第二并发项目", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	var file string
	if err = f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&file); err != nil {
		t.Fatal(err)
	}
	other, err := store.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	second, err := dag.New(other, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	engines := []*dag.Engine{f.e, second}
	actors := []Principal{f.user, actor}
	projects := []string{f.project, project.ID}
	workflows := make([]dag.Workflow, 2)
	for i := range engines {
		workflows[i], err = engines[i].Publish(ctx, actors[i].TenantID, projects[i], dag.Workflow{Name: "隔离组织事件", Nodes: []dag.Node{{ID: "command", Type: "command", Command: []string{"fixture-command"}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	var before int64
	if err = f.s.DB.QueryRow(`SELECT seq FROM dag_lock WHERE id=1`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range engines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			for n := 0; n < 10; n++ {
				_, e := engines[i].CreateRunBoundAuthorized(ctx, actors[i].TenantID, projects[i], workflows[i].ID, workflows[i].Version, newID(), fmt.Sprintf("cross-%d-%d", i, n), func(tx *sql.Tx) error {
					return f.s.authorizeProjectTx(ctx, tx, actors[i], projects[i], true, false)
				}, func(tx *sql.Tx, run string) error {
					return f.s.activity(ctx, tx, actors[i], projects[i], "test.bound", run, "真实并发事务")
				})
				if e != nil {
					errs <- e
					return
				}
			}
			errs <- nil
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	rows, err := f.s.DB.Query(`SELECT sequence,tenant_id,project_id FROM dag_events WHERE sequence>? ORDER BY sequence`, before)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	next := before + 1
	counts := map[string]int{}
	for rows.Next() {
		var seq int64
		var tenant, project string
		if err = rows.Scan(&seq, &tenant, &project); err != nil {
			t.Fatal(err)
		}
		if seq != next {
			t.Fatalf("global event cursor skipped or duplicated: got %d want %d", seq, next)
		}
		next++
		counts[tenant+"/"+project]++
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(counts) != 2 || counts[actors[0].TenantID+"/"+projects[0]] == 0 || counts[actors[1].TenantID+"/"+projects[1]] == 0 {
		t.Fatalf("organization event scopes lost: %v", counts)
	}
	var after int64
	if err = f.s.DB.QueryRow(`SELECT seq FROM dag_lock WHERE id=1`).Scan(&after); err != nil || after != next-1 {
		t.Fatalf("event tail differs from durable cursor: %d %d %v", after, next-1, err)
	}
}
