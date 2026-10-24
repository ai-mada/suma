package toolcontract

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNativeDiagnosticProjectionRejectsUnprovenMetadata(t *testing.T) {
	if got := ProjectNativeDiagnostic("foreign raw message", map[string]any{"message_key": "common.invalid_input"}); got != nil {
		t.Fatal("foreign key accepted")
	}
	if got := ProjectNativeDiagnostic("foreign raw message", map[string]any{"message_key": "native.config.missing_model"}); got != nil {
		t.Fatal("wrong source accepted")
	}
	if got := SanitizeDiagnostic("foreign raw message", &ToolDiagnostic{SchemaVersion: 1, MessageKey: "native.config.missing_model"}, func(s string) string { return s }); got != nil {
		t.Fatal("unproven metadata survived")
	}
}

func TestNativeDiagnosticRequiresNativeErrorEvent(t *testing.T) {
	d := ProjectNativeDiagnostic("Configure SUMA_MODEL and SUMA_MODEL_BASE_URL to run an agent", map[string]any{"message_key": "native.config.missing_model"})
	if d == nil {
		t.Fatal("known native message lost")
	}
	e := Event{SchemaVersion: 1, CommandID: "native", ProfileSHA256: strings.Repeat("a", 64), Protocol: "codex-jsonl-v1", Kind: "error", Message: "Configure SUMA_MODEL and SUMA_MODEL_BASE_URL to run an agent", Diagnostic: d}
	if ValidateEvent(e) == nil {
		t.Fatal("foreign protocol accepted native metadata")
	}
	e.Protocol = "suma-jsonl-v1"
	e.Kind = "message"
	if ValidateEvent(e) == nil {
		t.Fatal("model text accepted native metadata")
	}
	e.Kind = "error"
	if err := ValidateEvent(e); err != nil {
		t.Fatal(err)
	}
}

func TestNativeDiagnosticTypedParamsAndUntrustedProse(t *testing.T) {
	const raw = "Endpoint rejected request (HTTP 429)"
	display := map[string]any{"message_key": "native.endpoint.rejected", "message_params": map[string]any{"status": json.Number("429")}, "locale": "foreign", "localized_message": "private data must never persist"}
	d := ProjectNativeDiagnostic(raw, display)
	if d == nil || d.MessageParams["status"] != int64(429) {
		t.Fatal("typed integer lost", d)
	}
	display["message_params"].(map[string]any)["status"] = 500
	if d.MessageParams["status"] != int64(429) {
		t.Fatal("caller aliases parameter snapshot")
	}
	encoded, _ := json.Marshal(d)
	if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "locale") {
		t.Fatal("untrusted rendered prose retained")
	}
	for _, params := range []any{nil, []any{}, map[string]any{"status": 429.0}, map[string]any{"status": json.Number("429.0")}, map[string]any{"status": json.Number("4.29e2")}, map[string]any{"status": 429, "extra": "secret"}} {
		if ProjectNativeDiagnostic(raw, map[string]any{"message_key": "native.endpoint.rejected", "message_params": params}) != nil {
			t.Fatal("invalid params accepted", params)
		}
	}
	if SanitizeDiagnostic(raw, d, func(s string) string { return strings.ReplaceAll(s, "native.endpoint", "redacted") }) != nil {
		t.Fatal("known secret retained in metadata key")
	}
	copy := SanitizeDiagnostic(raw, d, func(s string) string { return s })
	copy.MessageParams["status"] = 1
	if d.MessageParams["status"] != int64(429) {
		t.Fatal("sanitized snapshot aliases source")
	}
}

func TestNativeDiagnosticWireRejectsUnknownDuplicateAndInvalidParams(t *testing.T) {
	base := `{"schema_version":1,"command_id":"native","profile_sha256":"` + strings.Repeat("a", 64) + `","protocol":"suma-jsonl-v1","kind":"error","message":"Endpoint rejected request (HTTP 429)","diagnostic":%s}`
	valid := `{"schema_version":1,"message_key":"native.endpoint.rejected","message_params":{"status":429}}`
	var nullParams ToolDiagnostic
	if err := json.Unmarshal([]byte(`{"schema_version":1,"message_key":"native.config.missing_model","message_params":null}`), &nullParams); err == nil {
		t.Fatal("explicit null parameters accepted")
	}
	e, err := DecodeEvent([]byte(fmt.Sprintf(base, valid)))
	if err != nil || e.Diagnostic == nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(valid, `"schema_version":1`, `"schema_version":2`, 1),
		strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1),
		strings.Replace(valid, `"status":429`, `"status":429,"status":429`, 1),
		strings.Replace(valid, `"status":429`, `"status":429.0`, 1),
		strings.Replace(valid, `"status":429`, `"status":null`, 1),
		strings.Replace(valid, `"message_key"`, `"locale":"zh-CN","message_key"`, 1),
	} {
		if _, err := DecodeEvent([]byte(fmt.Sprintf(base, bad))); err == nil {
			t.Fatal("unbounded diagnostic accepted", bad)
		}
	}
	e.Protocol = "codex-jsonl-v1"
	if !errors.Is(ValidateEvent(e), ErrInvalidDiagnostic) {
		t.Fatal("wrong diagnostic category")
	}
}

func TestDiagnosticWireRejectsCaseAliasesAndLossyUnicode(t *testing.T) {
	base := `{"schema_version":1,"command_id":"native","profile_sha256":"` + strings.Repeat("a", 64) + `","protocol":"suma-jsonl-v1","kind":"error","message":%s,"diagnostic":%s}`
	for name, diagnostic := range map[string]string{
		"params alias null":  `{"schema_version":1,"message_key":"native.response.no_text","MESSAGE_PARAMS":null}`,
		"key alias":          `{"schema_version":1,"MESSAGE_KEY":"native.response.no_text"}`,
		"version alias":      `{"SCHEMA_VERSION":1,"message_key":"native.response.no_text"}`,
		"semantic duplicate": `{"schema_version":1,"SCHEMA_VERSION":1,"message_key":"native.response.no_text"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeEvent([]byte(fmt.Sprintf(base, `"Model returned no text"`, diagnostic))); !errors.Is(err, ErrInvalidDiagnostic) {
				t.Fatalf("case alias bypassed diagnostic schema: %v", err)
			}
		})
	}
	for name, fields := range map[string]string{
		"event version alias":      `"SCHEMA_VERSION":1`,
		"event semantic duplicate": `"schema_version":1,"SCHEMA_VERSION":1`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := strings.Replace(fmt.Sprintf(base, `"Model returned no text"`, `null`), `"schema_version":1`, fields, 1)
			if _, err := DecodeEvent([]byte(raw)); err == nil {
				t.Fatal("formal event accepted noncanonical property names")
			}
		})
	}
	for name, text := range map[string]string{"high alone": `\ud800`, "low alone": `\udc00`, "wrong pair": `\ud800\u0041`, "high high": `\ud800\udbff`} {
		t.Run(name, func(t *testing.T) {
			message := `"CLI probe exited ` + text + `"`
			diagnostic := `{"schema_version":1,"message_key":"native.catalog.probe_exited","message_params":{"status":"` + text + `"}}`
			if _, err := DecodeEvent([]byte(fmt.Sprintf(base, message, diagnostic))); !errors.Is(err, ErrInvalidDiagnostic) {
				t.Fatalf("lossy selected Unicode accepted: %v", err)
			}
		})
	}
}

func TestDiagnosticWirePreservesValidUnicodeEscapesAndNullAbsence(t *testing.T) {
	base := `{"schema_version":1,"command_id":"native","profile_sha256":"` + strings.Repeat("a", 64) + `","protocol":"suma-jsonl-v1","kind":"error","message":%s,"diagnostic":%s}`
	for name, text := range map[string]string{"literal replacement": "�", "escaped replacement": `\ufffd`, "escaped pair": `\ud83d\ude00`, "literal emoji": "😀", "literal escape sequence": `\\ud800`, "slash": `\/`, "backslash": `\\`} {
		t.Run(name, func(t *testing.T) {
			message := `"CLI probe exited ` + text + `"`
			diagnostic := `{"schema_version":1,"message_key":"native.catalog.probe_exited","message_params":{"status":"` + text + `"}}`
			event, err := DecodeEvent([]byte(fmt.Sprintf(base, message, diagnostic)))
			if err != nil || event.Diagnostic == nil {
				t.Fatalf("valid selected Unicode rejected: %+v %v", event, err)
			}
		})
	}
	event, err := DecodeEvent([]byte(fmt.Sprintf(base, `"Model returned no text"`, `null`)))
	if err != nil || event.Diagnostic != nil {
		t.Fatalf("optional null diagnostic changed: %+v %v", event, err)
	}
}
