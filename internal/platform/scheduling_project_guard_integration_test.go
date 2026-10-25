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

// R2 current reads serialize with authority coordination. SQLite's one writer
// can therefore delay a candidate display even when its business row belongs to
// another organization. No result may escape before current authority is held;
// release completes the read, while the caller deadline refuses every result.
func TestCandidateDisplayCurrentReadIsBoundedByConcurrentWriter(t *testing.T) {
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
	var beforeOperations, beforeActivity int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&beforeOperations); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity`).Scan(&beforeActivity); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"detail", "list"} {
		for _, completion := range []string{"release", "deadline"} {
			t.Run(kind+"/"+completion, func(t *testing.T) {
				writer, e := other.BeginTx(context.Background(), nil)
				if e != nil {
					t.Fatal(e)
				}
				defer writer.Rollback()
				if _, e = writer.Exec(`UPDATE suma_organizations SET name=name WHERE id=?`, created["id"]); e != nil {
					t.Fatal(e)
				}
				budget := 2 * time.Second
				if completion == "deadline" {
					budget = 250 * time.Millisecond
				}
				ctx, cancel := context.WithTimeout(context.Background(), budget)
				defer cancel()
				type result struct {
					err     error
					hasData bool
				}
				done := make(chan result, 1)
				go func() {
					if kind == "detail" {
						out, e := f.s.ReleaseCandidate(ctx, f.actor, f.project, "missing-candidate")
						done <- result{e, out.Candidate.ID != "" || out.Eligibility || len(out.Blockers) != 0}
					} else {
						items, total, next, e := f.s.ReleaseCandidates(ctx, f.actor, f.project, 20, "")
						done <- result{e, len(items) != 0 || total != 0 || next != ""}
					}
				}()
				select {
				case out := <-done:
					t.Fatalf("read escaped before current authority lock: err=%v data=%v", out.err, out.hasData)
				case <-time.After(100 * time.Millisecond):
				}
				if completion == "deadline" {
					<-ctx.Done()
				}
				if e = writer.Rollback(); e != nil {
					t.Fatal(e)
				}
				select {
				case out := <-done:
					if out.hasData {
						t.Fatal("blocked/expired read exposed candidate data")
					}
					if completion == "deadline" {
						if !errors.Is(out.err, context.DeadlineExceeded) {
							t.Fatalf("expired current read = %v, want deadline", out.err)
						}
					} else if kind == "detail" {
						var api *APIError
						if !errors.As(out.err, &api) || api.Status != 404 {
							t.Fatalf("released detail = %v, want authoritative 404", out.err)
						}
					} else if out.err != nil {
						t.Fatalf("released list failed: %v", out.err)
					}
				case <-time.After(1500 * time.Millisecond):
					t.Fatal("current read exceeded its bounded completion after writer release")
				}
			})
		}
	}
	var afterOperations, afterActivity int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&afterOperations); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity`).Scan(&afterActivity); err != nil {
		t.Fatal(err)
	}
	if afterOperations != beforeOperations || afterActivity != beforeActivity {
		t.Fatal("candidate GET created business receipts or activity")
	}
}

// A concurrent durable execution invalidation must win before an uploader can
// create a new session. This is a controlled lease-loss fault, not a fabricated
// artifact or successful task result.
func TestArtifactAdmissionRechecksAssignmentAfterConcurrentSQLiteWriter(t *testing.T) {
	f, token, ref, _, request := artifactProtocolFixture(t)
	ctx := context.Background()
	actor := domainCActor(t, f.s, token)
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
	go func() { _, e := f.s.StartArtifactUpload(ctx, actor, f.project, request); done <- e }()
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
	org, err := f.s.CreateOrganization(ctx, f.actor, map[string]any{"name": "第二并发组织", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	browser, err := f.s.AuthenticateActor(ctx, f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	actor, switchedToken, err := f.s.SwitchOrganization(ctx, browser, map[string]any{"organization_id": org["id"], "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	chosen, e := f.s.AuthenticateActor(ctx, switchedToken)
	if e != nil {
		t.Fatal(e)
	}
	project, err := f.s.CreateProject(ctx, chosen, map[string]any{"title": "第二并发项目", "idempotency_key": newID()})
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
	commandActors := []Actor{f.actor, chosen}
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
				var proof *authorizedCommand
				_, e := engines[i].CreateRunBoundAuthorized(ctx, actors[i].TenantID, projects[i], workflows[i].ID, workflows[i].Version, newID(), fmt.Sprintf("cross-%d-%d", i, n), func(tx *sql.Tx) error {
					var e error
					proof, e = f.s.authorizeTx(ctx, tx, commandActors[i], commandTarget{tenantID: actors[i].TenantID, projectID: projects[i]}, purposeProjectWrite)
					return e
				}, func(tx *sql.Tx, run string) error {
					return f.s.commandActivity(ctx, proof, "test.bound", run, "真实并发事务")
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
