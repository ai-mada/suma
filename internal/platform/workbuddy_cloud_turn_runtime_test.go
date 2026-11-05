package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/workbuddy"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// A genuine HTTP/SSE loopback transport exercises the existing ACP SDK. Only
// the transactional bridge is in-memory here; SQL/GUI evidence is separate.
type cloudTurnHTTPFixture struct {
	mu                sync.Mutex
	events            chan string
	stopStream        chan struct{}
	posts             []map[string]json.RawMessage
	entries           []wc.CloudTurnEntry
	queue             []cloudTurnRuntimeCommand
	acks              map[string]string
	finished          cloudTurnRuntimeEvent
	begin, rest, gets int
	strictDenied      atomic.Bool
	hooks             cloudTurnRuntimeHooks
	onPost            func(http.ResponseWriter, *http.Request, map[string]json.RawMessage) bool
	permissionOption  string
}

func newCloudTurnHTTPFixture(t *testing.T) *cloudTurnHTTPFixture {
	t.Helper()
	f := &cloudTurnHTTPFixture{events: make(chan string, 64), stopStream: make(chan struct{}), acks: map[string]string{}}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/v2/tasks/task169" {
			f.mu.Lock()
			f.rest++
			f.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"task_id":"task169","status":"working","link":"https://acp.workbuddy.cn/sessions/task169","token":"private-ticket-169"}`)
			return
		}
		if r.URL.Path != "/sessions/task169" {
			t.Error("unexpected loopback route")
			w.WriteHeader(404)
			return
		}
		if r.Method == http.MethodGet {
			f.mu.Lock()
			f.gets++
			f.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Acp-Connection-Id", "turn169-connection")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			for {
				select {
				case raw := <-f.events:
					_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
					w.(http.Flusher).Flush()
				case <-f.stopStream:
					return
				case <-r.Context().Done():
					return
				}
			}
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid controlled POST")
			w.WriteHeader(400)
			return
		}
		f.mu.Lock()
		f.posts = append(f.posts, body)
		f.mu.Unlock()
		if f.onPost != nil && f.onPost(w, r, body) {
			return
		}
		switch cloudTurnHTTPField(body, "method") {
		case "initialize":
			f.events <- cloudTurnHTTPReply(body["id"], `{"protocolVersion":1,"agentCapabilities":{"loadSession":true}}`)
		case "session/load":
			f.events <- cloudTurnHTTPReply(body["id"], `{}`)
		}
		w.WriteHeader(202)
	}))
	t.Cleanup(s.Close)
	u, _ := url.Parse(s.URL)
	base := s.Client().Transport
	client := &http.Client{Transport: cloudTurnRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://acp.workbuddy.cn/sessions/task169" {
			t.Error("ACP transport escaped fixed endpoint")
			return nil, errors.New("unexpected endpoint")
		}
		copy := r.Clone(r.Context())
		target := *r.URL
		target.Scheme, target.Host = u.Scheme, u.Host
		copy.URL = &target
		return base.RoundTrip(copy)
	})}
	vendor, err := workbuddy.NewClient(workbuddy.Config{BaseURL: s.URL, Token: "private-oauth-169", HTTPClient: s.Client()})
	if err != nil {
		t.Fatal("controlled REST client invalid")
	}
	lease := cloudTurnLease{Turn: wc.CloudTurn{ID: "turn169", Source: wc.CloudTurnSource{Collaboration: wc.CloudCollaborationSource{TaskID: "task169"}}}, Prompt: "a new follow-up only", Client: vendor, Secrets: []string{"private-oauth-169"}, Remaining: 4 * time.Second}
	f.hooks = cloudTurnRuntimeHooks{
		Check: func(context.Context) (cloudTurnLease, error) { return lease, nil },
		CheckWork: func(context.Context) (cloudTurnLease, error) {
			if f.strictDenied.Load() {
				return cloudTurnLease{}, errors.New("new work paused")
			}
			return lease, nil
		},
		Begin: func(context.Context) error { f.mu.Lock(); defer f.mu.Unlock(); f.begin++; return nil },
		Append: func(_ context.Context, e cloudTurnRuntimeEvent) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if e.Entry != nil {
				f.entries = append(f.entries, *e.Entry)
			}
			if e.Permission != nil && f.permissionOption != "" {
				p := *e.Permission
				f.queue = append(f.queue, cloudTurnRuntimeCommand{Command: wc.CloudTurnCommand{ID: "reply169", TurnID: "turn169", Kind: "permission", PermissionID: p.ID, OptionID: f.permissionOption}, Permission: p})
			}
			return nil
		},
		Finish: func(_ context.Context, e cloudTurnRuntimeEvent) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.finished = e
			return nil
		},
		Heartbeat: func(context.Context) error { return nil },
		Take: func(context.Context) ([]cloudTurnRuntimeCommand, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			q := f.queue
			f.queue = nil
			return q, nil
		},
		Ack: func(_ context.Context, c cloudTurnRuntimeCommand, state, code string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.acks[c.Command.ID] = state
			return nil
		},
		HTTP: client, Poll: time.Millisecond, Beat: time.Second,
	}
	return f
}

func cloudTurnHTTPField(body map[string]json.RawMessage, key string) string {
	var text string
	_ = json.Unmarshal(body[key], &text)
	return text
}
func cloudTurnHTTPReply(id json.RawMessage, result string) string {
	return `{"jsonrpc":"2.0","id":` + string(id) + `,"result":` + result + `}`
}
func cloudTurnHTTPUpdate(update string) string {
	return `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"task169","update":` + update + `}}`
}
func (f *cloudTurnHTTPFixture) run() { runCloudTurnRuntime(context.Background(), "turn169", f.hooks) }
func (f *cloudTurnHTTPFixture) methodCount(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, b := range f.posts {
		if cloudTurnHTTPField(b, "method") == method {
			n++
		}
	}
	return n
}

func TestCloudTurnRuntimePermissionUnblocksPromptHTTPAndRedactsSplitSecret(t *testing.T) {
	f := newCloudTurnHTTPFixture(t)
	f.permissionOption = "once"
	replied := make(chan struct{})
	var wire json.RawMessage
	f.onPost = func(w http.ResponseWriter, r *http.Request, b map[string]json.RawMessage) bool {
		switch cloudTurnHTTPField(b, "method") {
		case "session/prompt":
			wire = b["id"]
			f.events <- `{"jsonrpc":"2.0","id":"permission-rpc","method":"session/request_permission","params":{"sessionId":"task169","toolCall":{"toolCallId":"tool169","title":"Run controlled tests","kind":"execute","rawInput":{"command":"go test ./controlled"}},"options":[{"optionId":"once","name":"Once","kind":"allow_once"},{"optionId":"reject","name":"Reject","kind":"reject_once"}]}}`
			select {
			case <-replied:
			case <-r.Context().Done():
				return true
			}
			// Emit the terminal RPC before the independent Prompt HTTP ACK.
			f.events <- cloudTurnHTTPUpdate(`{"sessionUpdate":"tool_call","toolCallId":"tool169","title":"Tests","status":"in_progress"}`)
			f.events <- cloudTurnHTTPUpdate(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"回复 private-tic"}}`)
			f.events <- cloudTurnHTTPUpdate(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"ket-169 完成"}}`)
			f.events <- cloudTurnHTTPUpdate(`{"sessionUpdate":"tool_call_update","toolCallId":"tool169","status":"completed"}`)
			f.events <- `{"jsonrpc":"2.0","method":"_codebuddy.ai/artifact","params":{"sessionId":"task169","event":"created","artifact":{"type":"plan","uri":"agent:///artifacts/private-plan.md","text":"核对计划 private-ticket-169"}}}`
			f.events <- cloudTurnHTTPReply(wire, `{"stopReason":"end_turn","usage":{"inputTokens":3,"outputTokens":4,"totalTokens":7}}`)
			time.Sleep(10 * time.Millisecond)
			w.WriteHeader(202)
			return true
		case "":
			if string(b["id"]) == `"permission-rpc"` {
				close(replied)
				w.WriteHeader(202)
				return true
			}
		}
		return false
	}
	f.run()
	if f.finished.State != "completed" || f.finished.UsageState != "partial" || f.finished.Usage == nil || f.acks["reply169"] != "acknowledged" {
		t.Fatalf("permission/Prompt ACK did not complete accurately: state=%s code=%s ack=%s", f.finished.State, f.finished.ErrorCode, f.acks["reply169"])
	}
	var text strings.Builder
	var artifacts int
	for _, e := range f.entries {
		text.WriteString(e.Text)
		if e.Kind == "artifact" {
			artifacts++
			if e.ArtifactType != "plan" || e.ArtifactEvent != "created" || e.Text != "核对计划 "+cloudTurnHidden {
				t.Fatal("incorrect plan artifact projection")
			}
		}
	}
	if artifacts != 1 {
		t.Fatalf("expected one projected artifact entry, got %d", artifacts)
	}
	if strings.Contains(text.String(), "private-ticket-169") || !strings.Contains(text.String(), "回复") || !strings.Contains(text.String(), cloudTurnHidden) {
		t.Fatal("streamed reply leaked secret or lost display text")
	}
	if f.begin != 1 || f.rest != 1 || f.gets != 1 || f.methodCount("session/prompt") != 1 || f.methodCount("session/cancel") != 0 {
		t.Fatal("runtime replayed initial/new prompt or auto-cancelled")
	}
}

func TestCloudTurnRuntimeDisconnectedOrUnfinishedNeverCompletes(t *testing.T) {
	for _, mode := range []string{"disconnect", "unfinished"} {
		t.Run(mode, func(t *testing.T) {
			f := newCloudTurnHTTPFixture(t)
			f.onPost = func(w http.ResponseWriter, r *http.Request, b map[string]json.RawMessage) bool {
				if cloudTurnHTTPField(b, "method") != "session/prompt" {
					return false
				}
				if mode == "disconnect" {
					w.WriteHeader(202)
					w.(http.Flusher).Flush()
					close(f.stopStream)
					return true
				}
				f.events <- cloudTurnHTTPUpdate(`{"sessionUpdate":"tool_call","toolCallId":"unfinished","status":"in_progress"}`)
				f.events <- cloudTurnHTTPReply(b["id"], `{"stopReason":"end_turn"}`)
				w.WriteHeader(202)
				return true
			}
			f.run()
			if f.finished.State != "unknown" || f.methodCount("session/prompt") != 1 || f.methodCount("session/cancel") != 0 || f.gets != 1 {
				t.Fatal("disconnect/unfinished tool was healed, replayed or auto-cancelled")
			}
		})
	}
}

func TestCloudTurnRuntimeCancelUnknownSurvivesTerminalRPC(t *testing.T) {
	f := newCloudTurnHTTPFixture(t)
	var wire json.RawMessage
	f.onPost = func(w http.ResponseWriter, r *http.Request, b map[string]json.RawMessage) bool {
		switch cloudTurnHTTPField(b, "method") {
		case "session/prompt":
			f.mu.Lock()
			wire = append(json.RawMessage{}, b["id"]...)
			f.queue = append(f.queue, cloudTurnRuntimeCommand{Command: wc.CloudTurnCommand{ID: "cancel169", TurnID: "turn169", Kind: "cancel"}})
			f.mu.Unlock()
			f.strictDenied.Store(true) // Budget/pause blocks new work, not exact cancel.
			w.WriteHeader(202)
			return true
		case "session/cancel":
			f.events <- cloudTurnHTTPReply(wire, `{"stopReason":"cancelled"}`)
			w.WriteHeader(503)
			return true
		}
		return false
	}
	f.run()
	if f.finished.State != "unknown" || f.acks["cancel169"] != "unknown" || f.methodCount("session/cancel") != 1 || f.methodCount("session/prompt") != 1 {
		t.Fatalf("unknown cancel ACK was fabricated or new-work budget blocked cancel: state=%s ack=%s", f.finished.State, f.acks["cancel169"])
	}
}

func TestCloudTurnRuntimeCloseRejectsLateOriginalPermitWithoutConnection(t *testing.T) {
	h, _, turn, permit := cloudTurnPrepared(t)
	var outgoing atomic.Int32
	h.w.app.HTTPClient = &http.Client{Transport: cloudTurnRoundTripper(func(*http.Request) (*http.Response, error) {
		outgoing.Add(1)
		return nil, errors.New("no network is authorized after close")
	})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.w.CloseCloudTurnRuntimes(ctx); err != nil {
		t.Fatal("empty local close failed")
	}
	h.w.cloudTurnStartRuntime(permit)
	page, err := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
	if err != nil || page.Turn.State != "rejected" || page.Turn.ErrorCode != "workbuddy_cloud_turn_interrupted" || outgoing.Load() != 0 {
		t.Fatal("a late original commit admitted external work or lost known-not-sent fact")
	}
	if _, exists := cloudTurnRuntimeHandles.Load(cloudTurnRuntimeKey{h.w, permit.id}); exists {
		t.Fatal("closed lifecycle registered a late stream")
	}
}

func TestCloudTurnRuntimeCancelDoesNotAcknowledgeLaterPermission(t *testing.T) {
	f := newCloudTurnHTTPFixture(t)
	permissionBObserved, cancelAcknowledged := make(chan struct{}), make(chan struct{})
	appendBefore, ackBefore := f.hooks.Append, f.hooks.Ack
	f.hooks.Append = func(ctx context.Context, event cloudTurnRuntimeEvent) error {
		if err := appendBefore(ctx, event); err != nil {
			return err
		}
		if event.Permission != nil {
			if event.Permission.ToolCallID == "tool-A" {
				f.mu.Lock()
				f.queue = append(f.queue, cloudTurnRuntimeCommand{Command: wc.CloudTurnCommand{ID: "cancel-race169", TurnID: "turn169", Kind: "cancel"}, cancelPermissions: []cloudTurnPermissionVersion{{ID: event.Permission.ID, Version: 1}}})
				f.mu.Unlock()
			} else {
				close(permissionBObserved)
			}
		}
		return nil
	}
	f.hooks.Ack = func(ctx context.Context, command cloudTurnRuntimeCommand, state, code string) error {
		if err := ackBefore(ctx, command, state, code); err != nil {
			return err
		}
		if command.Command.Kind == "cancel" && state == "acknowledged" {
			close(cancelAcknowledged)
		}
		return nil
	}
	permission := func(id, tool string) string {
		return `{"jsonrpc":"2.0","id":"` + id + `","method":"session/request_permission","params":{"sessionId":"task169","toolCall":{"toolCallId":"` + tool + `","title":"Controlled review","rawInput":{"command":"controlled-check"}},"options":[{"optionId":"once","name":"Once","kind":"allow_once"},{"optionId":"reject","name":"Reject","kind":"reject_once"}]}}`
	}
	var wire json.RawMessage
	var replyCount atomic.Int32
	f.onPost = func(w http.ResponseWriter, request *http.Request, body map[string]json.RawMessage) bool {
		switch cloudTurnHTTPField(body, "method") {
		case "session/prompt":
			wire = append(json.RawMessage{}, body["id"]...)
			f.events <- permission("permission-A", "tool-A")
			w.WriteHeader(202)
			return true
		case "":
			replyCount.Add(1)
			w.WriteHeader(202)
			return true
		case "session/cancel":
			// B arrives after the SDK has sent A's cancelled permission reply,
			// while the independent session/cancel POST still has no ACK.
			f.events <- permission("permission-B", "tool-B")
			select {
			case <-permissionBObserved:
			case <-request.Context().Done():
				return true
			}
			w.WriteHeader(202)
			w.(http.Flusher).Flush()
			select {
			case <-cancelAcknowledged:
			case <-time.After(time.Second):
				return true
			}
			// Let the runtime consume the ACK before this separate RPC end.
			time.Sleep(20 * time.Millisecond)
			f.events <- cloudTurnHTTPUpdate(`{"sessionUpdate":"tool_call_update","toolCallId":"tool-A","status":"completed"}`)
			f.events <- cloudTurnHTTPUpdate(`{"sessionUpdate":"tool_call_update","toolCallId":"tool-B","status":"completed"}`)
			f.events <- cloudTurnHTTPReply(wire, `{"stopReason":"cancelled"}`)
			return true
		}
		return false
	}
	f.run()
	if f.finished.State != "unknown" || f.acks["cancel-race169"] != "acknowledged" || replyCount.Load() != 1 {
		t.Fatalf("later unanswered permission was retired by an earlier cancel: state=%s cancel=%s replies=%d", f.finished.State, f.acks["cancel-race169"], replyCount.Load())
	}
}
