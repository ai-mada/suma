package platform

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

func TestAutomationHTTPCreateAndEnable(t *testing.T) {
	f := newFixture(t)
	w := f.call("POST", "/workflows", f.body(map[string]any{"name": "automation test", "nodes": []any{map[string]any{"id": "gate", "type": "approval", "name": "Human gate"}}, "edges": []any{}}), 201)
	spec := map[string]any{"name": "A rule", "workflow_id": w["id"], "workflow_version": 1, "trigger": "manual", "cron": "", "timezone": "UTC", "misfire": "skip", "catch_up_limit": 1, "overlap": "forbid"}
	a := f.call("POST", "/automations", f.body(map[string]any{"spec": spec}), 201)
	if a["state"] != "draft" {
		t.Fatal("new rule must be draft", a)
	}
}

func automationFixture(t *testing.T, overlap, trigger, misfire string) (*fixture, AutomationRule) {
	t.Helper()
	f := newFixture(t)
	w := f.call("POST", "/workflows", f.body(map[string]any{"name": "durable automation", "nodes": []any{map[string]any{"id": "gate", "type": "approval", "name": "Human gate"}}, "edges": []any{}}), 201)
	spec := AutomationSpec{Name: "Scheduled gate", WorkflowID: w["id"].(string), WorkflowVersion: 1, Trigger: trigger, Timezone: "UTC", Misfire: misfire, CatchUpLimit: 2, Overlap: overlap}
	if trigger == "cron" {
		spec.Cron = "* * * * *"
	}
	a, err := f.s.CreateAutomation(context.Background(), f.actor, f.project, AutomationCreateRequest{ProjectID: f.project, IdempotencyKey: newID(), Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	return f, a
}
func autoCommand(t *testing.T, f *fixture, a AutomationRule, action string) AutomationCommandResult {
	t.Helper()
	out, err := f.s.CommandAutomation(context.Background(), f.actor, f.project, a.ID, AutomationCommandRequest{ProjectID: f.project, Action: action, ExpectedVersion: a.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(action, err)
	}
	return out
}
func autoRead(t *testing.T, f *fixture, a AutomationRule) AutomationOccurrences {
	t.Helper()
	out, err := f.s.AutomationOccurrences(context.Background(), f.actor, f.project, a.ID, 200, "")
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func autoTick(t *testing.T, f *fixture) {
	t.Helper()
	if err := f.s.TickAutomations(context.Background(), f.e); err != nil {
		t.Fatal(err)
	}
}
func autoCount(t *testing.T, f *fixture, table string) int {
	t.Helper()
	var n int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAutomationReceiptImmutableRevisionAndFixedWorkflow(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	ctx := context.Background()
	enable := AutomationCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: a.Version, IdempotencyKey: newID()}
	first, err := f.s.CommandAutomation(ctx, f.actor, f.project, a.ID, enable)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.s.CommandAutomation(ctx, f.actor, f.project, a.ID, enable)
	if err != nil || again.Rule.Version != first.Rule.Version {
		t.Fatal("enable duplicated", again, err)
	}
	a = first.Rule
	// Publishing a later workflow version cannot change the rule's fixed target.
	if _, err = f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{ID: a.WorkflowID, Name: "later definition", Nodes: []dag.Node{{ID: "later", Type: "approval"}}}); err != nil {
		t.Fatal(err)
	}
	trigger := AutomationCommandRequest{ProjectID: f.project, Action: "trigger", ExpectedVersion: a.Version, IdempotencyKey: newID()}
	one, err := f.s.CommandAutomation(ctx, f.actor, f.project, a.ID, trigger)
	if err != nil {
		t.Fatal(err)
	}
	two, err := f.s.CommandAutomation(ctx, f.actor, f.project, a.ID, trigger)
	if err != nil || two.Occurrence.ID != one.Occurrence.ID {
		t.Fatal("trigger receipt duplicated", err)
	}
	if n := autoCount(t, f, "dag_runs"); n != 0 {
		t.Fatal("manual receipt fabricated run", n)
	}
	autoTick(t, f)
	occ := autoRead(t, f, a)
	if len(occ.Items) != 1 || occ.Items[0].State != "started" || occ.Items[0].RunID == "" {
		t.Fatal(occ)
	}
	run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, occ.Items[0].RunID)
	if err != nil || run.WorkflowVersion != 1 || run.Nodes[0].ID != "gate" {
		t.Fatal("workflow floated to latest", run, err)
	}
	a = one.Rule
	spec := a.AutomationSpec
	spec.Name = "Revised"
	spec.WorkflowVersion = 2
	revised, err := f.s.CommandAutomation(ctx, f.actor, f.project, a.ID, AutomationCommandRequest{ProjectID: f.project, Action: "revise", ExpectedVersion: a.Version, IdempotencyKey: newID(), Spec: &spec})
	if err != nil {
		t.Fatal(err)
	}
	if revised.Rule.State != "paused" || revised.Rule.Revision != 2 {
		t.Fatal("edit did not require reactivation", revised)
	}
	if _, err = f.s.CommandAutomation(ctx, f.actor, f.project, a.ID, AutomationCommandRequest{Action: "trigger", ExpectedVersion: revised.Rule.Version, IdempotencyKey: newID()}); err == nil {
		t.Fatal("revised rule triggered before human enable")
	}
	detail, err := f.s.GetAutomation(ctx, f.actor, f.project, a.ID)
	if err != nil || len(detail.Revisions) != 2 || detail.Revisions[1].Spec.WorkflowVersion != 1 || detail.Revisions[1].Spec.Name != "Scheduled gate" {
		t.Fatal("history mutated", detail, err)
	}
	page, err := f.s.AutomationRevisions(ctx, f.actor, f.project, a.ID, 1, "")
	if err != nil || page.NextCursor == "" || page.Items[0].Revision != 2 {
		t.Fatal(page, err)
	}
	page, err = f.s.AutomationRevisions(ctx, f.actor, f.project, a.ID, 1, page.NextCursor)
	if err != nil || len(page.Items) != 1 || page.Items[0].Revision != 1 {
		t.Fatal(page, err)
	}
}

func TestAutomationOverlapAndTerminalRelease(t *testing.T) {
	for _, policy := range []string{"allow", "forbid", "queue"} {
		t.Run(policy, func(t *testing.T) {
			f, a := automationFixture(t, policy, "manual", "skip")
			a = autoCommand(t, f, a, "enable").Rule
			a = autoCommand(t, f, a, "trigger").Rule
			autoTick(t, f)
			a = autoCommand(t, f, a, "trigger").Rule
			autoTick(t, f)
			occ := autoRead(t, f, a)
			states := map[string]int{}
			var first string
			for _, o := range occ.Items {
				states[o.State]++
				if o.RunID != "" {
					first = o.RunID
				}
			}
			switch policy {
			case "allow":
				if states["started"] != 2 {
					t.Fatal(occ)
				}
			case "forbid":
				if states["started"] != 1 || states["skipped"] != 1 {
					t.Fatal(occ)
				}
			case "queue":
				if states["started"] != 1 || states["queued"] != 1 {
					t.Fatal(occ)
				}
				r, err := f.e.GetRun(context.Background(), f.user.TenantID, f.project, first)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.e.Command(context.Background(), f.user.TenantID, f.project, r.ID, "pause", newID(), r.Version, nil); err != nil {
					t.Fatal(err)
				}
				autoTick(t, f)
				if autoCount(t, f, "dag_runs") != 1 {
					t.Fatal("paused run stopped counting as active")
				}
				r, err = f.e.GetRun(context.Background(), f.user.TenantID, f.project, first)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.e.Command(context.Background(), f.user.TenantID, f.project, r.ID, "cancel", newID(), r.Version, nil); err != nil {
					t.Fatal(err)
				}
				autoTick(t, f)
				if autoCount(t, f, "dag_runs") != 2 {
					t.Fatal("queue did not release after terminal run", autoRead(t, f, a))
				}
			}
		})
	}
}

func TestAutomationCronMisfireDatabaseClockCursorAndRestart(t *testing.T) {
	for _, policy := range []string{"skip", "latest", "catch_up"} {
		t.Run(policy, func(t *testing.T) {
			f, a := automationFixture(t, "allow", "cron", policy)
			a = autoCommand(t, f, a, "enable").Rule
			clock, err := f.s.automationClock(context.Background(), f.s.DB)
			if err != nil {
				t.Fatal(err)
			}
			old := clock.Truncate(time.Minute).Add(-10 * time.Minute)
			if _, err = f.s.DB.Exec(`UPDATE suma_automations SET next_at=? WHERE id=?`, automationStamp(old), a.ID); err != nil {
				t.Fatal(err)
			}
			// A fresh engine and service use only persisted cursor and immutable specs.
			restarted := &Service{DB: f.s.DB, Dialect: "sqlite"}
			engine, err := dag.New(f.s.DB, "sqlite")
			if err != nil {
				t.Fatal(err)
			}
			if err = restarted.TickAutomations(context.Background(), engine); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				autoTick(t, f)
			}
			occ := autoRead(t, f, a)
			states := map[string]int{}
			for _, o := range occ.Items {
				states[o.State]++
				if o.State == "skipped" && !strings.Contains(o.Reason, "window_end=") {
					t.Fatal("discarded window not recorded", o)
				}
			}
			want := 1
			if policy == "catch_up" {
				want = 2
			}
			if states["started"] != want {
				t.Fatal("misfire run count", policy, occ)
			}
			if policy != "latest" && states["skipped"] != 1 {
				t.Fatal("skip window missing", occ)
			}
			after, err := f.s.GetAutomation(context.Background(), f.actor, f.project, a.ID)
			if err != nil || after.Version != a.Version || after.Revision != a.Revision || after.NextAt <= automationStamp(clock) {
				t.Fatal("scheduler consumed user version or lost cursor", after, err)
			}
			if policy == "latest" && occ.Items[0].ScheduledAt != automationStamp(clock.Truncate(time.Minute)) {
				t.Fatal("latest not latest UTC minute", occ)
			}
		})
	}
}

func TestAutomationTwoConnectionsCompeteAndRevocation(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	ctx := context.Background()
	enable := AutomationCommandRequest{Action: "enable", ExpectedVersion: a.Version, IdempotencyKey: newID()}
	enabled, err := f.s.CommandAutomation(ctx, f.actor, f.project, a.ID, enable)
	if err != nil {
		t.Fatal(err)
	}
	a = enabled.Rule
	a = autoCommand(t, f, a, "trigger").Rule
	var seq int
	var name, path string
	if err = f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	otherDB, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	other := &Service{DB: otherDB, Dialect: "sqlite"}
	otherEngine, err := dag.New(otherDB, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for _, pair := range []struct {
		s *Service
		e *dag.Engine
	}{{f.s, f.e}, {other, otherEngine}} {
		wg.Add(1)
		go func(s *Service, e *dag.Engine) { defer wg.Done(); failures <- s.TickAutomations(ctx, e) }(pair.s, pair.e)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if autoCount(t, f, "dag_runs") != 1 || autoCount(t, f, "suma_automation_occurrences") != 1 {
		t.Fatal("multi-process dispatch duplicated")
	}
	a = autoCommand(t, f, a, "trigger").Rule
	if _, err = otherDB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = otherDB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'developer') ON CONFLICT(project_id,user_id)DO UPDATE SET role='developer'`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	autoTick(t, f)
	if autoCount(t, f, "dag_runs") != 1 {
		t.Fatal("revoked activation still dispatched")
	}
	occ := autoRead(t, f, a)
	var blocked bool
	for _, o := range occ.Items {
		blocked = blocked || o.State == "blocked" && o.Reason == "activation_authority_revoked"
	}
	if !blocked {
		t.Fatal("revocation reason lost", occ)
	}
	if _, err = f.s.CommandAutomation(ctx, f.actor, f.project, a.ID, enable); !automationAuthorityFailure(err) {
		t.Fatal("revoked approval receipt replayed", err)
	}
}

func TestAutomationBindingRollbackAndWaitingQueueFairness(t *testing.T) {
	f, a := automationFixture(t, "queue", "manual", "skip")
	ctx := context.Background()
	a = autoCommand(t, f, a, "enable").Rule
	a = autoCommand(t, f, a, "trigger").Rule
	eventsBefore, operationsBefore := autoCount(t, f, "dag_events"), autoCount(t, f, "dag_operations")
	if _, err := f.s.DB.Exec(`CREATE TRIGGER automation_binding_failure BEFORE UPDATE OF run_id ON suma_automation_occurrences WHEN NEW.run_id<>'' BEGIN SELECT RAISE(ABORT,'test binding rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickAutomations(ctx, f.e); err == nil {
		t.Fatal("injected binding failure not returned")
	}
	if autoCount(t, f, "dag_runs") != 0 || autoCount(t, f, "dag_events") != eventsBefore || autoCount(t, f, "dag_operations") != operationsBefore {
		t.Fatal("failed binding left DAG side effects", autoCount(t, f, "dag_runs"), autoCount(t, f, "dag_events"), eventsBefore, autoCount(t, f, "dag_operations"), operationsBefore)
	}
	if autoRead(t, f, a).Items[0].State != "queued" {
		t.Fatal("failed binding changed occurrence")
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER automation_binding_failure`); err != nil {
		t.Fatal(err)
	}
	autoTick(t, f)
	// 105 waiting occurrences behind this active rule cannot starve rule B.
	for i := 0; i < 105; i++ {
		a = autoCommand(t, f, a, "trigger").Rule
	}
	b, err := f.s.CreateAutomation(ctx, f.actor, f.project, AutomationCreateRequest{ProjectID: f.project, IdempotencyKey: newID(), Spec: a.AutomationSpec})
	if err != nil {
		t.Fatal(err)
	}
	b = autoCommand(t, f, b, "enable").Rule
	b = autoCommand(t, f, b, "trigger").Rule
	autoTick(t, f)
	if out := autoRead(t, f, b); len(out.Items) != 1 || out.Items[0].State != "started" {
		t.Fatal("blocked queue starved another rule", out)
	}
}

func TestAutomationHTTPStrictFieldsPaginationAndScope(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	spec := map[string]any{}
	if err := json.Unmarshal([]byte(stringify(a.AutomationSpec)), &spec); err != nil {
		t.Fatal(err)
	}
	bad := f.body(map[string]any{"spec": spec, "unknown": true})
	f.call("POST", "/automations", bad, 400)
	spec["workflow_version"] = 1.5
	f.call("POST", "/automations", f.body(map[string]any{"spec": spec}), 400)
	spec["workflow_version"] = 1
	spec["unknown"] = true
	f.call("POST", "/automations", f.body(map[string]any{"spec": spec}), 400)
	delete(spec, "unknown")
	f.call("POST", "/automations/preview", map[string]any{"project_id": f.project, "spec": spec}, 200)
	f.call("GET", "/automations?project_id="+f.project+"&limit=201", nil, 400)
	f.call("GET", "/automations?project_id="+f.project+"&cursor=not-a-cursor", nil, 400)
	for i := 0; i < 2; i++ {
		f.call("POST", "/automations", f.body(map[string]any{"spec": spec}), 201)
	}
	first := f.call("GET", "/automations?project_id="+f.project+"&limit=1", nil, 200)
	cursor := first["next_cursor"].(string)
	if cursor == "" || first["total"] != float64(3) {
		t.Fatal(first)
	}
	f.call("GET", "/automations/"+a.ID+"/occurrences?project_id="+f.project+"&cursor="+cursor, nil, 400)
	seen := map[string]bool{}
	for cursor != "" {
		next := f.call("GET", "/automations?project_id="+f.project+"&limit=1&cursor="+cursor, nil, 200)
		id := next["items"].([]any)[0].(map[string]any)["id"].(string)
		if seen[id] {
			t.Fatal("page repeated", id)
		}
		seen[id] = true
		cursor = next["next_cursor"].(string)
	}
	if len(seen) != 2 {
		t.Fatal("pagination omitted records", seen)
	}
	// Managed release definitions are rejected before storing any rule.
	if _, err := f.s.DB.Exec(`INSERT INTO dag_managed_workflows(tenant_id,project_id,workflow_id,run_id)VALUES(?,?,?,?)`, f.user.TenantID, f.project, a.WorkflowID, "reserved-test-run"); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/automations", f.body(map[string]any{"spec": spec}), 400)
	// A real project-scoped Runner credential cannot read business rules.
	issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "runner", Label: "automation test runner", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	token := issued.Token
	request := httptest.NewRequest("GET", "http://suma.test/api/v1/automations?project_id="+f.project, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("Runner accessed business rules", response.Code, response.Body.String())
	}
}

func TestAutomationAdmissionFailureBeforeDAGReceipt(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	ctx := context.Background()
	reject := errors.New("revoked live authority")
	before := func(*sql.Tx) error { return reject }
	bind := func(*sql.Tx, string) error { t.Fatal("binding reached after rejected admission"); return nil }
	if _, err := f.e.CreateRunBoundAuthorized(ctx, f.user.TenantID, f.project, a.WorkflowID, 1, "guarded-run", "guarded", before, bind); !errors.Is(err, reject) {
		t.Fatal(err)
	}
	r, err := f.e.CreateRunBoundAuthorized(ctx, f.user.TenantID, f.project, a.WorkflowID, 1, "guarded-run", "guarded", func(*sql.Tx) error { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.CreateRunBoundAuthorized(ctx, f.user.TenantID, f.project, a.WorkflowID, 1, "guarded-run", "guarded", before, bind); !errors.Is(err, reject) {
		t.Fatal("receipt replay escaped authorization", r, err)
	}
}

func TestAutomationPauseArchiveAndDurableDatabaseReopen(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	ctx := context.Background()
	a = autoCommand(t, f, a, "enable").Rule
	a = autoCommand(t, f, a, "trigger").Rule
	var seq int
	var name, path string
	if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f.s = &Service{DB: db, Dialect: "sqlite"}
	f.e, err = dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	autoTick(t, f)
	occ := autoRead(t, f, a)
	if occ.Total != 1 || occ.Items[0].State != "started" || occ.Items[0].RunID == "" {
		t.Fatal("reopen lost pending occurrence", occ)
	}
	autoTick(t, f)
	if autoCount(t, f, "dag_runs") != 1 {
		t.Fatal("reopen dispatched twice")
	}
	a = autoCommand(t, f, a, "trigger").Rule
	a = autoCommand(t, f, a, "pause").Rule
	autoTick(t, f)
	if autoCount(t, f, "dag_runs") != 1 {
		t.Fatal("paused pending occurrence started")
	}
	occ = autoRead(t, f, a)
	var blocked bool
	for _, o := range occ.Items {
		blocked = blocked || o.State == "blocked" && o.Reason == "rule_pause"
	}
	if !blocked {
		t.Fatal("pause did not retain blocked trigger", occ)
	}
	a = autoCommand(t, f, a, "enable").Rule
	autoTick(t, f)
	if autoCount(t, f, "dag_runs") != 1 {
		t.Fatal("re-enable resurrected blocked trigger")
	}
	a = autoCommand(t, f, a, "archive").Rule
	for _, action := range []string{"enable", "pause", "trigger"} {
		if _, err = f.s.CommandAutomation(ctx, f.actor, f.project, a.ID, AutomationCommandRequest{Action: action, ExpectedVersion: a.Version, IdempotencyKey: newID()}); err == nil {
			t.Fatal("archived rule accepted ", action)
		}
	}
}

func TestAutomationCronTwoConnectionsMaterializeExactlyOnce(t *testing.T) {
	f, a := automationFixture(t, "allow", "cron", "latest")
	ctx := context.Background()
	a = autoCommand(t, f, a, "enable").Rule
	clock, err := f.s.automationClock(ctx, f.s.DB)
	if err != nil {
		t.Fatal(err)
	}
	nominal := automationStamp(clock.Truncate(time.Minute))
	if _, err = f.s.DB.Exec(`UPDATE suma_automations SET next_at=? WHERE id=?`, nominal, a.ID); err != nil {
		t.Fatal(err)
	}
	var seq int
	var name, path string
	if err = f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	other := &Service{DB: db, Dialect: "sqlite"}
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	failures := make(chan error, 2)
	for _, pair := range []struct {
		s *Service
		e *dag.Engine
	}{{f.s, f.e}, {other, engine}} {
		go func(s *Service, e *dag.Engine) { <-start; failures <- s.TickAutomations(ctx, e) }(pair.s, pair.e)
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err = <-failures; err != nil {
			t.Fatal(err)
		}
	}
	occ := autoRead(t, f, a)
	if occ.Total != 1 || occ.Items[0].ScheduledAt != nominal || occ.Items[0].State != "started" || autoCount(t, f, "dag_runs") != 1 {
		t.Fatal("racing cron materialization duplicated or changed nominal UTC", occ)
	}
}

func TestAutomationOccurrenceCompositePaginationAndLateManagedGuard(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	ctx := context.Background()
	a = autoCommand(t, f, a, "enable").Rule
	for i := 0; i < 7; i++ {
		a = autoCommand(t, f, a, "trigger").Rule
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		page, err := f.s.AutomationOccurrences(ctx, f.actor, f.project, a.ID, 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range page.Items {
			if seen[o.ID] {
				t.Fatal("composite pagination duplicated same-second occurrence")
			}
			seen[o.ID] = true
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 7 {
		t.Fatal("composite pagination omitted same-second occurrences", seen)
	}
	page, err := f.s.AutomationOccurrences(ctx, f.actor, f.project, a.ID, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.s.CreateAutomation(ctx, f.actor, f.project, AutomationCreateRequest{IdempotencyKey: newID(), Spec: a.AutomationSpec})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AutomationOccurrences(ctx, f.actor, f.project, b.ID, 2, page.NextCursor); err == nil {
		t.Fatal("occurrence cursor crossed rules")
	}
	if _, err = f.s.DB.Exec(`INSERT INTO dag_managed_workflows(tenant_id,project_id,workflow_id,run_id)VALUES(?,?,?,?)`, f.user.TenantID, f.project, a.WorkflowID, "late-protected-run"); err != nil {
		t.Fatal(err)
	}
	autoTick(t, f)
	if autoCount(t, f, "dag_runs") != 0 {
		t.Fatal("workflow protected after rule enable still dispatched")
	}
	out := autoRead(t, f, a)
	var blocked bool
	for _, o := range out.Items {
		blocked = blocked || o.State == "blocked" && o.Reason == "workflow_unavailable"
	}
	if !blocked {
		t.Fatal("late workflow protection not persisted", out)
	}
}

func TestAutomationAPIProjectTokenCannotAccessHumanRules(t *testing.T) {
	for _, action := range []string{"read", "preview", "create", "enable", "revise", "trigger"} {
		t.Run(action, func(t *testing.T) {
			f, a := automationFixture(t, "allow", "manual", "skip")
			if action == "trigger" {
				a = autoCommand(t, f, a, "enable").Rule
			}
			issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", Label: "scoped API", IdempotencyKey: newID()})
			if err != nil {
				t.Fatal(err)
			}
			token := issued.Token
			method, path, body := "POST", "/automations", map[string]any{"project_id": f.project, "idempotency_key": newID(), "spec": a.AutomationSpec}
			switch action {
			case "read":
				method = "GET"
				path += "?project_id=" + f.project
				body = nil
			case "preview":
				path += "/preview"
				delete(body, "idempotency_key")
			case "create":
			case "enable", "revise", "trigger":
				path += "/" + a.ID + "/commands"
				body = map[string]any{"project_id": f.project, "action": action, "idempotency_key": newID(), "expected_version": a.Version}
				if action == "revise" {
					body["spec"] = a.AutomationSpec
				}
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(method, "http://suma.test/api/v1"+path, bytes.NewReader(raw))
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			f.handler.ServeHTTP(response, request)
			if response.Code != 403 {
				t.Errorf("project API credential accessed human automation %s: got %d, want 403: %s", action, response.Code, response.Body.String())
			}
		})
	}
}

func TestAutomationGeneratedCompensationCannotBeTemplate(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	ctx := context.Background()
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "compensable source", Nodes: []dag.Node{{ID: "apply", Type: "command", Command: []string{"apply"}, Idempotent: true, Config: map[string]any{"compensation": map[string]any{"type": "command", "command": []string{"undo"}, "idempotent": true}}}}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.e.CreateRunVersion(ctx, f.user.TenantID, f.project, w.ID, w.Version, newID())
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := f.e.Claim(ctx, f.user.TenantID, f.project, "compensation-test", toolFixtureCapabilities(t, f))
	if err != nil || assignment == nil {
		t.Fatal(assignment, err)
	}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, ExitCode: 0, OperationID: newID()}); err != nil {
		t.Fatal(err)
	}
	source, err = f.e.GetRun(ctx, f.user.TenantID, f.project, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	source, err = f.e.Command(ctx, f.user.TenantID, f.project, source.ID, "compensate", newID(), source.Version, map[string]any{"reason": "explicit approved inverse fixture"})
	if err != nil {
		t.Fatal(err)
	}
	inverse, err := f.e.GetRun(ctx, f.user.TenantID, f.project, source.CompensationRunID)
	if err != nil {
		t.Fatal(err)
	}
	spec := a.AutomationSpec
	spec.WorkflowID = inverse.WorkflowID
	spec.WorkflowVersion = inverse.WorkflowVersion
	if _, err = f.s.CreateAutomation(ctx, f.actor, f.project, AutomationCreateRequest{IdempotencyKey: newID(), Spec: spec}); err == nil {
		t.Error("generated inverse accepted as a new automation template")
	}
	if _, err = f.s.PreviewAutomation(ctx, f.actor, f.project, spec); err == nil {
		t.Error("generated inverse accepted for automation preview")
	}
	if _, err = f.s.CommandAutomation(ctx, f.actor, f.project, a.ID, AutomationCommandRequest{Action: "revise", ExpectedVersion: a.Version, IdempotencyKey: newID(), Spec: &spec}); err == nil {
		t.Error("generated inverse accepted as a revision")
	}
	// Model an already-persisted rule from the former permissive admission path.
	// The scheduler must retain a blocked occurrence rather than repeatedly fail.
	a, err = f.s.GetAutomation(ctx, f.actor, f.project, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_automations SET state='enabled',activated_by=?,spec=? WHERE id=?`, f.user.ID, stringify(spec), a.ID); err != nil {
		t.Fatal(err)
	}
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacyAt := automationStamp(time.Now())
	_, err = tx.ExecContext(ctx, `INSERT INTO suma_automation_occurrences(id,tenant_id,project_id,rule_id,revision,source,scheduled_at,state,reason,run_id,dedupe_key,created_at,updated_at) VALUES(?,?,?,?,?,'manual',?,'queued','legacy pre-fix admission','',?,?,?)`, newID(), f.user.TenantID, f.project, a.ID, a.Revision, legacyAt, digest(newID()), legacyAt, legacyAt)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickAutomations(ctx, f.e); err != nil {
		t.Error("protected legacy rule kept failing scheduler", err)
	}
	out := autoRead(t, f, a)
	if out.Total != 1 || out.Items[0].State != "blocked" || out.Items[0].Reason != "workflow_unavailable" || out.Items[0].RunID != "" {
		t.Error("inverse dispatch was not blocked durably", out)
	}
}
