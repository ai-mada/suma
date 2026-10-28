package dag

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	wo "github.com/ai-mada/suma/internal/workspaceoperationcontract"
)

func hostEventFixture(a Assignment, revision int) wo.Event {
	e := wo.Event{Protocol: wo.Protocol, Source: wo.Source, SchemaVersion: 1, Sequence: revision, OperationID: "go-invocation", Revision: revision, Action: "write", Phase: "started", StartedAt: "2026-10-29T01:00:00Z", RecordedAt: "2026-10-29T01:00:00Z", RequestSHA256: strings.Repeat("c", 64), PolicySHA256: strings.Repeat("d", 64), ProfileSHA256: strings.Repeat("a", 64)}
	if revision == 2 {
		e.Phase, e.GenerationAfter, e.CompletedAt, e.RecordedAt = "completed", 1, "2026-10-29T01:00:01Z", "2026-10-29T01:00:01Z"
	}
	e.EventID = wo.CanonicalEventID(WorkspaceOperationIdentity("t", "p", a), e)
	return e
}

func TestHostOperationDurableOutOfOrderReplayAndOriginalAttemptPage(t *testing.T) {
	n := toolNode()
	n.Idempotent = true
	e, r := fixture(t, Workflow{Nodes: []Node{n}})
	a, err := e.Claim(ctx, "t", "p", "w", []string{"command:native-agent"})
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	a.TenantID, a.ProjectID = "t", "p"
	terminal := hostEventFixture(*a, 2)
	if err = e.HostOperationProgressWithAuthority(ctx, "t", "p", *a, terminal.EventID, terminal, nil); err != nil {
		t.Fatal(err)
	}
	page, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, a.NodeID, a.Attempt, 1, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	start := hostEventFixture(*a, 1)
	for range 2 {
		if err = e.HostOperationProgressWithAuthority(ctx, "t", "p", *a, start.EventID, start, nil); err != nil {
			t.Fatal(err)
		}
	}
	frozen, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, a.NodeID, a.Attempt, 20, page.PageCursor)
	if err != nil || len(frozen.Items) != 1 || frozen.Watermark != page.Watermark {
		t.Fatal("host page watermark moved", frozen, err)
	}
	current, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, a.NodeID, a.Attempt, 20, "")
	if err != nil || len(current.Items) != 2 || current.Items[0].Data["tool_event"] != nil || current.Items[0].Data["host_operation"] == nil {
		t.Fatal("host facts were duplicated or disguised as SDK activity", current, err)
	}
	changed := start
	changed.Path = "changed.txt"
	if err = e.HostOperationProgressWithAuthority(ctx, "t", "p", *a, changed.EventID, changed, nil); !errors.Is(err, ErrConflict) {
		t.Fatal("same transition accepted new body", err)
	}
	other := start
	other.OperationID = "other-invocation"
	other.EventID = wo.CanonicalEventID(WorkspaceOperationIdentity("t", "p", *a), other)
	if err = e.HostOperationProgressWithAuthority(ctx, "t", "p", *a, other.EventID, other, nil); !errors.Is(err, ErrConflict) {
		t.Fatal("host sequence reused", err)
	}
	if err = e.HostOperationProgressWithAuthority(ctx, "t", "p", *a, start.EventID, start, func(*sql.Tx) error { return ErrInvalid }); !errors.Is(err, ErrInvalid) {
		t.Fatal("cached host receipt bypassed current authority", err)
	}
	stale := *a
	stale.Fence++
	bad := hostEventFixture(stale, 1)
	if err = e.HostOperationProgressWithAuthority(ctx, "t", "p", stale, bad.EventID, bad, nil); !errors.Is(err, ErrStale) {
		t.Fatal("invented old attempt admitted", err)
	}
	state, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || state.State != "running" || state.Nodes[0].Attempt != a.Attempt {
		t.Fatal("metadata changed dispatch state", state, err)
	}
	sealStart := hostEventFixture(*a, 1)
	sealStart.OperationID, sealStart.Action, sealStart.Sequence = "seal-invocation", "seal", 3
	sealStart.EventID = wo.CanonicalEventID(WorkspaceOperationIdentity("t", "p", *a), sealStart)
	sealDone := sealStart
	sealDone.Revision, sealDone.Phase, sealDone.Sequence = 2, "completed", 4
	sealDone.CompletedAt, sealDone.RecordedAt, sealDone.EffectOperationID = "2026-10-29T01:00:01Z", "2026-10-29T01:00:01Z", "actual-parent-seal"
	sealDone.EventID = wo.CanonicalEventID(WorkspaceOperationIdentity("t", "p", *a), sealDone)
	for _, event := range []wo.Event{sealStart, sealDone} {
		if err = e.HostOperationProgressWithAuthority(ctx, "t", "p", *a, event.EventID, event, nil); err != nil {
			t.Fatal("seal could not learn its actual effect identity", err)
		}
	}
	proof := &tc.Evidence{Profile: tc.Profile{CommandID: "native-agent", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "host metadata fixture"}, Terminal: "failed", EventCount: 1, UsageState: tc.UsageUnreported}
	if err = e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "host-fixture-failure", ExitCode: 1, ToolEvidence: proof}); err != nil {
		t.Fatal(err)
	}
	state, err = e.GetRun(ctx, "t", "p", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Command(ctx, "t", "p", r.ID, "retry", "host-fixture-retry", state.Version, map[string]any{"node_id": a.NodeID}); err != nil {
		t.Fatal(err)
	}
	next, err := e.Claim(ctx, "t", "p", "next-worker", []string{"command:native-agent"})
	if err != nil || next == nil || next.Attempt != a.Attempt+1 {
		t.Fatal(next, err)
	}
	late := hostEventFixture(*a, 1)
	late.Sequence, late.OperationID = 5, "late-original-invocation"
	late.EventID = wo.CanonicalEventID(WorkspaceOperationIdentity("t", "p", *a), late)
	if err = e.HostOperationProgressWithAuthority(ctx, "t", "p", *a, late.EventID, late, nil); err != nil {
		t.Fatal("known old-attempt outbox was discarded", err)
	}
	state, err = e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || state.Nodes[0].Attempt != next.Attempt || state.Nodes[0].WorkerID != next.WorkerID || state.Nodes[0].State != "running" {
		t.Fatal("late metadata overwrote current execution", state, err)
	}
	newPage, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, a.NodeID, next.Attempt, 20, "")
	if err != nil || len(newPage.Items) != 0 {
		t.Fatal("old facts crossed into new attempt", newPage, err)
	}
}
