package toolcontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
)

var ErrInvalidDiagnostic = errors.New("tool diagnostic metadata invalid")

// canonicalDiagnostic checks the complete original source, not an English
// prefix or arbitrary vendor key. Rendered locale/prose never crosses this
// boundary. A copied parameter map prevents later mutation of accepted facts.
func canonicalDiagnostic(message string, d *ToolDiagnostic) *ToolDiagnostic {
	if d == nil || d.SchemaVersion != 1 || !localization.IsNativeMessageKey(d.MessageKey) || message == "" {
		return nil
	}
	proof := localization.AnnotateError(errors.New(message), d.MessageKey, localization.Params(d.MessageParams))
	rendered, ok := localization.FormatError(localization.EnUS, proof)
	if !ok {
		return nil
	}
	return &ToolDiagnostic{SchemaVersion: 1, MessageKey: d.MessageKey, MessageParams: rendered.Params}
}

// ProjectNativeDiagnostic selects only semantic metadata from a native error.
// Malformed presentation is dropped; it must not rewrite a process's terminal,
// usage or raw message facts. The caller enforces native protocol and event kind.
func ProjectNativeDiagnostic(message string, display map[string]any) *ToolDiagnostic {
	key, ok := display["message_key"].(string)
	if !ok {
		return nil
	}
	var params map[string]any
	if value, exists := display["message_params"]; exists {
		params, ok = value.(map[string]any)
		if !ok || params == nil {
			return nil
		}
	}
	return canonicalDiagnostic(message, &ToolDiagnostic{SchemaVersion: 1, MessageKey: key, MessageParams: params})
}

// SanitizeDiagnostic drops presentation if any parameter needs redaction, and
// binds it again to the caller's already-sanitized original message. No hidden
// copy of a credential may survive inside metadata, journals or durable events.
func SanitizeDiagnostic(message string, d *ToolDiagnostic, redact func(string) string) *ToolDiagnostic {
	copy := canonicalDiagnostic(message, d)
	if copy == nil {
		return nil
	}
	if redact != nil {
		if redact(copy.MessageKey) != copy.MessageKey {
			return nil
		}
		for _, v := range copy.MessageParams {
			if text, ok := v.(string); ok && redact(text) != text {
				return nil
			}
		}
	}
	return copy
}

func validateEventDiagnostic(e Event) error {
	if e.Diagnostic != nil && (e.Protocol != "suma-jsonl-v1" || e.Kind != "error" || canonicalDiagnostic(e.Message, e.Diagnostic) == nil) {
		return ErrInvalidDiagnostic
	}
	return nil
}

// Strict decoding preserves integer lexemes and rejects duplicate/unknown
// fields before treating externally supplied display metadata as a typed fact.
func (d *ToolDiagnostic) UnmarshalJSON(raw []byte) error {
	if len(raw) > 4096 || !ValidJSONUnicode(raw) || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ErrInvalidDiagnostic
	}
	scan := json.NewDecoder(bytes.NewReader(raw))
	scan.UseNumber()
	if err := scanValue(scan, 0); err != nil {
		return ErrInvalidDiagnostic
	}
	if _, err := scan.Token(); err != io.EOF {
		return ErrInvalidDiagnostic
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return ErrInvalidDiagnostic
	}
	for key := range fields {
		if key != "schema_version" && key != "message_key" && key != "message_params" {
			return ErrInvalidDiagnostic
		}
	}
	if params, present := fields["message_params"]; present && bytes.Equal(bytes.TrimSpace(params), []byte("null")) {
		return ErrInvalidDiagnostic
	}
	type wire ToolDiagnostic
	var value wire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.SchemaVersion != 1 {
		return ErrInvalidDiagnostic
	}
	*d = ToolDiagnostic(value)
	return nil
}

// UnmarshalJSON keeps formal Event fields case-sensitive and retains selected
// raw text long enough to reject a lossy Unicode replacement. In particular an
// optional diagnostic:null remains equivalent to absent presentation metadata.
func (e *ToolEvent) UnmarshalJSON(raw []byte) error {
	if len(raw) > 16384 || !utf8.Valid(raw) {
		return fmt.Errorf("invalid event encoding/size")
	}
	scan := json.NewDecoder(bytes.NewReader(raw))
	scan.UseNumber()
	if err := scanValue(scan, 0); err != nil {
		return err
	}
	if _, err := scan.Token(); err != io.EOF {
		return fmt.Errorf("trailing tool event content")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return fmt.Errorf("tool event must be an object")
	}
	for key := range fields {
		switch key {
		case "schema_version", "command_id", "profile_sha256", "protocol", "kind", "session_id", "item_id", "message", "status", "usage", "usage_state", "diagnostic":
		default:
			return fmt.Errorf("unknown tool event field")
		}
	}
	if message, present := fields["message"]; present && !ValidJSONUnicode(message) {
		if diagnostic, present := fields["diagnostic"]; present && !bytes.Equal(bytes.TrimSpace(diagnostic), []byte("null")) {
			return ErrInvalidDiagnostic
		}
		return fmt.Errorf("invalid tool event text encoding")
	}
	type wire ToolEvent
	var value wire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*e = ToolEvent(value)
	return nil
}
