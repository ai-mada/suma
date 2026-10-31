package acpcontract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestACPToolPermissionClosedRequestAndDecision(t *testing.T) {
	valid := `{"idempotency_key":"permission-original-170","session_id":"session170","tool_call_id":"tool170","title":"Write reviewed file","kind":"edit","details":"line one\nline two","details_complete":false,"options":[{"id":"once","name":"Allow once","kind":"allow_once"},{"id":"no","name":"Reject once","kind":"reject_once"}]}`
	var request PermissionRequest
	if err := json.Unmarshal([]byte(valid), &request); err != nil || request.DetailsComplete {
		t.Fatal("valid incomplete request rejected", err)
	}
	for _, raw := range []string{
		strings.Replace(valid, `"details_complete":false,`, "", 1),
		strings.Replace(valid, `"details_complete":false`, `"details_complete":null`, 1),
		strings.Replace(valid, `"details_complete":false`, `"details_complete":false,"details_complete":true`, 1),
		strings.Replace(valid, `"details_complete":false`, `"Details_Complete":false`, 1),
		strings.Replace(valid, `"title":"Write reviewed file"`, `"title":"\ud800"`, 1),
		strings.Replace(valid, `"kind":"edit"`, `"kind":"edit","url":"https://untrusted.invalid"`, 1),
		strings.Replace(valid, `"id":"no"`, `"id":"once"`, 1),
	} {
		if json.Unmarshal([]byte(raw), &request) == nil {
			t.Fatal("unsafe request accepted", raw)
		}
	}
	var decision DecisionRequest
	for _, raw := range []string{
		`{"project_id":"project170","expected_version":1,"idempotency_key":"decision170","option_id":"no"}`,
	} {
		if err := json.Unmarshal([]byte(raw), &decision); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{
		`{"project_id":"project170","expected_version":0,"idempotency_key":"decision170","option_id":"no"}`,
		`{"project_id":"project170","expected_version":null,"idempotency_key":"decision170","option_id":"no"}`,
		`{"project_id":"project170","expected_version":1,"idempotency_key":"decision170","option_id":"no","kind":"allow_always"}`,
	} {
		if json.Unmarshal([]byte(raw), &decision) == nil {
			t.Fatal("unsafe decision accepted", raw)
		}
	}
}

func TestACPToolPermissionListAndEscapedRequestRemainClosed(t *testing.T) {
	p := Permission{ID: "permission170", ProjectID: "project170", RunID: "run170", NodeID: "node170", Attempt: 1, Fence: 1, DispatchID: "dispatch170", WorkerID: "worker170", ProfileSHA256: strings.Repeat("a", 64), Version: 1, State: "pending", CreatedAt: "2026-10-31T05:00:00Z", ExpiresAt: "2026-10-31T05:05:00Z", AllowedActions: []string{}, Request: PermissionRequest{IdempotencyKey: "permission-170", SessionID: "session170", ToolCallID: "tool170", Title: "Write file", Kind: "edit", Details: strings.Repeat("<", 10800), DetailsComplete: true, Options: []PermissionOption{{ID: "once", Name: "Once", Kind: "allow_once"}}}}
	request, _ := json.Marshal(p.Request)
	if len(request) > 64<<10 {
		t.Fatal("test request exceeds HTTP boundary")
	}
	raw, _ := json.Marshal(p)
	var decoded Permission
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal("valid escaped request poisoned its larger response envelope", err)
	}
	list := `{"items":[],"items":[]}`
	var values PermissionList
	if json.Unmarshal([]byte(list), &values) == nil {
		t.Fatal("duplicate list field accepted")
	}
	raw, _ = json.Marshal(p)
	raw = bytes.Replace(raw, []byte(`"allowed_actions":[]`), []byte(`"allowed_actions":[],"Decision":null`), 1)
	if json.Unmarshal(raw, &decoded) == nil {
		t.Fatal("case alias optional decision accepted")
	}
}

func TestACPToolPermissionTerminalViewPreservesHumanDecision(t *testing.T) {
	p := Permission{ID: "permission170", ProjectID: "project170", RunID: "run170", NodeID: "node170", Attempt: 1, Fence: 1, DispatchID: "dispatch170", WorkerID: "worker170", ProfileSHA256: strings.Repeat("a", 64), Version: 2, State: "decided", CreatedAt: "2026-10-31T05:00:00Z", ExpiresAt: "2026-10-31T05:05:00Z", AllowedActions: []string{}, Request: PermissionRequest{IdempotencyKey: "permission-170", SessionID: "session170", ToolCallID: "tool170", Title: "Write file", Kind: "edit", Details: "exact data", DetailsComplete: true, Options: []PermissionOption{{ID: "once", Name: "Once", Kind: "allow_once"}}}, Decision: &PermissionDecision{OptionID: "once", Kind: "allow_once", ActorID: "reviewer170", DecidedAt: "2026-10-31T05:01:00Z"}}
	raw, _ := json.Marshal(p)
	var view Permission
	if err := json.Unmarshal(raw, &view); err != nil || view.Decision == nil {
		t.Fatal("stored decision fact rejected", err)
	}
	p.State = "expired"
	raw, _ = json.Marshal(p)
	if json.Unmarshal(raw, &view) == nil {
		t.Fatal("historical decision became a new grant")
	}
}
