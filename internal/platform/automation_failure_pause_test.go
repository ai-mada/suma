package platform

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

func TestAutomationFailurePauseHTTP(t *testing.T) {
	f, rule := automationFixture(t, "allow", "manual", "skip")
	ctx := context.Background()
	path := "/automations/" + rule.ID
	command := func(action string, version int, spec *AutomationSpec, status int) AutomationCommandResult {
		t.Helper()
		request := map[string]any{"action": action, "expected_version": version}
		if spec != nil {
			request["spec"] = spec
		}
		return issueRepairRuntimeDecode[AutomationCommandResult](t, f.call(http.MethodPost, path+"/commands", f.body(request), status))
	}
	read := func() AutomationRule {
		t.Helper()
		return issueRepairRuntimeDecode[AutomationRule](t, f.call(http.MethodGet, path+"?project_id="+f.project, nil, http.StatusOK))
	}

	for _, invalidThreshold := range []any{nil, -1, 1, 11, 2.5, "2"} {
		spec := issueRepairRuntimeDecode[map[string]any](t, rule.AutomationSpec)
		spec["failure_pause_threshold"] = invalidThreshold
		f.call(http.MethodPost, "/automations", f.body(map[string]any{"spec": spec}), http.StatusBadRequest)
	}
	validBoundary := issueRepairRuntimeDecode[map[string]any](t, rule.AutomationSpec)
	validBoundary["failure_pause_threshold"] = 10
	f.call(http.MethodPost, "/automations", f.body(map[string]any{"spec": validBoundary}), http.StatusCreated)

	spec := rule.AutomationSpec
	spec.FailurePauseThreshold = 2
	rule = command("revise", rule.Version, &spec, http.StatusOK).Rule
	rule = command("enable", rule.Version, nil, http.StatusOK).Rule
	if rule.FailurePauseThreshold != 2 {
		t.Fatalf("expected the enabled revision to retain its threshold: %+v", rule)
	}

	start := func() AutomationOccurrence {
		t.Helper()
		queued := command("trigger", rule.Version, nil, http.StatusOK)
		rule = queued.Rule
		if queued.Occurrence == nil || queued.Occurrence.State != "queued" {
			t.Fatalf("trigger did not queue: %+v", queued.Occurrence)
		}
		autoTick(t, f)
		for _, item := range autoRead(t, f, rule).Items {
			if item.ID == queued.Occurrence.ID && item.State == "started" && item.RunID != "" {
				return item
			}
		}
		t.Fatal("authorized trigger did not start", queued.Occurrence.ID)
		return AutomationOccurrence{}
	}
	runFor := func(id string) dag.Run {
		t.Helper()
		run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, id)
		if err != nil {
			t.Fatal(err)
		}
		return run
	}
	decide := func(occurrence AutomationOccurrence, decision, state string) {
		t.Helper()
		resolveAutomationTestApproval(t, f, runFor(occurrence.RunID), decision, state)
	}
	for i := 0; i < 4; i++ {
		start()
	}
	// Tie the first four creation times so the configured history order uses the
	// occurrence ID. A success between two failures must break the streak.
	clock, err := f.s.automationClock(ctx, f.s.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.ExecContext(ctx, `UPDATE suma_automation_occurrences SET created_at=? WHERE rule_id=?`, automationStamp(clock.Add(-time.Hour)), rule.ID); err != nil {
		t.Fatal(err)
	}
	history := autoRead(t, f, rule).Items
	if len(history) != 4 {
		t.Fatalf("expected four initial runs: %+v", history)
	}
	decide(history[0], "reject", "failed")
	decide(history[1], "approve", "succeeded")
	decide(history[2], "reject", "failed")
	active := runFor(history[3].RunID)
	latest := start()
	decide(latest, "reject", "failed")

	first := command("trigger", rule.Version, nil, http.StatusOK)
	second := command("trigger", first.Rule.Version, nil, http.StatusOK)
	if first.Occurrence == nil || second.Occurrence == nil {
		t.Fatal("missing queued triggers")
	}
	queued := map[string]bool{first.Occurrence.ID: false, second.Occurrence.ID: false}
	runs := autoCount(t, f, "dag_runs")
	dagReceipts := autoCount(t, f, "dag_operations")

	// A queue update failure must roll back the pause and any earlier rows in
	// the same transaction. Retrying the tick should perform one durable pause.
	if _, err := f.s.DB.ExecContext(ctx, `CREATE TRIGGER failure_pause_test_abort BEFORE UPDATE OF state ON suma_automation_occurrences WHEN NEW.reason='failure_pause_threshold_reached' BEGIN SELECT RAISE(ABORT,'fixture pause rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickAutomations(ctx, f.e); err == nil {
		t.Fatal("injected queue update failure was ignored")
	}
	if after := read(); after.State != "enabled" || after.Version != second.Rule.Version || after.LastError != "" {
		t.Fatalf("failed queue update partially paused the rule: %+v", after)
	}
	for _, item := range autoRead(t, f, rule).Items {
		if _, wasQueued := queued[item.ID]; wasQueued && (item.State != "queued" || item.Reason != "" || item.RunID != "") {
			t.Fatalf("failed pause changed the queue: %+v", item)
		}
	}
	if _, err := f.s.DB.ExecContext(ctx, `DROP TRIGGER failure_pause_test_abort`); err != nil {
		t.Fatal(err)
	}
	autoTick(t, f)
	paused := read()
	if paused.State != "paused" || paused.Version != second.Rule.Version+1 || paused.Revision != rule.Revision || paused.NextAt != "" || paused.LastError != automationFailurePauseReason {
		t.Fatalf("threshold did not persist one pause of the current revision: %+v", paused)
	}
	for _, item := range autoRead(t, f, paused).Items {
		if _, wasQueued := queued[item.ID]; wasQueued {
			if item.State != "blocked" || item.Reason != automationFailurePauseReason || item.RunID != "" {
				t.Fatalf("queued trigger was not blocked without a run: %+v", item)
			}
			queued[item.ID] = true
		}
	}
	for id, found := range queued {
		if !found {
			t.Fatalf("blocked trigger disappeared: %s", id)
		}
	}
	if autoCount(t, f, "dag_runs") != runs || autoCount(t, f, "dag_operations") != dagReceipts {
		t.Fatal("blocked admission created a DAG run or receipt")
	}
	if current := runFor(active.ID); current.Version != active.Version || current.State != active.State || current.Nodes[0].State != active.Nodes[0].State {
		t.Fatalf("already-started run changed on pause: before=%+v after=%+v", active, current)
	}
	autoTick(t, f)
	if after := read(); after.Version != paused.Version {
		t.Fatalf("repeated tick mutated the paused rule: %+v", after)
	}
	command("enable", paused.Version, nil, http.StatusConflict)

	spec.Name = "Reviewed retry policy"
	rule = command("revise", paused.Version, &spec, http.StatusOK).Rule
	if rule.Revision != paused.Revision+1 || rule.State != "paused" || rule.LastError != "" {
		t.Fatalf("review did not establish a new revision: %+v", rule)
	}
	rule = command("enable", rule.Version, nil, http.StatusOK).Rule
	if next := start(); next.Revision != rule.Revision {
		t.Fatalf("old failures leaked into the new revision: %+v", next)
	}
}

func TestAutomationFailurePauseDisabledHTTP(t *testing.T) {
	for _, explicitZero := range []bool{false, true} {
		name := "omitted"
		if explicitZero {
			name = "zero"
		}
		t.Run(name, func(t *testing.T) {
			f, rule := automationFixture(t, "allow", "manual", "skip")
			if explicitZero {
				spec := issueRepairRuntimeDecode[map[string]any](t, rule.AutomationSpec)
				spec["failure_pause_threshold"] = 0
				rule = issueRepairRuntimeDecode[AutomationRule](t, f.call(http.MethodPost, "/automations", f.body(map[string]any{"spec": spec}), http.StatusCreated))
			}
			rule = autoCommand(t, f, rule, "enable").Rule
			for i := 0; i < 3; i++ {
				trigger := autoCommand(t, f, rule, "trigger")
				rule = trigger.Rule
				autoTick(t, f)
				var runID string
				for _, item := range autoRead(t, f, rule).Items {
					if item.ID == trigger.Occurrence.ID {
						runID = item.RunID
					}
				}
				if runID == "" {
					t.Fatal("a disabled failure policy blocked dispatch")
				}
				run, err := f.e.GetRun(context.Background(), f.user.TenantID, f.project, runID)
				if err != nil {
					t.Fatal(err)
				}
				resolveAutomationTestApproval(t, f, run, "reject", "failed")
			}
		})
	}
}

func resolveAutomationTestApproval(t *testing.T, f *fixture, run dag.Run, decision, state string) {
	t.Helper()
	resolved, err := f.e.Command(context.Background(), f.user.TenantID, f.project, run.ID, "approval", newID(), run.Version, map[string]any{"node_id": "gate", "decision": decision, "reason": "test decision"})
	if err != nil || resolved.State != state {
		t.Fatalf("could not produce actual run state %q: %+v %v", state, resolved, err)
	}
}
