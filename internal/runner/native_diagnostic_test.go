package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The bridge must retain only proven semantic identity. Copying the rendered
// display or omitting the diagnostic makes these consumer-visible checks fail.
func TestNativeDiagnosticFragmentedProjectionKeepsRawFacts(t *testing.T) {
	var events []tc.Event
	s := newToolStream(structuredProfile("suma-jsonl-v1"), []string{"private-secret"}, func(e tc.Event) error {
		events = append(events, e)
		return nil
	}, "request-1")
	body := []byte(`{"type":"error","request_id":"request-1","code":"missing_model","message":"Configure SUMA_MODEL and SUMA_MODEL_BASE_URL to run an agent","display":{"message_key":"native.config.missing_model","locale":"untrusted-private-secret","localized_message":"伪造 private-secret","unknown":{"token":"private-secret"}}}` + "\n")
	for _, b := range body {
		if _, err := s.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	evidence, err := s.finish()
	if err != nil || evidence.Terminal != "failed" || evidence.SessionID != "request-1" || evidence.EventCount != 1 || evidence.UsageState != tc.UsageUnreported {
		t.Fatalf("display changed terminal facts: %+v %v", evidence, err)
	}
	if len(events) != 1 || events[0].Diagnostic == nil {
		t.Fatalf("native diagnostic lost: %+v", events)
	}
	e := events[0]
	if e.Message != "Configure SUMA_MODEL and SUMA_MODEL_BASE_URL to run an agent" || e.Kind != "error" || e.Status != "missing_model" || e.Diagnostic.SchemaVersion != 1 || e.Diagnostic.MessageKey != "native.config.missing_model" {
		t.Fatalf("raw or semantic facts changed: %+v", e)
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "private-secret") || strings.Contains(string(raw), "localized_message") || strings.Contains(string(raw), "locale") || s.output.String() != e.Message+"\n" {
		t.Fatalf("untrusted presentation retained or raw output changed: %s", raw)
	}
}

func TestNativeDiagnosticIntegerDoesNotChangeTransientRejection(t *testing.T) {
	var events []tc.Event
	s := newToolStream(structuredProfile("suma-jsonl-v1"), nil, func(e tc.Event) error { events = append(events, e); return nil }, "request-429")
	body := `{"type":"started","request_id":"request-429"}
{"type":"request_started","request_id":"request-429","step":0,"output_allowance":100}
{"type":"usage","request_id":"request-429","step":0,"usage_reported":false,"usage_state":"unreported"}
{"type":"error","request_id":"request-429","source":"model","code":"upstream_rejected","failure_class":"transient","http_status":429,"message":"Endpoint rejected request (HTTP 429)","display":{"message_key":"native.endpoint.rejected","message_params":{"status":429},"locale":"en-US","localized_message":"forged"}}
`
	_, _ = s.Write([]byte(body))
	evidence, err := s.finish()
	if err != nil || evidence.Terminal != "failed" || evidence.EventCount != 5 || evidence.Usage != nil || evidence.UsageState != tc.UsageUnreported || s.failureClass != "transient" || s.requestStep != 1 || s.requestPending {
		t.Fatalf("rejection semantics changed: %+v %+v %v", evidence, s, err)
	}
	last := events[len(events)-1]
	if last.Diagnostic == nil || last.Message != "Endpoint rejected request (HTTP 429)" || last.Status != "upstream_rejected" {
		t.Fatalf("native HTTP diagnostic lost: %+v", last)
	}
	raw, _ := json.Marshal(last.Diagnostic)
	if string(raw) != `{"schema_version":1,"message_key":"native.endpoint.rejected","message_params":{"status":429}}` {
		t.Fatalf("integer parameter was guessed or display persisted: %s", raw)
	}
}

func TestNativeDiagnosticInvalidPresentationKeepsErrorTerminal(t *testing.T) {
	for name, input := range map[string]struct{ message, display string }{
		"forged native key":  {"Model returned no text", `{"message_key":"native.future.forbidden"}`},
		"foreign key":        {"Model returned no text", `{"message_key":"common.invalid_input"}`},
		"wrong source":       {"foreign provider text", `{"message_key":"native.response.no_text"}`},
		"source suffix":      {"Model returned no text plus hidden suffix", `{"message_key":"native.response.no_text"}`},
		"no source":          {"", `{"message_key":"native.response.no_text"}`},
		"unknown param":      {"Model returned no text", `{"message_key":"native.response.no_text","message_params":{"token":"do-not-retain"}}`},
		"wrong integer type": {"Endpoint rejected request (HTTP 429)", `{"message_key":"native.endpoint.rejected","message_params":{"status":"429"}}`},
		"decimal integer":    {"Endpoint rejected request (HTTP 429)", `{"message_key":"native.endpoint.rejected","message_params":{"status":429.0}}`},
		"control param":      {"CLI probe exited bad\nvalue", `{"message_key":"native.catalog.probe_exited","message_params":{"status":"bad\nvalue"}}`},
		"oversize param":     {"CLI probe exited " + strings.Repeat("x", 513), `{"message_key":"native.catalog.probe_exited","message_params":{"status":"` + strings.Repeat("x", 513) + `"}}`},
		"array display":      {"Model returned no text", `[]`},
		"null params":        {"Model returned no text", `{"message_key":"native.response.no_text","message_params":null}`},
	} {
		t.Run(name, func(t *testing.T) {
			var events []tc.Event
			s := newToolStream(structuredProfile("suma-jsonl-v1"), nil, func(e tc.Event) error { events = append(events, e); return nil }, "request")
			message, _ := json.Marshal(input.message)
			_, _ = s.Write([]byte(`{"type":"error","request_id":"request","code":"original_error","message":` + string(message) + `,"display":` + input.display + "}\n"))
			evidence, err := s.finish()
			if err != nil || evidence.Terminal != "failed" || len(events) != 1 || events[0].Diagnostic != nil || events[0].Message != input.message || events[0].Status != "original_error" {
				t.Fatalf("invalid display rewrote original facts: %+v %+v %v", events, evidence, err)
			}
		})
	}
}

func TestNativeDiagnosticNeverInferredFromOtherOutput(t *testing.T) {
	for name, input := range map[string]struct{ protocol, body string }{
		"codex error": {"codex-jsonl-v1", `{"type":"error","message":"Model returned no text","display":{"message_key":"native.response.no_text"}}`},
		"native completed output": {"suma-jsonl-v1", `{"type":"started","request_id":"request"}
{"type":"completed","request_id":"request","output":"Model returned no text","display":{"message_key":"native.response.no_text"}}`},
		"native waiting input": {"suma-jsonl-v1", `{"type":"started","request_id":"request"}
{"type":"waiting_input","request_id":"request","message":"Model returned no text","display":{"message_key":"native.response.no_text"}}
{"type":"error","request_id":"request","message":"foreign original error"}`},
	} {
		t.Run(name, func(t *testing.T) {
			var events []tc.Event
			s := newToolStream(structuredProfile(input.protocol), nil, func(e tc.Event) error { events = append(events, e); return nil }, "request")
			_, _ = s.Write([]byte(input.body + "\n"))
			if _, err := s.finish(); err != nil {
				t.Fatal(err)
			}
			for _, e := range events {
				if e.Diagnostic != nil {
					t.Fatalf("non-native-error inferred display: %+v", e)
				}
			}
		})
	}
}

func TestNativeDiagnosticRedactionBeforeJournalAndNetwork(t *testing.T) {
	for name, secret := range map[string]string{"parameter": "private-secret", "fixed key": "native.response.no_text"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			j, err := OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			a := Assignment{RunID: "r", NodeID: "n", WorkerID: "w", Attempt: 1, Fence: 1}
			if err = j.Accept(a); err != nil {
				t.Fatal(err)
			}
			s := newToolStream(structuredProfile("suma-jsonl-v1"), []string{secret}, func(e tc.Event) error { return j.RecordToolProgress(a, e) }, "request")
			body := `{"type":"error","request_id":"request","code":"original","message":"CLI probe exited private-secret","display":{"message_key":"native.catalog.probe_exited","message_params":{"status":"private-secret"}}}`
			if name == "fixed key" {
				body = `{"type":"error","request_id":"request","code":"original","message":"Model returned no text","display":{"message_key":"native.response.no_text"}}`
			}
			_, _ = s.Write([]byte(body + "\n"))
			if e, err := s.finish(); err != nil || e.Terminal != "failed" {
				t.Fatalf("sanitization changed failure: %+v %v", e, err)
			}
			pending := j.PendingProgress()
			if len(pending) != 1 || pending[0].ToolEvent.Diagnostic != nil {
				t.Fatalf("sensitive display retained: %+v", pending)
			}
			journal, err := os.ReadFile(filepath.Join(dir, "journal.json"))
			if err != nil || strings.Contains(string(journal), secret) {
				t.Fatalf("journal leaked metadata: %s %v", journal, err)
			}
			var received []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received, _ = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			client := Client{BaseURL: server.URL, ProjectID: "p", Token: "runner-test-token"}
			if err = client.Progress(context.Background(), pending[0]); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(received), secret) || strings.Contains(string(received), "diagnostic") {
				t.Fatalf("network leaked metadata: %s", received)
			}
		})
	}
}

func TestNativeDiagnosticJournalSnapshotsAndRestart(t *testing.T) {
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	a := Assignment{RunID: "r", NodeID: "n", WorkerID: "w", Attempt: 1, Fence: 1}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	event := tc.Event{SchemaVersion: 1, CommandID: "agent", ProfileSHA256: strings.Repeat("a", 64), Protocol: "suma-jsonl-v1", Kind: "error", Message: "CLI probe exited 23", Status: "original", Diagnostic: &tc.ToolDiagnostic{SchemaVersion: 1, MessageKey: "native.catalog.probe_exited", MessageParams: map[string]any{"status": "23"}}}
	if err = j.RecordToolProgress(a, event); err != nil {
		t.Fatal(err)
	}
	event.Diagnostic.MessageParams["status"] = "caller-mutated"
	pending := j.PendingProgress()
	if len(pending) != 1 || pending[0].ToolEvent.Diagnostic.MessageParams["status"] != "23" {
		t.Fatalf("journal aliased caller metadata: %+v", pending)
	}
	operation := pending[0].OperationID
	pending[0].ToolEvent.Diagnostic.MessageParams["status"] = "reader-mutated"
	pending[0].ToolEvent.Diagnostic.MessageKey = "native.future.forbidden"
	if got := j.PendingProgress()[0].ToolEvent.Diagnostic; got.MessageKey != "native.catalog.probe_exited" || got.MessageParams["status"] != "23" {
		t.Fatalf("pending reader mutated durable facts: %+v", got)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	pending = j.PendingProgress()
	if len(pending) != 1 || pending[0].OperationID != operation || pending[0].ToolEvent.Diagnostic.MessageKey != "native.catalog.probe_exited" || pending[0].ToolEvent.Diagnostic.MessageParams["status"] != "23" || pending[0].ToolEvent.Message != "CLI probe exited 23" {
		t.Fatalf("restart changed diagnostic or receipt identity: %+v", pending)
	}
}

func TestNativeDiagnosticDuplicateJSONFailsClosed(t *testing.T) {
	s := newToolStream(structuredProfile("suma-jsonl-v1"), nil, nil, "request")
	_, _ = s.Write([]byte(`{"type":"error","request_id":"request","message":"Model returned no text","display":{"message_key":"native.response.no_text","message_key":"native.response.no_text"}}` + "\n"))
	e, err := s.finish()
	if err == nil || e.Terminal != "unknown" || e.EventCount != 0 {
		t.Fatalf("duplicate object changed protocol guard: %+v %v", e, err)
	}
}

func TestNativeDiagnosticRequiresOriginalSourceBeforeCleanup(t *testing.T) {
	for name, fields := range map[string]string{
		"ANSI":               `"message":"Model returned no text\u001b[31m","display":{"message_key":"native.response.no_text"}`,
		"control":            `"message":"Model returned no text\u0000","display":{"message_key":"native.response.no_text"}`,
		"credential forgery": `"message":"CLI probe exited private-secret","display":{"message_key":"native.catalog.probe_exited","message_params":{"status":"[REDACTED]"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var events []tc.Event
			s := newToolStream(structuredProfile("suma-jsonl-v1"), []string{"private-secret"}, func(e tc.Event) error { events = append(events, e); return nil }, "request")
			_, _ = s.Write([]byte(`{"type":"error","request_id":"request","code":"original",` + fields + "}\n"))
			proof, err := s.finish()
			if err != nil || proof.Terminal != "failed" || proof.EventCount != 1 || len(events) != 1 || events[0].Status != "original" {
				t.Fatalf("cleanup changed execution facts: %+v %+v %v", events, proof, err)
			}
			if events[0].Diagnostic != nil {
				t.Fatalf("cleanup created unproven source identity: %+v", events[0])
			}
		})
	}
}

func TestNativeDiagnosticSelectedUnicodeDoesNotCertifyReplacement(t *testing.T) {
	for name, input := range map[string]struct {
		fields     string
		diagnostic bool
	}{
		"raw malformed message":      {`"message":"CLI probe exited \ud800","display":{"message_key":"native.catalog.probe_exited","message_params":{"status":"\ud800"}}`, false},
		"malformed parameter":        {`"message":"CLI probe exited �","display":{"message_key":"native.catalog.probe_exited","message_params":{"status":"\ud800"}}`, false},
		"malformed selected code":    {`"code":"bad\ud800","message":"Model returned no text","display":{"message_key":"native.response.no_text"}`, false},
		"ignored localized prose":    {`"message":"Model returned no text","display":{"message_key":"native.response.no_text","localized_message":"\ud800"}`, true},
		"ignored vendor rich fields": {`"message":"Model returned no text","vendor":{"rich":"\ud800"},"display":{"message_key":"native.response.no_text"}`, true},
		"literal replacement":        {`"message":"CLI probe exited �","display":{"message_key":"native.catalog.probe_exited","message_params":{"status":"�"}}`, true},
		"escaped pair":               {`"message":"CLI probe exited \ud83d\ude00","display":{"message_key":"native.catalog.probe_exited","message_params":{"status":"\ud83d\ude00"}}`, true},
		"literal escape sequence":    {`"message":"CLI probe exited \\ud800","display":{"message_key":"native.catalog.probe_exited","message_params":{"status":"\\ud800"}}`, true},
	} {
		t.Run(name, func(t *testing.T) {
			var events []tc.Event
			s := newToolStream(structuredProfile("suma-jsonl-v1"), nil, func(e tc.Event) error { events = append(events, e); return nil }, "request")
			for _, b := range []byte(`{"type":"error","request_id":"request",` + input.fields + "}\n") {
				_, _ = s.Write([]byte{b})
			}
			proof, err := s.finish()
			if err != nil || proof.Terminal != "failed" || proof.EventCount != 1 || len(events) != 1 {
				t.Fatalf("Unicode presentation changed execution facts: %+v %+v %v", events, proof, err)
			}
			if (events[0].Diagnostic != nil) != input.diagnostic {
				t.Fatalf("selected Unicode provenance changed: %+v", events[0])
			}
		})
	}
}
