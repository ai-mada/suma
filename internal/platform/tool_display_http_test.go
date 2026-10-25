package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/displaycontract"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func displayHTTP(t *testing.T, f *fixture, method, path, locale string, body any, want int) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
	r.AddCookie(f.cookie)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept-Language", locale)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s => %d want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func assertDisplayWire(t *testing.T, got map[string]any, key, source string) {
	t.Helper()
	if got["reason"] != source {
		t.Errorf("raw reason changed: %v", got["reason"])
	}
	raw, _ := json.Marshal(got["reason_diagnostic"])
	var d displaycontract.DisplayDiagnostic
	if err := json.Unmarshal(raw, &d); err != nil || d.MessageKey != key || displaycontract.ValidateToolReason(source, &d) == nil {
		t.Errorf("unbound identity: %s (%v)", raw, err)
	}
}
func TestToolDisplayHTTPConcurrentReadsAndOperationReceipts(t *testing.T) {
	f, _, p := toolFixture(t)
	ctx := context.Background()
	create := ToolProfileCreateRequest{ProjectID: f.project, WorkerID: p.WorkerID, Name: "HTTP display", Profile: p.Profile, IdempotencyKey: newID()}
	got := displayHTTP(t, f, "POST", "/tool-profiles", "en-US", create, 201)
	id := got["id"].(string)
	const paused = "工具档案已暂停，尚未授权派发"
	assertDisplayWire(t, got, "display.tool.profile_paused", paused)
	// Manufacture an old-format operation receipt by removing only the new optional
	// field. Replays must preserve its raw response and cannot retrofit identity.
	var old map[string]any
	raw, _ := json.Marshal(got)
	json.Unmarshal(raw, &old)
	delete(old, "reason_diagnostic")
	oldRaw := stringify(old)
	if _, err := f.s.DB.Exec(`UPDATE suma_operations SET response=? WHERE tenant_id=? AND project_id=? AND operation_id=?`, oldRaw, f.user.TenantID, f.project, create.IdempotencyKey); err != nil {
		t.Fatal(err)
	}
	var storedHash string
	if err := f.s.DB.QueryRow(`SELECT request_hash FROM suma_operations WHERE operation_id=?`, create.IdempotencyKey).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"en-US", "zh-CN"} {
		replay := displayHTTP(t, f, "POST", "/tool-profiles", locale, create, 201)
		if stringify(replay) != oldRaw {
			t.Fatal("old replay changed", stringify(replay), oldRaw)
		}
	}
	var version = 1
	for _, c := range []struct{ action, key, reason string }{{"enable", "inventory_missing", "执行节点尚未上报工具清单"}, {"pause", "profile_paused", paused}, {"revoke", "profile_revoked", "工具档案已永久撤销"}} {
		req := ToolProfileCommandRequest{ProjectID: f.project, Action: c.action, ExpectedVersion: version, IdempotencyKey: newID()}
		got = displayHTTP(t, f, "POST", "/tool-profiles/"+id+"/commands", "en-US", req, 200)
		assertDisplayWire(t, got, "display.tool."+c.key, c.reason)
		again := displayHTTP(t, f, "POST", "/tool-profiles/"+id+"/commands", "zh-CN", req, 200)
		if stringify(got) != stringify(again) {
			t.Fatal("command receipt language changed")
		}
		version++
	}
	// Concurrent authenticated HTTP list/detail reads use one raw fact in both
	// languages. No new operations, activities, attempts or observations result.
	snapshot := func() string {
		t.Helper()
		var out []any
		for _, table := range []string{"suma_operations", "suma_activity", "dag_runs", "dag_nodes", "suma_tool_inventories"} {
			var n int
			if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
				t.Fatal(err)
			}
			out = append(out, n)
		}
		var response, hash string
		if err := f.s.DB.QueryRow(`SELECT response,request_hash FROM suma_operations WHERE operation_id=?`, create.IdempotencyKey).Scan(&response, &hash); err != nil {
			t.Fatal(err)
		}
		out = append(out, response, hash)
		return stringify(out)
	}
	before := snapshot()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			locale := "zh-CN"
			if i%2 == 1 {
				locale = "en-US"
			}
			path := "/tool-profiles/" + id + "?project_id=" + f.project
			if i%4 < 2 {
				path = "/tool-profiles?project_id=" + f.project
			}
			r := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
			r.AddCookie(f.cookie)
			r.Header.Set("Accept-Language", locale)
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Errorf("concurrent read failed %d %s", w.Code, w.Body.String())
				return
			}
			var got map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Error(err)
				return
			}
			if items, ok := got["items"].([]any); ok {
				for _, v := range items {
					item := v.(map[string]any)
					if item["id"] == id {
						assertDisplayWire(t, item, "display.tool.profile_revoked", "工具档案已永久撤销")
					}
				}
			} else {
				assertDisplayWire(t, got, "display.tool.profile_revoked", "工具档案已永久撤销")
			}
		}(i)
	}
	wg.Wait()
	if snapshot() != before {
		t.Fatal("read changed receipts/activity/execution/observation counters")
	}
	if storedHash != digest(f.user.ID+stringify(map[string]any{"tool_create": create})) {
		t.Fatal("display metadata changed request hashing")
	}
	// Authority is checked before replay, including old response receipts.
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreateToolProfile(ctx, f.user, f.project, create); err == nil {
		t.Fatal("old receipt bypassed current authority")
	}
}
func TestToolDisplayHTTPRejectsInjectedHintsAndPreservesLongObservation(t *testing.T) {
	f, token, p := toolFixture(t)
	hint := map[string]any{"schema_version": 1, "message_key": "display.tool.dispatchable"}
	create := map[string]any{"project_id": f.project, "worker_id": p.WorkerID, "name": "Injected", "profile": p.Profile, "idempotency_key": newID(), "reason_diagnostic": hint}
	displayHTTP(t, f, "POST", "/tool-profiles", "en-US", create, 400)
	command := map[string]any{"project_id": f.project, "action": "enable", "expected_version": p.Version, "idempotency_key": newID(), "reason_diagnostic": hint}
	displayHTTP(t, f, "POST", "/tool-profiles/"+p.ID+"/commands", "en-US", command, 400)
	inventory := map[string]any{"project_id": f.project, "worker_id": p.WorkerID, "operation_id": newID(), "items": []any{map[string]any{"profile": p.Profile, "available": true, "reason_diagnostic": hint}}}
	f.bearer("POST", "/runner/tool-inventory", token, inventory, 400)
	for _, n := range []int{512, 513, 1000} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			reason := strings.Repeat("x", n)
			req := ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: p.Profile, Available: false, Reason: reason}}}
			if _, err := f.s.ReportToolInventory(context.Background(), f.user, f.project, token, req); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.DB.Exec(`UPDATE suma_tool_profiles SET state='enabled',enabled_by=? WHERE id=?`, f.user.ID, p.ID); err != nil {
				t.Fatal(err)
			}
			got := displayHTTP(t, f, "GET", "/tool-profiles/"+p.ID+"?project_id="+f.project, "en-US", nil, 200)
			source := "执行节点报告工具不可用：" + reason
			if got["reason"] != source || got["observation"].(map[string]any)["reason"] != reason {
				t.Fatal("raw external observation truncated/translated")
			}
			if n <= 512 {
				assertDisplayWire(t, got, "display.tool.reported_unavailable", source)
			} else if _, exists := got["reason_diagnostic"]; exists {
				t.Fatal("unprovable long reason retained display identity")
			}
			var persisted string
			if err := f.s.DB.QueryRow(`SELECT items FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, p.WorkerID).Scan(&persisted); err != nil || persisted != stringify(req.Items) {
				t.Fatal("raw observation changed", err)
			}
		})
	}
}

// Optional presentation must never make an otherwise valid durable response
// unreadable, nor cause replay to recalculate business eligibility.
func TestToolDisplayReceiptOptionalHintFallback(t *testing.T) {
	for _, operation := range []string{"create", "command"} {
		t.Run(operation, func(t *testing.T) {
			f, _, p := toolFixture(t)
			var request any
			path := "/tool-profiles"
			status := 201
			if operation == "create" {
				request = ToolProfileCreateRequest{ProjectID: f.project, WorkerID: p.WorkerID, Name: "receipt fallback", Profile: p.Profile, IdempotencyKey: newID()}
			} else {
				request = ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: p.Version, IdempotencyKey: newID()}
				path += "/" + p.ID + "/commands"
				status = 200
			}
			got := displayHTTP(t, f, "POST", path, "zh-CN", request, status)
			id := got["id"].(string)
			var key string
			if operation == "create" {
				key = request.(ToolProfileCreateRequest).IdempotencyKey
			} else {
				key = request.(ToolProfileCommandRequest).IdempotencyKey
			}
			originalRaw, _ := json.Marshal(got)
			var original map[string]json.RawMessage
			if err := json.Unmarshal(originalRaw, &original); err != nil {
				t.Fatal(err)
			}
			validHint := original["reason_diagnostic"]
			var originalHash string
			if err := f.s.DB.QueryRow(`SELECT request_hash FROM suma_operations WHERE operation_id=?`, key).Scan(&originalHash); err != nil {
				t.Fatal(err)
			}
			// The live resource deliberately differs. Replays must use the saved reason,
			// version and state even when their optional hint is unusable.
			if _, err := f.s.DB.Exec(`UPDATE suma_tool_profiles SET state='revoked',version=99 WHERE id=?`, id); err != nil {
				t.Fatal(err)
			}
			cases := []struct {
				name, hint string
				valid      bool
			}{
				{"absent", "", false}, {"valid", string(validHint), true},
				{"unknown_version", `{"schema_version":2,"message_key":"display.tool.profile_paused"}`, false},
				{"non_scalar_params", `{"schema_version":1,"message_key":"display.tool.profile_paused","message_params":{"extra":true}}`, false},
				{"wrong_shape", `true`, false},
				{"null_params", `{"schema_version":1,"message_key":"display.tool.profile_paused","message_params":null}`, false},
				{"source_mismatch", `{"schema_version":1,"message_key":"display.tool.profile_revoked"}`, false},
				{"unknown_key", `{"schema_version":1,"message_key":"display.tool.future"}`, false},
				{"unknown_field", `{"schema_version":1,"message_key":"display.tool.profile_paused","extra":true}`, false},
				{"duplicate_field", `{"schema_version":1,"schema_version":2,"message_key":"display.tool.profile_paused"}`, false},
				{"lossy_unicode", `{"schema_version":1,"message_key":"display.tool.reported_unavailable","message_params":{"reason":"\ud800"}}`, false},
			}
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					fields := map[string]json.RawMessage{}
					for k, v := range original {
						fields[k] = v
					}
					delete(fields, "reason_diagnostic")
					if c.hint != "" {
						fields["reason_diagnostic"] = json.RawMessage(c.hint)
					}
					saved, _ := json.Marshal(fields)
					if _, err := f.s.DB.Exec(`UPDATE suma_operations SET response=? WHERE operation_id=?`, string(saved), key); err != nil {
						t.Fatal(err)
					}
					expected := map[string]any{}
					if err := json.Unmarshal(originalRaw, &expected); err != nil {
						t.Fatal(err)
					}
					if !c.valid {
						delete(expected, "reason_diagnostic")
					}
					var beforeActivity int
					if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity`).Scan(&beforeActivity); err != nil {
						t.Fatal(err)
					}
					for _, locale := range []string{"zh-CN", "en-US"} {
						replay := displayHTTP(t, f, "POST", path, locale, request, status)
						if stringify(replay) != stringify(expected) {
							t.Errorf("optional hint changed authoritative receipt: got=%s want=%s", stringify(replay), stringify(expected))
						}
					}
					var persisted, hash, state string
					var activity, version int
					if err := f.s.DB.QueryRow(`SELECT response,request_hash FROM suma_operations WHERE operation_id=?`, key).Scan(&persisted, &hash); err != nil {
						t.Fatal(err)
					}
					if err := f.s.DB.QueryRow(`SELECT state,version FROM suma_tool_profiles WHERE id=?`, id).Scan(&state, &version); err != nil {
						t.Fatal(err)
					}
					if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity`).Scan(&activity); err != nil {
						t.Fatal(err)
					}
					if persisted != string(saved) || hash != originalHash || state != "revoked" || version != 99 || activity != beforeActivity {
						t.Fatal("fallback rewrote durable receipt, hash, resource or activity")
					}
				})
			}
			// Optional presentation tolerance must not hide corrupt authoritative fields.
			bad := map[string]json.RawMessage{}
			for k, v := range original {
				bad[k] = v
			}
			bad["version"] = json.RawMessage(`"not-an-integer"`)
			bad["reason_diagnostic"] = json.RawMessage(cases[2].hint)
			raw, _ := json.Marshal(bad)
			if _, err := f.s.DB.Exec(`UPDATE suma_operations SET response=? WHERE operation_id=?`, string(raw), key); err != nil {
				t.Fatal(err)
			}
			displayHTTP(t, f, "POST", path, "en-US", request, 500)
			if _, err := f.s.DB.Exec(`UPDATE suma_operations SET response=? WHERE operation_id=?`, string(originalRaw[:len(originalRaw)-1]), key); err != nil {
				t.Fatal(err)
			}
			displayHTTP(t, f, "POST", path, "en-US", request, 500)
			// Current permission is checked before even trying to read a corrupt receipt.
			if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
				t.Fatal(err)
			}
			displayHTTP(t, f, "POST", path, "en-US", request, 403)
		})
	}
}
