package runner

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const dshReadyFixture = `{"v":1,"type":"ready","runtime":"dsh","plugin_version":"protocol-fixture","capabilities":{"cancel":true}}`
const dshSessionFixture = `{"v":1,"type":"session","request_id":"req-1","session_id":"session-1","resumed":false}`
const dshResultFixture = `{"v":1,"type":"result","request_id":"req-1","session_id":"session-1","status":"completed","output":"done","resume_rejected":false}`

func dshDecodeFixture(lines ...string) (*tc.Evidence, []tc.Event, error) {
	var events []tc.Event
	s := newToolStream(tc.Profile{CommandID: "dsh-team", Adapter: "harness", Protocol: "dsh-multica-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "dsh protocol-fixture"}, []string{"private-key"}, func(e tc.Event) error { events = append(events, e); return nil }, "req-1")
	for _, line := range lines {
		_, _ = s.Write([]byte(line + "\n"))
	}
	e, err := s.finish()
	return e, events, err
}

func TestDshStreamOfficialFramesAndUsagePresence(t *testing.T) {
	usage := `{"v":1,"type":"usage","request_id":"req-1","provider":"fixture","model":"not-a-vendor-call","input_tokens":0,"output_tokens":0,"cache_read_tokens":0,"reasoning_tokens":0}`
	e, events, err := dshDecodeFixture(dshReadyFixture, dshSessionFixture,
		`{"v":1,"type":"text","request_id":"req-1","content":"private-key done"}`,
		`{"v":1,"type":"thinking","request_id":"req-1","content":"do not retain hidden reasoning"}`,
		`{"v":1,"type":"tool_call","request_id":"req-1","call_id":"call-1","name":"read","arguments":"{}"}`,
		`{"v":1,"type":"tool_result","request_id":"req-1","call_id":"call-1","name":"read","output":"sensitive file contents","is_error":false}`,
		usage, dshResultFixture)
	if err != nil || e.Terminal != "completed" || e.SessionID != "session-1" || e.UsageState != tc.UsageComplete || e.Usage == nil || e.Usage.InputTokens == nil || *e.Usage.InputTokens != 0 || e.Usage.CacheWriteInputTokens != nil {
		t.Fatalf("official stream rejected or zero/missing confused: evidence=%+v err=%v", e, err)
	}
	if err := tc.ValidateEvidence(*e); err != nil {
		t.Fatalf("DSH projection does not satisfy the shared evidence contract: %v", err)
	}
	for _, event := range events {
		if strings.Contains(event.Message, "private-key") || strings.Contains(event.Message, "hidden reasoning") || strings.Contains(event.Message, "sensitive file") {
			t.Fatalf("private source material retained: %+v", event)
		}
	}
	e, _, err = dshDecodeFixture(dshReadyFixture, dshSessionFixture, dshResultFixture)
	if err != nil || e.Usage != nil || e.UsageState != tc.UsageUnreported {
		t.Fatalf("absent usage invented: %+v %v", e, err)
	}
	e, _, err = dshDecodeFixture(dshReadyFixture, dshSessionFixture, `{"v":1,"type":"usage","request_id":"req-1","input_tokens":2}`, dshResultFixture)
	if err != nil || e.UsageState != tc.UsagePartial || e.Usage.OutputTokens != nil {
		t.Fatalf("partial usage invented: %+v %v", e, err)
	}
}

func TestDshStreamRejectsUnprovenCompletion(t *testing.T) {
	for name, frames := range map[string][]string{
		"text-only":           {dshReadyFixture, dshSessionFixture, `{"v":1,"type":"text","request_id":"req-1","content":"done"}`},
		"missing-ready":       {dshSessionFixture, dshResultFixture},
		"runtime":             {strings.Replace(dshReadyFixture, `"dsh"`, `"other"`, 1), dshSessionFixture, dshResultFixture},
		"future-version":      {strings.Replace(dshReadyFixture, `"v":1`, `"v":2`, 1), dshSessionFixture, dshResultFixture},
		"foreign-request":     {dshReadyFixture, strings.Replace(dshSessionFixture, "req-1", "req-2", 1), dshResultFixture},
		"changed-session":     {dshReadyFixture, dshSessionFixture, strings.Replace(dshResultFixture, "session-1", "session-2", 1)},
		"resumed":             {dshReadyFixture, strings.Replace(dshSessionFixture, "false", "true", 1), dshResultFixture},
		"duplicate-result":    {dshReadyFixture, dshSessionFixture, dshResultFixture, dshResultFixture},
		"unknown-frame":       {dshReadyFixture, dshSessionFixture, `{"v":1,"type":"made_up","request_id":"req-1"}`, dshResultFixture},
		"unknown-status":      {dshReadyFixture, dshSessionFixture, strings.Replace(dshResultFixture, "completed", "success", 1)},
		"protocol-error":      {`{"v":1,"type":"protocol_error","code":"INVALID_COMMAND","message":"private-key"}`, dshReadyFixture, dshSessionFixture, dshResultFixture},
		"invalid-usage":       {dshReadyFixture, dshSessionFixture, `{"v":1,"type":"usage","request_id":"req-1","input_tokens":null}`, dshResultFixture},
		"foreign-tool-result": {dshReadyFixture, dshSessionFixture, `{"v":1,"type":"tool_result","request_id":"req-1","call_id":"unseen","name":"read","output":"","is_error":false}`, dshResultFixture},
	} {
		t.Run(name, func(t *testing.T) {
			e, _, err := dshDecodeFixture(frames...)
			if err == nil || e.Terminal != "unknown" {
				t.Fatalf("unproven completion accepted: %+v %v", e, err)
			}
		})
	}
	for _, status := range []string{"failed", "aborted", "cancelled"} {
		e, _, err := dshDecodeFixture(dshReadyFixture, dshSessionFixture, strings.Replace(dshResultFixture, "completed", status, 1))
		if err != nil || e.Terminal != "failed" {
			t.Fatalf("known non-success %s: %+v %v", status, e, err)
		}
	}
}

func TestDshInputDuplexKeepsStdinAndCancelsOriginalRequest(t *testing.T) {
	input, err := newDshInput("req-1", t.TempDir(), "inspect actual fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	r := bufio.NewReader(input)
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var execute map[string]any
	if json.Unmarshal(line, &execute) != nil || len(execute) != 6 || execute["type"] != "execute" || execute["request_id"] != "req-1" || execute["prompt"] != "inspect actual fixture" || execute["v"] != float64(1) || len(execute["mcp_servers"].([]any)) != 0 {
		t.Fatalf("uncontrolled execute: %s", line)
	}
	read := make(chan []byte, 1)
	go func() { b, _ := r.ReadBytes('\n'); read <- b }()
	select {
	case <-read:
		t.Fatal("stdin ended before cancellation/result")
	case <-time.After(20 * time.Millisecond):
	}
	if err = input.Cancel(); err != nil {
		t.Fatal(err)
	}
	if err = input.Cancel(); err != nil {
		t.Fatal(err)
	}
	select {
	case line = <-read:
		var cancel map[string]any
		if json.Unmarshal(line, &cancel) != nil || len(cancel) != 3 || cancel["request_id"] != "req-1" || cancel["type"] != "cancel" || cancel["v"] != float64(1) {
			t.Fatalf("invalid cancellation: %s", line)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not reach the held stdin")
	}
	if err = input.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = r.ReadByte(); err != io.EOF {
		t.Fatalf("close did not release stdin: %v", err)
	}
	if err = input.Cancel(); err != io.ErrClosedPipe {
		t.Fatalf("cancel after close: %v", err)
	}
}
