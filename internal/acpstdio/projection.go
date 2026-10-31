package acpstdio

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	tc "github.com/ai-mada/suma/internal/toolcontract"

	sdk "github.com/coder/acp-go-sdk"
)

// Presence comes from the validated raw object, not SDK integer defaults.
// No absent token counter or provider cost is materialized as zero.
func projectUsage(raw []byte) (*tc.Usage, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	m := object(raw)
	if m == nil {
		return nil, errors.New("acp_usage_invalid")
	}
	u := &tc.Usage{}
	count := 0
	for _, f := range []struct {
		key    string
		target **int64
	}{{"inputTokens", &u.InputTokens}, {"outputTokens", &u.OutputTokens}, {"cachedReadTokens", &u.CachedInputTokens}, {"cachedWriteTokens", &u.CacheWriteInputTokens}, {"thoughtTokens", &u.ReasoningOutputTokens}} {
		if r, present := m[f.key]; present {
			var n int64
			if string(r) == "null" || json.Unmarshal(r, &n) != nil || n < 0 || n > tc.MaxSafeUsage {
				return nil, errors.New("acp_usage_invalid")
			}
			x := n
			*f.target = &x
			count++
		}
	}
	if count == 0 {
		return nil, nil
	}
	return u, nil
}

func text(s string, max int, multiline bool) bool {
	if !utf8.ValidString(s) || len(s) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
	}
	return true
}
func token(s string, max int) bool {
	return s != "" && strings.TrimSpace(s) == s && text(s, max, false)
}
func hasSecret(s string, secrets []string) bool { _, changed := safeText(s, secrets); return changed }

type textFilter struct {
	mu      sync.Mutex
	secrets []string
	tail    string
}

func newTextFilter(secrets []string) *textFilter {
	set := map[string]bool{}
	for _, s := range secrets {
		if s == "" {
			continue
		}
		set[s] = true
		set[url.QueryEscape(s)] = true
		set[url.PathEscape(s)] = true
		b, _ := json.Marshal(s)
		if len(b) >= 2 {
			set[string(b[1:len(b)-1])] = true
		}
	}
	values := []string{}
	for s := range set {
		if s != "" {
			values = append(values, s)
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return &textFilter{secrets: values}
}
func (f *textFilter) push(s string) (string, bool) { return f.consume(s, false) }
func (f *textFilter) flush() (string, bool)        { return f.consume("", true) }
func (f *textFilter) consume(s string, final bool) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s = f.tail + s
	f.tail = ""
	var out strings.Builder
	changed := false
	for len(s) > 0 {
		full := ""
		partial := false
		for _, secret := range f.secrets {
			if strings.HasPrefix(s, secret) && len(secret) > len(full) {
				full = secret
			}
			if len(s) < len(secret) && strings.HasPrefix(secret, s) {
				partial = true
			}
		}
		if partial && !final {
			f.tail = s
			break
		}
		if full != "" {
			out.WriteString("[REDACTED]")
			s = s[len(full):]
			changed = true
			continue
		}
		if partial && final {
			out.WriteString("[REDACTED]")
			changed = true
			break
		}
		_, size := utf8.DecodeRuneInString(s)
		out.WriteString(s[:size])
		s = s[size:]
	}
	return out.String(), changed
}
func safeText(s string, secrets []string) (string, bool) {
	f := newTextFilter(secrets)
	a, c := f.push(s)
	b, d := f.flush()
	return a + b, c || d
}
func operationID() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func (d *Driver) projectUpdate(params map[string]json.RawMessage) error {
	update := object(params["update"])
	tag := stringField(update, "sessionUpdate")
	session := stringField(params, "sessionId")
	if update == nil || tag == "" {
		return errors.New("acp_update_invalid")
	}
	d.mu.Lock()
	started := d.result.Evidence.PromptStarted
	modelID := d.modelConfig
	d.mu.Unlock()
	if tag == "config_option_update" {
		var options []sdk.SessionConfigOption
		if !closed(update, "_meta", "sessionUpdate", "configOptions") || !configOptionsValid(update["configOptions"]) || json.Unmarshal(update["configOptions"], &options) != nil {
			return errors.New("acp_update_invalid")
		}
		// Once the requested model option is fixed, later wire observations
		// must match it even while run has not yet recorded the SDK ACK.
		if modelID != "" {
			option, e := modelOption(options, d.options.Model)
			if e != nil || string(option.Id) != modelID || string(option.CurrentValue) != d.options.Model {
				return errors.New("acp_model_changed")
			}
		}
		return nil
	}
	if !started {
		return errors.New("acp_update_before_prompt")
	}
	switch tag {
	case "agent_thought_chunk", "user_message_chunk":
		return nil
	case "agent_message_chunk":
		content := object(update["content"])
		if stringField(content, "type") != "text" {
			d.mu.Lock()
			d.incomplete = true
			d.mu.Unlock()
			return d.emit(Event{Kind: "notice", SessionID: session, Text: "unsupported ACP content"})
		}
		var original string
		if json.Unmarshal(content["text"], &original) != nil || !text(original, frameLimit, true) {
			return errors.New("acp_update_invalid")
		}
		safe, _ := d.filter.push(original)
		if safe != "" {
			return d.emit(Event{Kind: "message", SessionID: session, Text: safe})
		}
		return nil
	case "tool_call", "tool_call_update":
		id := stringField(update, "toolCallId")
		if !token(id, 128) || hasSecret(id, d.options.Secrets) {
			return errors.New("acp_tool_invalid")
		}
		state := stringField(update, "status")
		title := stringField(update, "title")
		if !text(title, 4096, true) {
			return errors.New("acp_tool_invalid")
		}
		d.mu.Lock()
		before, exists := d.tools[id]
		if len(d.tools) >= 1000 && !exists {
			d.mu.Unlock()
			return errors.New("acp_tool_limited")
		}
		if state == "" {
			if exists {
				state = before
			} else {
				state = "pending"
			}
		}
		if state != "pending" && state != "in_progress" && state != "completed" && state != "failed" {
			state = "unknown"
			d.toolInvalid = true
		}
		if tag == "tool_call" && exists && before != "permission_pending" || tag == "tool_call_update" && !exists || exists && (before == "completed" || before == "failed") && state != before {
			d.toolInvalid = true
			state = "unknown"
		}
		d.tools[id] = state
		d.mu.Unlock()
		safe, _ := safeText(title, d.options.Secrets)
		return d.emit(Event{Kind: "tool", SessionID: session, ToolCallID: id, ToolState: state, Text: safe})
	case "plan":
		var entries []map[string]json.RawMessage
		if json.Unmarshal(update["entries"], &entries) != nil || len(entries) > 128 {
			return errors.New("acp_plan_invalid")
		}
		lines := []string{}
		for _, entry := range entries {
			line := stringField(entry, "content")
			if !text(line, 4096, true) {
				return errors.New("acp_plan_invalid")
			}
			safe, _ := safeText(line, d.options.Secrets)
			lines = append(lines, safe)
		}
		return d.emit(Event{Kind: "notice", SessionID: session, Text: strings.Join(lines, "\n")})
	case "usage_update":
		return nil // Context occupancy/cost is not exact turn token usage.
	default:
		d.mu.Lock()
		d.incomplete = true
		d.mu.Unlock()
		return d.emit(Event{Kind: "notice", SessionID: session, Text: "unsupported ACP observation"})
	}
}
