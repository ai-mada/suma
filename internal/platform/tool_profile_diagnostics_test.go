package platform

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/toolcontract"
)

func toolDiagnosticsRead(t *testing.T, f *fixture, p ToolProfile) ToolProfileDiagnostics {
	t.Helper()
	value := f.call("GET", "/tool-profiles/"+p.ID+"/diagnostics?project_id="+f.project, nil, 200)
	raw, _ := json.Marshal(value)
	var out ToolProfileDiagnostics
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Tool.ID != p.ID || out.Tool.ProjectID != f.project || out.Tool.WorkerID != p.WorkerID || !toolcontract.EqualProfile(out.Tool.Profile, p.Profile) {
		t.Fatalf("diagnostics escaped exact source: %+v", out)
	}
	codes := []string{}
	for _, check := range out.Checks {
		codes = append(codes, check.Code)
	}
	if !reflect.DeepEqual(codes, []string{"authorization", "worker", "inventory", "profile"}) {
		t.Fatalf("diagnostic prerequisites changed: %v", codes)
	}
	checked, err := time.Parse(time.RFC3339Nano, out.CheckedAt)
	if err != nil {
		t.Fatal(err)
	}
	expires, err := time.Parse(time.RFC3339Nano, out.ExpiresAt)
	if err != nil || !expires.After(checked) || expires.After(checked.Add(60*time.Second)) {
		t.Fatalf("invalid display deadline: %+v (%v)", out, err)
	}
	return out
}

func toolDiagnosticsReportInventory(t *testing.T, f *fixture, token string, p ToolProfile, observation toolcontract.Observation) ToolInventoryReceipt {
	t.Helper()
	receipt, err := f.s.ReportToolInventory(context.Background(), toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{observation}})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestToolDiagnosticsPausedRetainsIndependentFactsAndReadOnly(t *testing.T) {
	f, token, p := toolFixture(t)
	receipt := toolDiagnosticsReportInventory(t, f, token, p, toolcontract.Observation{Profile: p.Profile, Available: true})
	before := []int{}
	for _, table := range []string{"suma_operations", "suma_tool_profiles", "suma_tool_inventories", "suma_activity", "dag_runs", "dag_nodes"} {
		before = append(before, domainACount(t, f.s, table))
	}
	out := toolDiagnosticsRead(t, f, p)
	if out.Tool.Available || out.NextAction != "review_profile" || out.Checks[0].Status != "blocked" || out.Checks[0].NextAction != "review_profile" {
		t.Fatalf("paused configuration became a grant: %+v", out)
	}
	for _, check := range out.Checks[1:] {
		if check.Status != "pass" || check.NextAction != "none" || check.ReasonDiagnostic == nil {
			t.Fatalf("paused profile hid independent current facts: %+v", check)
		}
	}
	observed, _ := time.Parse(time.RFC3339Nano, receipt.ObservedAt)
	expires, _ := time.Parse(time.RFC3339Nano, out.ExpiresAt)
	if expires.After(observed.Add(60 * time.Second)) {
		t.Fatal("report outlived its fresh inventory")
	}
	current, err := f.s.GetToolProfile(context.Background(), f.actor, f.project, p.ID)
	if err != nil || stringify(current) != stringify(out.Tool) {
		t.Fatalf("diagnostic changed original availability: %+v / %+v / %v", current, out.Tool, err)
	}
	after := []int{}
	for _, table := range []string{"suma_operations", "suma_tool_profiles", "suma_tool_inventories", "suma_activity", "dag_runs", "dag_nodes"} {
		after = append(after, domainACount(t, f.s, table))
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("read wrote domain state: %v -> %v", before, after)
	}
}

func toolDiagnosticsEnable(t *testing.T, f *fixture, p ToolProfile) ToolProfile {
	t.Helper()
	out, err := f.s.CommandToolProfile(context.Background(), f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: p.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func toolDiagnosticsSameObservation(t *testing.T, f *fixture, p ToolProfile, out ToolProfileDiagnostics) {
	t.Helper()
	current, err := f.s.GetToolProfile(context.Background(), f.actor, f.project, p.ID)
	if err != nil || stringify(current) != stringify(out.Tool) {
		t.Fatalf("diagnostic diverged from original observation: %+v / %+v / %v", current, out.Tool, err)
	}
}

func TestToolDiagnosticsFreshExactAndDrift(t *testing.T) {
	f, token, p := toolFixture(t)
	toolDiagnosticsReportInventory(t, f, token, p, toolcontract.Observation{Profile: p.Profile, Available: true})
	p = toolDiagnosticsEnable(t, f, p)
	for _, tc := range []struct {
		name, key, action string
		mutate            func(*toolcontract.Observation)
	}{
		{name: "exact", key: "profile_matches", action: "none"},
		{name: "profile_digest", key: "profile_mismatch", action: "register_profile", mutate: func(o *toolcontract.Observation) { o.Profile.ProfileSHA256 = strings.Repeat("c", 64) }},
		{name: "executable_digest", key: "profile_mismatch", action: "register_profile", mutate: func(o *toolcontract.Observation) { o.Profile.ExecutableSHA256 = strings.Repeat("c", 64) }},
		{name: "version", key: "profile_mismatch", action: "register_profile", mutate: func(o *toolcontract.Observation) { o.Profile.ToolVersion = "different reported version" }},
		{name: "command_missing", key: "command_missing", action: "open_worker", mutate: func(o *toolcontract.Observation) { o.Profile.CommandID = "other-native-agent" }},
		{name: "reported_unavailable", key: "reported_unavailable", action: "open_worker", mutate: func(o *toolcontract.Observation) { o.Available, o.Reason = false, "model login has not been verified" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observation := toolcontract.Observation{Profile: toolcontract.CloneProfile(p.Profile), Available: true}
			if tc.mutate != nil {
				tc.mutate(&observation)
			}
			toolDiagnosticsReportInventory(t, f, token, p, observation)
			out := toolDiagnosticsRead(t, f, p)
			for _, check := range out.Checks[:3] {
				if check.Status != "pass" || check.NextAction != "none" || check.ReasonDiagnostic == nil {
					t.Fatalf("unrelated current prerequisite changed: %+v", check)
				}
			}
			check := out.Checks[3]
			if check.NextAction != tc.action || out.NextAction != tc.action || check.ReasonDiagnostic == nil || check.ReasonDiagnostic.MessageKey != "display.tool."+tc.key || out.Tool.Available != (tc.action == "none") {
				t.Fatalf("configuration fact was broadened or lost: %+v", out)
			}
			if tc.action == "none" && check.Status != "pass" || tc.action != "none" && check.Status != "blocked" {
				t.Fatalf("unexpected exact configuration state: %+v", check)
			}
			toolDiagnosticsSameObservation(t, f, p, out)
		})
	}
}

func TestToolDiagnosticsNeverPassesProfileFromNonfreshInventory(t *testing.T) {
	f, token, p := toolFixture(t)
	toolDiagnosticsReportInventory(t, f, token, p, toolcontract.Observation{Profile: p.Profile, Available: true})
	p = toolDiagnosticsEnable(t, f, p)
	for _, tc := range []struct{ name, stamp, key string }{
		{name: "missing", key: "inventory_missing"},
		{name: "old", stamp: "2000-01-01T00:00:00Z", key: "inventory_stale"},
		{name: "future", stamp: "2099-01-01T00:00:00Z", key: "inventory_stale"},
		{name: "invalid_clock", stamp: "not-a-timestamp", key: "inventory_stale"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			toolDiagnosticsReportInventory(t, f, token, p, toolcontract.Observation{Profile: p.Profile, Available: true})
			var err error
			if tc.stamp == "" {
				_, err = f.s.DB.Exec(`DELETE FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, p.WorkerID)
			} else {
				_, err = f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at=? WHERE tenant_id=? AND project_id=? AND worker_id=?`, tc.stamp, f.user.TenantID, f.project, p.WorkerID)
			}
			if err != nil {
				t.Fatal(err)
			}
			out := toolDiagnosticsRead(t, f, p)
			inventory, profile := out.Checks[2], out.Checks[3]
			if out.Tool.Available || out.NextAction != "open_worker" || inventory.Status != "blocked" || inventory.NextAction != "open_worker" || inventory.ReasonDiagnostic == nil || inventory.ReasonDiagnostic.MessageKey != "display.tool."+tc.key || profile.Status != "not_checked" || profile.Reason != inventory.Reason || profile.NextAction != "open_worker" {
				t.Fatalf("nonfresh inventory was treated as evidence: %+v", out)
			}
			toolDiagnosticsSameObservation(t, f, p, out)
		})
	}
}

func TestToolDiagnosticsAuthorizationRemainsCurrentAndIndependent(t *testing.T) {
	for _, state := range []string{"revoked", "approver_revoked"} {
		t.Run(state, func(t *testing.T) {
			f, token, p := toolFixture(t)
			toolDiagnosticsReportInventory(t, f, token, p, toolcontract.Observation{Profile: p.Profile, Available: true})
			p = toolDiagnosticsEnable(t, f, p)
			key, action := "profile_revoked", "register_profile"
			if state == "revoked" {
				var err error
				p, err = f.s.CommandToolProfile(context.Background(), f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "revoke", ExpectedVersion: p.Version, IdempotencyKey: newID()})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				key, action = "activation_revoked", "review_profile"
				if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'developer')`, f.project, f.user.ID); err != nil {
					t.Fatal(err)
				}
			}
			out := toolDiagnosticsRead(t, f, p)
			if out.Tool.Available || out.NextAction != action || out.Checks[0].Status != "blocked" || out.Checks[0].ReasonDiagnostic == nil || out.Checks[0].ReasonDiagnostic.MessageKey != "display.tool."+key {
				t.Fatalf("saved activation replaced current approval: %+v", out)
			}
			for _, check := range out.Checks[1:] {
				if check.Status != "pass" {
					t.Fatalf("activation hid independent fact: %+v", check)
				}
			}
			toolDiagnosticsSameObservation(t, f, p, out)
		})
	}
}

func TestToolDiagnosticsPrivateWorkerIdentityIsIndependentOfInventory(t *testing.T) {
	f, token, p := toolFixture(t)
	toolDiagnosticsReportInventory(t, f, token, p, toolcontract.Observation{Profile: p.Profile, Available: true})
	p = toolDiagnosticsEnable(t, f, p)
	if _, err := f.s.DB.Exec(`UPDATE suma_release_workers SET status='revoked' WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, p.WorkerID); err != nil {
		t.Fatal(err)
	}
	out := toolDiagnosticsRead(t, f, p)
	if out.Tool.Available || out.NextAction != "open_worker" || out.Checks[0].Status != "pass" || out.Checks[1].Status != "blocked" || out.Checks[1].ReasonDiagnostic == nil || out.Checks[1].ReasonDiagnostic.MessageKey != "display.tool.worker_expired" || out.Checks[2].Status != "pass" || out.Checks[3].Status != "pass" {
		t.Fatalf("inventory replaced current worker identity: %+v", out)
	}
	toolDiagnosticsSameObservation(t, f, p, out)
	if _, err := f.s.DB.Exec(`UPDATE suma_tool_profiles SET state='paused' WHERE id=?`, p.ID); err != nil {
		t.Fatal(err)
	}
	out = toolDiagnosticsRead(t, f, p)
	if out.NextAction != "review_profile" || out.Checks[1].Status != "blocked" || out.Tool.ReasonDiagnostic.MessageKey != "display.tool.profile_paused" {
		t.Fatalf("later worker blocker replaced original authorization priority: %+v", out)
	}
}

func TestToolDiagnosticsRequiresCurrentBrowserScopeBeforeResource(t *testing.T) {
	f, token, p := toolFixture(t)
	path := "/tool-profiles/" + p.ID + "/diagnostics?project_id=" + f.project
	for _, kind := range []string{"api", "agent", "runner"} {
		t.Run(kind, func(t *testing.T) {
			key := token
			if kind != "runner" {
				key = f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": kind}), 201)["token"].(string)
			}
			f.bearer("GET", path, key, nil, 403)
			f.bearer("GET", "/tool-profiles/not-readable/diagnostics?project_id="+f.project, key, nil, 403)
		})
	}
	other := f.call("POST", "/projects", map[string]any{"title": "Other diagnostic scope", "idempotency_key": newID()}, 201)
	f.call("GET", "/tool-profiles/"+p.ID+"/diagnostics?project_id="+other["id"].(string), nil, 404)
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'developer')`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.GetToolProfileDiagnostics(context.Background(), f.actor, f.project, p.ID); err != nil {
		t.Fatal("current reader unnecessarily required approval", err)
	}
	if _, err := f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.GetToolProfileDiagnostics(context.Background(), f.actor, f.project, "not-readable")
	varStatus(t, err, 403)
	if err := logoutCredentialForTest(context.Background(), f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetToolProfileDiagnostics(context.Background(), f.actor, f.project, p.ID)
	varStatus(t, err, 401)
}

func TestToolDiagnosticsPoolIdentityUsesOriginalConsentAndCredential(t *testing.T) {
	h := newPoolMachineFixture(t)
	f, runner := h.f, h.credential.Runner
	profile := toolTestProfile()
	f.bearer("POST", "/pool-runner/tool-inventory", h.credential.Token, PoolToolInventoryRequest{RunnerID: runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []toolcontract.Observation{{Profile: profile, Available: true}}}, 200)
	p, err := f.s.CreateToolProfile(context.Background(), f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: runner.WorkerID, Name: "Current shared worker diagnostic", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	p = toolDiagnosticsEnable(t, f, p)
	f.bearer("GET", "/tool-profiles/"+p.ID+"/diagnostics?project_id="+f.project, h.credential.Token, nil, 401)
	for _, tc := range []struct{ name, key, query string }{
		{name: "active", key: "worker_current"},
		{name: "paused_pool", key: "pool_paused", query: `UPDATE suma_execution_pools SET status='paused' WHERE id=?`},
		{name: "expired_credential", key: "pool_expired", query: `UPDATE suma_execution_pool_credentials SET expires_at='2000-01-01T00:00:00Z' WHERE runner_id=?`},
		{name: "withdrawn_consent", key: "worker_unregistered", query: `UPDATE suma_execution_pool_grants SET status='revoked' WHERE pool_id=?`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.s.DB.Exec(`UPDATE suma_execution_pools SET status='active' WHERE id=?`, h.pool.ID); err != nil {
				t.Fatal(err)
			}
			if tc.query != "" {
				id := h.pool.ID
				if tc.name == "expired_credential" {
					id = runner.ID
				}
				if _, err := f.s.DB.Exec(tc.query, id); err != nil {
					t.Fatal(err)
				}
			}
			out := toolDiagnosticsRead(t, f, p)
			check := out.Checks[1]
			if check.ReasonDiagnostic == nil || check.ReasonDiagnostic.MessageKey != "display.tool."+tc.key || out.Tool.Available != (tc.name == "active") {
				t.Fatalf("pool identity did not follow original gate: %+v", out)
			}
			if tc.name == "active" && (check.Status != "pass" || out.NextAction != "none") || tc.name != "active" && (check.Status != "blocked" || out.NextAction != "open_worker") {
				t.Fatalf("pool observation implied a new grant: %+v", out)
			}
			toolDiagnosticsSameObservation(t, f, p, out)
		})
	}
}
