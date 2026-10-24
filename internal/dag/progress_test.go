package dag

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/localization"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestProgressReceiptOwnershipAndFinalResult(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "command", Command: []string{"local"}}}})
	a := claim(t, e)
	message := "building token=secret\x1b[31m\x00\n"
	if err := e.Progress(ctx, "t", "p", *a, "chunk-1", message); err != nil {
		t.Fatal(err)
	}
	stale := *a
	stale.Fence++
	if err := e.Progress(ctx, "t", "p", stale, "stale", "text"); !errors.Is(err, ErrStale) {
		t.Fatalf("stale: %v", err)
	}
	if err := e.Progress(ctx, "t", "other", *a, "foreign", "text"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("scope: %v", err)
	}
	if err := e.Progress(ctx, "t", "p", *a, "chunk-1", "different"); !errors.Is(err, ErrConflict) {
		t.Fatalf("receipt: %v", err)
	}
	done(t, e, a, 0)
	if err := e.Progress(ctx, "t", "p", *a, "chunk-1", message); err != nil {
		t.Fatalf("lost ACK replay: %v", err)
	}
	if err := e.Progress(ctx, "t", "p", *a, "late", "text"); !errors.Is(err, ErrStale) {
		t.Fatalf("terminal progress: %v", err)
	}
	events, err := e.Events(ctx, "t", "p", r.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.Type == "node_progress" {
			count++
			s, _ := event.Data["message"].(string)
			if strings.Contains(s, "secret") || strings.Contains(s, "\x1b") || strings.Contains(s, "\x00") {
				t.Fatalf("unsafe progress: %q", s)
			}
		}
	}
	if count != 1 {
		t.Fatalf("duplicate events: %d", count)
	}
	current, _ := e.GetRun(ctx, "t", "p", r.ID)
	if current.State != "succeeded" || current.Nodes[0].Output != "" {
		t.Fatalf("progress altered completion: %+v", current)
	}
}

func TestProgressMalformedDiagnosticHasControlledReason(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{toolNode()}})
	a := claim(t, e)
	for name, change := range map[string]func(*tc.Event){
		"version":        func(v *tc.Event) { v.Diagnostic.SchemaVersion = 2 },
		"foreign key":    func(v *tc.Event) { v.Diagnostic.MessageKey = "common.invalid_input" },
		"unknown key":    func(v *tc.Event) { v.Diagnostic.MessageKey = "native.future.forbidden" },
		"wrong source":   func(v *tc.Event) { v.Message = "arbitrary provider message" },
		"extra param":    func(v *tc.Event) { v.Diagnostic.MessageParams = map[string]any{"token": "never-persist"} },
		"wrong protocol": func(v *tc.Event) { v.Protocol = "codex-jsonl-v1" },
		"wrong kind":     func(v *tc.Event) { v.Kind = "message" },
	} {
		t.Run(name, func(t *testing.T) {
			event := tc.Event{SchemaVersion: 1, CommandID: "native-agent", ProfileSHA256: strings.Repeat("a", 64), Protocol: "suma-jsonl-v1", Kind: "error", Message: "Model returned no text", Diagnostic: &tc.ToolDiagnostic{SchemaVersion: 1, MessageKey: "native.response.no_text"}}
			change(&event)
			err := e.Progress(ctx, "t", "p", *a, "invalid-"+name, "", &event)
			if !errors.Is(err, ErrInvalid) || err.Error() != "invalid: tool diagnostic metadata invalid" {
				t.Fatalf("formal diagnostic reason/category changed: %v", err)
			}
			rendered, ok := localization.FormatError(localization.EnUS, err)
			if !ok || rendered.Key != "dag.tool.diagnostic" {
				t.Fatalf("malformed formal metadata lost controlled reason: %+v %v", rendered, err)
			}
		})
	}
	events, err := e.Events(ctx, "t", "p", r.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == "tool_progress" {
			t.Fatal("invalid formal diagnostic persisted")
		}
	}
}

// Storing the original Diagnostic while redacting only Message would leak the
// same credential and break raw-body receipt binding after server restart.
func TestProgressNativeDiagnosticSanitizationReceiptAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnostic.db")
	e := openEngine(t, path)
	n := toolNode()
	n.Idempotent = true
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "wf", Name: "wf", Nodes: []Node{n}}); err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, "t", "p", "wf", "create")
	if err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	if a == nil {
		t.Fatal("missing current assignment")
	}
	static := tc.Event{SchemaVersion: 1, CommandID: "native-agent", ProfileSHA256: strings.Repeat("a", 64), Protocol: "suma-jsonl-v1", Kind: "error", SessionID: "request", Message: "Endpoint rejected request (HTTP 429)", Status: "upstream_rejected", Diagnostic: &tc.ToolDiagnostic{SchemaVersion: 1, MessageKey: "native.endpoint.rejected", MessageParams: map[string]any{"status": 429}}}
	if err = e.Progress(ctx, "t", "p", *a, "static", "", &static); err != nil {
		t.Fatal(err)
	}
	if err = e.Progress(ctx, "t", "p", *a, "static", "", &static); err != nil {
		t.Fatalf("duplicate progress: %v", err)
	}
	sensitive := static
	sensitive.Message = "CLI probe exited token=private-value"
	sensitive.Diagnostic = &tc.ToolDiagnostic{SchemaVersion: 1, MessageKey: "native.catalog.probe_exited", MessageParams: map[string]any{"status": "token=private-value"}}
	if err = e.Progress(ctx, "t", "p", *a, "sensitive", "", &sensitive); err != nil {
		t.Fatal(err)
	}
	if sensitive.Message != "CLI probe exited token=private-value" || sensitive.Diagnostic.MessageParams["status"] != "token=private-value" {
		t.Fatal("sanitization mutated caller facts")
	}
	changed := sensitive
	changed.Message = "CLI probe exited token=different-value"
	changed.Diagnostic = &tc.ToolDiagnostic{SchemaVersion: 1, MessageKey: "native.catalog.probe_exited", MessageParams: map[string]any{"status": "token=different-value"}}
	if err = e.Progress(ctx, "t", "p", *a, "sensitive", "", &changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("redaction hid changed original body: %v", err)
	}
	for name, change := range map[string]func(*Assignment){"fence": func(v *Assignment) { v.Fence++ }, "attempt": func(v *Assignment) { v.Attempt++ }, "worker": func(v *Assignment) { v.WorkerID = "other" }} {
		bad := *a
		change(&bad)
		if err = e.Progress(ctx, "t", "p", bad, "stale-"+name, "", &static); !errors.Is(err, ErrStale) {
			t.Fatalf("%s assignment accepted: %v", name, err)
		}
	}
	wrongProfile := static
	wrongProfile.ProfileSHA256 = strings.Repeat("c", 64)
	if err = e.Progress(ctx, "t", "p", *a, "wrong-profile", "", &wrongProfile); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong fixed profile accepted: %v", err)
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.State != "running" || current.Nodes[0].Attempt != 1 || current.Nodes[0].ToolEvidence != nil || current.Nodes[0].Output != "" {
		t.Fatalf("progress changed attempt or terminal facts: %+v %v", current, err)
	}
	if err = e.db.Close(); err != nil {
		t.Fatal(err)
	}
	e = openEngine(t, path)
	for op, event := range map[string]*tc.Event{"static": &static, "sensitive": &sensitive} {
		if err = e.Progress(ctx, "t", "p", *a, op, "", event); err != nil {
			t.Fatalf("restart lost ACK %s: %v", op, err)
		}
	}
	events, err := e.Events(ctx, "t", "p", r.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	var lastSequence int64
	for _, event := range events {
		if event.Type != "tool_progress" {
			continue
		}
		count++
		lastSequence = event.Sequence
		raw, _ := json.Marshal(event.Data)
		if strings.Contains(string(raw), "private-value") || strings.Contains(string(raw), "different-value") {
			t.Fatalf("durable display leaked original credential: %s", raw)
		}
		var payload struct {
			ToolEvent tc.Event `json:"tool_event"`
		}
		if err = json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		if event.Data["operation_id"] == "static" {
			if payload.ToolEvent.Diagnostic == nil || payload.ToolEvent.Diagnostic.MessageKey != "native.endpoint.rejected" || payload.ToolEvent.Diagnostic.MessageParams["status"] != json.Number("429") || payload.ToolEvent.Message != "Endpoint rejected request (HTTP 429)" || payload.ToolEvent.Status != "upstream_rejected" {
				t.Fatalf("typed integer diagnostic changed on durable replay: %s", raw)
			}
		} else if payload.ToolEvent.Message != "CLI probe exited [REDACTED]" || payload.ToolEvent.Diagnostic != nil {
			t.Fatalf("redacted message kept unproven metadata: %s", raw)
		}
	}
	if count != 2 {
		t.Fatalf("receipt duplicated durable events: %d", count)
	}
	after, err := e.Events(ctx, "t", "p", r.ID, lastSequence)
	if err != nil || len(after) != 0 {
		t.Fatalf("event cursor replay changed: %+v %v", after, err)
	}
	proof := &tc.Evidence{Profile: tc.Profile{CommandID: "native-agent", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture"}, Terminal: "failed", EventCount: 2, UsageState: tc.UsageUnreported}
	if err = e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, OperationID: "failed", ExitCode: 1, Error: "raw original failure", ToolEvidence: proof}); err != nil {
		t.Fatal(err)
	}
	if err = e.Progress(ctx, "t", "p", *a, "static", "", &static); err != nil {
		t.Fatalf("completed lost ACK replay: %v", err)
	}
	if err = e.Progress(ctx, "t", "p", *a, "late", "", &static); !errors.Is(err, ErrStale) {
		t.Fatalf("new terminal progress accepted: %v", err)
	}
	current, err = e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.State != "failed" || current.Nodes[0].Error != "raw original failure" || current.Nodes[0].ToolEvidence == nil || current.Nodes[0].ToolEvidence.Terminal != "failed" || current.Nodes[0].ToolEvidence.UsageState != tc.UsageUnreported {
		t.Fatalf("display changed completion evidence: %+v %v", current, err)
	}
	if _, err = e.Command(ctx, "t", "p", r.ID, "retry", "retry", current.Version, map[string]any{"node_id": a.NodeID}); err != nil {
		t.Fatal(err)
	}
	next := claim(t, e)
	if next == nil || next.Attempt != 2 || next.Fence == a.Fence {
		t.Fatalf("retry did not retain attempt fencing: %+v", next)
	}
	if err = e.Progress(ctx, "t", "p", *a, "old-attempt", "", &static); !errors.Is(err, ErrStale) {
		t.Fatalf("prior attempt accepted: %v", err)
	}
}

func TestProgressExpiredLeaseAndSizeAreRejected(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	a := claim(t, e)
	if err := e.Progress(ctx, "t", "p", *a, "large", strings.Repeat("x", 8193)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("size: %v", err)
	}
	if _, err := e.db.Exec("UPDATE dag_nodes SET lease_until=0 WHERE run_id=?", a.RunID); err != nil {
		t.Fatal(err)
	}
	if err := e.Progress(ctx, "t", "p", *a, "expired", "text"); !errors.Is(err, ErrStale) {
		t.Fatalf("lease: %v", err)
	}
}
