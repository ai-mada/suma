package toolpermissioncontract

import (
	"strings"
	"testing"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
)

func TestClaudePermissionUnionKeepsExactNativeSourceAndNoACK(t *testing.T) {
	raw := `{"command":"git status"}`
	source := cl.PermissionSource{Basis: cl.Basis{SessionID: "session192", ToolUseID: "tool192", RequestID: "native192", CallbackID: "host192", ConfigurationSHA256: strings.Repeat("c", 64), ParamsSHA256: cl.PayloadDigest(raw)}, ToolName: "Bash", InputJSON: raw}
	r := Request{IdempotencyKey: "original-operation192", Protocol: cl.Protocol, Claude: &source}
	choice := Choice{Protocol: cl.Protocol, Claude: &cl.PermissionChoice{Outcome: "allow_once"}}
	if ValidateRequest(r) != nil || !ChoiceMatchesRequest(r, choice) || !IsGrant(choice) {
		t.Fatal("exact one-call source rejected")
	}
	session, request, err := RequestSourceDigests(r)
	copy := source
	copy.CallbackID = "different-host-id"
	r.Claude = &copy
	otherSession, otherRequest, otherErr := RequestSourceDigests(r)
	if err != nil || otherErr != nil || session != otherSession || request != otherRequest {
		t.Fatal("regenerated host callback created another native request identity")
	}
	r.Codex = &cc.CodexApprovalRequest{}
	if ValidateRequest(r) == nil {
		t.Fatal("mixed native source accepted")
	}
	r.Codex, r.Claude = nil, &source
	digest, err := cl.PermissionDecisionDigest(source, *choice.Claude)
	if err != nil {
		t.Fatal(err)
	}
	p := acpPermissionFixture()
	p.Request, p.State = r, "decided"
	p.Decision = &Decision{Choice: choice, ActorID: "reviewer192", DecidedAt: "2026-10-31T05:01:00Z"}
	p.ClaudeObservation = &cl.PermissionObservation{Sequence: 1, ParamsSHA256: source.ParamsSHA256, AnswerSHA256: digest, AnswerObserved: true, ReplyStarted: true, ReplyWritten: true, ExecutionClosed: true}
	if ValidatePermission(p) != nil {
		t.Fatal("exact saved decision and callback delivery rejected")
	}
	p.ClaudeObservation.RequestResolved = true
	if ValidatePermission(p) == nil {
		t.Fatal("callback write fabricated native acceptance ACK")
	}
	p.ClaudeObservation.RequestResolved = false
	p.ClaudeObservation.AnswerSHA256 = strings.Repeat("e", 64)
	if ValidatePermission(p) == nil {
		t.Fatal("another decision digest borrowed approval")
	}
}
