package displaycontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/ai-mada/suma/internal/toolcontract"
)

var errInvalidDiagnostic = errors.New("display diagnostic metadata invalid")

// UnmarshalJSON enforces a closed, case-sensitive envelope before source
// validation. In particular duplicate fields and lossy Unicode must not acquire
// a trusted identity by passing through encoding/json's permissive defaults.
func (d *DisplayDiagnostic) UnmarshalJSON(raw []byte) error {
	if len(raw) > 4096 || !toolcontract.ValidJSONUnicode(raw) {
		return errInvalidDiagnostic
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errInvalidDiagnostic
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || fields[key] != nil || (key != "schema_version" && key != "message_key" && key != "message_params") {
			return errInvalidDiagnostic
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return errInvalidDiagnostic
		}
		fields[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return errInvalidDiagnostic
	}
	if _, err = decoder.Token(); err != io.EOF {
		return errInvalidDiagnostic
	}
	var out DisplayDiagnostic
	if json.Unmarshal(fields["schema_version"], &out.SchemaVersion) != nil || out.SchemaVersion != 1 || json.Unmarshal(fields["message_key"], &out.MessageKey) != nil {
		return errInvalidDiagnostic
	}
	if params, exists := fields["message_params"]; exists {
		p := json.NewDecoder(bytes.NewReader(params))
		p.UseNumber()
		if token, err = p.Token(); err != nil || token != json.Delim('{') {
			return errInvalidDiagnostic
		}
		out.MessageParams = map[string]any{}
		for p.More() {
			token, err = p.Token()
			key, ok := token.(string)
			if err != nil || !ok {
				return errInvalidDiagnostic
			}
			if _, exists := out.MessageParams[key]; exists {
				return errInvalidDiagnostic
			}
			var value any
			if p.Decode(&value) != nil {
				return errInvalidDiagnostic
			}
			switch value.(type) {
			case string, json.Number:
			default:
				return errInvalidDiagnostic
			}
			out.MessageParams[key] = value
		}
		if token, err = p.Token(); err != nil || token != json.Delim('}') {
			return errInvalidDiagnostic
		}
	}
	*d = out
	return nil
}
