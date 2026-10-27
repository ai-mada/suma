package workspacewritecontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkspaceWriteBindsTheCompleteCASRequest(t *testing.T) {
	r := CreateRequest{ProjectID: "project-one", Scope: Scope{RunID: "run-one", NodeID: "code", Attempt: 1, Fence: 2, WorkerID: "worker-one", ToolProfileID: "native", ProfileSHA256: strings.Repeat("a", 64)}, WorkspacePolicySHA256: strings.Repeat("b", 64), Path: "source.txt", ExpectedSHA256: strings.Repeat("c", 64), Content: "new text\n", ContentSHA256: ContentDigest("new text\n"), IdempotencyKey: "save-original"}
	if ValidateCreate(r) != nil {
		t.Fatal("valid bounded save rejected")
	}
	c := Command{ID: "command-one", ProjectID: r.ProjectID, Version: 2, State: "leased", Scope: r.Scope, WorkspacePolicySHA256: r.WorkspacePolicySHA256, Path: r.Path, ExpectedSHA256: r.ExpectedSHA256, ContentSHA256: r.ContentSHA256, RequestSHA256: RequestDigest(r), ExecutionFence: 1}
	e := &Effect{NativeOperationID: NativeOperationID(c.ID), Path: c.Path, PreviousSHA256: c.ExpectedSHA256, SHA256: c.ContentSHA256, Generation: 3}
	result := ResultRequest{ProjectID: c.ProjectID, WorkerID: c.Scope.WorkerID, CommandID: c.ID, Scope: c.Scope, WorkspacePolicySHA256: c.WorkspacePolicySHA256, RequestSHA256: c.RequestSHA256, ExecutionFence: 1, ClaimID: "execute-one", Mode: "execute", Outcome: "applied", Effect: e, ResultSHA256: ResultDigest("applied", e, "")}
	if err := ValidateResult(c, result); err != nil {
		t.Fatal(err)
	}
	result.Effect.PreviousSHA256 = strings.Repeat("d", 64)
	result.ResultSHA256 = ResultDigest("applied", e, "")
	if ValidateResult(c, result) == nil {
		t.Fatal("a different file transition was accepted")
	}
	r.Content = "changed body"
	if ValidateCreate(r) == nil {
		t.Fatal("body digest mismatch was accepted")
	}
	encoded, _ := json.Marshal(c)
	if strings.Contains(string(encoded), "new text") {
		t.Fatal("status exposed source content")
	}
}

func TestWorkspaceWriteRejectsAmbiguousJSONAndUnknownEffects(t *testing.T) {
	for _, raw := range []string{`{"content":"a","content":"b"}`, `{"content":"\ud800"}`, `{"scope":{"attempt":null}}`, `{"Scope":{}}`, `{"force":true}`} {
		var r CreateRequest
		if Decode([]byte(raw), &r) == nil {
			t.Fatal("ambiguous save input accepted", raw)
		}
	}
	if ValidError("raw private host error") {
		t.Fatal("unbounded host diagnostic accepted")
	}
}
