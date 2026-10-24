package dag

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/toolcontract"
)

func toolNode() Node {
	return Node{ID: "agent", Type: "agent", Command: []string{"native-agent"}, Config: map[string]any{"goal": "inspect", "tool_profile_id": "profile-1", "tool_profile_sha256": strings.Repeat("a", 64)}}
}
func TestToolCompileAndCapabilitiesPreventAttempt(t *testing.T) {
	for _, n := range []Node{{ID: "bad", Type: "agent"}, {ID: "bad", Type: "command", Command: []string{"one", "two"}}, {ID: "bad", Type: "test", Command: []string{"../path"}}, {ID: "bad", Type: "agent", Command: []string{"agent"}, Config: map[string]any{"tool_profile_id": "p"}}, {ID: "bad", Type: "command", Command: []string{"agent"}, Config: map[string]any{"tool_profile_id": "p", "tool_profile_sha256": strings.Repeat("a", 64)}}} {
		if Compile(Workflow{Nodes: []Node{n}}) == nil {
			t.Fatal("unsafe command/tool binding compiled", n)
		}
	}
	e, r := fixture(t, Workflow{Nodes: []Node{toolNode()}})
	for _, caps := range [][]string{nil, {}, {"agent"}, {"command:other"}} {
		a, err := e.Claim(ctx, "t", "p", "w", caps)
		if err != nil || a != nil {
			t.Fatal(a, err)
		}
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.Nodes[0].State != "ready" || current.Nodes[0].Attempt != 0 {
		t.Fatal("ineligible claim consumed attempt", current, err)
	}
	if current.Nodes[0].ToolProfileID != "profile-1" || current.Nodes[0].ToolProfileSHA256 != strings.Repeat("a", 64) {
		t.Fatal("ready binding missing", current.Nodes[0])
	}
	a, err := e.Claim(ctx, "t", "p", "w", []string{"command:native-agent"})
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
}

func TestToolPublicBindingUsesCurrentSpecAndRetryClearsEvidence(t *testing.T) {
	n := toolNode()
	n.Idempotent = true
	e, r := fixture(t, Workflow{Nodes: []Node{n}})
	a, err := e.Claim(ctx, "t", "p", "w", []string{"command:native-agent"})
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	proof := &toolcontract.Evidence{UsageState: "unreported", Profile: toolcontract.Profile{CommandID: "native-agent", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture"}, Terminal: "failed", EventCount: 1}
	if err = e.Complete(ctx, "t", "p", Result{RunID: r.ID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "failed-proof", ExitCode: 1, ToolEvidence: proof}); err != nil {
		t.Fatal(err)
	}
	r, err = e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || r.Nodes[0].ToolEvidence == nil {
		t.Fatal("historical failed proof absent", r, err)
	}
	r, err = e.Command(ctx, "t", "p", r.ID, "retry", "retry-proof", r.Version, map[string]any{"node_id": a.NodeID})
	if err != nil || r.Nodes[0].ToolEvidence != nil || r.Nodes[0].ToolProfileID != "profile-1" {
		t.Fatal("ready retry inherited old proof", r, err)
	}
	// A stale result object must never become the source of binding metadata.
	state := runState{nodes: []*nodeState{{NodeRun: NodeRun{ID: "stale", State: "failed", ToolEvidence: proof}, spec: Node{ID: "stale", Type: "agent", Command: []string{"other"}, Config: map[string]any{"tool_profile_id": "current-profile", "tool_profile_sha256": strings.Repeat("c", 64)}}}, {NodeRun: NodeRun{ID: "ordinary", State: "ready", ToolEvidence: proof}, spec: Node{ID: "ordinary", Type: "command", Command: []string{"other"}}}}}
	public := state.public()
	if public.Nodes[0].ToolProfileID != "current-profile" || public.Nodes[0].ToolProfileSHA256 != strings.Repeat("c", 64) || public.Nodes[1].ToolProfileID != "" || public.Nodes[1].ToolEvidence != nil {
		t.Fatal("read model inferred binding from evidence", public)
	}
}

func TestToolPublicBindingForDynamicMapAndCompensation(t *testing.T) {
	template := toolNode()
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "fan", Type: "map", Config: map[string]any{"items": []any{"item"}, "template": template}}}})
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range current.Nodes {
		if n.Type == "agent" {
			found = true
			if n.ToolProfileID != "profile-1" || n.ToolProfileSHA256 != strings.Repeat("a", 64) {
				t.Fatal("map child lost binding", n)
			}
		}
	}
	if !found {
		t.Fatal("map did not expand")
	}
	source := toolNode()
	source.Config["compensation"] = Node{Type: "agent", Command: []string{"inverse-agent"}, Config: map[string]any{"goal": "approved compensation", "tool_profile_id": "inverse-profile", "tool_profile_sha256": strings.Repeat("c", 64)}}
	e, r = fixture(t, Workflow{Nodes: []Node{source}})
	a, err := e.Claim(ctx, "t", "p", "w", []string{"command:native-agent"})
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	proof := &toolcontract.Evidence{UsageState: "unreported", Profile: toolcontract.Profile{CommandID: "native-agent", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture"}, Terminal: "completed", EventCount: 1}
	if err = e.Complete(ctx, "t", "p", Result{RunID: r.ID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "source-completed", ToolEvidence: proof}); err != nil {
		t.Fatal(err)
	}
	r, err = e.GetRun(ctx, "t", "p", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	r, err = e.Command(ctx, "t", "p", r.ID, "compensate", "inverse-plan", r.Version, map[string]any{"reason": "approved inverse"})
	if err != nil {
		t.Fatal(err)
	}
	inverse, err := e.GetRun(ctx, "t", "p", r.CompensationRunID)
	if err != nil || len(inverse.Nodes) != 1 || inverse.Nodes[0].ToolProfileID != "inverse-profile" || inverse.Nodes[0].ToolProfileSHA256 != strings.Repeat("c", 64) || inverse.Nodes[0].ToolEvidence != nil {
		t.Fatal("inverse did not project its own fixed binding", inverse, err)
	}
}
func TestToolProgressEvidenceAndReplay(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{toolNode()}})
	a, err := e.Claim(ctx, "t", "p", "w", []string{"command:native-agent"})
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	ev := toolcontract.Event{SchemaVersion: 1, CommandID: "native-agent", ProfileSHA256: strings.Repeat("a", 64), Protocol: "suma-jsonl-v1", Kind: "message", Message: "token=private-value"}
	if err = e.Progress(ctx, "t", "p", *a, "progress-1", "", &ev); err != nil {
		t.Fatal(err)
	}
	bad := ev
	bad.ProfileSHA256 = strings.Repeat("c", 64)
	if err = e.Progress(ctx, "t", "p", *a, "progress-other", "", &bad); !errors.Is(err, ErrInvalid) {
		t.Fatal("wrong profile event accepted", err)
	}
	result := Result{RunID: r.ID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, OperationID: "complete-tool", ExitCode: 0}
	if err = e.Complete(ctx, "t", "p", result); !errors.Is(err, ErrInvalid) {
		t.Fatal("success without proof accepted", err)
	}
	result.ToolEvidence = &toolcontract.Evidence{UsageState: "unreported", Profile: toolcontract.Profile{CommandID: "native-agent", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture"}, Terminal: "failed", EventCount: 1}
	if err = e.Complete(ctx, "t", "p", result); !errors.Is(err, ErrInvalid) {
		t.Fatal("failed protocol labelled success", err)
	}
	result.ToolEvidence.Terminal = "completed"
	if err = e.Complete(ctx, "t", "p", result); err != nil {
		t.Fatal(err)
	}
	if err = e.Progress(ctx, "t", "p", *a, "progress-1", "", &ev); err != nil {
		t.Fatal("lost ACK replay failed", err)
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.Nodes[0].ToolEvidence == nil {
		t.Fatal("evidence missing", current, err)
	}
	events, err := e.Events(ctx, "t", "p", r.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, v := range events {
		if v.Type == "tool_progress" {
			count++
			if strings.Contains(encoded(v.Data), "private-value") {
				t.Fatal("raw credential stored")
			}
		}
	}
	if count != 1 {
		t.Fatal("duplicate typed event", count)
	}
}
func TestCommandAuthorityBeforeCachedReceipt(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "task", Type: "task"}}})
	before := func(*sql.Tx) error { return nil }
	paused, err := e.CommandWithAuthority(ctx, "t", "p", r.ID, "pause", "pause-auth", r.Version, nil, before, nil)
	if err != nil {
		t.Fatal(err)
	}
	before = func(*sql.Tx) error { return ErrInvalid }
	if _, err = e.CommandWithAuthority(ctx, "t", "p", r.ID, "pause", "pause-auth", paused.Version, nil, before, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("cached receipt skipped authority", err)
	}
}

func TestHeartbeatAuthorityBeforeLeaseMutation(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "work", Type: "task"}}})
	a, err := e.Claim(ctx, "t", "p", "worker", []string{"task"})
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	var before int64
	if err = e.db.QueryRow(`SELECT lease_until FROM dag_nodes WHERE run_id=? AND id=?`, a.RunID, a.NodeID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	err = e.HeartbeatWithAuthority(ctx, "t", "p", *a, func(*sql.Tx) error { return ErrInvalid })
	if !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	var after int64
	if err = e.db.QueryRow(`SELECT lease_until FROM dag_nodes WHERE run_id=? AND id=?`, a.RunID, a.NodeID).Scan(&after); err != nil || before != after {
		t.Fatal("unauthorized heartbeat changed lease", before, after, err)
	}
}

func TestToolConfirmedStopAcceptsUnknownProtocolWithoutSuccess(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "stopped_before_terminal", true: "cancelled_after_stop_ack"}[cancelRun], func(t *testing.T) {
			e, r := fixture(t, Workflow{Nodes: []Node{toolNode()}})
			a, err := e.Claim(ctx, "t", "p", "w", []string{"command:native-agent"})
			if err != nil || a == nil {
				t.Fatal(a, err)
			}
			if cancelRun {
				r, err = e.GetRun(ctx, "t", "p", r.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = e.Command(ctx, "t", "p", r.ID, "cancel", "cancel-before-stop", r.Version, map[string]any{"reason": "user cancelled"}); err != nil {
					t.Fatal(err)
				}
			}
			proof := &toolcontract.Evidence{UsageState: "unreported", Profile: toolcontract.Profile{CommandID: "native-agent", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture"}, Terminal: "unknown", EventCount: 2}
			// The process stop was confirmed. Its absent protocol terminal must not
			// block the stop receipt, or turn a zero OS exit into tool success.
			result := Result{RunID: r.ID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "confirmed-process-stop", Stopped: true, ToolEvidence: proof}
			if err = e.Complete(ctx, "t", "p", result); err != nil {
				t.Fatal("confirmed stop rejected", err)
			}
			current, err := e.GetRun(ctx, "t", "p", r.ID)
			want := "failed"
			if cancelRun {
				want = "cancelled"
			}
			if err != nil || current.State != want || current.Nodes[0].State != want || current.Nodes[0].ToolEvidence == nil || current.Nodes[0].ToolEvidence.Terminal != "unknown" {
				t.Fatal("stop changed protocol evidence or claimed success", current, err)
			}
		})
	}
}
