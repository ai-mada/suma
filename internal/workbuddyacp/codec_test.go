package workbuddyacp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const codecSession = "session-123"

func codecFrame(data string) Frame { return Frame{Data: []byte(data), Event: "message", ID: "sse-7"} }
func codecUpdate(update string) string {
	return `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"` + codecSession + `","update":` + update + `}}`
}
func codecPermission(options string) string {
	return `{"jsonrpc":"2.0","id":9007199254740993,"method":"session/request_permission","params":{"sessionId":"` + codecSession + `","toolCall":{"toolCallId":"tool-1","title":"Run checks"},"options":` + options + `}}`
}

func TestCodecTypedUpdatesAndExactRPCIdentity(t *testing.T) {
	packet, err := DecodeFrame(codecFrame(codecUpdate(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"working"}}`)), codecSession)
	if err != nil || packet.Kind != "session_update" || packet.Update == nil || packet.Update.Update.AgentMessageChunk == nil || packet.SessionID != codecSession {
		t.Fatalf("typed update missing: %v", err)
	}
	for _, id := range []string{`9007199254740993`, `"request-1"`, `-1`} {
		packet, err = DecodeFrame(codecFrame(`{"jsonrpc":"2.0","id":`+id+`,"result":{"stopReason":"end_turn","usage":{"inputTokens":9007199254740993}}}`), codecSession)
		if err != nil || packet.Kind != "response" || string(packet.RPCID) != id || !strings.Contains(string(packet.Result), "9007199254740993") || packet.Update != nil || packet.Permission != nil {
			t.Fatalf("response identity or unclassified result lost: %v", err)
		}
	}
	packet, err = DecodeFrame(codecFrame(`{"jsonrpc":"2.0","id":7,"error":{"code":-32602,"message":"private-token https://private.example","data":{"secret":"private-token"}}}`), codecSession)
	if err != nil || packet.ErrorCode == nil || *packet.ErrorCode != -32602 || len(packet.Result) != 0 || packet.Kind != "response" {
		t.Fatalf("RPC error not retained safely: %v", err)
	}
}

func TestCodecPermissionBoundariesAndNoDecision(t *testing.T) {
	valid := `[{"optionId":"once","name":"Allow once","kind":"allow_once"},{"optionId":"deny","name":"Reject","kind":"reject_once"}]`
	packet, err := DecodeFrame(codecFrame(codecPermission(valid)), codecSession)
	if err != nil || packet.Kind != "permission_request" || packet.Permission == nil || len(packet.Permission.Options) != 2 || string(packet.RPCID) != "9007199254740993" || len(packet.Result) != 0 {
		t.Fatalf("permission fact changed: %v", err)
	}
	for i, options := range []string{`[]`, `null`, `[{"optionId":"same","name":"One","kind":"allow_once"},{"optionId":"same","name":"Two","kind":"reject_once"}]`, `[{"optionId":"x","name":"One","kind":"vendor_auto_approve"}]`, `[{"optionId":"","name":"One","kind":"allow_once"}]`, `[{"optionId":"x","kind":"allow_once"}]`, `[{"optionId":"` + strings.Repeat("x", 129) + `","name":"One","kind":"allow_once"}]`} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, err := DecodeFrame(codecFrame(codecPermission(options)), codecSession); err == nil {
				t.Fatal("invalid permission options accepted")
			}
		})
	}
	options := make([]string, 33)
	for i := range options {
		options[i] = fmt.Sprintf(`{"optionId":"%d","name":"Option","kind":"allow_once"}`, i)
	}
	if _, err := DecodeFrame(codecFrame(codecPermission("["+strings.Join(options, ",")+"]")), codecSession); err == nil {
		t.Fatal("unbounded permission options accepted")
	}
}

func TestCodecUnknownEventsStayUnsupportedAndRawIsOwned(t *testing.T) {
	for _, raw := range []string{codecUpdate(`{"sessionUpdate":"future_unknown","content":{"type":"text","text":"do not guess user message"}}`), `{"jsonrpc":"2.0","method":"_vendor.ai/new-event","params":{"sessionId":"` + codecSession + `","secret":"private-token"}}`, codecUpdate(`{"sessionUpdate":"agent_message_chunk","content":{"type":"future_media","text":"do not guess text"}}`)} {
		frame := codecFrame(raw)
		packet, err := DecodeFrame(frame, codecSession)
		if err != nil || packet.Kind != "unsupported" || packet.Update != nil || packet.Permission != nil || string(packet.Raw.Data) != raw {
			t.Fatalf("unknown event guessed: %v", err)
		}
		frame.Data[0] = '!'
		if string(packet.Raw.Data) != raw {
			t.Fatal("packet aliases reusable receive buffer")
		}
	}
}

func TestCodecRejectsCrossSessionBeforeSDKOrUnknownHandling(t *testing.T) {
	for _, raw := range []string{codecUpdate(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hello"}}`), codecUpdate(`{"sessionUpdate":"future_unknown"}`), codecPermission(`[{"optionId":"once","name":"Allow","kind":"allow_once"}]`), `{"jsonrpc":"2.0","method":"_codebuddy.ai/artifact","params":{"sessionId":"` + codecSession + `","event":"deleted","artifact":{"type":"plan","uri":"agent:///artifacts/plan.md"}}}`, `{"jsonrpc":"2.0","method":"_unknown","params":{"sessionId":"` + codecSession + `"}}`} {
		if _, err := DecodeFrame(codecFrame(strings.ReplaceAll(raw, codecSession, "other-session")), codecSession); err == nil {
			t.Fatal("cross-session frame accepted")
		}
	}
}

func TestCodecArtifactChangesAreNotCompletionOrDownloads(t *testing.T) {
	for _, event := range []string{"created", "updated", "deleted"} {
		raw := `{"jsonrpc":"2.0","method":"_codebuddy.ai/artifact","params":{"sessionId":"` + codecSession + `","event":"` + event + `","artifact":{"type":"plan","uri":"agent:///artifacts/plan.md","text":"private-token","version":2}}}`
		packet, err := DecodeFrame(codecFrame(raw), codecSession)
		if err != nil || packet.Kind != "artifact" || packet.Artifact == nil || packet.Artifact.Event != event || packet.Artifact.URI != "agent:///artifacts/plan.md" || packet.Update != nil {
			t.Fatalf("artifact fact lost: %v", err)
		}
	}
	if _, err := DecodeFrame(codecFrame(`{"jsonrpc":"2.0","method":"_codebuddy.ai/artifact","params":{"sessionId":"`+codecSession+`","event":"deleted","artifact":{"type":"plan"}}}`), codecSession); err == nil {
		t.Fatal("artifact without stable identity accepted")
	}
}

func TestCodecStrictEnvelopeDuplicatesLimitsAndSafeErrors(t *testing.T) {
	valid := codecUpdate(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"private-token"}}`)
	invalid := []string{valid + `{}`, strings.Replace(valid, `"jsonrpc":"2.0"`, `"jsonrpc":"1.0"`, 1), strings.Replace(valid, `"jsonrpc":"2.0"`, `"jsonrpc":"2.0","jsonrpc":"2.0"`, 1), strings.Replace(valid, `"text":"private-token"`, `"text":"one","text":"private-token"`, 1), strings.Replace(valid, `"text":"private-token"`, `"text":"\ud800"`, 1), strings.Replace(valid, "private-token", string([]byte{0xff}), 1), `[]`, `null`, `{"jsonrpc":"2.0","id":null,"result":{}}`, `{"jsonrpc":"2.0","id":1.5,"result":{}}`, `{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":1,"message":"bad"}}`, `{"jsonrpc":"2.0","id":1,"error":{"code":2147483648,"message":"private-token"}}`, `{"jsonrpc":"2.0","id":1,"method":"session/update","params":{}}`, `{"jsonrpc":"2.0","method":"session/request_permission","params":{}}`, strings.Repeat(" ", 1<<20) + valid, `{"jsonrpc":"2.0","id":1,"result":` + strings.Repeat("[", 70) + `0` + strings.Repeat("]", 70) + `}`}
	for i, raw := range invalid {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			packet, err := DecodeFrame(codecFrame(raw), codecSession)
			if err == nil {
				t.Fatal("invalid wire frame accepted")
			}
			if len(packet.Raw.Data) != 0 || packet.Kind != "" {
				t.Fatal("failed parse returned partial facts")
			}
			for _, message := range []string{err.Error(), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)} {
				if strings.Contains(message, "private-token") || strings.Contains(message, "https://") {
					t.Fatal("error leaked source body")
				}
			}
		})
	}
}

func TestCodecPacketJSONAndFormattingNeverExposeBody(t *testing.T) {
	packet, err := DecodeFrame(codecFrame(codecUpdate(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"private-token https://private.example"}}`)), codecSession)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{string(raw), fmt.Sprintf("%v", packet), fmt.Sprintf("%+v", packet), fmt.Sprintf("%#v", &packet), fmt.Sprintf("%q", packet)} {
		if strings.Contains(text, "private-token") || strings.Contains(text, "https://private.example") || strings.Contains(text, "sse-7") {
			t.Fatal("packet exposed internal payload")
		}
	}
}

func TestCodecAllPinnedSDKUpdateDiscriminators(t *testing.T) {
	updates := map[string]string{
		"user_message_chunk":        `"content":{"type":"text","text":"question"}`,
		"agent_message_chunk":       `"content":{"type":"text","text":"answer"}`,
		"agent_thought_chunk":       `"content":{"type":"text","text":"reasoning observation"}`,
		"tool_call":                 `"toolCallId":"tool-1","title":"Check","status":"pending"`,
		"tool_call_update":          `"toolCallId":"tool-1","status":"completed"`,
		"plan":                      `"entries":[{"content":"Check","priority":"high","status":"pending"}]`,
		"plan_update":               `"plan":{"type":"markdown","id":"plan-1","content":"# Plan"}`,
		"plan_removed":              `"id":"plan-1"`,
		"available_commands_update": `"availableCommands":[{"name":"review","description":"Review changes"}]`,
		"current_mode_update":       `"currentModeId":"code"`,
		"config_option_update":      `"configOptions":[{"id":"flag","name":"Flag","type":"boolean","currentValue":false},{"id":"mode","name":"Mode","type":"select","currentValue":"code","options":[{"name":"Code","value":"code"}]}]`,
		"session_info_update":       `"title":null,"updatedAt":"2026-10-30T00:00:00Z"`,
		"usage_update":              `"size":1024,"used":8,"cost":{"amount":0.01,"currency":"USD"}`,
	}
	for tag, fields := range updates {
		t.Run(tag, func(t *testing.T) {
			raw := " \n" + codecUpdate(`{"sessionUpdate":"`+tag+`",`+fields+`}`) + " \n"
			packet, err := DecodeFrame(codecFrame(raw), codecSession)
			if err != nil || packet.Kind != "session_update" || packet.Update == nil {
				t.Fatalf("known SDK discriminator not decoded: %v", err)
			}
			encoded, err := json.Marshal(packet.Update.Update)
			if err != nil {
				t.Fatal(err)
			}
			var actual map[string]json.RawMessage
			_ = json.Unmarshal(encoded, &actual)
			if string(actual["sessionUpdate"]) != `"`+tag+`"` {
				t.Fatal("SDK union selected a different discriminator")
			}
		})
	}
}

func TestCodecMalformedKnownUpdateCannotUseSDKShapeFallback(t *testing.T) {
	for i, update := range []string{
		`{"sessionUpdate":"tool_call","content":{"type":"text","text":"not a tool call"}}`,
		`{"sessionUpdate":"agent_message_chunk","content":{"type":"text"}}`,
		`{"sessionUpdate":"plan","entries":null}`,
		`{"sessionUpdate":"usage_update","size":9223372036854775808,"used":0}`,
		`{"sessionUpdate":"usage_update","size":10,"used":0,"cost":{"amount":-1,"currency":"USD"}}`,
		`{"sessionUpdate":"config_option_update","configOptions":[{"id":"x","name":"X","type":"select","currentValue":"x","options":[{"bogus":true}]}]}`,
		`{"sessionUpdate":"config_option_update","configOptions":[{"id":"x","name":"X","type":"boolean","currentValue":null}]}`,
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, err := DecodeFrame(codecFrame(codecUpdate(update)), codecSession); err == nil {
				t.Fatal("malformed known payload accepted")
			}
		})
	}
}

func TestCodecPreservesActualReceiveConnectionForPermission(t *testing.T) {
	wire := strings.ReplaceAll(codecPermission(`[{"optionId":"once","name":"Allow once","kind":"allow_once"}]`), codecSession, fixtureAccess().SessionID)
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		streamHeaders(w)
		_, _ = io.WriteString(w, "data: "+wire+"\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	connection := mustConnect(t, client)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	frame, err := connection.Receive(ctx)
	if err != nil || frame.source != connection {
		t.Fatalf("actual receive did not establish source: %v", err)
	}
	packet, err := DecodeFrame(frame, fixtureAccess().SessionID)
	if err != nil || packet.Kind != "permission_request" || packet.Raw.source != connection {
		t.Fatalf("permission decode lost actual source: %v", err)
	}
	frame.Data[0] = '!'
	if string(packet.Raw.Data) != wire || packet.Raw.source != connection {
		t.Fatal("owned original bytes and connection must survive receive buffer reuse")
	}
}
