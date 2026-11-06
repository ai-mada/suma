package platform

import (
	"net/http"
	"testing"
)

func TestAutomationManualTriggerRechecksSubmitterWriteAuthority(t *testing.T) {
	f, rule := automationFixture(t, "allow", "manual", "skip")
	ownerCookie := f.cookie
	path := "/automations/" + rule.ID
	commands := path + "/commands"
	command := func(action string, version int, status int) map[string]any {
		t.Helper()
		return f.call(http.MethodPost, commands, f.body(map[string]any{
			"action": action, "expected_version": version,
		}), status)
	}
	enabled := issueRepairRuntimeDecode[AutomationCommandResult](t, command("enable", rule.Version, http.StatusOK)).Rule
	if enabled.ActivatedBy != f.user.ID {
		t.Fatalf("rule was not enabled by the project owner: %+v", enabled)
	}

	queueAsDeveloper := func(email string, version int) (string, AutomationCommandResult) {
		t.Helper()
		f.cookie = ownerCookie
		f.call(http.MethodGet, "/auth/session", nil, http.StatusOK)
		inv := governanceInvite(f, email, "member", []any{map[string]any{"project_id": f.project, "role": "developer"}})
		member := governanceAccept(f, inv, email)["user"].(map[string]any)["id"].(string)
		out := issueRepairRuntimeDecode[AutomationCommandResult](t, command("trigger", version, http.StatusOK))
		if out.Occurrence == nil || out.Occurrence.ID == "" || out.Occurrence.State != "queued" || out.Occurrence.RunID != "" {
			t.Fatalf("manual trigger did not return a queued occurrence: %+v", out.Occurrence)
		}
		return member, out
	}
	revokedMember, revoked := queueAsDeveloper("automation-revoked@example.test", enabled.Version)
	_, allowed := queueAsDeveloper("automation-retained@example.test", revoked.Rule.Version)
	if runs := autoCount(t, f, "dag_runs"); runs != 0 {
		t.Fatalf("queued manual triggers created %d runs before dispatch", runs)
	}

	f.cookie = ownerCookie
	f.call(http.MethodGet, "/auth/session", nil, http.StatusOK)
	f.call(http.MethodPost, "/project-access", f.body(map[string]any{
		"user_id": revokedMember, "role": "viewer", "expected_version": 1,
	}), http.StatusOK)

	readDispatched := func() map[string]AutomationOccurrence {
		t.Helper()
		out := issueRepairRuntimeDecode[AutomationOccurrences](t, f.call(http.MethodGet,
			path+"/occurrences?project_id="+f.project, nil, http.StatusOK))
		if len(out.Items) != 2 || out.Total != 2 {
			t.Fatalf("expected the two original occurrences: %+v", out)
		}
		byID := make(map[string]AutomationOccurrence, len(out.Items))
		for _, item := range out.Items {
			byID[item.ID] = item
		}
		blocked := byID[revoked.Occurrence.ID]
		if blocked.State != "blocked" || blocked.Reason != "manual_trigger_authority_revoked" || blocked.RunID != "" {
			t.Fatalf("revoked submitter's occurrence was not blocked without a run: %+v", blocked)
		}
		started := byID[allowed.Occurrence.ID]
		if started.State != "started" || started.RunID == "" {
			t.Fatalf("authorized developer's occurrence did not start: %+v", started)
		}
		if runs := autoCount(t, f, "dag_runs"); runs != 1 {
			t.Fatalf("expected only the authorized developer's run, got %d", runs)
		}
		return byID
	}
	// Tick selects one head per rule. Two passes cover both IDs even if their timestamps tie.
	autoTick(t, f)
	autoTick(t, f)
	first := readDispatched()
	autoTick(t, f)
	repeated := readDispatched()
	for id, original := range first {
		if repeated[id] != original {
			t.Fatalf("repeat tick changed occurrence %s: first=%+v repeated=%+v", id, original, repeated[id])
		}
	}

	unproven := issueRepairRuntimeDecode[AutomationCommandResult](t, command("trigger", allowed.Rule.Version, http.StatusOK))
	if unproven.Occurrence == nil {
		t.Fatal("manual trigger was not queued")
	}
	// Simulate a pre-existing queue entry without initiator evidence.
	if _, err := f.s.DB.Exec(`DELETE FROM suma_automation_manual_invocations WHERE occurrence_id=?`, unproven.Occurrence.ID); err != nil {
		t.Fatal(err)
	}
	autoTick(t, f)
	history := issueRepairRuntimeDecode[AutomationOccurrences](t, f.call(http.MethodGet, path+"/occurrences?project_id="+f.project, nil, http.StatusOK))
	for _, item := range history.Items {
		if item.ID == unproven.Occurrence.ID {
			if item.State != "blocked" || item.Reason != "manual_trigger_authority_unverified" || item.RunID != "" {
				t.Fatalf("unproven trigger did not stop without a run: %+v", item)
			}
			if runs := autoCount(t, f, "dag_runs"); runs != 1 {
				t.Fatalf("unproven trigger unexpectedly created a run: %d", runs)
			}
			current := issueRepairRuntimeDecode[AutomationRule](t, f.call(http.MethodGet, path+"?project_id="+f.project, nil, http.StatusOK))
			if current.State != "enabled" || current.LastError != "" {
				t.Fatalf("occurrence-specific denial was reported as rule failure: %+v", current)
			}
			return
		}
	}
	t.Fatal("original unproven trigger was not readable")
}
