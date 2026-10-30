package workbuddyacp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/workbuddy"
	acp "github.com/coder/acp-go-sdk"
)

// Setup supplies unique, caller-owned request IDs. Attach never replays a prompt.
type Setup struct {
	InitializeID, LoadID string
	// OnHistory consumes replay observations in arrival order before Attach
	// returns. The trusted caller must return promptly (and make its own I/O
	// context-aware); the callback grants no authority to execute content.
	OnHistory func(Packet) error `json:"-"`
}

// Call describes transport observation, not provider execution or completion.
type Call struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

func (Call) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddyacp.Call{request:redacted}")
}

// Event contains transient untrusted provider content. prompt_response and
// prompt_error retire a correlated turn; neither is task acceptance or proof
// of process stop. A formal RPC error is not successful completion.
type Event struct {
	Kind           string              `json:"kind"`
	Packet         Packet              `json:"-"`
	CallID         string              `json:"-"`
	PromptResponse *acp.PromptResponse `json:"-"`
	PermissionRef  string              `json:"-"`
}

func (Event) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddyacp.Event{payload:redacted}")
}

type sessionPermission struct {
	packet  Packet
	turn    string
	decided bool
}

// Session has no reconnect, retry, token refresh or background cancellation.
// Close affects this local stream only. All outgoing decisions are explicit.
type Session struct {
	conn           *Connection
	sessionID      string
	mu             sync.Mutex
	nextMu         sync.Mutex
	closed         bool
	active         string
	activeWire     string
	cancelling     bool
	cancelInFlight bool
	cancelSent     bool
	// Session-wide cancellation has no turn ID on the wire. Its external-effect
	// fence survives RPC turn retirement until a known HTTP outcome is observed.
	cancelFence   string
	calls         map[string]bool
	buffered      []Packet
	permissions   map[string]*sessionPermission
	permissionIDs map[string]bool
}

func (*Session) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddyacp.Session{content:redacted}")
}

func Attach(ctx context.Context, access workbuddy.SessionAccess, client *http.Client, setup Setup) (*Session, error) {
	if !safeID(setup.InitializeID) || !safeID(setup.LoadID) || setup.InitializeID == setup.LoadID {
		return nil, failure("invalid_request", 0, false)
	}
	initializeWire, err := sessionWireID()
	if err != nil {
		return nil, err
	}
	loadWire, err := sessionWireID()
	if err != nil {
		return nil, err
	}
	conn, err := Connect(ctx, access, client)
	if err != nil {
		return nil, err
	}
	s := &Session{conn: conn, sessionID: access.SessionID, calls: map[string]bool{setup.InitializeID: true, setup.LoadID: true}, permissions: make(map[string]*sessionPermission), permissionIDs: make(map[string]bool)}
	fail := func(err error) (*Session, error) { _ = s.Close(); return nil, err }
	// The SDK's capability booleans have omitempty tags; use an explicit closed
	// wire projection to advertise false rather than silently omit permissions.
	initialize := struct {
		ProtocolVersion    acp.ProtocolVersion `json:"protocolVersion"`
		ClientCapabilities struct {
			Fs struct {
				ReadTextFile  bool `json:"readTextFile"`
				WriteTextFile bool `json:"writeTextFile"`
			} `json:"fs"`
			Terminal bool `json:"terminal"`
		} `json:"clientCapabilities"`
	}{ProtocolVersion: 1}
	result, err := s.handshake(ctx, initializeWire, "initialize", initialize, setup.OnHistory)
	if err != nil {
		return fail(err)
	}
	var initialized acp.InitializeResponse
	fields := codecObject(result)
	if fields == nil || string(fields["protocolVersion"]) != "1" || json.Unmarshal(result, &initialized) != nil {
		return fail(failure("invalid_response", 0, false))
	}
	rawCapabilities, present := fields["agentCapabilities"]
	if !present {
		return fail(failure("load_unsupported", 0, false))
	}
	capabilities := codecObject(rawCapabilities)
	if capabilities == nil {
		return fail(failure("invalid_response", 0, false))
	}
	if string(capabilities["loadSession"]) != "true" {
		return fail(failure("load_unsupported", 0, false))
	}
	// ACP v1 requires an explicit loadSession capability before session/load;
	// omission is not permission to probe an unsupported method.
	load := acp.LoadSessionRequest{SessionId: acp.SessionId(access.SessionID), Cwd: "/workspace", McpServers: []acp.McpServer{}}
	result, err = s.handshake(ctx, loadWire, "session/load", load, setup.OnHistory)
	if err != nil {
		return fail(err)
	}
	var loaded acp.LoadSessionResponse
	if codecObject(result) == nil || json.Unmarshal(result, &loaded) != nil {
		return fail(failure("invalid_response", 0, false))
	}
	return s, nil
}

func (s *Session) sendRequest(ctx context.Context, id, method string, params any) error {
	raw, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      string `json:"id,omitempty"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", id, method, params})
	if err != nil {
		return failure("invalid_request", 0, false)
	}
	return s.conn.send(ctx, raw)
}

// handshake reads SSE while its one POST is in flight. Neither HTTP acceptance
// nor an RPC response alone establishes successful attachment. The send worker
// is always joined; failures cancel only our own request and local connection.
func (s *Session) handshake(ctx context.Context, id, method string, params any, sink func(Packet) error) (json.RawMessage, error) {
	requestCtx, cancel := context.WithCancel(ctx)
	sent := make(chan error, 1)
	finished := make(chan struct{})
	defer func() { cancel(); <-finished }()
	go func() {
		defer close(finished)
		err := s.sendRequest(requestCtx, id, method, params)
		sent <- err
		if err != nil {
			cancel()
		}
	}()
	result, readErr := s.waitResponse(requestCtx, id, sink)
	if readErr != nil {
		// If the sender caused Receive cancellation, retain its observed HTTP
		// unknown-effect classification rather than replacing it with cancelled.
		select {
		case sendErr := <-sent:
			if sendErr != nil {
				return nil, sendErr
			}
			return nil, readErr
		default:
		}
		cancel()
		<-sent
		return nil, readErr
	}
	if err := <-sent; err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Session) waitResponse(ctx context.Context, id string, sink func(Packet) error) (json.RawMessage, error) {
	for {
		frame, err := s.conn.Receive(ctx)
		if err != nil {
			return nil, err
		}
		packet, err := DecodeFrame(frame, s.sessionID)
		if err != nil {
			return nil, err
		}
		if packet.Kind == "response" && sessionResponseID(packet.RPCID) == id {
			if packet.ErrorCode != nil {
				return nil, failure("rpc_error", 0, false)
			}
			return packet.Result, nil
		}
		if sink != nil && len(packet.RPCID) == 0 && packet.Method != "" && codecOneOf(packet.Kind, "session_update", "artifact", "unsupported") {
			if err := sink(packet); err != nil {
				return nil, failure("history_rejected", 0, false)
			}
			continue
		}
		if len(s.buffered) >= 32 {
			return nil, failure("event_limit", 0, false)
		}
		s.buffered = append(s.buffered, packet)
	}
}
func sessionResponseID(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

// Wire IDs are fresh correlation nonces, not provider idempotency keys. Stable
// caller IDs remain in calls; queued observations cannot predict a new request.
func sessionWireID() (string, error) {
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return "", failure("local_failure", 0, false)
	}
	return "rpc_" + hex.EncodeToString(nonce), nil
}
func sessionUnknown(err error) bool { var e *Error; return errors.As(err, &e) && e.EffectUnknown }

// Prompt sends precisely one caller-authorized text turn. IDs are never reused
// after a sent or uncertain request, even if the text is identical.
func (s *Session) Prompt(ctx context.Context, id, text string) (Call, error) {
	if s == nil || ctx == nil || !safeID(id) || !utf8.ValidString(text) || len(text) > 256<<10 || strings.TrimSpace(text) == "" || strings.ContainsRune(text, 0) {
		return Call{}, failure("invalid_request", 0, false)
	}
	if ctx.Err() != nil {
		return Call{}, failure("cancelled", 0, false)
	}
	wireID, err := sessionWireID()
	if err != nil {
		return Call{}, err
	}
	s.mu.Lock()
	if s.closed || s.active != "" || s.cancelFence != "" || len(s.buffered) != 0 || s.calls[id] || len(s.calls) >= 256 {
		s.mu.Unlock()
		return Call{}, failure("call_conflict", 0, false)
	}
	s.calls[id] = true
	s.active = id
	s.activeWire = wireID
	s.cancelling = false
	s.cancelInFlight = false
	s.cancelSent = false
	s.mu.Unlock()
	err = s.sendRequest(ctx, wireID, "session/prompt", acp.PromptRequest{SessionId: acp.SessionId(s.sessionID), Prompt: []acp.ContentBlock{acp.TextBlock(text)}})
	if err != nil {
		if sessionUnknown(err) {
			return Call{ID: id, State: "unknown"}, err
		}
		s.mu.Lock()
		if s.active == id {
			s.active = ""
			s.activeWire = ""
			delete(s.calls, id)
		}
		s.mu.Unlock()
		return Call{}, err
	}
	return Call{ID: id, State: "sent"}, nil
}

func (s *Session) Next(ctx context.Context) (Event, error) {
	if s == nil || ctx == nil {
		return Event{}, failure("invalid_request", 0, false)
	}
	if ctx.Err() != nil {
		return Event{}, failure("cancelled", 0, false)
	}
	s.nextMu.Lock()
	defer s.nextMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return Event{}, failure("closed", 0, false)
	}
	var packet Packet
	buffered := len(s.buffered) > 0
	if buffered {
		packet = s.buffered[0]
		s.buffered = s.buffered[1:]
	}
	s.mu.Unlock()
	if !buffered {
		frame, err := s.conn.Receive(ctx)
		if err != nil {
			return Event{}, err
		}
		var errDecode error
		packet, errDecode = DecodeFrame(frame, s.sessionID)
		if errDecode != nil {
			return Event{}, errDecode
		}
	}
	event := Event{Kind: packet.Kind, Packet: packet}
	// SSE is independent of POST acknowledgement. Process observations under a
	// short state lock, so permission handling can unblock a still-pending POST.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Event{}, failure("closed", 0, false)
	}
	if packet.Kind == "permission_request" {
		if packet.Raw.source != s.conn {
			return Event{}, failure("permission_source", 0, false)
		}
		key := sessionRPCKey(packet.RPCID)
		pending := 0
		for _, p := range s.permissions {
			if !p.decided {
				pending++
			}
		}
		if s.permissionIDs[key] || pending >= 32 || len(s.permissionIDs) >= 256 {
			return Event{}, failure("permission_limit", 0, false)
		}
		// Decode a fresh copy: mutating an Event must never alter stored authority.
		trusted, err := DecodeFrame(packet.Raw, s.sessionID)
		if err != nil {
			return Event{}, err
		}
		nonce := make([]byte, 24)
		if _, err = rand.Read(nonce); err != nil {
			return Event{}, failure("local_failure", 0, false)
		}
		ref := hex.EncodeToString(nonce)
		s.permissions[ref] = &sessionPermission{packet: trusted, turn: s.active}
		s.permissionIDs[key] = true
		event.PermissionRef = ref
	}
	if packet.Kind == "response" && s.active != "" && sessionResponseID(packet.RPCID) == s.activeWire {
		if packet.ErrorCode != nil {
			event.Kind = "prompt_error"
			event.CallID = s.active
			s.retireTurnLocked()
			return event, nil
		}
		var response acp.PromptResponse
		fields := codecObject(packet.Result)
		reason := codecString(fields, "stopReason", 80)
		if !codecOneOf(reason, "end_turn", "max_tokens", "max_turn_requests", "refusal", "cancelled") || json.Unmarshal(packet.Result, &response) != nil {
			event.Kind = "unsupported"
			return event, nil
		}
		if !sessionUsageKnown(fields["usage"]) {
			response.Usage = nil
		}
		event.Kind = "prompt_response"
		event.CallID = s.active
		event.PromptResponse = &response
		s.retireTurnLocked()
	}
	return event, nil
}

func (s *Session) retireTurnLocked() {
	for _, permission := range s.permissions {
		if permission.turn == s.active {
			permission.decided = true
		}
	}
	s.active = ""
	s.activeWire = ""
	s.cancelling = false
	s.cancelInFlight = false
	s.cancelSent = false
}
func sessionUsageKnown(raw json.RawMessage) bool {
	fields := codecObject(raw)
	if fields == nil {
		return false
	}
	for _, key := range []string{"inputTokens", "outputTokens", "totalTokens"} {
		if !codecNumber(fields, key) {
			return false
		}
	}
	for _, key := range []string{"cachedReadTokens", "cachedWriteTokens", "thoughtTokens"} {
		if _, ok := fields[key]; ok && !codecNumber(fields, key) {
			return false
		}
	}
	return true
}
func sessionRPCKey(raw json.RawMessage) string {
	if len(raw) > 0 && raw[0] == '"' {
		var value string
		_ = json.Unmarshal(raw, &value)
		return "s:" + value
	}
	number := new(big.Int)
	number.SetString(string(raw), 10)
	return "n:" + number.String()
}

// ReplyPermission is valid only for a locally observed opaque reference and an
// original offered option. nil is an explicit cancelled response, never approval.
func (s *Session) ReplyPermission(ctx context.Context, ref string, optionID *string) error {
	if s == nil || ctx == nil {
		return failure("invalid_request", 0, false)
	}
	return s.replyPermission(ctx, ref, optionID, "")
}
func (s *Session) replyPermission(ctx context.Context, ref string, optionID *string, cancelTurn string) error {
	if ctx.Err() != nil {
		return failure("cancelled", 0, false)
	}
	s.mu.Lock()
	permission := s.permissions[ref]
	// Another explicit nil decision may have reserved this same reference after
	// Cancel took its snapshot. Its once-only reservation is sufficient; never
	// wait for or duplicate the other POST's acknowledgement.
	if cancelTurn != "" && !s.closed && s.active == cancelTurn && permission != nil && permission.turn == cancelTurn && permission.decided {
		s.mu.Unlock()
		return nil
	}
	if s.closed || permission == nil || permission.decided || permission.packet.Raw.source != s.conn || permission.turn != s.active {
		s.mu.Unlock()
		return failure("permission_conflict", 0, false)
	}
	outcome := acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{Outcome: "cancelled"}}
	if optionID != nil {
		found := false
		for _, option := range permission.packet.Permission.Options {
			if string(option.OptionId) == *optionID {
				found = true
			}
		}
		if !found || s.cancelling {
			s.mu.Unlock()
			return failure("permission_option", 0, false)
		}
		outcome = acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{Outcome: "selected", OptionId: acp.PermissionOptionId(*optionID)}}
	}
	permission.decided = true
	raw, err := json.Marshal(struct {
		JSONRPC string                        `json:"jsonrpc"`
		ID      json.RawMessage               `json:"id"`
		Result  acp.RequestPermissionResponse `json:"result"`
	}{"2.0", permission.packet.RPCID, acp.RequestPermissionResponse{Outcome: outcome}})
	s.mu.Unlock()
	if err != nil {
		return failure("invalid_request", 0, false)
	}
	err = s.conn.send(ctx, raw)
	if err != nil && !sessionUnknown(err) {
		s.mu.Lock()
		if permission.turn == s.active && !s.closed {
			permission.decided = false
		}
		s.mu.Unlock()
	}
	return err
}

// Cancel explicitly requests cancellation; acknowledgement still comes from
// the original prompt response. It never claims the remote process has stopped.
func (s *Session) Cancel(ctx context.Context) error {
	if s == nil || ctx == nil {
		return failure("invalid_request", 0, false)
	}
	if ctx.Err() != nil {
		return failure("cancelled", 0, false)
	}
	s.mu.Lock()
	if s.closed || s.active == "" || s.cancelInFlight || s.cancelSent {
		s.mu.Unlock()
		return failure("call_conflict", 0, false)
	}
	s.cancelling = true
	s.cancelInFlight = true
	turn := s.active
	refs := []string{}
	for ref, p := range s.permissions {
		if !p.decided && p.turn == turn {
			refs = append(refs, ref)
		}
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.active == turn {
			s.cancelInFlight = false
		}
		s.mu.Unlock()
	}()
	for _, ref := range refs {
		if err := s.replyPermission(ctx, ref, nil, turn); err != nil {
			return err
		}
	}
	// Re-check after awaiting permission POSTs: a real stop may have arrived.
	s.mu.Lock()
	current := !s.closed && s.active == turn
	if current {
		s.cancelSent = true
		s.cancelFence = "sending"
	}
	s.mu.Unlock()
	if !current {
		return failure("call_conflict", 0, false)
	}
	err := s.sendRequest(ctx, "", "session/cancel", acp.CancelNotification{SessionId: acp.SessionId(s.sessionID)})
	s.mu.Lock()
	if sessionUnknown(err) {
		s.cancelFence = "unknown"
	} else {
		s.cancelFence = ""
	}
	if err != nil && !sessionUnknown(err) {
		if s.active == turn {
			s.cancelSent = false
		}
	}
	s.mu.Unlock()
	// Keep the cancellation intent even on a known unsent step, so a resumed
	// explicit Cancel cannot race a newly approved permission. Sent/unknown
	// references remain reserved; only a definitely unsent notification retries.
	return err
}
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.closed = true
	s.buffered = nil
	s.permissions = make(map[string]*sessionPermission)
	s.mu.Unlock()
	return s.conn.Close()
}
