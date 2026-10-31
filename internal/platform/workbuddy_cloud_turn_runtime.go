package platform

import (
	"context"
	"sync"
	"time"
)

type cloudTurnRuntimeKey struct {
	owner *WorkBuddyConnections
	turn  string
}
type cloudTurnRuntimeHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// This registry owns only local connection shutdown, never dispatch eligibility.
// The durable one-shot permit and all commands remain owned by the SQL bridge.
var cloudTurnRuntimeHandles sync.Map
var cloudTurnRuntimeLifecycle sync.Mutex

func (w *WorkBuddyConnections) cloudTurnStartRuntime(p cloudTurnPermit) {
	ctx, cancel := context.WithCancel(context.Background())
	key := cloudTurnRuntimeKey{w, p.id}
	handle := &cloudTurnRuntimeHandle{cancel: cancel, done: make(chan struct{})}
	cloudTurnRuntimeLifecycle.Lock()
	if w.turnRuntimesClosed {
		cloudTurnRuntimeLifecycle.Unlock()
		cancel()
		// A commit racing graceful shutdown retains its original permit to
		// record known-not-sent, without admitting a new connection.
		finishCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = w.cloudTurnFinish(finishCtx, p, cloudTurnRuntimeEvent{Kind: "terminal", State: "rejected", KnownNotSent: true, ErrorCode: "workbuddy_cloud_turn_interrupted", UsageState: "unreported"})
		return
	}
	if _, loaded := cloudTurnRuntimeHandles.LoadOrStore(key, handle); loaded {
		cloudTurnRuntimeLifecycle.Unlock()
		cancel()
		return
	}
	cloudTurnRuntimeLifecycle.Unlock()
	go func() {
		defer cancel()
		defer close(handle.done)
		defer cloudTurnRuntimeHandles.Delete(key)
		w.cloudTurnRun(ctx, p)
	}()
}

func (w *WorkBuddyConnections) cloudTurnRun(ctx context.Context, p cloudTurnPermit) {
	runCloudTurnRuntime(ctx, p.id, cloudTurnRuntimeHooks{
		Check:     func(ctx context.Context) (cloudTurnLease, error) { return w.cloudTurnCheck(ctx, p) },
		CheckWork: func(ctx context.Context) (cloudTurnLease, error) { return w.cloudTurnCheckWork(ctx, p) },
		Begin:     func(ctx context.Context) error { return w.cloudTurnBeginPrompt(ctx, p) },
		Append:    func(ctx context.Context, event cloudTurnRuntimeEvent) error { return w.cloudTurnAppend(ctx, p, event) },
		Finish:    func(ctx context.Context, event cloudTurnRuntimeEvent) error { return w.cloudTurnFinish(ctx, p, event) },
		Heartbeat: func(ctx context.Context) error { return w.cloudTurnHeartbeat(ctx, p) },
		Take:      func(ctx context.Context) ([]cloudTurnRuntimeCommand, error) { return w.cloudTurnTakeCommands(ctx, p) },
		Ack: func(ctx context.Context, command cloudTurnRuntimeCommand, state, code string) error {
			return w.cloudTurnAckCommand(ctx, p, command, state, code)
		},
		HTTP: w.app.HTTPClient,
	})
}

// CloseCloudTurnRuntimes cooperatively closes only this service's local streams.
// It never sends session/cancel, reconnects, or reconstructs a send permit.
func (w *WorkBuddyConnections) CloseCloudTurnRuntimes(ctx context.Context) error {
	var handles []*cloudTurnRuntimeHandle
	cloudTurnRuntimeLifecycle.Lock()
	w.turnRuntimesClosed = true
	cloudTurnRuntimeHandles.Range(func(key, value any) bool {
		if key.(cloudTurnRuntimeKey).owner == w {
			handle := value.(*cloudTurnRuntimeHandle)
			handle.cancel()
			handles = append(handles, handle)
		}
		return true
	})
	cloudTurnRuntimeLifecycle.Unlock()
	for _, handle := range handles {
		select {
		case <-handle.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
