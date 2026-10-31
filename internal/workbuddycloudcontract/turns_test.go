package workbuddycloudcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCloudTurnPromptClosedRequiredZeroAndBounds(t *testing.T) {
	req := CloudTurnPromptRequest{ProjectID: "project", ExpectedSubmissionVersion: 1, ExpectedRunVersion: 1, ExpectedVersion: 0, Prompt: "A precise follow-up", IdempotencyKey: "turn-command169"}
	raw, _ := json.Marshal(req)
	var parsed CloudTurnPromptRequest
	if Decode(raw, &parsed) != nil || parsed.Validate() != nil {
		t.Fatal("explicit zero head must be a valid closed request")
	}
	for _, candidate := range []string{
		strings.Replace(string(raw), `"expected_version":0,`, "", 1),
		strings.Replace(string(raw), `"expected_version":0`, `"expected_version":null`, 1),
		strings.Replace(string(raw), `"expected_version":0`, `"expected_version":0.0`, 1),
		strings.TrimSuffix(string(raw), "}") + `,"prompt":"another"}`,
		strings.TrimSuffix(string(raw), "}") + `,"url":"https://example.test"}`,
		strings.Replace(string(raw), `"expected_run_version":1`, `"expected_run_version":0`, 1),
	} {
		if Decode([]byte(candidate), &parsed) == nil && parsed.Validate() == nil {
			t.Fatal("unreviewed request accepted")
		}
	}
	req.Prompt = strings.Repeat("界", 10667)
	if req.Validate() == nil {
		t.Fatal("prompt byte bound ignored")
	}
	req.Prompt = "Follow this code:\n\treturn next\r\n"
	if req.Validate() != nil {
		t.Fatal("ordinary multiline goal rejected")
	}
	req.Prompt = "unsafe\u0000"
	if req.Validate() == nil {
		t.Fatal("control text accepted")
	}
}

func TestCloudTurnPermissionAndPageFactsAreClosed(t *testing.T) {
	source := CloudTurnSource{Collaboration: CloudCollaborationSource{ProjectID: "project", RunID: "run", NodeID: "cloud", SubmissionID: "submission", SubmissionVersion: 1, TaskID: "cloud-task", NodeSpecSHA256: strings.Repeat("a", 64), RequestSHA256: strings.Repeat("b", 64)}, Attempt: 1, Fence: 1, DispatchID: "dispatch", AssignmentSHA256: strings.Repeat("c", 64)}
	turn := CloudTurn{ID: "turn", Version: 1, OperationID: "turn-operation169", Source: source, OwnerID: "owner", State: "prepared", PromptSHA256: strings.Repeat("d", 64), StartedAt: "2026-10-31T00:00:00Z", DeadlineAt: "2026-10-31T00:15:00Z", UsageState: "unreported"}
	page := CloudTurnPage{Turn: turn, Items: []CloudTurnEntry{}, Permissions: []CloudTurnPermission{}, Commands: []CloudTurnCommand{}}
	if turn.Validate() != nil || page.Validate() != nil {
		t.Fatal("valid unreported prepared page rejected")
	}
	page.Items = nil
	if page.Validate() == nil {
		t.Fatal("required collection null accepted")
	}
	permission := CloudTurnPermission{ID: "permission", TurnID: turn.ID, Version: 1, ToolCallID: "tool", Title: "Edit", Details: "safe", DetailsSHA256: strings.Repeat("a", 64), State: "pending", Options: []CloudTurnPermissionOption{{ID: "allow", Name: "Allow once", Kind: "allow_once"}, {ID: "reject", Name: "Reject once", Kind: "reject_once"}}}
	if permission.Validate() != nil {
		t.Fatal("safe rejection-only permission rejected")
	}
	permission.ApproveAllowed = true
	if permission.Validate() == nil {
		t.Fatal("unverified details became approvable")
	}
	permission.ApproveAllowed = false
	permission.Details = "{\n  \"path\": \"safe\"\n}"
	permission.ToolCallID = "tool:权限"
	permission.Options[0].ID = "option:一次"
	if permission.Validate() != nil {
		t.Fatal("multiline displayed details/provider token rejected")
	}
	commandCode := CloudTurnCommand{ID: "c", TurnID: turn.ID, OperationID: "operation-169", Kind: "cancel", State: "unknown", ErrorCode: "workbuddy_cloud_turn_arbitrary"}
	if commandCode.Validate() == nil {
		t.Fatal("unknown safe-code namespace accepted")
	}
	command := CloudTurnCommand{ID: "command", TurnID: turn.ID, OperationID: "turn-decision169", Kind: "permission", State: "prepared", PermissionID: permission.ID, OptionID: "reject"}
	if command.Validate() != nil {
		t.Fatal("exact permission command rejected")
	}
	command.Kind = "arbitrary_rpc"
	if command.Validate() == nil {
		t.Fatal("arbitrary RPC accepted")
	}
}
