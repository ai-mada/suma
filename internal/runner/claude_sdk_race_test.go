package runner

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

func TestClaudeSDKClosedAuthorityCannotLeakQueuedResponse(t *testing.T) {
	a, p, r, permission := claudeSDKOwnedExecution192(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	j, err := OpenJournal(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	entered, released := make(chan struct{}), make(chan struct{})
	cfg := ExecConfig{Journal: j, ToolInputClient: &Client{}, ToolPermission: func(_ context.Context, source pc.Request) (pc.Decision, error) {
		permission.Request = source
		if err := j.observeToolPermission(a, permission); err != nil {
			return pc.Decision{}, err
		}
		return *permission.Decision, nil
	}, ToolPermissionPoll: func(context.Context, string) (pc.Permission, error) {
		close(entered)
		<-released
		return permission, nil
	}}
	d, err := newClaudeSDKDriver(ctx, a, cfg, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	frames := make(chan map[string]any, 8)
	done := make(chan struct{})
	go func() { defer close(done); _, _ = d.CopyInput(claudeSDKFrameWriter192{frames}) }()
	select {
	case <-frames:
	case <-ctx.Done():
		t.Fatal("missing original execute")
	}
	write := func(frame map[string]any) {
		frame["v"], frame["request_id"] = 1, d.requestID
		raw, _ := json.Marshal(frame)
		if _, err := d.Write(append(raw, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	write(map[string]any{"type": "initialized", "session_id": r.Claude.SessionID, "model": p.ClaudeSDKExecution.Model, "configuration_sha256": p.ClaudeSDKExecution.ConfigurationSHA256, "sdk_version": cl.SDKVersion, "cli_version": cl.CLIVersion})
	write(map[string]any{"type": "permission_request", "source": r.Claude})
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("authority poll did not start")
	}
	// A real terminal can arrive while the final current-authority read is in
	// flight. Persisted intent must never let that late response reach stdin.
	write(map[string]any{"type": "failed", "session_id": r.Claude.SessionID, "evidence": map[string]any{"result_subtype": "error_during_execution", "result_is_error": true, "turn_ended": true, "tools_completed": false, "callback_count": 1, "returned_count": 0, "pending_callbacks": 1, "callback_denial_count": 0}})
	close(released)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("single writer did not drain")
	}
	proof := d.finish(true, true)
	if proof.Terminal != "unknown" || len(frames) != 0 {
		t.Fatal("closed callback leaked a new native response")
	}
	if len(proof.ClaudeSDKExecution.Permissions) != 1 {
		t.Fatal("original permission evidence lost")
	}
	o := proof.ClaudeSDKExecution.Permissions[0].Observation
	if o == nil || !o.ReplyStarted || o.ReplyWritten || !o.ExecutionClosed || o.RequestResolved || o.FailureCode == "" {
		t.Fatalf("unknown persisted intent was fabricated or lost: %+v", o)
	}
}
