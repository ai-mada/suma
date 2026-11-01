package runner

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	sc "github.com/ai-mada/suma/internal/steeringcontract"
)

type steeringDriverFixture188 struct {
	mu           sync.Mutex
	session      sc.SteeringSession
	instructions []sc.SteeringInstruction
	observations map[string]cc.CodexSteeringObservation
	opened       chan struct{}
	closed       chan struct{}
	before       func(context.Context, cc.CodexSteeringObservation) error
	openHook     func(context.Context) error
}

func newSteeringDriver188(t *testing.T) (*codexAppServer, *steeringDriverFixture188) {
	t.Helper()
	d := codexApprovalTestDriver(t, codexApprovalHandler{})
	f := &steeringDriverFixture188{observations: map[string]cc.CodexSteeringObservation{}, opened: make(chan struct{}), closed: make(chan struct{})}
	h := codexSteeringHandler{
		Open: func(ctx context.Context, source cc.CodexSteeringSource) (sc.SteeringSession, error) {
			f.mu.Lock()
			f.session = sc.SteeringSession{ID: "steering188", ProjectID: "project188", TaskID: "task188", RunID: "run188", NodeID: "code188", Attempt: 1, Fence: 1, WorkerID: "worker188", DispatchID: "dispatch188", ProfileSHA256: d.stream.profile.ProfileSHA256, Source: source, Version: 1, State: "active", OpenedAt: "2026-11-01T00:00:00Z", ExpiresAt: "2099-11-01T00:00:00Z", AllowedActions: []string{}}
			session := f.session
			if f.instructions == nil {
				body := "Keep the original reviewed public API.\n"
				f.instructions = []sc.SteeringInstruction{{ID: "instruction188", SessionID: session.ID, ProjectID: session.ProjectID, TaskID: session.TaskID, RunID: session.RunID, NodeID: session.NodeID, Attempt: 1, Fence: 1, Version: 1, Message: sc.SteeringMessage{ID: "message188", Sequence: 1, AuthorID: "human188", Body: body, BodySHA256: sc.MessageDigest(body)}, ActorID: "human188", CreatedAt: session.OpenedAt, ExpiresAt: session.ExpiresAt}}
			}
			f.mu.Unlock()
			close(f.opened)
			if f.openHook != nil {
				if err := f.openHook(ctx); err != nil {
					return sc.SteeringSession{}, err
				}
			}
			return session, nil
		},
		Poll: func(context.Context, sc.SteeringSession) (sc.SteeringPoll, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return sc.SteeringPoll{Session: f.session, Instructions: append([]sc.SteeringInstruction(nil), f.instructions...)}, nil
		},
		Observe: func(_ context.Context, _ sc.SteeringSession, i sc.SteeringInstruction, o cc.CodexSteeringObservation) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if old, ok := f.observations[i.ID]; ok && !steeringObservationAdvances(old, o) {
				return errSteering
			}
			f.observations[i.ID] = o
			return nil
		},
		BeforeWrite: func(ctx context.Context, _ sc.SteeringSession, i sc.SteeringInstruction, o cc.CodexSteeringObservation) error {
			f.mu.Lock()
			old := f.observations[i.ID]
			if !steeringObservationAdvances(old, o) {
				f.mu.Unlock()
				return errSteering
			}
			f.observations[i.ID] = o
			f.mu.Unlock()
			if f.before != nil {
				return f.before(ctx, o)
			}
			return nil
		},
		Close: func(_ context.Context, _ sc.SteeringSession) (sc.SteeringPoll, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.session.State = "closed"
			f.session.Version++
			items := append([]sc.SteeringInstruction(nil), f.instructions...)
			for i := range items {
				if o, ok := f.observations[items[i].ID]; ok {
					copy := o
					items[i].Observation = &copy
				}
			}
			close(f.closed)
			return sc.SteeringPoll{Session: f.session, Instructions: items}, nil
		},
	}
	if err := d.configureSteering(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	return d, f
}
func steeringProof188(d *codexAppServer) *cc.CodexSteeringEvidence {
	d.mu.Lock()
	defer d.mu.Unlock()
	return cc.CloneEvidence(d.evidence).Steering
}

func TestSteeringDriverExactSameTurnAckAndFinalUnsentInstruction(t *testing.T) {
	for _, outcome := range []string{"accepted", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			d, f := newSteeringDriver188(t)
			if d.steering.started {
				t.Fatal("session opened before the original turn ACK")
			}
			inputAck187(t, d)
			codexApprovalAwait(t, f.opened)
			var writes atomic.Int32
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = d.CopyInput(codexApprovalWriter(func(raw []byte) (int, error) {
					m, err := decodeToolObject(raw)
					if err != nil || textAt(m, "method") != "turn/steer" {
						t.Errorf("unexpected host frame: %s", raw)
					}
					params := objectAt(m, "params")
					if textAt(params, "expectedTurnId") != d.turn || textAt(params, "threadId") != d.thread || textAt(params, "clientUserMessageId") != "instruction188" {
						t.Error("steering escaped original source")
					}
					writes.Add(1)
					if d.waitingID != "" {
						t.Error("steering overwrote host startup waitingID")
					}
					ack := map[string]any{"id": m["id"], "result": map[string]any{"turnId": d.turn}}
					if outcome == "rejected" {
						ack = map[string]any{"id": m["id"], "error": map[string]any{"code": -32000, "message": "Active turn is not steerable"}}
					}
					codexApprovalFrame(t, d, ack)
					// A human Create may race the last active poll. Closed poll must
					// include it as never dispatched, without a fake native observation.
					f.mu.Lock()
					extra := f.instructions[0]
					extra.ID = "never-polled188"
					extra.Message.ID = "late-message188"
					extra.Message.Sequence++
					f.instructions = append(f.instructions, extra)
					f.mu.Unlock()
					inputTerminal187(t, d)
					return len(raw), nil
				}))
			}()
			codexApprovalAwait(t, done)
			d.Finish()
			proof := steeringProof188(d)
			if d.bad != nil || writes.Load() != 1 || proof == nil || !proof.Closed || len(proof.Instructions) != 2 || proof.Instructions[1].Observation != nil {
				t.Fatalf("incorrect final steering evidence: %+v %v", proof, d.bad)
			}
			o := proof.Instructions[0].Observation
			if o == nil || !o.WriteWritten || !o.TurnClosed || o.NativeAccepted != (outcome == "accepted") || o.NativeRejected != (outcome == "rejected") || o.FailureCode != "" {
				t.Fatalf("native ACK/write facts were conflated: %+v", o)
			}
			raw, _ := json.Marshal(proof)
			if strings.Contains(string(raw), "reviewed public API") {
				t.Fatal("message body leaked into execution evidence")
			}
		})
	}
}

func TestSteeringDriverCloseDuringIntentAndPartialWriteStayUnknown(t *testing.T) {
	for _, kind := range []string{"closure_during_intent", "partial_write", "wrong_turn_ack", "malformed_error"} {
		t.Run(kind, func(t *testing.T) {
			d, f := newSteeringDriver188(t)
			entered, release := make(chan struct{}), make(chan struct{})
			f.before = func(context.Context, cc.CodexSteeringObservation) error {
				close(entered)
				if kind == "closure_during_intent" {
					<-release
				}
				return nil
			}
			inputAck187(t, d)
			var writes atomic.Int32
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = d.CopyInput(codexApprovalWriter(func(raw []byte) (int, error) {
					writes.Add(1)
					if kind == "wrong_turn_ack" || kind == "malformed_error" {
						m, _ := decodeToolObject(raw)
						if kind == "malformed_error" {
							codexApprovalFrame(t, d, map[string]any{"id": m["id"], "error": map[string]any{"code": true}})
						} else {
							codexApprovalFrame(t, d, map[string]any{"id": m["id"], "result": map[string]any{"turnId": "another-turn"}})
						}
						return len(raw), nil
					}
					return len(raw) - 1, nil
				}))
			}()
			codexApprovalAwait(t, entered)
			if kind == "closure_during_intent" {
				inputTerminal187(t, d)
				close(release)
			}
			codexApprovalAwait(t, done)
			d.Finish()
			proof := steeringProof188(d)
			if d.bad == nil || proof == nil || len(proof.Instructions) != 1 || proof.Instructions[0].Observation == nil {
				t.Fatalf("unknown steer became successful: %+v %v", proof, d.bad)
			}
			o := proof.Instructions[0].Observation
			if !o.WriteStarted || o.NativeAccepted || o.NativeRejected || o.FailureCode == "" || kind == "closure_during_intent" && writes.Load() != 0 || kind == "partial_write" && o.WriteWritten {
				t.Fatalf("uncertain response was replayable: %+v writes=%d", o, writes.Load())
			}
			f.mu.Lock()
			last := f.observations["instruction188"]
			f.mu.Unlock()
			if last.FailureCode == "" || !last.TurnClosed {
				t.Fatalf("late actual facts were not persisted: %+v", last)
			}
		})
	}
}

func TestSteeringDriverNonTaskOpenAndStopBeforeDispatch(t *testing.T) {
	for _, kind := range []string{"not_task_execution", "stop_before_dispatch"} {
		t.Run(kind, func(t *testing.T) {
			d, f := newSteeringDriver188(t)
			release := make(chan struct{})
			f.openHook = func(ctx context.Context) error {
				<-release
				if kind == "not_task_execution" {
					return errSteeringUnavailable
				}
				return nil
			}
			inputAck187(t, d)
			codexApprovalAwait(t, f.opened)
			var writes atomic.Int32
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = d.CopyInput(codexApprovalWriter(func(raw []byte) (int, error) { writes.Add(1); return len(raw), nil }))
			}()
			inputTerminal187(t, d)
			close(release)
			codexApprovalAwait(t, done)
			d.Finish()
			if writes.Load() != 0 || d.bad != nil {
				t.Fatalf("closed source dispatched a steer: writes=%d %v", writes.Load(), d.bad)
			}
			if kind == "not_task_execution" && steeringProof188(d) != nil {
				t.Fatal("normal non-task response invented a live session")
			}
		})
	}
}
