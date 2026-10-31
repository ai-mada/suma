package toolpermissioncontract

import (
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

func nativePermissionRequest() Request {
	return Request{IdempotencyKey: "native-request-174", Protocol: cc.Protocol, Codex: &cc.CodexApprovalRequest{RequestID: cc.CodexRPCRequestID{Kind: "number", Value: "9223372036854775807"}, Method: "item/commandExecution/requestApproval", ThreadID: "thread-174", TurnID: "turn-174", ItemID: "command-174", StartedAtMS: 1793448000000, BindingSHA256: strings.Repeat("a", 64), ParamsSHA256: strings.Repeat("b", 64), ItemSHA256: strings.Repeat("c", 64), Title: "Run exact command", Details: "Review this command", DetailsComplete: true, ScopeSupported: true, Command: "git status", CWD: "."}}
}

func TestNativePermissionUnionAndExactRPCIndex(t *testing.T) {
	r := nativePermissionRequest()
	raw, _ := json.Marshal(r)
	var decoded Request
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Codex.RequestID.Value != "9223372036854775807" {
		t.Fatal("exact int64 RPC value lost", err)
	}
	session, request, err := RequestSourceDigests(r)
	if err != nil {
		t.Fatal(err)
	}
	changed := r
	copy := *r.Codex
	changed.Codex = &copy
	changed.Codex.RequestID.Kind = "string"
	sameSession, differentRequest, err := RequestSourceDigests(changed)
	if err != nil || session != sameSession || request == differentRequest {
		t.Fatal("number and string identities conflated", err)
	}
	changed.Codex.RequestID.Kind = "number"
	changed.Codex.ItemID = "different-item"
	changed.Codex.ParamsSHA256 = strings.Repeat("d", 64)
	_, sameIndex, err := RequestSourceDigests(changed)
	if err != nil || sameIndex != request {
		t.Fatal("same native ID could evade duplicate-body collision", err)
	}
	r.ACP = acpPermissionFixture().Request.ACP
	if ValidateRequest(r) == nil {
		t.Fatal("mixed protocol source accepted")
	}
}

func TestNativePermissionObservationPreservesHumanAndRejectsFalsePresence(t *testing.T) {
	r := nativePermissionRequest()
	o := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: r.Codex.ParamsSHA256, Decision: "accept", DecisionObserved: true}
	raw, _ := json.Marshal(o)
	for _, invalid := range []string{strings.Replace(string(raw), `"reply_written":false,`, "", 1), strings.Replace(string(raw), `"reply_written":false`, `"reply_written":false,"reply_written":true`, 1), strings.Replace(string(raw), `"reply_written":false`, `"reply_written":null`, 1)} {
		var value cc.CodexApprovalObservation
		if json.Unmarshal([]byte(invalid), &value) == nil {
			t.Fatal("incomplete or ambiguous observation accepted")
		}
	}
	p := acpPermissionFixture()
	p.Request = r
	p.State = "expired"
	p.Version = 2
	p.Observation = &o
	p.Decision = &Decision{Choice: Choice{Protocol: cc.Protocol, Codex: &cc.CodexApprovalChoice{Decision: "accept"}}, ActorID: "reviewer-174", DecidedAt: "2026-10-31T05:01:00Z"}
	if ValidatePermission(p) != nil {
		t.Fatal("terminal view erased an original valid decision")
	}
	p.Decision.Codex.Decision = "decline"
	if ValidatePermission(p) == nil {
		t.Fatal("observation contradicted the recorded human choice")
	}
	p.Decision = nil
	p.State = "pending"
	o.Decision = ""
	o.DecisionObserved = false
	o.RequestResolved = true
	p.AllowedActions = []string{"accept"}
	if ValidatePermission(p) == nil {
		t.Fatal("remote cleanup left a new grant action")
	}
}

func TestNativePermissionUnsupportedScopeHasNoGrant(t *testing.T) {
	r := nativePermissionRequest()
	r.Codex.ScopeSupported = false
	r.Codex.UnsupportedReason = "unsupported_persistent_scope"
	if ChoiceMatchesRequest(r, Choice{Protocol: cc.Protocol, Codex: &cc.CodexApprovalChoice{Decision: "accept"}}) {
		t.Fatal("unsupported request acquired a grant")
	}
	if !ChoiceMatchesRequest(r, Choice{Protocol: cc.Protocol, Codex: &cc.CodexApprovalChoice{Decision: "decline"}}) {
		t.Fatal("unsupported request could not be declined")
	}
	if ChoiceMatchesRequest(r, Choice{Protocol: cc.Protocol, Codex: &cc.CodexApprovalChoice{Decision: "acceptForSession"}}) {
		t.Fatal("session grant entered platform choice")
	}
}
