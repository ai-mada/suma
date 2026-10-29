package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type terminalPlatformFixture struct {
	f       *fixture
	token   string
	profile ToolProfile
	request WorkspaceTerminalCreateRequest
}

func newTerminalPlatformFixture(t *testing.T) terminalPlatformFixture {
	t.Helper()
	f, ctx := newFixture(t), context.Background()
	worker := "terminal-worker129"
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	definition := tc.Profile{CommandID: "shell129", Adapter: "command", Protocol: "text-v1", ToolVersion: "owned protocol shell fixture", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), TerminalSessionProtocol: term.Protocol}
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "Owned interactive terminal fixture", Profile: definition, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{{Profile: definition, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Interactive terminal context"}), 201)
	return terminalPlatformFixture{f: f, token: token, profile: p, request: WorkspaceTerminalCreateRequest{ProjectID: f.project, TaskID: task["id"].(string), ExpectedTaskVersion: int(task["version"].(float64)), ToolProfileID: p.ID, ExpectedToolVersion: p.Version, Columns: 100, Rows: 30, TimeoutSeconds: 600, AcknowledgeShell: true, IdempotencyKey: newID()}}
}

func (h terminalPlatformFixture) start(t *testing.T) (WorkspaceTerminalSession, dag.Assignment) {
	t.Helper()
	session, err := h.f.s.CreateWorkspaceTerminal(context.Background(), h.f.e, h.f.actor, h.f.project, h.request)
	if err != nil {
		t.Fatal(err)
	}
	claim := h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	var a dag.Assignment
	if err = json.Unmarshal([]byte(stringify(claim["assignment"])), &a); err != nil || a.RunID != session.RunID || a.DispatchID == "" {
		t.Fatal("original terminal assignment missing", claim, err)
	}
	return session, a
}

func terminalExchangeForTest(s WorkspaceTerminalSession, a dag.Assignment) term.ExchangeRequest {
	return term.ExchangeRequest{SessionID: s.ID, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, OperationID: newID(), OutputChunks: []term.OutputChunk{}, InputReceipts: []term.InputReceipt{}, State: "running", Directory: "owned-attempt-worktree", ProcessID: 129}
}

func TestWorkspaceTerminalOriginalDispatchAndUnknownInput(t *testing.T) {
	h, ctx := newTerminalPlatformFixture(t), context.Background()
	f := h.f
	session, a := h.start(t)
	if again, err := f.s.CreateWorkspaceTerminal(ctx, f.e, f.actor, f.project, h.request); err != nil || again.ID != session.ID {
		t.Fatal("create receipt did not preserve session", again, err)
	}
	rpc := terminalExchangeForTest(session, a)
	preflight := rpc
	preflight.State, preflight.Directory, preflight.ProcessID = "starting", "", 0
	preflight.OperationID = newID()
	if _, err := f.s.ExchangeWorkspaceTerminal(ctx, toolActorForTest(t, f, h.token), f.project, preflight); err != nil {
		t.Fatal(err)
	}
	queued, err := f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
	if err != nil || queued.State != "queued" {
		t.Fatal("Preauthorization cannot publish an unsupported or running PTY state", queued, err)
	}
	if _, err := f.s.ExchangeWorkspaceTerminal(ctx, toolActorForTest(t, f, h.token), f.project, rpc); err != nil {
		t.Fatal(err)
	}
	current, err := f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
	if err != nil || current.State != "running" {
		t.Fatal(current, err)
	}
	data := []byte("echo bounded\r")
	input := WorkspaceTerminalCommandRequest{ProjectID: f.project, ExpectedVersion: current.Version, ControlEpoch: 1, Action: "input", InputID: newID(), InputSequence: 1, DataBase64: base64.StdEncoding.EncodeToString(data), IdempotencyKey: newID()}
	receipt, err := f.s.CommandWorkspaceTerminal(ctx, f.actor, f.project, session.ID, input)
	if err != nil || receipt.Input == nil || receipt.Input.State != "accepted" {
		t.Fatal(receipt, err)
	}
	if again, e := f.s.CommandWorkspaceTerminal(ctx, f.actor, f.project, session.ID, input); e != nil || again.Input == nil || again.Input.InputID != receipt.Input.InputID {
		t.Fatal("input acceptance replay lost original", again, e)
	}
	rpc.OperationID = newID()
	response, err := f.s.ExchangeWorkspaceTerminal(ctx, toolActorForTest(t, f, h.token), f.project, rpc)
	if err != nil || len(response.Inputs) != 1 || response.Inputs[0].InputID != input.InputID {
		t.Fatal(response, err)
	}
	rpc.OperationID = newID()
	rpc.InputReceipts = []term.InputReceipt{{InputID: input.InputID, Sequence: 1, SHA256: digest(string(data)), State: "delivery_unknown", BytesWritten: 0}}
	response, err = f.s.ExchangeWorkspaceTerminal(ctx, toolActorForTest(t, f, h.token), f.project, rpc)
	if err != nil || len(response.Inputs) != 0 {
		t.Fatal("uncertain input was redelivered", response, err)
	}
	current, err = f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
	if err != nil || current.Reason != workspaceTerminalInputUnknown {
		t.Fatal(current, err)
	}
	input.ExpectedVersion, input.InputID, input.InputSequence, input.IdempotencyKey = current.Version, newID(), 2, newID()
	if _, err = f.s.CommandWorkspaceTerminal(ctx, f.actor, f.project, session.ID, input); err == nil {
		t.Fatal("new input bypassed unknown delivery")
	}
	stop := WorkspaceTerminalCommandRequest{ProjectID: f.project, ExpectedVersion: current.Version, ControlEpoch: 1, Action: "stop", IdempotencyKey: newID()}
	if _, err = f.s.CommandWorkspaceTerminal(ctx, f.actor, f.project, session.ID, stop); err != nil {
		t.Fatal(err)
	}
	rpc.OperationID = newID()
	rpc.InputReceipts = []term.InputReceipt{}
	response, err = f.s.ExchangeWorkspaceTerminal(ctx, toolActorForTest(t, f, h.token), f.project, rpc)
	if err != nil || !response.StopRequested || len(response.Inputs) != 0 {
		t.Fatal(response, err)
	}
	// A later receipt from the same single write can settle its delivery fact,
	// but cannot cancel the human's stop request or requeue those bytes.
	rpc.OperationID = newID()
	rpc.InputReceipts = []term.InputReceipt{{InputID: receipt.Input.InputID, Sequence: 1, SHA256: digest(string(data)), State: "delivered", BytesWritten: len(data)}}
	response, err = f.s.ExchangeWorkspaceTerminal(ctx, toolActorForTest(t, f, h.token), f.project, rpc)
	if err != nil || !response.StopRequested || len(response.Inputs) != 0 {
		t.Fatal("late original completion replayed input or cancelled stop", response, err)
	}
	current, err = f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
	if err != nil || current.Reason != "" || current.State != "stop_requested" {
		t.Fatal("late facts changed original stop policy", current, err)
	}
	var retained string
	if err = f.s.DB.QueryRow(`SELECT data_base64 FROM suma_workspace_terminal_inputs WHERE session_id=? AND input_id=?`, session.ID, receipt.Input.InputID).Scan(&retained); err != nil || retained != "" {
		t.Fatal("delivered input payload retained", retained, err)
	}
	actual, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.request.TaskID)
	if err != nil || actual.Version != h.request.ExpectedTaskVersion || actual.RunID != "" || actual.Status == "accepted" {
		t.Fatal("terminal changed development facts", actual, err)
	}
}

func TestWorkspaceTerminalOutputReplayAndOriginalStopProof(t *testing.T) {
	h, ctx := newTerminalPlatformFixture(t), context.Background()
	f := h.f
	session, a := h.start(t)
	rpc := terminalExchangeForTest(session, a)
	rpc.OutputChunks = []term.OutputChunk{{Sequence: 1, DataBase64: base64.StdEncoding.EncodeToString([]byte("real protocol bytes\r\n中文"))}}
	worker := toolActorForTest(t, f, h.token)
	for i := 0; i < 2; i++ {
		out, err := f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, rpc)
		if err != nil || out.OutputAckSequence != 1 {
			t.Fatal("output replay changed watermark", out, err)
		}
	}
	page, err := f.s.WorkspaceTerminalOutputs(ctx, f.actor, f.project, session.ID, 0)
	if err != nil || len(page.Chunks) != 1 || page.Gap {
		t.Fatal(page, err)
	}
	forged := rpc
	forged.OperationID = newID()
	forged.Fence++
	if _, err = f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, forged); err == nil {
		t.Fatal("late fence changed terminal")
	}
	forged = rpc
	forged.OperationID = newID()
	forged.DispatchID = newID()
	if _, err = f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, forged); err == nil {
		t.Fatal("forged dispatch changed terminal")
	}
	// Output retention is bounded and a caller receives an explicit replay gap.
	chunk := base64.StdEncoding.EncodeToString(make([]byte, term.MaxOutputChunkBytes))
	for batch := 0; batch < 3; batch++ {
		rpc.OperationID = newID()
		rpc.OutputChunks = []term.OutputChunk{}
		for i := 0; i < 24; i++ {
			rpc.OutputChunks = append(rpc.OutputChunks, term.OutputChunk{Sequence: int64(2 + batch*24 + i), DataBase64: chunk})
		}
		if _, err = f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, rpc); err != nil {
			t.Fatal(err)
		}
	}
	page, err = f.s.WorkspaceTerminalOutputs(ctx, f.actor, f.project, session.ID, 0)
	if err != nil || !page.Gap || page.FirstSequence < 2 || len(page.Chunks) > 64 {
		t.Fatal("bounded replay did not report gap", page, err)
	}
	rpc.OperationID = newID()
	rpc.OutputChunks = []term.OutputChunk{}
	rpc.State = "stopped"
	rpc.TreeStopped = true
	if _, err = f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, rpc); err != nil {
		t.Fatal(err)
	}
	if domainACount(t, f.s, "suma_workspace_terminal_active") != 1 {
		t.Fatal("tree stop released slot before original final result")
	}
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), Stopped: true, ExitCode: 0, ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, Terminal: "completed", EventCount: 1, UsageState: tc.UsageUnreported}}}), 200)
	if domainACount(t, f.s, "suma_workspace_terminal_active") != 0 {
		t.Fatal("known original final result did not release slot")
	}
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	f.bearer("GET", "/workspace/terminal-sessions/"+session.ID+"?project_id="+newID(), issued.Token, nil, 403)
	_, err = f.s.GetWorkspaceTerminal(ctx, worker, f.project, session.ID)
	requireAuthorityStatus(t, err, 403)
}
