package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func nativeProgressHTTPFixture(t *testing.T) (*fixture, string, dag.Assignment) {
	t.Helper()
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	if _, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: profile.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	profile, err := f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w := f.call("POST", "/workflows", f.body(map[string]any{"name": "Native diagnostic HTTP fixture", "nodes": []dag.Node{{ID: "agent", Type: "agent", Command: []string{profile.Profile.CommandID}, Config: map[string]any{"goal": "fixture only", "tool_profile_id": profile.ID, "tool_profile_sha256": profile.Profile.ProfileSHA256}}}}), 201)
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"]}), 201)
	claimed := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
	var a dag.Assignment
	raw, _ := json.Marshal(claimed["assignment"])
	if err = json.Unmarshal(raw, &a); err != nil || a.WorkerID != profile.WorkerID {
		t.Fatalf("no current native assignment: %+v %v", a, err)
	}
	return f, token, a
}

func nativeProgressRequest(t *testing.T, f *fixture, token, locale, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "http://suma.test/api/v1/runner/progress", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Suma-Language", locale)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("non-JSON HTTP diagnostic: %d %s", w.Code, w.Body.String())
	}
	return w.Code, response
}

func TestRunnerProgressMalformedWireDiagnosticUsesControlledHTTPReason(t *testing.T) {
	f, token, a := nativeProgressHTTPFixture(t)
	assignment, _ := json.Marshal(a)
	base := `{"schema_version":1,"command_id":"native-agent","profile_sha256":"` + strings.Repeat("a", 64) + `","protocol":"suma-jsonl-v1","kind":"error","message":%s,"diagnostic":%s}`
	for name, input := range map[string]struct{ message, diagnostic string }{
		"version":                    {`"Model returned no text"`, `{"schema_version":2,"message_key":"native.response.no_text"}`},
		"null params":                {`"Model returned no text"`, `{"schema_version":1,"message_key":"native.response.no_text","message_params":null}`},
		"params alias null":          {`"Model returned no text"`, `{"schema_version":1,"message_key":"native.response.no_text","MESSAGE_PARAMS":null}`},
		"key alias":                  {`"Model returned no text"`, `{"schema_version":1,"MESSAGE_KEY":"native.response.no_text"}`},
		"semantic version duplicate": {`"Model returned no text"`, `{"schema_version":1,"SCHEMA_VERSION":1,"message_key":"native.response.no_text"}`},
		"lossy nested Unicode":       {`"CLI probe exited \ud800"`, `{"schema_version":1,"message_key":"native.catalog.probe_exited","message_params":{"status":"\ud800"}}`},
		"malformed parameter only":   {`"CLI probe exited �"`, `{"schema_version":1,"message_key":"native.catalog.probe_exited","message_params":{"status":"\ud800"}}`},
	} {
		for _, locale := range []string{"en-US", "zh-CN"} {
			t.Run(name+"/"+locale, func(t *testing.T) {
				event := fmt.Sprintf(base, input.message, input.diagnostic)
				body := fmt.Sprintf(`{"project_id":%q,"assignment":%s,"operation_id":%q,"tool_event":%s}`, f.project, assignment, newID(), event)
				status, response := nativeProgressRequest(t, f, token, locale, body)
				errorBody, _ := response["error"].(map[string]any)
				want := "Tool diagnostic metadata is invalid."
				if locale == "zh-CN" {
					want = "工具诊断元数据无效"
				}
				if status != 400 || errorBody["message_key"] != "dag.tool.diagnostic" || errorBody["message"] != want || errorBody["localized"] != true {
					t.Errorf("wrong controlled HTTP reason: status=%d response=%v", status, response)
				}
			})
		}
	}
	var events, receipts int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_events WHERE run_id=? AND event_type='tool_progress'`, a.RunID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_operations WHERE scope=?`, "progress:"+a.RunID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if events != 0 || receipts != 0 {
		t.Fatalf("malformed diagnostics created durable facts: events=%d receipts=%d", events, receipts)
	}
}

func TestRunnerProgressStrictEventBodyKeepsAuthorizationNullAndReceiptSemantics(t *testing.T) {
	f, token, a := nativeProgressHTTPFixture(t)
	assignment, _ := json.Marshal(a)
	event := `{"schema_version":1,"command_id":"native-agent","profile_sha256":"` + strings.Repeat("a", 64) + `","protocol":"suma-jsonl-v1","kind":"error","message":"Model returned no text","diagnostic":null}`
	body := fmt.Sprintf(`{"project_id":%q,"assignment":%s,"operation_id":"native-null-receipt","tool_event":%s}`, f.project, assignment, event)
	if status, _ := nativeProgressRequest(t, f, "", "en-US", body); status != 401 {
		t.Fatalf("missing credential status=%d", status)
	}
	apiToken := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	if status, _ := nativeProgressRequest(t, f, apiToken, "en-US", body); status != 403 {
		t.Fatalf("wrong scope status=%d", status)
	}
	for _, bad := range []string{strings.Replace(event, `"diagnostic":null`, `"DIAGNOSTIC":null`, 1), strings.Replace(event, `"diagnostic":null`, `"raw_arguments":"never-persist"`, 1)} {
		request := fmt.Sprintf(`{"project_id":%q,"assignment":%s,"operation_id":%q,"tool_event":%s}`, f.project, assignment, newID(), bad)
		status, response := nativeProgressRequest(t, f, token, "en-US", request)
		errorBody, _ := response["error"].(map[string]any)
		if status != 400 || errorBody["message_key"] != "control.runner.tool_event_object_required" || errorBody["localized"] != true {
			t.Errorf("unknown event field escaped controlled body reason: %d %v", status, response)
		}
	}
	for range 2 {
		if status, response := nativeProgressRequest(t, f, token, "en-US", body); status != 200 || response["acknowledged"] != true {
			t.Fatalf("null absence or duplicate receipt changed: %d %v", status, response)
		}
	}
	changed := strings.Replace(body, "Model returned no text", "different original body", 1)
	if status, _ := nativeProgressRequest(t, f, token, "en-US", changed); status != 409 {
		t.Fatalf("changed body reused operation: %d", status)
	}
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_events WHERE run_id=? AND event_type='tool_progress'`, a.RunID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("invalid/duplicate event persisted: count=%d %v", count, err)
	}
}
