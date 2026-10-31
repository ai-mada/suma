package toolpermissioncontract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	ac "github.com/ai-mada/suma/internal/acpcontract"
)

func acpPermissionFixture() Permission {
	return Permission{ID: "permission170", ProjectID: "project170", RunID: "run170", NodeID: "node170", Attempt: 1, Fence: 1, DispatchID: "dispatch170", WorkerID: "worker170", ProfileSHA256: strings.Repeat("a", 64), Version: 1, State: "pending", CreatedAt: "2026-10-31T05:00:00Z", ExpiresAt: "2026-10-31T05:05:00Z", AllowedActions: []string{}, Request: Request{IdempotencyKey: "permission-170", Protocol: ac.Protocol, ACP: &ac.NativeSource{SessionID: "session170", ToolCallID: "tool170", Title: "Write file", Kind: "edit", Details: "exact data", DetailsComplete: true, Options: []ac.PermissionOption{{ID: "once", Name: "Once", Kind: "allow_once"}, {ID: "no", Name: "Reject", Kind: "reject_once"}}}}}
}

func TestACPToolPermissionClosedRequestAndDecision(t *testing.T) {
	valid := `{"idempotency_key":"permission-original-170","protocol":"acp-stdio-v1","acp":{"session_id":"session170","tool_call_id":"tool170","title":"Write reviewed file","kind":"edit","details":"line one\nline two","details_complete":false,"options":[{"id":"once","name":"Allow once","kind":"allow_once"},{"id":"no","name":"Reject once","kind":"reject_once"}]}}`
	var request Request
	if err := json.Unmarshal([]byte(valid), &request); err != nil || request.ACP.DetailsComplete {
		t.Fatal("valid incomplete native source rejected", err)
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
	validDecision := `{"project_id":"project170","expected_version":1,"idempotency_key":"decision170","protocol":"acp-stdio-v1","acp":{"option_id":"no","kind":"reject_once"}}`
	var decision DecisionRequest
	if err := json.Unmarshal([]byte(validDecision), &decision); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{strings.Replace(validDecision, `"expected_version":1`, `"expected_version":0`, 1), strings.Replace(validDecision, `"expected_version":1`, `"expected_version":null`, 1), strings.Replace(validDecision, `"kind":"reject_once"`, `"kind":"allow_always"`, 1)} {
		if json.Unmarshal([]byte(raw), &decision) == nil {
			t.Fatal("unsafe decision accepted", raw)
		}
	}
}

func TestACPToolPermissionListAndEscapedRequestRemainClosed(t *testing.T) {
	p := acpPermissionFixture()
	p.Request.ACP.Details = strings.Repeat("<", 10800)
	request, _ := json.Marshal(p.Request)
	if len(request) > 64<<10 {
		t.Fatal("fixture request exceeds typed boundary")
	}
	raw, _ := json.Marshal(p)
	var decoded Permission
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal("escaped native source poisoned larger response", err)
	}
	var values List
	if json.Unmarshal([]byte(`{"items":[],"items":[]}`), &values) == nil {
		t.Fatal("duplicate list property accepted")
	}
	raw = bytes.Replace(raw, []byte(`"allowed_actions":[]`), []byte(`"allowed_actions":[],"Decision":null`), 1)
	if json.Unmarshal(raw, &decoded) == nil {
		t.Fatal("case alias accepted")
	}
}

func TestACPToolPermissionTerminalViewPreservesHumanDecision(t *testing.T) {
	p := acpPermissionFixture()
	p.Version = 2
	p.State = "expired"
	p.Decision = &Decision{Choice: Choice{Protocol: ac.Protocol, ACP: &ac.Choice{OptionID: "once", Kind: "allow_once"}}, ActorID: "reviewer170", DecidedAt: "2026-10-31T05:01:00Z"}
	raw, _ := json.Marshal(p)
	var view Permission
	if err := json.Unmarshal(raw, &view); err != nil || view.Decision == nil || view.Decision.ACP.OptionID != "once" || len(view.AllowedActions) != 0 {
		t.Fatal("terminal projection erased original human fact", err)
	}
}
