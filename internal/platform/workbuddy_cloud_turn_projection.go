package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/workbuddyacp"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

var errCloudTurnProjection = errors.New("workbuddy_cloud_turn_unavailable")

const cloudTurnHidden = "[REDACTED]"

// Only an undecided secret prefix remains in memory. A published prefix cannot
// later become a credential when a subsequent provider chunk is concatenated.
type cloudTurnTextFilter struct {
	secrets []string
	tail    string
}

func (*cloudTurnTextFilter) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "cloudTurnTextFilter{redacted}")
}

func newCloudTurnTextFilter(secrets []string) *cloudTurnTextFilter {
	set := map[string]bool{}
	for _, secret := range secrets {
		if secret == "" || !utf8.ValidString(secret) {
			continue
		}
		set[secret] = true
		set[url.QueryEscape(secret)] = true
		set[url.PathEscape(secret)] = true
		quoted, _ := json.Marshal(secret)
		if len(quoted) >= 2 {
			set[string(quoted[1:len(quoted)-1])] = true
		}
	}
	values := make([]string, 0, len(set))
	for secret := range set {
		if secret != "" {
			values = append(values, secret)
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return &cloudTurnTextFilter{secrets: values}
}

func (f *cloudTurnTextFilter) Push(text string) (string, bool) { return f.consume(text, false) }
func (f *cloudTurnTextFilter) Flush() (string, bool)           { return f.consume("", true) }
func (f *cloudTurnTextFilter) consume(text string, final bool) (string, bool) {
	input := f.tail + text
	f.tail = ""
	var output strings.Builder
	changed := false
	for len(input) > 0 {
		full, partial := "", false
		for _, secret := range f.secrets {
			if strings.HasPrefix(input, secret) && len(secret) > len(full) {
				full = secret
			}
			if len(input) < len(secret) && strings.HasPrefix(secret, input) {
				partial = true
			}
		}
		if partial && !final {
			f.tail = input
			break
		}
		if full != "" {
			output.WriteString(cloudTurnHidden)
			input = input[len(full):]
			changed = true
			continue
		}
		if partial && final {
			output.WriteString(cloudTurnHidden)
			changed = true
			break
		}
		_, width := utf8.DecodeRuneInString(input)
		output.WriteString(input[:width])
		input = input[width:]
	}
	return output.String(), changed
}

func cloudTurnSafeText(text string, secrets []string) (string, bool) {
	filter := newCloudTurnTextFilter(secrets)
	first, changed := filter.Push(text)
	last, tailChanged := filter.Flush()
	return first + last, changed || tailChanged
}

type cloudTurnToolLedger struct {
	states map[string]string
	bad    bool
}

func newCloudTurnToolLedger() *cloudTurnToolLedger {
	return &cloudTurnToolLedger{states: map[string]string{}}
}
func (l *cloudTurnToolLedger) Permission(id string) {
	if _, exists := l.states[id]; !exists {
		l.states[id] = "permission_pending"
	}
}
func (l *cloudTurnToolLedger) Observe(id, state string, start bool) string {
	before, exists := l.states[id]
	implicit := before == "permission_pending"
	if id == "" || start && exists && !implicit || !start && !exists {
		l.bad = true
	}
	if state == "" {
		if exists && !implicit {
			return before
		}
		state = "pending"
	}
	if state != "pending" && state != "in_progress" && state != "completed" && state != "failed" {
		l.bad = true
		state = "unknown"
	}
	if exists && (before == "completed" || before == "failed") && state != before {
		l.bad = true
	}
	l.states[id] = state
	return state
}
func (l *cloudTurnToolLedger) Complete() bool {
	if l.bad {
		return false
	}
	for _, state := range l.states {
		if state != "completed" && state != "failed" {
			return false
		}
	}
	return true
}

func cloudTurnProjectionSHA(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Marshal through JSON values to remove wire escaping before secret filtering.
// The complete operation input is retained only when every observed detail can
// be displayed safely. Raw output, rich content and locations never enter DB.
func cloudTurnProjectPermission(packet workbuddyacp.Packet, id, turn string, secrets []string) (wc.CloudTurnPermission, error) {
	var raw struct {
		Params struct {
			ToolCall map[string]json.RawMessage `json:"toolCall"`
		} `json:"params"`
	}
	if packet.Permission == nil || json.Unmarshal(packet.Raw.Data, &raw) != nil || raw.Params.ToolCall == nil {
		return wc.CloudTurnPermission{}, errCloudTurnProjection
	}
	call := raw.Params.ToolCall
	readString := func(key string) string { var value string; _ = json.Unmarshal(call[key], &value); return value }
	toolID, title := readString("toolCallId"), readString("title")
	if toolID == "" {
		return wc.CloudTurnPermission{}, errCloudTurnProjection
	}
	if _, changed := cloudTurnSafeText(toolID, secrets); changed {
		return wc.CloudTurnPermission{}, errCloudTurnProjection
	}
	allowed := true
	// Future request/option extensions may affect the operation. Preserve the
	// original options for an exact rejection, but never approve hidden scope.
	var envelope struct {
		Params map[string]json.RawMessage `json:"params"`
	}
	if json.Unmarshal(packet.Raw.Data, &envelope) != nil {
		return wc.CloudTurnPermission{}, errCloudTurnProjection
	}
	for key := range envelope.Params {
		if key != "sessionId" && key != "toolCall" && key != "options" {
			allowed = false
		}
	}
	var rawOptions []map[string]json.RawMessage
	if json.Unmarshal(envelope.Params["options"], &rawOptions) != nil {
		return wc.CloudTurnPermission{}, errCloudTurnProjection
	}
	for _, option := range rawOptions {
		for key := range option {
			if key != "optionId" && key != "name" && key != "kind" {
				allowed = false
			}
		}
	}
	if title == "" {
		title, allowed = "Permission request", false
	}
	filtered, originalFields := map[string]any{}, map[string]any{}
	for key, value := range call {
		decoder := json.NewDecoder(strings.NewReader(string(value)))
		decoder.UseNumber()
		var item any
		if decoder.Decode(&item) != nil {
			return wc.CloudTurnPermission{}, errCloudTurnProjection
		}
		originalFields[key] = item
		switch key {
		case "toolCallId", "title", "kind", "status", "rawInput":
			filtered[key] = item
		default:
			allowed = false
		}
	}
	if _, exists := call["rawInput"]; !exists || string(call["rawInput"]) == "null" {
		allowed = false
	}
	details, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return wc.CloudTurnPermission{}, errCloudTurnProjection
	}
	allDetails, _ := json.MarshalIndent(map[string]any{"toolCall": originalFields, "request": envelope.Params}, "", "  ")
	originalSHA := cloudTurnProjectionSHA(string(allDetails))
	display, changed := cloudTurnSafeText(string(details), secrets)
	if changed {
		allowed = false
	}
	title, changed = cloudTurnSafeText(title, secrets)
	if changed {
		allowed = false
	}
	if len(display) > wc.CloudTurnMaxEntryBytes {
		allowed = false
		display = display[:wc.CloudTurnMaxEntryBytes]
		for !utf8.ValidString(display) {
			display = display[:len(display)-1]
		}
	}
	options := make([]wc.CloudTurnPermissionOption, 0, len(packet.Permission.Options))
	for _, option := range packet.Permission.Options {
		optionID := string(option.OptionId)
		if _, unsafe := cloudTurnSafeText(optionID, secrets); unsafe {
			return wc.CloudTurnPermission{}, errCloudTurnProjection
		}
		name, unsafe := cloudTurnSafeText(option.Name, secrets)
		if unsafe {
			allowed = false
		}
		options = append(options, wc.CloudTurnPermissionOption{ID: optionID, Name: name, Kind: string(option.Kind)})
	}
	if allowed {
		originalSHA = cloudTurnProjectionSHA(display)
	}
	return wc.CloudTurnPermission{ID: id, TurnID: turn, Version: 1, ToolCallID: toolID, Title: title, Details: display, DetailsSHA256: originalSHA, ApproveAllowed: allowed, State: "pending", Options: options}, nil
}

type cloudTurnProjectedUpdate struct {
	Entries               []wc.CloudTurnEntry
	ToolID, ToolState     string
	ToolStart, Incomplete bool
}

// Artifact identity and download metadata remain private to the session codec.
// Only the current plan or overview text can become a bounded plain-text preview.
func cloudTurnArtifactPreview(notice *workbuddyacp.ArtifactNotice, secrets []string) string {
	if notice.Event == "deleted" || notice.Type != "plan" && notice.Type != "overview" {
		return ""
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(notice.Body, &fields) != nil {
		return ""
	}
	var text string
	if json.Unmarshal(fields["text"], &text) != nil || text == "" || !wc.CloudTurnArtifactPreviewValid(text) {
		return ""
	}
	private := append(append([]string{}, secrets...), notice.URI)
	var downloadURL string
	if json.Unmarshal(fields["url"], &downloadURL) == nil && downloadURL != "" && len(downloadURL) <= wc.CloudTurnArtifactPreviewMaxBytes {
		private = append(private, downloadURL)
	}
	safe, _ := cloudTurnSafeText(text, private)
	if !wc.CloudTurnArtifactPreviewValid(safe) {
		return ""
	}
	return safe
}

func cloudTurnProjectUpdate(packet workbuddyacp.Packet, filter *cloudTurnTextFilter, ledger *cloudTurnToolLedger, secrets []string) (cloudTurnProjectedUpdate, error) {
	var out cloudTurnProjectedUpdate
	if packet.Kind == "artifact" {
		if notice := packet.Artifact; notice != nil {
			out.Entries = []wc.CloudTurnEntry{{
				Kind:          "artifact",
				ArtifactType:  notice.Type,
				ArtifactEvent: notice.Event,
				Text:          cloudTurnArtifactPreview(notice, secrets),
			}}
		}
		return out, nil
	}
	if packet.Kind != "session_update" && packet.Kind != "unsupported" {
		return out, nil
	}
	var raw struct {
		Params struct {
			Update struct {
				Tag           string `json:"sessionUpdate"`
				ToolID        string `json:"toolCallId"`
				Title, Status string
				Content       struct{ Type, Text string }
				Entries       []struct{ Content, Status string }
				Plan          struct {
					Type    string
					Entries []struct{ Content, Status string }
				}
			} `json:"update"`
		} `json:"params"`
	}
	if json.Unmarshal(packet.Raw.Data, &raw) != nil {
		return out, errCloudTurnProjection
	}
	update := raw.Params.Update
	switch update.Tag {
	case "agent_thought_chunk", "user_message_chunk":
		return out, nil // No thoughts or replayed/user input in assistant output.
	case "agent_message_chunk":
		if packet.Kind == "session_update" && update.Content.Type == "text" {
			text, _ := filter.Push(update.Content.Text)
			if text != "" {
				out.Entries = []wc.CloudTurnEntry{{Kind: "assistant_text", Text: text}}
			}
			return out, nil
		}
	case "tool_call", "tool_call_update":
		if packet.Kind != "session_update" {
			out.Incomplete = true
			break
		}
		if update.ToolID == "" {
			return out, errCloudTurnProjection
		}
		if _, changed := cloudTurnSafeText(update.ToolID, secrets); changed {
			return out, errCloudTurnProjection
		}
		out.ToolID, out.ToolStart = update.ToolID, update.Tag == "tool_call"
		out.ToolState = ledger.Observe(update.ToolID, update.Status, out.ToolStart)
		text, _ := cloudTurnSafeText(update.Title, secrets)
		if out.ToolState == "unknown" {
			out.Entries = []wc.CloudTurnEntry{{Kind: "notice", Text: "tool_state_unconfirmed"}}
		} else {
			out.Entries = []wc.CloudTurnEntry{{Kind: "tool_state", Text: text, ToolCallID: out.ToolID, ToolState: out.ToolState}}
		}
		return out, nil
	case "plan", "plan_update":
		entries := update.Entries
		if update.Tag == "plan_update" {
			if update.Plan.Type != "items" {
				break
			}
			entries = update.Plan.Entries
		}
		var lines []string
		for _, entry := range entries {
			text, _ := cloudTurnSafeText(strings.TrimSpace(entry.Content+" "+entry.Status), secrets)
			lines = append(lines, text)
		}
		out.Entries = []wc.CloudTurnEntry{{Kind: "plan_summary", Text: strings.Join(lines, "\n")}}
		return out, nil
	case "usage_update", "available_commands_update", "current_mode_update", "config_option_update", "session_info_update", "plan_removed":
		return out, nil // Observations do not authorize mode/config changes.
	default:
		out.Incomplete = true
	}
	out.Entries = []wc.CloudTurnEntry{{Kind: "unsupported", Text: "unsupported_provider_observation"}}
	return out, nil
}
