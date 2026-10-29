package platform

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestWorkspaceTerminalRecoverySeparatesOriginalStopAndUnknownEffectReview(t *testing.T) {
	h, ctx := newTerminalPlatformFixture(t), context.Background()
	f := h.f
	session, a := h.start(t)
	worker := toolActorForTest(t, f, h.token)
	rpc := terminalExchangeForTest(session, a)
	if _, err := f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, rpc); err != nil {
		t.Fatal(err)
	}
	current, err := f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("one original effect\r")
	accepted, err := f.s.CommandWorkspaceTerminal(ctx, f.actor, f.project, session.ID, WorkspaceTerminalCommandRequest{ProjectID: f.project, ExpectedVersion: current.Version, ControlEpoch: 1, Action: "input", InputID: newID(), InputSequence: 1, DataBase64: base64.StdEncoding.EncodeToString(data), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	rpc.OperationID = newID()
	response, err := f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, rpc)
	if err != nil || len(response.Inputs) != 1 {
		t.Fatal(response, err)
	}
	// The input was offered; loss of its acknowledgement cannot become proof
	// of non-delivery merely because a later original stop receipt arrives.
	rpc.OperationID = newID()
	rpc.State = "stopped"
	rpc.TreeStopped = true
	rpc.ExitCode = 1
	if _, err = f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, rpc); err != nil {
		t.Fatal(err)
	}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), ExitCode: 1, Stopped: true, OutcomeUnknown: true, ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, SessionID: session.ID, Terminal: "unknown", UsageState: tc.UsageUnreported}}
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	view, err := f.s.GetWorkspaceTerminalRecovery(ctx, f.actor, f.project, session.ID)
	if err != nil || !view.CanRecover || !view.TreeStopped || len(view.UnknownInputs) != 1 || view.UnknownInputs[0].InputID != accepted.Input.InputID {
		t.Fatal(view, err)
	}
	current, err = f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	req := WorkspaceTerminalRecoveryRequest{ProjectID: f.project, ExpectedVersion: current.Version, ExpectedRunVersion: view.RunVersion, RunID: a.RunID, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, Reason: "Reviewed original stopped execution and accept its unknown input effect without replay", IdempotencyKey: newID()}
	if _, err = f.s.RecoverWorkspaceTerminal(ctx, f.e, f.actor, f.project, session.ID, req); err == nil {
		t.Fatal("unknown input was not individually reviewed")
	}
	req.UnknownInputs = append([]WorkspaceTerminalRecoveryInput{}, view.UnknownInputs...)
	forged := req
	forged.Fence++
	if _, err = f.s.RecoverWorkspaceTerminal(ctx, f.e, f.actor, f.project, session.ID, forged); err == nil {
		t.Fatal("foreign attempt fence recovery accepted")
	}
	closed, err := f.s.RecoverWorkspaceTerminal(ctx, f.e, f.actor, f.project, session.ID, req)
	if err != nil || closed.Session.State != "stopped" || closed.Recovery.CanRecover {
		t.Fatal(closed, err)
	}
	if again, e := f.s.RecoverWorkspaceTerminal(ctx, f.e, f.actor, f.project, session.ID, req); e != nil || again.Session.ID != closed.Session.ID {
		t.Fatal("original recovery receipt lost", again, e)
	}
	var state string
	if err = f.s.DB.QueryRow(`SELECT state FROM suma_workspace_terminal_inputs WHERE session_id=? AND input_id=?`, session.ID, accepted.Input.InputID).Scan(&state); err != nil || state != "delivery_unknown" {
		t.Fatal("review rewrote original unknown delivery", state, err)
	}
	if domainACount(t, f.s, "suma_workspace_terminal_active") != 0 {
		t.Fatal("reviewed stopped original retained task slot")
	}
	run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, a.RunID)
	if err != nil || run.Nodes[0].State != "failed" || run.Nodes[0].Attempt != 1 {
		t.Fatal("recovery reran or certified success", run, err)
	}
	rpc.OperationID = newID()
	late, err := f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, rpc)
	if err != nil || late.RecoveryOperationID != req.IdempotencyKey || !late.StopRequested || len(late.Inputs) != 0 {
		t.Fatal("original outbox did not get explicit reviewed disposition", late, err)
	}
}

func TestWorkspaceTerminalRecoveryCannotReplaceTreeStopProof(t *testing.T) {
	h, ctx := newTerminalPlatformFixture(t), context.Background()
	f := h.f
	session, a := h.start(t)
	rpc := terminalExchangeForTest(session, a)
	if _, err := f.s.ExchangeWorkspaceTerminal(ctx, toolActorForTest(t, f, h.token), f.project, rpc); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), OutcomeUnknown: true, ExitCode: -1, ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, SessionID: session.ID, Terminal: "unknown", UsageState: tc.UsageUnreported}}}), 200)
	view, err := f.s.GetWorkspaceTerminalRecovery(ctx, f.actor, f.project, session.ID)
	if err != nil || view.CanRecover || view.TreeStopped {
		t.Fatal(view, err)
	}
	current, err := f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.RecoverWorkspaceTerminal(ctx, f.e, f.actor, f.project, session.ID, WorkspaceTerminalRecoveryRequest{ProjectID: f.project, ExpectedVersion: current.Version, ExpectedRunVersion: view.RunVersion, RunID: a.RunID, Attempt: 1, Fence: a.Fence, DispatchID: a.DispatchID, Reason: "User review is not process stop evidence", IdempotencyKey: newID()})
	if err == nil || domainACount(t, f.s, "suma_workspace_terminal_active") != 1 {
		t.Fatal("manual review bypassed original tree stop", err)
	}
}

func TestWorkspaceTerminalExpiredOriginalStopRecoveryRetainsFailedDisposition(t *testing.T) {
	h, ctx := newTerminalPlatformFixture(t), context.Background()
	f := h.f
	session, a := h.start(t)
	worker := toolActorForTest(t, f, h.token)
	rpc := terminalExchangeForTest(session, a)
	rpc.State = "exited"
	rpc.TreeStopped = true
	rpc.ExitCode = 0
	if _, err := f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, rpc); err != nil {
		t.Fatal(err)
	}
	// Advance the persisted scheduler wake of this exact fixture assignment;
	// expiration itself cannot release the original task/effect hold.
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id='terminal'`, a.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE dag_wakeups SET due_at=0 WHERE run_id=? AND node_id='terminal' AND kind='lease'`, a.RunID); err != nil {
		t.Fatal(err)
	}
	if err := f.e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	view, err := f.s.GetWorkspaceTerminalRecovery(ctx, f.actor, f.project, session.ID)
	if err != nil || !view.CanRecover || !view.TreeStopped || domainACount(t, f.s, "suma_workspace_terminal_active") != 1 {
		t.Fatal(view, err)
	}
	current, err := f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	req := WorkspaceTerminalRecoveryRequest{ProjectID: f.project, ExpectedVersion: current.Version, ExpectedRunVersion: view.RunVersion, RunID: a.RunID, Attempt: 1, Fence: a.Fence, DispatchID: a.DispatchID, UnknownInputs: []WorkspaceTerminalRecoveryInput{}, Reason: "Inspect the exact original durable stop; close interrupted execution to failed without replay", IdempotencyKey: newID()}
	closed, err := f.s.RecoverWorkspaceTerminal(ctx, f.e, f.actor, f.project, session.ID, req)
	if err != nil || closed.Recovery.CanRecover {
		t.Fatal(closed, err)
	}
	run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, a.RunID)
	if err != nil || run.Nodes[0].State != "failed" || run.Nodes[0].Attempt != 1 {
		t.Fatal("expired recovery changed attempt or declared success", run, err)
	}
	before := domainACount(t, f.s, "suma_workspace_terminal_exchanges")
	rpc.OperationID = newID()
	rpc.OutputChunks = []term.OutputChunk{{Sequence: 1, DataBase64: base64.StdEncoding.EncodeToString([]byte("late output must not mutate reviewed original"))}}
	disposition, err := f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, rpc)
	if err != nil || disposition.RecoveryOperationID != req.IdempotencyKey || domainACount(t, f.s, "suma_workspace_terminal_exchanges") != before {
		t.Fatal("late original facts mutated reviewed execution", disposition, err)
	}
	rpc.Fence++
	if _, err = f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, rpc); err == nil {
		t.Fatal("foreign fence received original disposition")
	}
}
