package workbuddyacp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type sessionFixture struct {
	client *http.Client
	events chan string
	mu     sync.Mutex
	posts  []map[string]json.RawMessage
	hook   func(map[string]json.RawMessage) (int, []string)
}

func newSessionFixture(t *testing.T, hook func(map[string]json.RawMessage) (int, []string)) *sessionFixture {
	t.Helper()
	f := &sessionFixture{events: make(chan string, 128), hook: hook}
	f.client = fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			streamHeaders(w)
			for {
				select {
				case raw := <-f.events:
					_, _ = io.WriteString(w, "data: "+raw+"\n\n")
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("bad request JSON")
			w.WriteHeader(400)
			return
		}
		f.mu.Lock()
		f.posts = append(f.posts, body)
		f.mu.Unlock()
		status := 202
		var messages []string
		if hook != nil {
			status, messages = hook(body)
		}
		if status == 0 {
			status = 202
			switch sessionField(body, "method") {
			case "initialize":
				messages = []string{sessionReply(body["id"], `{"protocolVersion":1,"agentCapabilities":{"loadSession":true}}`)}
			case "session/load":
				messages = []string{sessionReply(body["id"], `{}`)}
			}
		}
		for _, raw := range messages {
			f.events <- raw
		}
		w.WriteHeader(status)
	})
	return f
}
func sessionField(m map[string]json.RawMessage, key string) string {
	var s string
	_ = json.Unmarshal(m[key], &s)
	return s
}
func sessionReply(id json.RawMessage, result string) string {
	return `{"jsonrpc":"2.0","id":` + string(id) + `,"result":` + result + `}`
}
func (f *sessionFixture) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.posts) }
func attachFixture(t *testing.T, f *sessionFixture) *Session {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	t.Cleanup(cancel)
	s, err := Attach(ctx, fixtureAccess(), f.client, Setup{InitializeID: "init", LoadID: "load"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func sessionNext(t *testing.T, s *Session) Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	e, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func permissionFrame(id string) string {
	return `{"jsonrpc":"2.0","id":"` + id + `","method":"session/request_permission","params":{"sessionId":"task_one","toolCall":{"toolCallId":"tool"},"options":[{"optionId":"once","name":"Once","kind":"allow_once"},{"optionId":"always","name":"Always","kind":"allow_always"},{"optionId":"reject","name":"Reject","kind":"reject_once"},{"optionId":"never","name":"Never","kind":"reject_always"}]}}`
}

const sessionUpdateFixture = `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"task_one","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"sensitive-output"}}}}`

func TestSessionAttachExactHandshakeDoesNotRepeatPrompt(t *testing.T) {
	f := newSessionFixture(t, func(b map[string]json.RawMessage) (int, []string) {
		if sessionField(b, "method") == "session/load" {
			return 202, []string{sessionUpdateFixture, sessionReply(b["id"], `{}`)}
		}
		return 0, nil
	})
	s := attachFixture(t, f)
	if f.count() != 2 {
		t.Fatal("attach sent something other than handshake")
	}
	first := codecObject(f.posts[0]["params"])
	caps := codecObject(first["clientCapabilities"])
	fs := codecObject(caps["fs"])
	if string(first["protocolVersion"]) != "1" || string(caps["terminal"]) != "false" || string(fs["readTextFile"]) != "false" || string(fs["writeTextFile"]) != "false" {
		t.Fatal("excessive client capability")
	}
	load := codecObject(f.posts[1]["params"])
	if sessionField(load, "sessionId") != "task_one" || sessionField(load, "cwd") != "/workspace" || string(load["mcpServers"]) != "[]" {
		t.Fatal("load changed task or resources")
	}
	if e := sessionNext(t, s); e.Kind != "session_update" {
		t.Fatal("load history dropped")
	}
}

func TestSessionAttachRejectsUnsupportedAndBoundedHistory(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		overflow     bool
	}{
		{"version", `{"protocolVersion":2}`, false},
		{"load_false", `{"protocolVersion":1,"agentCapabilities":{"loadSession":false}}`, false},
		{"null_capability", `{"protocolVersion":1,"agentCapabilities":{"loadSession":null}}`, false},
		{"missing_capabilities", `{"protocolVersion":1}`, false},
		{"empty_capabilities", `{"protocolVersion":1,"agentCapabilities":{}}`, false},
		{"missing_load_capability", `{"protocolVersion":1,"agentCapabilities":{"promptCapabilities":{}}}`, false},
		{"overflow", `{"protocolVersion":1,"agentCapabilities":{"loadSession":true}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSessionFixture(t, func(b map[string]json.RawMessage) (int, []string) {
				if sessionField(b, "method") == "initialize" {
					messages := []string{}
					if tc.overflow {
						for i := 0; i < 33; i++ {
							messages = append(messages, sessionUpdateFixture)
						}
					}
					return 202, append(messages, sessionReply(b["id"], tc.result))
				}
				return 0, nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			s, err := Attach(ctx, fixtureAccess(), f.client, Setup{InitializeID: "init", LoadID: "load"})
			if s != nil {
				_ = s.Close()
			}
			if err == nil || f.count() != 1 {
				t.Fatal("invalid handshake advanced")
			}
		})
	}
}

func TestSessionPromptCorrelatesExactTurnAndPreservesUnknownUsage(t *testing.T) {
	f := newSessionFixture(t, func(b map[string]json.RawMessage) (int, []string) { return 0, nil })
	s := attachFixture(t, f)
	id := strings.Repeat("a", 200)
	if call, err := s.Prompt(context.Background(), id, "private-prompt"); err != nil || call.State != "sent" {
		t.Fatalf("prompt: %v", err)
	}
	f.events <- `{"jsonrpc":"2.0","id":"another","result":{"stopReason":"end_turn"}}`
	if e := sessionNext(t, s); e.Kind != "response" || e.PromptResponse != nil {
		t.Fatal("foreign response ended turn")
	}
	if _, err := s.Prompt(context.Background(), "second", "text"); err == nil {
		t.Fatal("parallel prompt allowed")
	}
	rawID := f.posts[2]["id"]
	f.events <- sessionReply(rawID, `{"stopReason":"future_reason"}`)
	if e := sessionNext(t, s); e.Kind != "unsupported" {
		t.Fatal("unknown stop fabricated")
	}
	f.events <- sessionReply(rawID, `{"stopReason":"end_turn","usage":{"inputTokens":2}}`)
	e := sessionNext(t, s)
	if e.Kind != "prompt_response" || e.CallID != id || e.PromptResponse == nil || e.PromptResponse.Usage != nil {
		t.Fatal("missing usage fabricated as zero or turn lost")
	}
	if _, err := s.Prompt(context.Background(), id, "changed"); err == nil {
		t.Fatal("completed ID reused")
	}
	if _, err := s.Prompt(context.Background(), "second", "text"); err != nil {
		t.Fatal(err)
	}
}

func TestSessionUnknownSendAndContextNeverRetryOrAutoCancel(t *testing.T) {
	f := newSessionFixture(t, func(b map[string]json.RawMessage) (int, []string) {
		if sessionField(b, "method") == "session/prompt" {
			return 503, nil
		}
		return 0, nil
	})
	s := attachFixture(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Prompt(ctx, "turn", "private-prompt"); err == nil || f.count() != 2 {
		t.Fatal("cancelled context sent prompt")
	}
	call, err := s.Prompt(context.Background(), "turn", "private-prompt")
	assertFailure(t, err, true)
	if call.State != "unknown" {
		t.Fatal("unknown send lost")
	}
	if _, err = s.Prompt(context.Background(), "turn", "private-prompt"); err == nil {
		t.Fatal("unknown was retried")
	}
	_, _ = s.Next(ctx)
	_ = s.Close()
	if f.count() != 3 {
		t.Fatal("context or Close sent remote cancel/retry")
	}
}

func TestSessionPermissionBoundToOriginalConnectionAndOptions(t *testing.T) {
	f := newSessionFixture(t, func(map[string]json.RawMessage) (int, []string) { return 0, nil })
	s := attachFixture(t, f)
	other := attachFixture(t, newSessionFixture(t, func(map[string]json.RawMessage) (int, []string) { return 0, nil }))
	_, _ = s.Prompt(context.Background(), "turn", "question")
	f.events <- permissionFrame("permission")
	e := sessionNext(t, s)
	if e.Kind != "permission_request" || e.PermissionRef == "" {
		t.Fatal("permission not exposed")
	}
	invalid := "forged"
	e.Packet.Permission.Options[0].OptionId = "forged"
	if err := s.ReplyPermission(context.Background(), e.PermissionRef, &invalid); err == nil {
		t.Fatal("mutated event authorized invented option")
	}
	once := "once"
	if err := other.ReplyPermission(context.Background(), e.PermissionRef, &once); err == nil {
		t.Fatal("cross connection reply")
	}
	if err := s.ReplyPermission(context.Background(), e.PermissionRef, &once); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplyPermission(context.Background(), e.PermissionRef, nil); err == nil {
		t.Fatal("second decision allowed")
	}
	f.events <- permissionFrame("permission")
	if _, err := s.Next(context.Background()); err == nil {
		t.Fatal("provider repeated pending identity")
	}
	if f.count() != 4 {
		t.Fatal("invalid reply had external effects")
	}
}

func TestSessionExplicitCancelCancelsPendingThenWaitsForStop(t *testing.T) {
	f := newSessionFixture(t, func(map[string]json.RawMessage) (int, []string) { return 0, nil })
	s := attachFixture(t, f)
	_, _ = s.Prompt(context.Background(), "turn", "text")
	f.events <- permissionFrame("permission")
	e := sessionNext(t, s)
	if err := s.Cancel(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.count() != 5 || sessionField(f.posts[4], "method") != "session/cancel" {
		t.Fatal("cancel sequence missing")
	}
	outcome := codecObject(codecObject(f.posts[3]["result"])["outcome"])
	if sessionField(outcome, "outcome") != "cancelled" {
		t.Fatal("pending permission not cancelled")
	}
	if err := s.ReplyPermission(context.Background(), e.PermissionRef, nil); err == nil {
		t.Fatal("stale permission usable")
	}
	if err := s.Cancel(context.Background()); err == nil {
		t.Fatal("cancel sent twice")
	}
	if _, err := s.Prompt(context.Background(), "next", "text"); err == nil {
		t.Fatal("cancel HTTP acceptance claimed stopped")
	}
	f.events <- sessionReply(f.posts[2]["id"], `{"stopReason":"cancelled"}`)
	if e := sessionNext(t, s); e.Kind != "prompt_response" {
		t.Fatal("actual stop missing")
	}
	if _, err := s.Prompt(context.Background(), "next", "text"); err != nil {
		t.Fatal(err)
	}

	t.Run("local_failure_resumes_without_repeating_decision", func(t *testing.T) {
		f := newSessionFixture(t, func(map[string]json.RawMessage) (int, []string) { return 0, nil })
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		base := f.client.Transport
		f.client.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
			response, err := base.RoundTrip(r)
			if r.Method == http.MethodPost && f.count() == 4 {
				cancel()
			}
			return response, err
		})
		s := attachFixture(t, f)
		_, _ = s.Prompt(context.Background(), "turn", "text")
		f.events <- permissionFrame("permission")
		_ = sessionNext(t, s)
		err := s.Cancel(ctx)
		assertFailure(t, err, false)
		if f.count() != 4 {
			t.Fatal("cancelled local step touched wire")
		}
		if err = s.Cancel(context.Background()); err != nil {
			t.Fatal("safe unsent cancellation could not resume", err)
		}
		if f.count() != 5 || sessionField(f.posts[4], "method") != "session/cancel" {
			t.Fatal("resume repeated permission decision")
		}
	})
}

func TestSessionBoundsPermissionsAndRedacts(t *testing.T) {
	f := newSessionFixture(t, func(map[string]json.RawMessage) (int, []string) { return 0, nil })
	s := attachFixture(t, f)
	_, _ = s.Prompt(context.Background(), "turn", "private-prompt")
	for i := 0; i < 32; i++ {
		f.events <- permissionFrame(fmt.Sprintf("p%d", i))
		e := sessionNext(t, s)
		raw, _ := json.Marshal(e)
		if strings.Contains(string(raw), "toolCall") || strings.Contains(fmt.Sprintf("%+v", e), "Once") {
			t.Fatal("event secret leaked")
		}
	}
	f.events <- permissionFrame("overflow")
	if _, err := s.Next(context.Background()); err == nil {
		t.Fatal("unbounded permission queue")
	}
	if strings.Contains(fmt.Sprintf("%#v", s), "private-prompt") {
		t.Fatal("session secret leaked")
	}
	if f.count() != 3 {
		t.Fatal("permission received became automatic action")
	}
}

func TestSessionDrainsHandshakeBeforeNewPromptAndSerializesCalls(t *testing.T) {
	f := newSessionFixture(t, func(b map[string]json.RawMessage) (int, []string) {
		if sessionField(b, "method") == "session/load" {
			return 202, []string{`{"jsonrpc":"2.0","id":"turn","result":{"stopReason":"end_turn"}}`, sessionReply(b["id"], `{}`)}
		}
		return 0, nil
	})
	s := attachFixture(t, f)
	if _, err := s.Prompt(context.Background(), "turn", "private-prompt"); err == nil || f.count() != 2 {
		t.Fatal("preexisting response could complete a newly sent prompt")
	}
	if e := sessionNext(t, s); e.Kind != "response" || e.PromptResponse != nil {
		t.Fatal("preexisting response completed a turn")
	}
	if _, err := s.Prompt(context.Background(), strings.Repeat("x", 201), "private-prompt"); err == nil || f.count() != 2 {
		t.Fatal("oversized ID sent")
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"turn", "other"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := s.Prompt(context.Background(), id, "private-prompt")
			results <- err
		}(id)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 || f.count() != 3 {
		t.Fatal("concurrent prompts escaped single turn permit")
	}
}

func TestSessionRPCErrorRetiresOnlyMatchingTurnWithoutRetry(t *testing.T) {
	f := newSessionFixture(t, func(map[string]json.RawMessage) (int, []string) { return 0, nil })
	s := attachFixture(t, f)
	_, _ = s.Prompt(context.Background(), "turn", "private-prompt")
	f.events <- permissionFrame("permission")
	permission := sessionNext(t, s)
	f.events <- `{"jsonrpc":"2.0","id":"foreign","error":{"code":-32603,"message":"private-error"}}`
	if e := sessionNext(t, s); e.Kind != "response" || e.CallID != "" {
		t.Fatal("foreign error retired turn")
	}
	if _, err := s.Prompt(context.Background(), "next", "text"); err == nil {
		t.Fatal("foreign error freed slot")
	}
	f.events <- `{"jsonrpc":"2.0","id":` + string(f.posts[2]["id"]) + `,"error":{"code":-32603,"message":"private-error"}}`
	e := sessionNext(t, s)
	if e.Kind != "prompt_error" || e.CallID != "turn" || e.PromptResponse != nil || e.Packet.ErrorCode == nil || *e.Packet.ErrorCode != -32603 {
		t.Fatal("matching RPC error lost call identity or pretended completion")
	}
	if strings.Contains(fmt.Sprintf("%+v", e), "private-error") {
		t.Fatal("error content leaked")
	}
	if err := s.Cancel(context.Background()); err == nil {
		t.Fatal("errored call remained cancellable")
	}
	if err := s.ReplyPermission(context.Background(), permission.PermissionRef, nil); err == nil {
		t.Fatal("retired turn permission remained usable")
	}
	if _, err := s.Prompt(context.Background(), "turn", "private-prompt"); err == nil {
		t.Fatal("errored stable call ID reused")
	}
	if f.count() != 3 {
		t.Fatal("retirement emitted automatic traffic")
	}
	if _, err := s.Prompt(context.Background(), "next", "text"); err != nil {
		t.Fatal("formal error stranded slot", err)
	}
}

func TestSessionQueuedPreexistingResponseCannotCompleteNewCall(t *testing.T) {
	f := newSessionFixture(t, func(b map[string]json.RawMessage) (int, []string) {
		if sessionField(b, "method") == "session/load" {
			return 202, []string{sessionReply(b["id"], `{}`), `{"jsonrpc":"2.0","id":"turn","result":{"stopReason":"end_turn"}}`}
		}
		return 0, nil
	})
	s := attachFixture(t, f)
	// Confirm actual SSE reception after the load response and before Prompt;
	// this lives in Connection.frames, not Session's handshake buffer.
	deadline := time.Now().Add(time.Second)
	for len(s.conn.frames) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(s.conn.frames) == 0 || len(s.buffered) != 0 {
		t.Fatal("fixture did not exercise transport queue")
	}
	if _, err := s.Prompt(context.Background(), "turn", "private-prompt"); err != nil {
		t.Fatal(err)
	}
	e := sessionNext(t, s)
	if e.Kind != "response" || e.PromptResponse != nil || e.CallID != "" {
		t.Fatal("queued preexisting response falsely completed new call")
	}
	if _, err := s.Prompt(context.Background(), "next", "text"); err == nil {
		t.Fatal("queued response freed current slot")
	}
	for i, public := range []string{"init", "load", "turn"} {
		if sessionField(f.posts[i], "id") == public {
			t.Fatal("public operation ID exposed as predictable wire correlation")
		}
	}
	f.events <- sessionReply(f.posts[2]["id"], `{"stopReason":"end_turn"}`)
	e = sessionNext(t, s)
	if e.Kind != "prompt_response" || e.CallID != "turn" {
		t.Fatal("actual private wire response lost stable operation identity")
	}
}

func TestSessionPermissionCanProgressBeforePromptHTTPAcknowledgement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	permissionReplied := make(chan struct{})
	promptEntered := make(chan struct{})
	allowAcknowledgement := make(chan struct{})
	var f *sessionFixture
	f = newSessionFixture(t, func(b map[string]json.RawMessage) (int, []string) {
		if sessionField(b, "method") == "session/prompt" {
			close(promptEntered)
			f.events <- permissionFrame("awaiting_permission")
			select {
			case <-permissionReplied:
			case <-ctx.Done():
				return 503, nil
			}
			f.events <- sessionReply(b["id"], `{"stopReason":"end_turn"}`)
			select {
			case <-allowAcknowledgement:
				return 202, nil
			case <-ctx.Done():
				return 503, nil
			}
		}
		if sessionField(b, "id") == "awaiting_permission" && b["result"] != nil {
			close(permissionReplied)
			return 202, nil
		}
		return 0, nil
	})
	s := attachFixture(t, f)
	type result struct {
		call Call
		err  error
	}
	finished := make(chan result, 1)
	go func() { call, err := s.Prompt(ctx, "turn", "private-prompt"); finished <- result{call, err} }()
	select {
	case <-promptEntered:
	case <-ctx.Done():
		t.Fatal("prompt never reached server")
	}
	e, err := s.Next(ctx)
	if err != nil || e.Kind != "permission_request" || e.PermissionRef == "" {
		t.Fatal("stream permission blocked behind prompt HTTP", err)
	}
	once := "once"
	if err = s.ReplyPermission(ctx, e.PermissionRef, &once); err != nil {
		t.Fatal("permission reply blocked behind prompt HTTP", err)
	}
	e, err = s.Next(ctx)
	if err != nil || e.Kind != "prompt_response" || e.CallID != "turn" {
		t.Fatal("actual turn response could not precede HTTP acknowledgement", err)
	}
	select {
	case <-finished:
		t.Fatal("fixture acknowledged prompt before release")
	default:
	}
	close(allowAcknowledgement)
	select {
	case r := <-finished:
		if r.err != nil || r.call.State != "sent" {
			t.Fatal("prompt was not acknowledged after reply", r.err)
		}
	case <-ctx.Done():
		t.Fatal("prompt acknowledgement deadlocked")
	}
	if err = s.Cancel(context.Background()); err == nil {
		t.Fatal("late HTTP acknowledgement restored retired turn")
	}
	if f.count() != 4 {
		t.Fatal("unexpected repeated request")
	}
}

func TestSessionIndependentPermissionRepliesAndCancelDoNotWaitForFirstAck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	defer release()
	f := newSessionFixture(t, func(b map[string]json.RawMessage) (int, []string) {
		if sessionField(b, "id") == "first" && b["result"] != nil {
			close(firstEntered)
			select {
			case <-releaseFirst:
				return 202, nil
			case <-ctx.Done():
				return 503, nil
			}
		}
		return 0, nil
	})
	s := attachFixture(t, f)
	_, _ = s.Prompt(ctx, "turn", "text")
	f.events <- permissionFrame("first")
	first := sessionNext(t, s)
	f.events <- permissionFrame("second")
	second := sessionNext(t, s)
	firstResult := make(chan error, 1)
	once := "once"
	go func() { firstResult <- s.ReplyPermission(ctx, first.PermissionRef, &once) }()
	select {
	case <-firstEntered:
	case <-ctx.Done():
		t.Fatal("first reply never reached server")
	}
	if err := s.ReplyPermission(ctx, second.PermissionRef, &once); err != nil {
		t.Fatal("independent reply waited for first HTTP acknowledgement", err)
	}
	f.events <- permissionFrame("third")
	third := sessionNext(t, s)
	if err := s.Cancel(ctx); err != nil {
		t.Fatal("explicit cancellation waited for unrelated acknowledgement", err)
	}
	if err := s.ReplyPermission(ctx, first.PermissionRef, &once); err == nil {
		t.Fatal("inflight permission sent twice")
	}
	if err := s.ReplyPermission(ctx, third.PermissionRef, nil); err == nil {
		t.Fatal("cancelled permission sent twice")
	}
	f.events <- permissionFrame("after_cancel")
	late := sessionNext(t, s)
	if err := s.ReplyPermission(ctx, late.PermissionRef, &once); err == nil {
		t.Fatal("cancel intent allowed a new positive decision")
	}
	if f.count() != 7 || sessionField(f.posts[6], "method") != "session/cancel" {
		t.Fatal("independent reply/cancel sequence incomplete")
	}
	thirdOutcome := codecObject(codecObject(f.posts[5]["result"])["outcome"])
	if sessionField(f.posts[5], "id") != "third" || sessionField(thirdOutcome, "outcome") != "cancelled" {
		t.Fatal("notification preceded known pending permission cancellation")
	}
	select {
	case <-firstResult:
		t.Fatal("first acknowledgement unexpectedly released")
	default:
	}
	release()
	select {
	case err := <-firstResult:
		if err != nil {
			t.Fatal("first reply acknowledgement lost", err)
		}
	case <-ctx.Done():
		t.Fatal("released acknowledgement did not complete")
	}
}

func TestSessionCancelNotificationFenceSurvivesTurnRetirement(t *testing.T) {
	for _, status := range []int{202, 503} {
		t.Run(fmt.Sprintf("http_%d", status), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			entered := make(chan struct{})
			releaseAck := make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(releaseAck) }) }
			defer release()
			var f *sessionFixture
			f = newSessionFixture(t, func(b map[string]json.RawMessage) (int, []string) {
				if sessionField(b, "method") == "session/cancel" {
					f.mu.Lock()
					id := append(json.RawMessage(nil), f.posts[2]["id"]...)
					f.mu.Unlock()
					f.events <- sessionReply(id, `{"stopReason":"cancelled"}`)
					close(entered)
					select {
					case <-releaseAck:
						return status, nil
					case <-ctx.Done():
						return 503, nil
					}
				}
				return 0, nil
			})
			s := attachFixture(t, f)
			_, _ = s.Prompt(ctx, "old_turn", "text")
			cancelled := make(chan error, 1)
			go func() { cancelled <- s.Cancel(ctx) }()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("cancel never reached server")
			}
			e, err := s.Next(ctx)
			if err != nil || e.Kind != "prompt_response" || e.CallID != "old_turn" {
				t.Fatal("real old-turn retirement blocked", err)
			}
			if _, err = s.Prompt(ctx, "new_turn", "text"); err == nil || f.count() != 4 {
				t.Fatal("old cancellation still in flight admitted new turn")
			}
			release()
			select {
			case err = <-cancelled:
			case <-ctx.Done():
				t.Fatal("cancel acknowledgement lost")
			}
			if status == 503 {
				assertFailure(t, err, true)
				if _, err = s.Prompt(ctx, "new_turn", "text"); err == nil || f.count() != 4 {
					t.Fatal("unknown cancellation admitted new turn")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err = s.Cancel(ctx); err == nil {
					t.Fatal("late acknowledgement restored old turn")
				}
				if _, err = s.Prompt(ctx, "new_turn", "text"); err != nil {
					t.Fatal("known cancellation acknowledgement stranded session", err)
				}
			}
		})
	}
}

func TestSessionLoadStreamsFullHistoryBeforeHTTPAcknowledgement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	consumed := make(chan struct{})
	var f *sessionFixture
	f = newSessionFixture(t, func(b map[string]json.RawMessage) (int, []string) {
		if sessionField(b, "method") != "session/load" {
			return 0, nil
		}
		for i := 0; i < 96; i++ {
			var raw string
			switch i % 3 {
			case 0:
				raw = fmt.Sprintf(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"task_one","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"history-%d"}},"index":%d}}`, i, i)
			case 1:
				raw = fmt.Sprintf(`{"jsonrpc":"2.0","method":"_codebuddy.ai/artifact","params":{"sessionId":"task_one","event":"updated","artifact":{"type":"plan","uri":"agent:///plan.md"},"index":%d}}`, i)
			case 2:
				raw = fmt.Sprintf(`{"jsonrpc":"2.0","method":"_future_history","params":{"sessionId":"task_one","index":%d}}`, i)
			}
			f.events <- raw
		}
		f.events <- permissionFrame("control_permission")
		f.events <- `{"jsonrpc":"2.0","id":"unmatched","result":{}}`
		f.events <- sessionReply(b["id"], `{}`)
		select {
		case <-consumed:
			return 202, nil
		case <-ctx.Done():
			return 503, nil
		}
	})
	observed := 0
	s, err := Attach(ctx, fixtureAccess(), f.client, Setup{InitializeID: "init", LoadID: "load", OnHistory: func(p Packet) error {
		fields := codecObject(p.Raw.Data)
		params := codecObject(fields["params"])
		if string(params["index"]) != fmt.Sprint(observed) || len(p.RPCID) != 0 || p.Kind == "permission_request" {
			return errors.New("history order or control isolation failed")
		}
		observed++
		if observed == 96 {
			close(consumed)
		}
		return nil
	}})
	if err != nil {
		t.Fatal("full replay could not progress before load POST acknowledgement", err)
	}
	defer s.Close()
	if observed != 96 || len(s.buffered) != 2 || f.count() != 2 {
		t.Fatal("history omitted, buffered without bound, or triggered an action")
	}
	if e := sessionNext(t, s); e.Kind != "permission_request" || e.PermissionRef == "" {
		t.Fatal("control request consumed as history")
	}
	if e := sessionNext(t, s); e.Kind != "response" {
		t.Fatal("unmatched response consumed as history")
	}
	if f.count() != 2 {
		t.Fatal("history auto-responded to permission")
	}
}

func TestSessionHistorySinkFailureOrCancellationStopsOwnedHandshake(t *testing.T) {
	for _, kind := range []string{"sink_rejected", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			events := make(chan string, 8)
			loadExited := make(chan struct{})
			streamExited := make(chan struct{})
			var posts atomic.Int32
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					defer close(streamExited)
					streamHeaders(w)
					for {
						select {
						case raw := <-events:
							_, _ = io.WriteString(w, "data: "+raw+"\n\n")
							w.(http.Flusher).Flush()
						case <-r.Context().Done():
							return
						}
					}
				}
				posts.Add(1)
				var body map[string]json.RawMessage
				_ = json.NewDecoder(r.Body).Decode(&body)
				if sessionField(body, "method") == "initialize" {
					events <- sessionReply(body["id"], `{"protocolVersion":1,"agentCapabilities":{"loadSession":true}}`)
					w.WriteHeader(202)
					return
				}
				defer close(loadExited)
				events <- sessionUpdateFixture
				<-r.Context().Done()
			})
			called := 0
			s, err := Attach(ctx, fixtureAccess(), client, Setup{InitializeID: "init", LoadID: "load", OnHistory: func(Packet) error {
				called++
				if kind == "cancelled" {
					cancel()
					return nil
				}
				return errors.New("private-history-secret")
			}})
			if s != nil {
				_ = s.Close()
				t.Fatal("failed sink produced live session")
			}
			if err == nil || called != 1 || posts.Load() != 2 || strings.Contains(fmt.Sprint(err), "private-history-secret") {
				t.Fatal("sink failure leaked or retried handshake", err)
			}
			timer := time.NewTimer(time.Second)
			defer timer.Stop()
			select {
			case <-loadExited:
			case <-timer.C:
				t.Fatal("owned POST was not cancelled")
			}
			select {
			case <-streamExited:
			case <-time.After(time.Second):
				t.Fatal("owned SSE stream was not closed")
			}
		})
	}
}
