package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type codexApprovalWriter func([]byte) (int, error)

func (w codexApprovalWriter) Write(p []byte) (int, error) { return w(p) }
func codexApprovalAwait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("bounded approval fixture channel did not become ready")
	}
}

func codexApprovalEventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("bounded approval transition did not complete")
		}
		time.Sleep(time.Millisecond)
	}
}
func codexApprovalTestDriver(t *testing.T, h codexApprovalHandler) *codexAppServer {
	t.Helper()
	if h.Decide == nil {
		h.Decide = func(context.Context, cc.CodexApprovalRequest) (codexApprovalDecision, error) {
			return codexApprovalDecision{PermissionID: "permission-174", Choice: cc.CodexApprovalChoice{Decision: "accept"}}, nil
		}
	}
	if h.Observe == nil {
		h.Observe = func(context.Context, cc.CodexApprovalRequest, codexApprovalDecision, cc.CodexApprovalObservation) error {
			return nil
		}
	}
	if h.BeforeWrite == nil {
		h.BeforeWrite = h.Observe
	}
	profile := tc.Profile{CommandID: "codex174", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "controlled-protocol-only"}
	root, home := t.TempDir(), t.TempDir()
	binding := &cc.Binding{BindingSHA256: strings.Repeat("c", 64), Model: "fixture-model-174", ReasoningEffort: "high"}
	d := newCodexAppServer(profile, CommandSpec{CodexAppServer: &CodexAppServerSpec{ProviderID: "fixture174"}}, home, root, "Fixed objective", binding, newToolStream(profile, []string{root, home, "private174-token"}, nil))
	codexSkillsDrain(t, d)
	ctx, cancel := context.WithCancel(context.Background())
	if err := d.configureApprovals(ctx, h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); d.Close() })
	d.thread, d.turn = "thread-174", "turn-174"
	d.turnStarted = true
	d.evidence.ThreadID, d.evidence.TurnID = d.thread, d.turn
	d.evidence.ConfigurationAcknowledged = true
	// The turn/started notification may precede the host turn/start ACK.
	d.waiting, d.waitingID, d.requestID = "turn/start", "4", 4
	return d
}
func codexApprovalFrame(t *testing.T, d *codexAppServer, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = d.Write(append(raw, '\n'))
}
func codexApprovalCommand(d *codexAppServer, id string) map[string]any {
	return map[string]any{"id": id, "type": "commandExecution", "command": "git diff --stat", "cwd": d.root, "commandActions": []any{}, "status": "inProgress"}
}
func codexApprovalItemEvent(t *testing.T, d *codexAppServer, item map[string]any, completed bool) {
	t.Helper()
	method, stamp := "item/started", "startedAtMs"
	if completed {
		method, stamp = "item/completed", "completedAtMs"
	}
	codexApprovalFrame(t, d, map[string]any{"method": method, "params": map[string]any{"threadId": d.thread, "turnId": d.turn, stamp: 1740000000000, "item": item}})
}
func codexApprovalParams(d *codexAppServer, item string) map[string]any {
	return map[string]any{"threadId": d.thread, "turnId": d.turn, "itemId": item, "startedAtMs": 1740000000000, "command": "git diff --stat", "cwd": d.root}
}
func codexApprovalSend(t *testing.T, d *codexAppServer, id any, method string, params map[string]any) {
	t.Helper()
	codexApprovalFrame(t, d, map[string]any{"id": id, "method": method, "params": params})
}
func codexApprovalProof(d *codexAppServer) []cc.CodexApprovalEvidence {
	d.mu.Lock()
	defer d.mu.Unlock()
	raw, _ := json.Marshal(d.evidence.Approvals)
	var out []cc.CodexApprovalEvidence
	_ = json.Unmarshal(raw, &out)
	return out
}

func TestCodexApprovalsAsyncExactReplyAndCausalClosure(t *testing.T) {
	decideEntered, decideRelease := make(chan struct{}), make(chan struct{})
	writeDelivered, writeRelease := make(chan struct{}), make(chan struct{})
	var observationsMu sync.Mutex
	var observations []cc.CodexApprovalObservation
	observe := func(_ context.Context, _ cc.CodexApprovalRequest, _ codexApprovalDecision, o cc.CodexApprovalObservation) error {
		observationsMu.Lock()
		observations = append(observations, o)
		observationsMu.Unlock()
		return nil
	}
	d := codexApprovalTestDriver(t, codexApprovalHandler{Decide: func(ctx context.Context, r cc.CodexApprovalRequest) (codexApprovalDecision, error) {
		close(decideEntered)
		select {
		case <-decideRelease:
		case <-ctx.Done():
			return codexApprovalDecision{}, ctx.Err()
		}
		return codexApprovalDecision{PermissionID: "permission-174", Choice: cc.CodexApprovalChoice{Decision: "accept"}}, nil
	}, Observe: observe, BeforeWrite: observe})
	item := codexApprovalCommand(d, "item-174")
	codexApprovalItemEvent(t, d, item, false)
	codexApprovalSend(t, d, json.Number("4"), "item/commandExecution/requestApproval", codexApprovalParams(d, "item-174"))
	codexApprovalAwait(t, decideEntered)
	readDone := make(chan struct{})
	go func() {
		codexApprovalFrame(t, d, map[string]any{"id": 4, "result": map[string]any{"turn": map[string]any{"id": d.turn, "status": "inProgress"}}})
		close(readDone)
	}()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("human wait held stdout mutex")
	}
	close(decideRelease)
	var replies atomic.Int32
	writerDone := make(chan error, 1)
	go func() {
		_, err := d.CopyInput(codexApprovalWriter(func(raw []byte) (int, error) {
			m, e := decodeToolObject(raw)
			if e != nil {
				return 0, e
			}
			if n, ok := m["id"].(json.Number); !ok || n.String() != "4" || textAt(objectAt(m, "result"), "decision") != "accept" {
				return 0, errors.New("native response changed exact reverse ID/choice")
			}
			replies.Add(1)
			close(writeDelivered)
			select {
			case <-writeRelease:
			case <-d.approvals.ctx.Done():
				return 0, context.Canceled
			}
			return len(raw), nil
		}))
		writerDone <- err
	}()
	codexApprovalAwait(t, writeDelivered)
	codexApprovalFrame(t, d, map[string]any{"method": "serverRequest/resolved", "params": map[string]any{"threadId": d.thread, "requestId": 4}})
	completed := codexApprovalCommand(d, "item-174")
	completed["status"] = "completed"
	codexApprovalItemEvent(t, d, completed, true)
	codexApprovalFrame(t, d, map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": d.thread, "turn": map[string]any{"id": d.turn, "status": "completed"}}})
	proof := codexApprovalProof(d)
	if len(proof) != 1 || proof[0].Observation == nil || proof[0].Observation.ReplyWritten || proof[0].Observation.RequestResolved || proof[0].Observation.ItemStatus != "" {
		t.Fatalf("remote fast reply invented completed write: %+v", proof)
	}
	close(writeRelease)
	codexApprovalEventually(t, func() bool { p := codexApprovalProof(d); return len(p) == 1 && cc.ApprovalEvidenceClosed(p[0]) })
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("causal terminal did not close stdin")
	}
	d.Finish()
	if d.bad != nil || d.stream.terminal != "completed" || replies.Load() != 1 || d.requestID != 4 {
		t.Fatalf("original single turn did not close: %v/%s/%d", d.bad, d.stream.terminal, replies.Load())
	}
	observationsMu.Lock()
	defer observationsMu.Unlock()
	if len(observations) != 3 || observations[0].Sequence != 1 || observations[0].ReplyStarted || observations[1].Sequence != 2 || !observations[1].ReplyStarted || observations[1].ReplyWritten || observations[2].Sequence != 3 || !observations[2].ReplyWritten || !observations[2].RequestResolved || observations[2].ItemStatus != "completed" {
		t.Fatalf("wrong durable causal order: %+v", observations)
	}
}

func TestCodexApprovalsRejectUnsafeScopeAndRedactBeforeCallback(t *testing.T) {
	for _, mode := range []string{"network", "grant_root", "write_stdin", "unknown_extension", "outside_workspace", "secret"} {
		t.Run(mode, func(t *testing.T) {
			requestCh := make(chan cc.CodexApprovalRequest, 1)
			d := codexApprovalTestDriver(t, codexApprovalHandler{Decide: func(_ context.Context, r cc.CodexApprovalRequest) (codexApprovalDecision, error) {
				requestCh <- r
				return codexApprovalDecision{PermissionID: "permission-unsafe", Choice: cc.CodexApprovalChoice{Decision: "accept"}}, nil
			}})
			item := codexApprovalCommand(d, "unsafe-item")
			params := codexApprovalParams(d, "unsafe-item")
			method := "item/commandExecution/requestApproval"
			switch mode {
			case "network":
				params["networkApprovalContext"] = map[string]any{"host": "example.invalid", "protocol": "https"}
			case "write_stdin":
				params["kind"] = "writeStdin"
			case "unknown_extension":
				params["additionalPermissions"] = map[string]any{"network": map[string]any{"enabled": true}}
			case "outside_workspace":
				item["cwd"] = filepath.Dir(d.root)
				params["cwd"] = item["cwd"]
			case "secret":
				item["command"] = "echo private174-token"
				params["command"] = item["command"]
			case "grant_root":
				item = map[string]any{"id": "unsafe-item", "type": "fileChange", "status": "inProgress", "changes": []any{map[string]any{"path": "a.txt", "kind": map[string]any{"type": "add"}, "diff": "+a"}}}
				params = map[string]any{"threadId": d.thread, "turnId": d.turn, "itemId": "unsafe-item", "startedAtMs": 1740000000000, "grantRoot": d.root}
				method = "item/fileChange/requestApproval"
			}
			codexApprovalItemEvent(t, d, item, false)
			codexApprovalSend(t, d, "unsafe-rpc", method, params)
			var request cc.CodexApprovalRequest
			select {
			case request = <-requestCh:
			case <-time.After(time.Second):
				t.Fatal("safe unsupported projection was not published")
			}
			if cc.ValidateApprovalRequest(request) != nil || request.ScopeSupported && request.DetailsComplete || strings.Contains(request.Details, "private174-token") || strings.Contains(request.Command, "private174-token") || strings.Contains(request.Details, d.root) {
				t.Fatalf("unsafe public projection: %+v", request)
			}
			codexApprovalEventually(t, func() bool { d.mu.Lock(); defer d.mu.Unlock(); return d.bad != nil })
			if len(codexSkillsDrain(t, d)) != 0 {
				t.Fatal("unsupported approval was granted")
			}
		})
	}
}

func TestCodexApprovalsTypedIDsDuplicateAndItemBinding(t *testing.T) {
	t.Run("unconfigured_probe_rejects_callback", func(t *testing.T) {
		d := newCodexAppServer(tc.Profile{}, CommandSpec{}, t.TempDir(), t.TempDir(), "", nil, nil)
		defer d.Close()
		codexSkillsDrain(t, d)
		codexApprovalSend(t, d, "untrusted-probe-rpc", "item/commandExecution/requestApproval", map[string]any{})
		if d.bad == nil || len(codexSkillsDrain(t, d)) != 0 {
			t.Fatal("read-only probe accepted an approval callback")
		}
	})
	t.Run("exact_int64_and_distinct_callbacks", func(t *testing.T) {
		requests := make(chan cc.CodexApprovalRequest, 3)
		release := make(chan struct{})
		d := codexApprovalTestDriver(t, codexApprovalHandler{Decide: func(ctx context.Context, r cc.CodexApprovalRequest) (codexApprovalDecision, error) {
			requests <- r
			select {
			case <-release:
			case <-ctx.Done():
			}
			return codexApprovalDecision{}, errors.New("controlled no decision")
		}})
		item := codexApprovalCommand(d, "same-item")
		codexApprovalItemEvent(t, d, item, false)
		p := codexApprovalParams(d, "same-item")
		p["approvalId"] = "callback-a"
		codexApprovalSend(t, d, json.Number("9223372036854775807"), "item/commandExecution/requestApproval", p)
		var first cc.CodexApprovalRequest
		select {
		case first = <-requests:
		case <-time.After(3 * time.Second):
			t.Fatal("first exact RPC request was not published")
		}
		if first.RequestID.Kind != "number" || first.RequestID.Value != "9223372036854775807" {
			t.Fatal("int64 RPC identity lost precision")
		}
		codexApprovalSend(t, d, json.Number("9223372036854775807"), "item/commandExecution/requestApproval", p)
		p2 := codexApprovalParams(d, "same-item")
		p2["approvalId"] = "callback-b"
		codexApprovalSend(t, d, "9223372036854775807", "item/commandExecution/requestApproval", p2)
		var second cc.CodexApprovalRequest
		select {
		case second = <-requests:
		case <-time.After(3 * time.Second):
			t.Fatal("second independent RPC request was not published")
		}
		if second.RequestID.Kind != "string" || second.ApprovalID != "callback-b" {
			t.Fatal("same item or string ID collapsed")
		}
		select {
		case <-requests:
			t.Fatal("duplicate request was published twice")
		case <-time.After(20 * time.Millisecond):
		}
		close(release)
	})
	for _, mode := range []string{"fractional_id", "null_id", "changed_same_id", "missing_item", "wrong_command"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			block := make(chan struct{})
			d := codexApprovalTestDriver(t, codexApprovalHandler{Decide: func(ctx context.Context, _ cc.CodexApprovalRequest) (codexApprovalDecision, error) {
				calls.Add(1)
				select {
				case <-ctx.Done():
				case <-block:
				}
				return codexApprovalDecision{}, errors.New("controlled")
			}})
			p := codexApprovalParams(d, "bound-item")
			if mode != "missing_item" {
				codexApprovalItemEvent(t, d, codexApprovalCommand(d, "bound-item"), false)
			}
			var id any = "bound-rpc"
			if mode == "fractional_id" {
				id = json.Number("1.5")
			}
			if mode == "null_id" {
				id = nil
			}
			if mode == "wrong_command" {
				p["command"] = "a different command"
			}
			codexApprovalSend(t, d, id, "item/commandExecution/requestApproval", p)
			if mode == "changed_same_id" {
				codexApprovalEventually(t, func() bool { return calls.Load() == 1 })
				p2 := codexApprovalParams(d, "bound-item")
				p2["reason"] = "changed callback"
				codexApprovalSend(t, d, id, "item/commandExecution/requestApproval", p2)
			}
			codexApprovalEventually(t, func() bool { d.mu.Lock(); defer d.mu.Unlock(); return d.bad != nil })
			close(block)
			want := int32(0)
			if mode == "changed_same_id" {
				want = 1
			}
			if calls.Load() != want || len(codexSkillsDrain(t, d)) != 0 {
				t.Fatal("invalid native identity reached decision or response")
			}
		})
	}
}

func TestCodexApprovalsCancellationAndPartialWriteStayUnknown(t *testing.T) {
	t.Run("no_human_cleanup_is_durable_before_finish", func(t *testing.T) {
		decideEntered, observeEntered, observeRelease := make(chan struct{}), make(chan struct{}), make(chan struct{})
		var observedMu sync.Mutex
		var observed []cc.CodexApprovalObservation
		d := codexApprovalTestDriver(t, codexApprovalHandler{Decide: func(ctx context.Context, _ cc.CodexApprovalRequest) (codexApprovalDecision, error) {
			close(decideEntered)
			<-ctx.Done()
			return codexApprovalDecision{}, ctx.Err()
		}, Observe: func(ctx context.Context, _ cc.CodexApprovalRequest, decision codexApprovalDecision, o cc.CodexApprovalObservation) error {
			if decision.PermissionID != "" || o.DecisionObserved || o.ReplyStarted || o.ReplyWritten {
				return errors.New("cleanup invented a human choice or response")
			}
			if o.Sequence == 1 {
				close(observeEntered)
				select {
				case <-observeRelease:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			observedMu.Lock()
			observed = append(observed, o)
			observedMu.Unlock()
			return nil
		}})
		codexApprovalItemEvent(t, d, codexApprovalCommand(d, "cleanup-item"), false)
		codexApprovalSend(t, d, "cleanup-rpc", "item/commandExecution/requestApproval", codexApprovalParams(d, "cleanup-item"))
		codexApprovalAwait(t, decideEntered)
		codexApprovalFrame(t, d, map[string]any{"method": "serverRequest/resolved", "params": map[string]any{"threadId": d.thread, "requestId": "cleanup-rpc"}})
		codexApprovalAwait(t, observeEntered)
		item := codexApprovalCommand(d, "cleanup-item")
		item["status"] = "declined"
		codexApprovalItemEvent(t, d, item, true)
		finished := make(chan struct{})
		go func() { d.Finish(); close(finished) }()
		select {
		case <-finished:
			t.Fatal("Finish froze the result before durable cleanup")
		case <-time.After(20 * time.Millisecond):
		}
		cancelled := make(chan struct{})
		go func() { _ = d.Cancel(); close(cancelled) }()
		codexApprovalAwait(t, cancelled)
		close(observeRelease)
		codexApprovalAwait(t, finished)
		observedMu.Lock()
		defer observedMu.Unlock()
		if len(observed) != 2 || !observed[1].RequestResolved || observed[1].ItemStatus != "declined" || d.bad == nil {
			t.Fatalf("known cleanup facts lost before unknown result: %+v", observed)
		}
	})
	for _, mode := range []string{"cancel_before_write", "partial_write", "missing_resolution"} {
		t.Run(mode, func(t *testing.T) {
			beforeEntered, beforeRelease := make(chan struct{}), make(chan struct{})
			var writes atomic.Int32
			h := codexApprovalHandler{}
			if mode == "cancel_before_write" {
				h.BeforeWrite = func(ctx context.Context, _ cc.CodexApprovalRequest, _ codexApprovalDecision, _ cc.CodexApprovalObservation) error {
					close(beforeEntered)
					select {
					case <-beforeRelease:
					case <-ctx.Done():
					}
					return nil
				}
			}
			d := codexApprovalTestDriver(t, h)
			codexApprovalItemEvent(t, d, codexApprovalCommand(d, "cancel-item"), false)
			codexApprovalSend(t, d, "cancel-rpc", "item/commandExecution/requestApproval", codexApprovalParams(d, "cancel-item"))
			copyDone := make(chan error, 1)
			go func() {
				_, err := d.CopyInput(codexApprovalWriter(func(raw []byte) (int, error) {
					m, _ := decodeToolObject(raw)
					if objectAt(m, "result") != nil {
						writes.Add(1)
						if mode == "partial_write" {
							return len(raw) - 1, nil
						}
					}
					return len(raw), nil
				}))
				copyDone <- err
			}()
			if mode == "cancel_before_write" {
				codexApprovalAwait(t, beforeEntered)
				cancelDone := make(chan struct{})
				go func() { _ = d.Cancel(); close(cancelDone) }()
				select {
				case <-cancelDone:
				case <-time.After(time.Second):
					t.Fatal("journal wait held Cancel mutex")
				}
				close(beforeRelease)
			} else {
				codexApprovalEventually(t, func() bool { return writes.Load() == 1 })
				if mode == "missing_resolution" {
					item := codexApprovalCommand(d, "cancel-item")
					item["status"] = "completed"
					codexApprovalItemEvent(t, d, item, true)
					d.input.close()
				}
			}
			select {
			case err := <-copyDone:
				if mode == "partial_write" && !errors.Is(err, io.ErrShortWrite) {
					t.Fatal("partial transport success invented")
				}
			case <-time.After(time.Second):
				t.Fatal("stdin did not close")
			}
			d.Finish()
			proof := codexApprovalProof(d)
			if len(proof) != 1 || cc.ApprovalEvidenceClosed(proof[0]) || d.stream.terminal == "completed" {
				t.Fatal("unknown permission became success")
			}
			if mode == "cancel_before_write" && writes.Load() != 0 {
				t.Fatal("cancelled queued approval was written")
			}
		})
	}
}
