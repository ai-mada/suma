package modelgateway

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Scan before decoding: encoding/json otherwise silently accepts duplicate
// members, invalid Unicode and aliases of a numeric counter.
func object(raw []byte) (map[string]any, error) {
	if len(raw) > maxResponseBytes || !tc.ValidJSONUnicode(raw) {
		return nil, errors.New("model_gateway_json")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var scan func(int) error
	scan = func(depth int) error {
		if depth > 64 {
			return errors.New("model_gateway_json")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, nested := token.(json.Delim)
		if !nested {
			return nil
		}
		seen := map[string]bool{}
		for d.More() {
			if delim == '{' {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] {
					return errors.New("model_gateway_json")
				}
				seen[name] = true
			}
			if err := scan(depth + 1); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	}
	if scan(0) != nil {
		return nil, errors.New("model_gateway_json")
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("model_gateway_json")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var result map[string]any
	if d.Decode(&result) != nil || result == nil {
		return nil, errors.New("model_gateway_json")
	}
	return result, nil
}

func integer(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	return i, err == nil && i >= 0 && i <= mc.MaxSafeInteger
}

func prepareRequest(raw []byte, binding mc.Binding, remaining int64) ([]byte, bool, int64, error) {
	value, err := object(raw)
	if err != nil || value["model"] != binding.Deployment.Model {
		return nil, false, 0, errors.New("model_gateway_request")
	}
	allowed := map[string]bool{"model": true, "messages": true, "tools": true, "tool_choice": true, "max_tokens": true, "max_completion_tokens": true, "stream": true, "stream_options": true, "parallel_tool_calls": true, "response_format": true}
	for key, item := range value {
		if !allowed[key] || item == nil {
			return nil, false, 0, errors.New("model_gateway_request")
		}
	}
	messages, ok := value["messages"].([]any)
	stream, streamOK := value["stream"].(bool)
	if !ok || len(messages) == 0 || len(messages) > 2048 || !streamOK || value["parallel_tool_calls"] != nil && value["parallel_tool_calls"] != false {
		return nil, false, 0, errors.New("model_gateway_request")
	}
	if choice := value["tool_choice"]; choice != nil && choice != "auto" && choice != "none" {
		return nil, false, 0, errors.New("model_gateway_request")
	}
	if options := value["stream_options"]; options != nil {
		m, ok := options.(map[string]any)
		if !ok || !stream || len(m) != 1 || m["include_usage"] != true {
			return nil, false, 0, errors.New("model_gateway_request")
		}
	}
	if stream {
		value["stream_options"] = map[string]bool{"include_usage": true}
	}
	key := "max_tokens"
	if _, exists := value["max_completion_tokens"]; exists {
		key = "max_completion_tokens"
		if _, duplicate := value["max_tokens"]; duplicate {
			return nil, false, 0, errors.New("model_gateway_request")
		}
	}
	allowance, ok := integer(value[key])
	if !ok || allowance < 1 || allowance > binding.Parameters.MaxOutputTokens || remaining < 1 {
		return nil, false, 0, errors.New("model_gateway_allowance")
	}
	if allowance > remaining {
		allowance = remaining
	}
	if allowance > binding.Deployment.ContextWindow {
		allowance = binding.Deployment.ContextWindow
	}
	value[key] = allowance
	encoded, err := json.Marshal(value)
	return encoded, stream, allowance, err
}

type responseFacts struct {
	model, id, finish string
	usage             *mc.RequestUsage
	suffix            []byte
	choice            bool
}

func (r responseFacts) coverage() string {
	if r.usage == nil {
		return "unreported"
	}
	if r.usage.InputTokens != nil && r.usage.OutputTokens != nil {
		return "complete"
	}
	return "partial"
}

func parseResponse(raw []byte, streaming bool) (responseFacts, error) {
	var facts responseFacts
	value, err := object(raw)
	if err != nil || value["error"] != nil {
		return facts, errors.New("model_gateway_response")
	}
	id, _ := value["id"].(string)
	model, _ := value["model"].(string)
	safe := func(v string) bool {
		return v != "" && len(v) <= 256 && utf8.ValidString(v) && strings.IndexFunc(v, unicode.IsControl) < 0
	}
	if !safe(id) || !safe(model) {
		return facts, errors.New("model_gateway_response")
	}
	facts.id, facts.model = id, model
	choices, ok := value["choices"].([]any)
	if !ok || len(choices) > 1 || !streaming && len(choices) != 1 {
		return facts, errors.New("model_gateway_response")
	}
	if len(choices) == 1 {
		facts.choice = true
		choice, ok := choices[0].(map[string]any)
		index, valid := integer(choice["index"])
		if !ok || !valid || index != 0 {
			return facts, errors.New("model_gateway_response")
		}
		if reason := choice["finish_reason"]; reason != nil {
			facts.finish, ok = reason.(string)
			if !ok || !map[string]bool{"stop": true, "tool_calls": true, "length": true, "content_filter": true}[facts.finish] {
				return facts, errors.New("model_gateway_response")
			}
		}
	}
	if usage := value["usage"]; usage != nil {
		m, ok := usage.(map[string]any)
		if !ok {
			return facts, errors.New("model_gateway_usage")
		}
		u := &mc.RequestUsage{}
		facts.usage = u
		for name, target := range map[string]**int64{"prompt_tokens": &u.InputTokens, "completion_tokens": &u.OutputTokens} {
			if v, present := m[name]; present {
				n, valid := integer(v)
				if !valid {
					return facts, errors.New("model_gateway_usage")
				}
				*target = &n
			}
		}
		if total, present := m["total_tokens"]; present {
			n, valid := integer(total)
			if !valid || u.InputTokens == nil || u.OutputTokens == nil || *u.InputTokens > mc.MaxSafeInteger-*u.OutputTokens || n != *u.InputTokens+*u.OutputTokens {
				return facts, errors.New("model_gateway_usage")
			}
		}
		for name, field := range map[string]string{"prompt_tokens_details": "cached_tokens", "completion_tokens_details": "reasoning_tokens"} {
			if detail := m[name]; detail != nil {
				d, valid := detail.(map[string]any)
				if !valid {
					return facts, errors.New("model_gateway_usage")
				}
				if item, present := d[field]; present {
					n, valid := integer(item)
					parent, target := u.InputTokens, &u.CachedInputTokens
					if name == "completion_tokens_details" {
						parent, target = u.OutputTokens, &u.ReasoningOutputTokens
					}
					if !valid || parent == nil || n > *parent {
						return facts, errors.New("model_gateway_usage")
					}
					*target = &n
				}
			}
		}
		if u.InputTokens == nil && u.OutputTokens == nil {
			facts.usage = nil
		}
	}
	return facts, nil
}

// Preserve token streaming. A terminal/usage suffix is released only after the
// gateway has persisted the authoritative response. SDKs cannot finish a turn
// solely because an uncommitted provider terminal happened to arrive first.
func readStream(reader io.Reader, w http.ResponseWriter, expectedModel string) ([]byte, responseFacts, error) {
	var facts responseFacts
	var whole, event, suffix bytes.Buffer
	scanner := bufio.NewScanner(io.LimitReader(reader, maxResponseBytes+1))
	scanner.Buffer(make([]byte, 8192), 2<<20)
	held, done := false, false
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	flush := func() error {
		raw := append([]byte{}, event.Bytes()...)
		event.Reset()
		var data []string
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
		text := strings.Join(data, "\n")
		if text == "[DONE]" {
			done, held = true, true
		} else if text != "" {
			part, err := parseResponse([]byte(text), true)
			if err != nil || part.model != expectedModel || facts.id != "" && (part.id != facts.id || part.model != facts.model) || facts.finish != "" && part.choice || part.usage != nil && (part.choice || facts.finish == "") || facts.usage != nil && part.usage != nil {
				return errors.New("model_gateway_stream")
			}
			facts.id, facts.model = part.id, part.model
			if part.finish != "" {
				facts.finish, held = part.finish, true
			}
			if part.usage != nil {
				facts.usage, held = part.usage, true
			}
		}
		if held {
			_, _ = suffix.Write(raw)
		} else {
			_, _ = w.Write(raw)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		// HTTP text-event-stream defines CRLF and LF as equivalent delimiters.
		line = strings.TrimSuffix(line, "\r")
		whole.WriteString(line + "\n")
		event.WriteString(line + "\n")
		if whole.Len() > maxResponseBytes {
			return whole.Bytes(), facts, errors.New("model_gateway_stream_limit")
		}
		if line == "" {
			if err := flush(); err != nil {
				return whole.Bytes(), facts, err
			}
			if done {
				facts.suffix = suffix.Bytes()
				return whole.Bytes(), facts, nil
			}
		}
	}
	return whole.Bytes(), facts, errors.New("model_gateway_stream_incomplete")
}
