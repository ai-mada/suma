package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const toolLineLimit = 64 << 10

type toolStream struct {
	mu                sync.Mutex
	profile           tc.Profile
	secrets           []string
	pending           []byte
	emit              func(tc.Event) error
	abort             func()
	output            *output
	session, terminal string
	count, terminals  int
	usage             *tc.Usage
	bad               error
	expectedRequestID string
	sessionStarted    bool
	turnStarted       bool
	requestPending    bool
	requestStep       int64
	usageSteps        int
	usageMissing      bool
	usagePartial      bool
	usagePresence     [5]int
	failureClass      string
	lastUsageState    string
}

func newToolStream(p tc.Profile, secrets []string, emit func(tc.Event) error, expectedID ...string) *toolStream {
	stream := &toolStream{profile: p, secrets: secrets, emit: emit, output: newOutput(128<<10, secrets)}
	if len(expectedID) > 0 {
		stream.expectedRequestID = expectedID[0]
	}
	return stream
}
func (s *toolStream) invalid(reason string) {
	if s.bad == nil {
		s.bad = fmt.Errorf("structured tool output: %s", reason)
		if s.abort != nil {
			s.abort()
		}
	}
}
func (s *toolStream) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(data)
	if s.bad != nil {
		return n, nil
	}
	for len(data) > 0 {
		end := bytes.IndexByte(data, '\n')
		part := data
		if end >= 0 {
			part = data[:end]
		}
		if len(s.pending)+len(part) > toolLineLimit {
			s.invalid("line exceeds 64 KiB")
			return n, nil
		}
		s.pending = append(s.pending, part...)
		if end < 0 {
			break
		}
		s.line(bytes.TrimSuffix(s.pending, []byte{'\r'}))
		s.pending = nil
		data = data[end+1:]
		if s.bad != nil {
			break
		}
	}
	return n, nil
}

// Vendor objects may contain richer fields, but only the explicitly selected
// public facts below enter the platform. Duplicate keys and deep input fail closed.
func decodeToolObject(raw []byte) (map[string]any, error) {
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("invalid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	tokens := 0
	var value func(int) (any, error)
	value = func(depth int) (any, error) {
		if depth > 32 || tokens > 32768 {
			return nil, fmt.Errorf("JSON structure limit")
		}
		t, err := dec.Token()
		tokens++
		if err != nil {
			return nil, err
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				m := map[string]any{}
				for dec.More() {
					k, err := dec.Token()
					tokens++
					if err != nil {
						return nil, err
					}
					key, ok := k.(string)
					if !ok {
						return nil, fmt.Errorf("invalid object key")
					}
					if _, exists := m[key]; exists {
						return nil, fmt.Errorf("duplicate object key")
					}
					v, err := value(depth + 1)
					if err != nil {
						return nil, err
					}
					m[key] = v
				}
				_, err = dec.Token()
				return m, err
			case '[':
				a := []any{}
				for dec.More() {
					v, err := value(depth + 1)
					if err != nil {
						return nil, err
					}
					a = append(a, v)
				}
				_, err = dec.Token()
				return a, err
			}
			return nil, fmt.Errorf("invalid JSON delimiter")
		}
		return t, nil
	}
	v, err := value(0)
	if err != nil {
		return nil, err
	}
	if _, err = dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("multiple JSON values")
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected object")
	}
	return m, nil
}
func textAt(m map[string]any, key string) string           { v, _ := m[key].(string); return v }
func objectAt(m map[string]any, key string) map[string]any { v, _ := m[key].(map[string]any); return v }
func (s *toolStream) id(value string) string {
	for _, secret := range s.secrets {
		if secret != "" && strings.Contains(value, secret) {
			return "redacted"
		}
	}
	return value
}
func (s *toolStream) event(kind, item, message, status string, usage *tc.Usage, state ...string) {
	e := tc.Event{SchemaVersion: 1, CommandID: s.profile.CommandID, ProfileSHA256: s.profile.ProfileSHA256, Protocol: s.profile.Protocol, Kind: kind, SessionID: s.session, ItemID: s.id(item), Message: progressText(message, s.secrets), Status: status, Usage: usage}
	if len(state) > 0 {
		e.UsageState = state[0]
	} else if usage != nil {
		e.UsageState = tc.UsageComplete
	}
	if err := tc.ValidateEvent(e); err != nil {
		s.invalid("selected event fields are invalid")
		return
	}
	if s.count >= 10000 {
		s.invalid("event count exceeds 10000")
		return
	}
	s.count++
	if message != "" {
		_, _ = s.output.Write([]byte(e.Message + "\n"))
	}
	if s.emit != nil {
		if err := s.emit(e); err != nil {
			s.invalid("durable tool progress unavailable")
		}
	}
}
func (s *toolStream) setTerminal(value string) {
	s.terminals++
	if s.terminals > 1 {
		s.invalid("multiple or conflicting whole-run terminals")
		s.terminal = "unknown"
		return
	}
	s.terminal = value
}
func parseUsage(m map[string]any) (*tc.Usage, error) {
	if m == nil {
		return nil, nil
	}
	u := &tc.Usage{}
	hasCounters := false
	for name, destination := range map[string]**int64{"input_tokens": &u.InputTokens, "cached_input_tokens": &u.CachedInputTokens, "cache_write_input_tokens": &u.CacheWriteInputTokens, "output_tokens": &u.OutputTokens, "reasoning_output_tokens": &u.ReasoningOutputTokens} {
		if v, present := m[name]; present {
			n, ok := v.(json.Number)
			if !ok {
				return nil, fmt.Errorf("invalid usage")
			}
			i, err := n.Int64()
			if err != nil || i < 0 || i > tc.MaxSafeUsage {
				return nil, fmt.Errorf("invalid usage")
			}
			*destination = &i
			hasCounters = true
		}
	}
	if !hasCounters {
		return nil, nil
	}
	return u, nil
}

func usageCounts(u *tc.Usage) []*int64 {
	return []*int64{u.InputTokens, u.CachedInputTokens, u.CacheWriteInputTokens, u.OutputTokens, u.ReasoningOutputTokens}
}

func (s *toolStream) accumulateUsage(u *tc.Usage) error {
	if u == nil {
		s.usageMissing = true
		return nil
	}
	if s.usage == nil {
		s.usage = &tc.Usage{}
	}
	sources := usageCounts(u)
	targets := []**int64{&s.usage.InputTokens, &s.usage.CachedInputTokens, &s.usage.CacheWriteInputTokens, &s.usage.OutputTokens, &s.usage.ReasoningOutputTokens}
	// Validate the whole addition before changing any subtotal.
	for i, source := range sources {
		if source != nil && *targets[i] != nil && **targets[i] > tc.MaxSafeUsage-*source {
			return fmt.Errorf("aggregate token usage exceeds safe integer range")
		}
	}
	for i, source := range sources {
		if source != nil {
			if *targets[i] == nil {
				zero := int64(0)
				*targets[i] = &zero
			}
			**targets[i] += *source
			s.usagePresence[i]++
		}
	}
	return nil
}

func (s *toolStream) coverage() string {
	if s.usage == nil {
		return tc.UsageUnreported
	}
	if s.bad != nil || s.requestPending || s.usageMissing || s.usagePartial {
		return tc.UsagePartial
	}
	for _, count := range s.usagePresence {
		if count > 0 && count < s.usageSteps {
			return tc.UsagePartial
		}
	}
	return tc.UsageComplete
}

func nativeInteger(m map[string]any, key string) (int64, bool) {
	n, ok := m[key].(json.Number)
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	return i, err == nil && i >= 0 && i <= tc.MaxSafeUsage
}
func (s *toolStream) line(raw []byte) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	if s.terminals > 0 {
		s.invalid("events after whole-run terminal")
		return
	}
	m, err := decodeToolObject(raw)
	if err != nil {
		s.invalid("invalid or oversized JSON structure")
		return
	}
	switch s.profile.Protocol {
	case "codex-jsonl-v1":
		s.codex(m)
	case "suma-jsonl-v1":
		s.suma(m)
	default:
		s.invalid("unsupported structured protocol")
	}
}
func (s *toolStream) codex(m map[string]any) {
	switch textAt(m, "type") {
	case "thread.started":
		if s.sessionStarted || s.turnStarted {
			s.invalid("duplicate or out-of-order session start")
			return
		}
		id := s.id(textAt(m, "thread_id"))
		if id == "" || (s.session != "" && s.session != id) {
			s.invalid("missing or changed session identity")
			return
		}
		s.session = id
		s.sessionStarted = true
		s.event("session_started", "", "", "", nil)
	case "turn.started":
		if !s.sessionStarted || s.turnStarted {
			s.invalid("missing session or duplicate turn start")
			return
		}
		s.turnStarted = true
		s.event("turn_started", "", "", "", nil)
	case "turn.completed":
		if !s.sessionStarted || !s.turnStarted {
			s.invalid("whole-run completion before session and turn start")
			return
		}
		if _, ok := m["usage"].(map[string]any); !ok {
			s.invalid("whole-run usage must be an object")
			return
		}
		// Official older exec releases serialize these three counters. Newer
		// releases add split counters; their absence is not a failed completion.
		for _, name := range []string{"input_tokens", "cached_input_tokens", "output_tokens"} {
			if _, ok := objectAt(m, "usage")[name].(json.Number); !ok {
				s.invalid("required Codex usage counter is missing")
				return
			}
		}
		u, err := parseUsage(objectAt(m, "usage"))
		if err != nil {
			s.invalid("invalid token usage")
			return
		}
		s.usage = u
		s.setTerminal("completed")
		if s.bad == nil {
			s.event("turn_completed", "", "", "", u)
		}
	case "turn.failed":
		if !s.sessionStarted || !s.turnStarted {
			s.invalid("turn failure before session and turn start")
			return
		}
		s.setTerminal("failed")
		s.event("turn_failed", "", textAt(objectAt(m, "error"), "message"), "", nil)
	case "error":
		s.setTerminal("failed")
		s.event("error", "", textAt(m, "message"), "", nil)
	case "item.started", "item.updated", "item.completed":
		if !s.sessionStarted || !s.turnStarted {
			s.invalid("activity before session and turn start")
			return
		}
		item := objectAt(m, "item")
		kind := textAt(item, "type")
		if kind != "" && textAt(item, "id") == "" {
			s.invalid("missing item identity")
			return
		}
		phase := map[string]string{"item.started": "activity_started", "item.updated": "activity_updated", "item.completed": "activity_completed"}[textAt(m, "type")]
		switch kind {
		case "agent_message":
			if textAt(m, "type") == "item.completed" {
				s.event("message", textAt(item, "id"), textAt(item, "text"), "", nil)
			}
		case "command_execution", "file_change", "mcp_tool_call", "collab_tool_call", "web_search", "todo_list":
			s.event(phase, textAt(item, "id"), kind, textAt(item, "status"), nil)
		case "error":
			s.event("activity_completed", textAt(item, "id"), textAt(item, "message"), "error", nil)
			// Reasoning and unknown item variants are intentionally not retained.
		}
	}
}
func (s *toolStream) suma(m map[string]any) {
	id := textAt(m, "request_id")
	if s.expectedRequestID == "" || id == "" || id != s.expectedRequestID {
		s.invalid("missing or foreign request identity")
		return
	}
	s.session = s.id(id)
	kind := textAt(m, "type")
	if kind != "started" && kind != "error" && !s.sessionStarted {
		s.invalid("native activity or completion before start")
		return
	}
	switch kind {
	case "started":
		if s.sessionStarted {
			s.invalid("duplicate native session start")
			return
		}
		s.sessionStarted = true
		s.turnStarted = true
		s.event("session_started", "", "", "", nil)
		s.event("turn_started", "", "", "", nil)
	case "tool_started":
		if s.requestPending {
			s.invalid("native tool activity before model request usage settled")
			return
		}
		s.event("activity_started", textAt(m, "call_id"), "project_tool", "", nil)
	case "tool_completed":
		if s.requestPending {
			s.invalid("native tool activity before model request usage settled")
			return
		}
		s.event("activity_completed", textAt(m, "call_id"), "project_tool", "", nil)
	case "request_started":
		step, ok := nativeInteger(m, "step")
		allowance, valid := nativeInteger(m, "output_allowance")
		if !ok || !valid || step != s.requestStep || step >= 16 || allowance < 1 || allowance > 4096 || s.requestPending {
			s.invalid("invalid or out-of-order native model request")
			return
		}
		s.requestPending = true
		s.event("activity_started", "model-step-"+strconv.FormatInt(step, 10), "model_request", "model_request", nil)
	case "usage":
		step, ok := nativeInteger(m, "step")
		if !ok || !s.requestPending || step != s.requestStep {
			s.invalid("native usage does not match a pending model request")
			return
		}
		u, err := parseUsage(m)
		if err != nil {
			s.invalid("invalid token usage")
			return
		}
		state := textAt(m, "usage_state")
		reported, valid := m["usage_reported"].(bool)
		if !valid || reported != (state == tc.UsageComplete) || state == tc.UsageComplete && (u == nil || u.InputTokens == nil || u.OutputTokens == nil) || state == tc.UsagePartial && u == nil || state == tc.UsageUnreported && u != nil || state != tc.UsageComplete && state != tc.UsagePartial && state != tc.UsageUnreported {
			s.invalid("native usage coverage contradicts its source counters")
			return
		}
		if err = s.accumulateUsage(u); err != nil {
			s.invalid(err.Error())
			return
		}
		s.requestPending = false
		s.requestStep++
		s.usageSteps++
		s.usagePartial = s.usagePartial || state == tc.UsagePartial
		s.lastUsageState = state
		s.event("usage", "model-step-"+strconv.FormatInt(step, 10), "", "", u, state)
	case "completed":
		if strings.TrimSpace(textAt(m, "output")) == "" || s.requestPending {
			s.invalid("missing completed output")
			return
		}
		s.setTerminal("completed")
		s.event("message", "", textAt(m, "output"), "", nil)
		s.event("turn_completed", "", "", "", nil)
	case "error":
		s.setTerminal("failed")
		status, ok := nativeInteger(m, "http_status")
		if ok && status == 429 && textAt(m, "source") == "model" && textAt(m, "code") == "upstream_rejected" && textAt(m, "failure_class") == "transient" && s.lastUsageState == tc.UsageUnreported && s.usageSteps > 0 && !s.requestPending {
			s.failureClass = "transient"
		}
		s.event("error", "", textAt(m, "message"), textAt(m, "code"), nil)
	case "waiting_input":
		s.event("waiting_input", "", textAt(m, "message"), "", nil)
	}
}
func (s *toolStream) finish() (*tc.Evidence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bad == nil && len(s.pending) > 0 {
		s.line(s.pending)
		s.pending = nil
	}
	if s.bad == nil && s.terminals != 1 {
		s.invalid("missing whole-run terminal")
	}
	terminal := s.terminal
	if s.bad != nil || terminal == "" {
		terminal = "unknown"
	}
	e := &tc.Evidence{Profile: s.profile, SessionID: s.session, Terminal: terminal, EventCount: s.count, Usage: s.usage, UsageState: s.coverage()}
	return e, s.bad
}
