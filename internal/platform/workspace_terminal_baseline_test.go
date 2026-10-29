package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestWorkspaceTerminalFreezesAcceptedBaselineWithoutBindingTaskRun(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	composition, _ := compositionCompleteFixedCandidate(t, h, h.start(t, h.create(t)))
	composition = compositionAcceptFixedCandidate(t, h, composition)
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Continue an explicitly accepted code baseline"}))
	if err != nil {
		t.Fatal(err)
	}
	task = compositionBindBaseline(t, h, composition, task, 0)
	worker := "baseline-terminal-worker131"
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	definition := tc.Profile{CommandID: "baseline-shell131", Adapter: "command", Protocol: "text-v1", ToolVersion: "Owned baseline terminal protocol fixture", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), TerminalSessionProtocol: term.Protocol}
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "Baseline terminal protocol fixture", Profile: definition, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{{Profile: definition, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	before := workspaceTerminalTaskSQL(t, f, task.ID)
	session, err := f.s.CreateWorkspaceTerminal(ctx, f.e, f.actor, f.project, WorkspaceTerminalCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, Columns: 100, Rows: 30, TimeoutSeconds: 600, AcknowledgeShell: true, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if session.CodeBaseline == nil || *session.CodeBaseline != *task.CodeBaseline || session.BaseCommit != task.CodeBaseline.Source.HeadCommit || session.AdminBaseCommit != task.CodeBaseline.Source.BundleBase() {
		t.Fatal("terminal did not freeze the actual adopted code source", session)
	}
	if workspaceTerminalTaskSQL(t, f, task.ID).SHA256 != before.SHA256 {
		t.Fatal("terminal creation bound or changed the business task run")
	}
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": worker, "capabilities": []string{"command:baseline-shell131"}, "idempotency_key": newID()}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &a) != nil || a.CodeBaseline == nil || *a.CodeBaseline != *task.CodeBaseline {
		t.Fatal("terminal claim lost its protected baseline", claim)
	}
	page, err := f.s.ListWorkspaceTerminals(ctx, f.actor, f.project, task.ID)
	if err != nil || page.CodeBaseline == nil || !page.BaselineCurrent {
		t.Fatal("page baseline is not the same authorized task snapshot", page, err)
	}
}
