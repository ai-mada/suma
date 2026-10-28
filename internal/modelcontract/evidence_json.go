package modelcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"
)

// Evidence is an authorization/accounting boundary. Do not normalize omitted,
// duplicate, null or unknown fields into facts which the host never recorded.
func decodeEvidenceObject(raw []byte, target any, required, optional []string) error {
	if len(raw) > 256*1024 || !validEvidenceUnicode(raw) {
		return fmt.Errorf("model gateway evidence exceeds its bound or has invalid Unicode")
	}
	scan := json.NewDecoder(bytes.NewReader(raw))
	scan.UseNumber()
	if err := scanEvidenceJSON(scan, 0); err != nil {
		return err
	}
	if _, err := scan.Token(); err != io.EOF {
		return fmt.Errorf("invalid trailing model gateway JSON")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return fmt.Errorf("model gateway object required")
	}
	allowed := map[string]bool{}
	for _, name := range required {
		allowed[name] = true
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("required model gateway field missing")
		}
	}
	for _, name := range optional {
		allowed[name] = true
	}
	for name := range fields {
		if !allowed[name] {
			return fmt.Errorf("unknown model gateway field")
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(target)
}

func validEvidenceUnicode(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	quoted := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		v, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if v >= 0xdc00 && v <= 0xdfff {
			return false
		}
		if v >= 0xd800 && v <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

func scanEvidenceJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("model gateway JSON nesting exceeds bound")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("model gateway facts cannot be null")
	}
	if text, ok := t.(string); ok && !boundedText(text, 256*1024) {
		return fmt.Errorf("invalid model gateway JSON text")
	}
	delim, container := t.(json.Delim)
	if !container {
		return nil
	}
	if delim == '{' {
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate model gateway field")
			}
			seen[name] = true
			if err := scanEvidenceJSON(d, depth+1); err != nil {
				return err
			}
		}
	} else if delim == '[' {
		for d.More() {
			if err := scanEvidenceJSON(d, depth+1); err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("invalid model gateway JSON container")
	}
	end, err := d.Token()
	if err != nil {
		return err
	}
	if delim == '{' && end != json.Delim('}') || delim == '[' && end != json.Delim(']') {
		return fmt.Errorf("invalid model gateway JSON end")
	}
	return nil
}

func (u *ModelRequestUsage) UnmarshalJSON(raw []byte) error {
	type wire ModelRequestUsage
	var value wire
	if err := decodeEvidenceObject(raw, &value, nil, []string{"input_tokens", "output_tokens", "cached_input_tokens", "cache_write_input_tokens", "reasoning_output_tokens"}); err != nil {
		return err
	}
	if err := validateRequestUsage("partial", (*RequestUsage)(&value)); err != nil {
		return err
	}
	*u = ModelRequestUsage(value)
	return nil
}

func (r *ModelRequestEvidence) UnmarshalJSON(raw []byte) error {
	type wire ModelRequestEvidence
	var value wire
	if err := decodeEvidenceObject(raw, &value, []string{"state", "step", "request_sha256", "output_allowance", "usage_state"}, []string{"response_sha256", "provider_model", "provider_request_id", "usage"}); err != nil {
		return err
	}
	*r = ModelRequestEvidence(value)
	return nil
}

func (e *ModelEvidence) UnmarshalJSON(raw []byte) error {
	type wire ModelEvidence
	var value wire
	if err := decodeEvidenceObject(raw, &value, []string{"gateway_protocol", "gateway_closed", "binding_sha256", "deployment_sha256", "parameters_sha256", "model", "requests"}, nil); err != nil {
		return err
	}
	if err := ValidateEvidence(Evidence(value)); err != nil {
		return err
	}
	*e = ModelEvidence(value)
	return nil
}
