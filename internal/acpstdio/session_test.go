package acpstdio

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
)

type fixtureWriter func([]byte) (int, error)

func (f fixtureWriter) Write(b []byte) (int, error) { return f(b) }
func modelOptions(model string) []any {
	return []any{map[string]any{"id": "model", "name": "Model", "category": "model", "type": "select", "currentValue": model, "options": []any{map[string]any{"value": "performance", "name": "Performance"}, map[string]any{"value": "lite", "name": "Lite"}}}}
}

type fixtureState struct {
	mu       sync.Mutex
	methods  []string
	promptID json.RawMessage
	option   string
}

func runFixture(t *testing.T, options Options, reply func(*Driver, map[string]json.RawMessage, *fixtureState)) (Result, *fixtureState) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if options.Cwd == "" {
		options.Cwd = t.TempDir()
	}
	if options.Goal == "" {
		options.Goal = "Develop this exact task"
	}
	if options.Model == "" {
		options.Model = "performance"
	}
	if options.ConfigSHA == "" {
		options.ConfigSHA = strings.Repeat("a", 64)
	}
	d, e := New(ctx, options)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	messages := make(chan []byte, 64)
	copyDone := make(chan struct{})
	state := &fixtureState{}
	go func() {
		defer close(copyDone)
		_, _ = d.CopyInput(fixtureWriter(func(raw []byte) (int, error) {
			select {
			case messages <- append([]byte{}, raw...):
				return len(raw), nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}))
	}()
	fixtureDone := make(chan struct{})
	go func() {
		defer close(fixtureDone)
		for {
			select {
			case raw := <-messages:
				var packet map[string]json.RawMessage
				if json.Unmarshal(raw, &packet) != nil {
					t.Error("bad SDK output")
					return
				}
				reply(d, packet, state)
			case <-copyDone:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	select {
	case <-copyDone:
	case <-ctx.Done():
		t.Fatal("owned SDK fixture did not close")
	}
	<-fixtureDone
	return d.Finish(), state
}
func sendResult(d *Driver, id json.RawMessage, result any) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	_, _ = d.Write(append(b, '\n'))
}
func basicReply(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
	var method string
	_ = json.Unmarshal(p["method"], &method)
	s.mu.Lock()
	s.methods = append(s.methods, method)
	s.mu.Unlock()
	switch method {
	case "initialize":
		sendResult(d, p["id"], map[string]any{"protocolVersion": 1, "agentInfo": map[string]any{"name": "owned protocol fixture", "version": "1"}, "authMethods": []any{}, "agentCapabilities": map[string]any{}})
	case "session/new":
		sendResult(d, p["id"], map[string]any{"sessionId": "fixture-session", "configOptions": modelOptions("lite")})
	case "session/set_config_option":
		sendResult(d, p["id"], map[string]any{"configOptions": modelOptions("performance")})
	case "session/prompt":
		sendResult(d, p["id"], map[string]any{"stopReason": "end_turn"})
	}
}
func TestSDKHandshakeModelACKAndOnePrompt(t *testing.T) {
	r, s := runFixture(t, Options{}, basicReply)
	if r.Terminal != "completed" || !r.Evidence.ModelSelectionAcknowledged || !r.Evidence.PromptCompleted || r.UsageState != "unreported" || r.Usage != nil {
		t.Fatal("accurate SDK completion missing", r.Terminal, r.Error)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.Join(s.methods, ",") != "initialize,session/new,session/set_config_option,session/prompt" {
		t.Fatal("unexpected or repeated calls", s.methods)
	}
	if r.Evidence.HostProcessStopped || r.Evidence.HostProcessTreeStopped {
		t.Fatal("SDK fabricated host stop")
	}
}

var _ io.Writer = (*Driver)(nil)

func update(d *Driver, session, tag string, fields map[string]any) {
	fields["sessionUpdate"] = tag
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": session, "update": fields}})
	_, _ = d.Write(append(raw, '\n'))
}
func methodOf(p map[string]json.RawMessage) string {
	var s string
	_ = json.Unmarshal(p["method"], &s)
	return s
}
func TestModelSelectionMustBeExplicitAndExactlyAcknowledged(t *testing.T) {
	for _, mode := range []string{"missing", "ambiguous", "wrong_ack", "legacy_models"} {
		t.Run(mode, func(t *testing.T) {
			r, s := runFixture(t, Options{}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
				m := methodOf(p)
				if m == "session/new" && (mode == "missing" || mode == "legacy_models") {
					sendResult(d, p["id"], map[string]any{"sessionId": "fixture-session", "models": map[string]any{"currentModelId": "performance"}})
					return
				}
				if m == "session/new" && mode == "ambiguous" {
					o := modelOptions("lite")
					sendResult(d, p["id"], map[string]any{"sessionId": "fixture-session", "configOptions": append(o, o...)})
					return
				}
				if m == "session/set_config_option" && mode == "wrong_ack" {
					sendResult(d, p["id"], map[string]any{"configOptions": modelOptions("lite")})
					return
				}
				basicReply(d, p, s)
			})
			if r.Terminal != "unknown" || r.Evidence.PromptStarted {
				t.Fatal("unacknowledged model reached Prompt")
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			for _, m := range s.methods {
				if m == "session/prompt" {
					t.Fatal("model fallback started Prompt")
				}
			}
		})
	}
	t.Run("nested_config_aliases", func(t *testing.T) {
		base := `[{"id":"model","name":"Model","category":"model","type":"select","currentValue":"performance","options":[{"value":"performance","name":"Performance"},{"value":"lite","name":"Lite"}]}]`
		grouped := strings.Replace(base, `"options":[`, `"options":[{"group":"original","name":"Models","options":[`, 1)
		grouped = strings.TrimSuffix(grouped, `]}]`) + `]}]}]`
		r, _ := runFixture(t, Options{}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
			if methodOf(p) == "session/new" || methodOf(p) == "session/set_config_option" {
				result := map[string]any{"configOptions": json.RawMessage(grouped)}
				if methodOf(p) == "session/new" {
					result["sessionId"] = "fixture-session"
				}
				sendResult(d, p["id"], result)
				return
			}
			basicReply(d, p, s)
		})
		if r.Terminal != "completed" {
			t.Fatal("canonical grouped model configuration was rejected")
		}
		variants := map[string]string{
			"current_value":     strings.Replace(base, `"currentValue":"performance"`, `"currentValue":"lite","CurrentValue":"performance"`, 1),
			"category_only":     strings.Replace(base, `"category":"model"`, `"Category":"model"`, 1),
			"category_override": strings.Replace(base, `"category":"model"`, `"category":"other","Category":"model"`, 1),
			"id":                strings.Replace(base, `"id":"model"`, `"id":"other","Id":"model"`, 1),
			"type":              strings.Replace(base, `"type":"select"`, `"type":"other","Type":"select"`, 1),
			"options":           strings.Replace(base, `"options":`, `"Options":`, 1),
			"value":             strings.Replace(base, `"value":"performance"`, `"value":"other","Value":"performance"`, 1),
			"group":             strings.Replace(base, `"options":[`, `"options":[{"group":"original","Group":"override","name":"Models","options":[`, 1),
		}
		variants["group"] = strings.TrimSuffix(variants["group"], `]}]`) + `]}]}]`
		for name, raw := range variants {
			for _, where := range []string{"session/new", "session/set_config_option", "config_option_update"} {
				r, _ := runFixture(t, Options{}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
					m := methodOf(p)
					if m == where {
						result := map[string]any{"configOptions": json.RawMessage(raw)}
						if m == "session/new" {
							result["sessionId"] = "fixture-session"
						}
						sendResult(d, p["id"], result)
						return
					}
					if m == "session/prompt" && where == "config_option_update" {
						update(d, "fixture-session", "config_option_update", map[string]any{"configOptions": json.RawMessage(raw)})
					}
					basicReply(d, p, s)
				})
				if r.Terminal != "unknown" || r.Error == nil || where != "config_option_update" && r.Evidence.PromptStarted {
					t.Errorf("%s at %s bypassed canonical model configuration", name, where)
				}
			}
		}
		// This state is reachable after the ACK frame enters the SDK but before
		// run records its ACK flag. A later observation cannot lose authority.
		d := &Driver{options: Options{Model: "performance"}, modelConfig: "model"}
		for _, current := range []string{"performance", "lite"} {
			raw, _ := json.Marshal(modelOptions(current))
			params := map[string]json.RawMessage{"sessionId": json.RawMessage(`"fixture-session"`), "update": json.RawMessage(`{"sessionUpdate":"config_option_update","configOptions":` + string(raw) + `}`)}
			err := d.projectUpdate(params)
			if current == "lite" && err == nil || current == "performance" && err != nil {
				t.Errorf("model update before asynchronous ACK flag was mishandled: matches=%t", current == "performance")
			}
		}
	})
}
func TestStrictFramesFailClosedBeforeSDK(t *testing.T) {
	for _, mode := range []string{"duplicate", "surrogate", "depth", "line_limit", "foreign_session", "late_response", "case_alias_stop"} {
		t.Run(mode, func(t *testing.T) {
			r, _ := runFixture(t, Options{Secrets: []string{"fixture-secret"}}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
				m := methodOf(p)
				if m == "initialize" {
					var raw string
					switch mode {
					case "duplicate":
						raw = `{"jsonrpc":"2.0","id":1,"result":{},"result":{"protocolVersion":1}}`
					case "surrogate":
						raw = `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"name":"\ud800"}}`
					case "depth":
						raw = `{"jsonrpc":"2.0","id":1,"result":{"nested":` + strings.Repeat("[", 40) + `0` + strings.Repeat("]", 40) + `}}`
					case "line_limit":
						raw = strings.Repeat("x", frameLimit+1)
					}
					if raw != "" {
						_, _ = d.Write([]byte(raw + "\n"))
						return
					}
				}
				if m == "session/prompt" && mode == "foreign_session" {
					update(d, "other-session", "agent_message_chunk", map[string]any{"content": map[string]any{"type": "text", "text": "not our reply"}})
					return
				}
				if m == "session/prompt" && mode == "case_alias_stop" {
					_, _ = d.Write([]byte(`{"jsonrpc":"2.0","id":` + string(p["id"]) + `,"result":{"stopReason":"future_done","StopReason":"end_turn"}}` + "\n"))
					return
				}
				basicReply(d, p, s)
				if m == "session/prompt" && mode == "late_response" {
					sendResult(d, p["id"], map[string]any{"stopReason": "end_turn"})
				}
			})
			if r.Terminal != "unknown" || r.Error == nil || strings.Contains(r.Error.Error(), "fixture-secret") {
				t.Fatal("invalid frame became success", r.Terminal)
			}
		})
	}
}
func TestToolLedgerAndStopReasonCannotFakeCompletion(t *testing.T) {
	for _, mode := range []string{"orphan", "open", "reopened", "unknown_stop"} {
		t.Run(mode, func(t *testing.T) {
			r, _ := runFixture(t, Options{}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
				if methodOf(p) != "session/prompt" {
					basicReply(d, p, s)
					return
				}
				if mode != "unknown_stop" {
					tag := "tool_call"
					if mode == "orphan" {
						tag = "tool_call_update"
					}
					update(d, "fixture-session", tag, map[string]any{"toolCallId": "tool:1", "title": "Inspect", "status": "pending"})
					if mode != "open" {
						update(d, "fixture-session", "tool_call_update", map[string]any{"toolCallId": "tool:1", "status": "completed"})
					}
					if mode == "reopened" {
						update(d, "fixture-session", "tool_call_update", map[string]any{"toolCallId": "tool:1", "status": "in_progress"})
					}
				}
				reason := "end_turn"
				if mode == "unknown_stop" {
					reason = "future_done"
				}
				sendResult(d, p["id"], map[string]any{"stopReason": reason})
			})
			if r.Terminal != "unknown" {
				t.Fatal("unclosed/unknown execution became completed")
			}
		})
	}
}
func TestSecretChunksAndUsagePreserveExactObservation(t *testing.T) {
	var mu sync.Mutex
	var shown strings.Builder
	r, _ := runFixture(t, Options{Secrets: []string{"secret/credential"}, Emit: func(e Event) error { mu.Lock(); defer mu.Unlock(); shown.WriteString(e.Text); return nil }}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
		if methodOf(p) != "session/prompt" {
			basicReply(d, p, s)
			return
		}
		for _, chunk := range []string{"safe\nsecret/", "credential\nreply"} {
			update(d, "fixture-session", "agent_message_chunk", map[string]any{"content": map[string]any{"type": "text", "text": chunk}})
		}
		update(d, "fixture-session", "agent_thought_chunk", map[string]any{"content": map[string]any{"type": "text", "text": "never persist thought"}})
		sendResult(d, p["id"], map[string]any{"stopReason": "end_turn", "usage": map[string]any{"inputTokens": 7}})
	})
	mu.Lock()
	defer mu.Unlock()
	if r.Terminal != "completed" || strings.Contains(shown.String(), "secret/") || strings.Contains(shown.String(), "credential") || strings.Contains(shown.String(), "thought") || !strings.Contains(shown.String(), "[REDACTED]") {
		t.Fatal("unsafe projection", r.Terminal)
	}
	if r.UsageState != "partial" || r.Usage == nil || r.Usage.InputTokens == nil || *r.Usage.InputTokens != 7 || r.Usage.OutputTokens != nil {
		t.Fatal("missing usage fabricated zero")
	}
}
func TestPermissionOnlyExactOnceWithCompleteSafeDetails(t *testing.T) {
	for _, mode := range []string{"allow", "reject", "always", "foreign_option", "redacted_allow", "redacted_reject", "completed_tool_reuse"} {
		t.Run(mode, func(t *testing.T) {
			var callbackCount int
			options := Options{Secrets: []string{"private-password"}, Permission: func(_ context.Context, r ac.PermissionRequest) (ac.PermissionDecision, error) {
				callbackCount++
				if r.IdempotencyKey == "" || r.ToolCallID != "tool:权限" || strings.Contains(r.Details, "private-password") {
					t.Error("unsafe permission request")
				}
				id, kind := "allow:一次", "allow_once"
				if mode == "reject" || mode == "redacted_reject" {
					id, kind = "reject:一次", "reject_once"
				}
				if mode == "always" {
					id, kind = "always", "allow_always"
				}
				if mode == "foreign_option" {
					id = "not-an-option"
				}
				if strings.HasPrefix(mode, "redacted") && r.DetailsComplete {
					t.Error("redacted details declared complete")
				}
				return ac.PermissionDecision{OptionID: id, Kind: kind, ActorID: "current-human", DecidedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
			}}
			r, s := runFixture(t, options, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
				m := methodOf(p)
				if m == "session/prompt" {
					s.promptID = append([]byte{}, p["id"]...)
					update(d, "fixture-session", "tool_call", map[string]any{"toolCallId": "tool:权限", "title": "Edit", "status": "pending"})
					if mode == "completed_tool_reuse" {
						update(d, "fixture-session", "tool_call_update", map[string]any{"toolCallId": "tool:权限", "status": "completed"})
					}
					value := "safe"
					if strings.HasPrefix(mode, "redacted") {
						value = "private-password"
					}
					raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "permission-rpc", "method": "session/request_permission", "params": map[string]any{"sessionId": "fixture-session", "toolCall": map[string]any{"toolCallId": "tool:权限", "title": "Edit file", "kind": "edit", "status": "pending", "rawInput": map[string]any{"content": value}}, "options": []any{map[string]any{"optionId": "allow:一次", "name": "Allow once", "kind": "allow_once"}, map[string]any{"optionId": "reject:一次", "name": "Reject once", "kind": "reject_once"}, map[string]any{"optionId": "always", "name": "Always", "kind": "allow_always"}}}})
					_, _ = d.Write(append(raw, '\n'))
					return
				}
				if m == "" {
					result := object(p["result"])
					s.option = stringField(object(result["outcome"]), "optionId")
					update(d, "fixture-session", "tool_call_update", map[string]any{"toolCallId": "tool:权限", "status": "completed"})
					sendResult(d, s.promptID, map[string]any{"stopReason": "end_turn"})
					return
				}
				basicReply(d, p, s)
			})
			if callbackCount != 1 {
				t.Fatal("missing/repeated explicit decision")
			}
			if mode == "allow" {
				if r.Terminal != "completed" || s.option != "allow:一次" || r.Evidence.PermissionsRequested != 1 {
					t.Fatal("exact allow failed", r.Terminal, r.Error)
				}
			} else if r.Terminal == "completed" {
				t.Fatal("denied/invalid permission became success")
			}
			if mode == "reject" || mode == "redacted_reject" {
				if s.option != "reject:一次" || r.Evidence.PermissionsDenied != 1 {
					t.Fatal("exact reject lost")
				}
			}
		})
	}
	t.Run("sanitized_kind_and_scalars", func(t *testing.T) {
		for _, value := range []struct{ name, kind, input, secret string }{
			{"kind", "private-password", `{"path":"safe"}`, "private-password"},
			{"integer", "edit", `{"pin":123456}`, "123456"},
			{"decimal", "edit", `{"pin":1.25}`, "1.25"},
			{"boolean", "edit", `{"pin":true}`, "true"},
			{"null", "edit", `{"pin":null}`, "null"},
		} {
			raw, _ := json.Marshal(map[string]any{"sessionId": "fixture-session-safe", "toolCall": map[string]any{"toolCallId": "tool:权限", "title": "Edit file", "kind": value.kind, "status": "pending", "rawInput": json.RawMessage(value.input)}, "options": []any{map[string]any{"optionId": "allow:一次", "name": "Allow once", "kind": "allow_once"}, map[string]any{"optionId": "reject:一次", "name": "Reject once", "kind": "reject_once"}}})
			r, err := permissionRequest(raw, []string{value.secret})
			if err != nil || r.DetailsComplete || strings.Contains(r.Details, value.secret) || strings.Contains(r.Kind, value.secret) || value.name == "kind" && r.Kind != "other" || len(r.Options) != 2 || r.Options[0].ID != "allow:一次" || r.Options[1].ID != "reject:一次" {
				t.Errorf("%s permission primitive escaped sanitization or changed opaque options: error=%v complete=%t details_secret=%t kind_secret=%t options=%d", value.name, err, r.DetailsComplete, strings.Contains(r.Details, value.secret), strings.Contains(r.Kind, value.secret), len(r.Options))
			}
		}
		safe, changed := safeValue([]any{json.Number("7"), json.Number("1.5"), true, nil}, []string{"unrelated-secret"})
		encoded, _ := json.Marshal(safe)
		if changed || string(encoded) != `[7,1.5,true,null]` {
			t.Error("safe JSON primitive types were changed")
		}
	})
	t.Run("reply_delivered_before_writer_returns", func(t *testing.T) {
		for _, writeFails := range []bool{false, true} {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			d, err := New(ctx, Options{Cwd: t.TempDir(), Goal: "Owned permission ordering fixture", Model: "performance", ConfigSHA: strings.Repeat("a", 64), Permission: func(_ context.Context, r ac.PermissionRequest) (ac.PermissionDecision, error) {
				return ac.PermissionDecision{OptionID: r.Options[0].ID, Kind: "allow_once", ActorID: "current-human", DecidedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
			}})
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			messages, endReceived, copyDone := make(chan []byte, 64), make(chan struct{}), make(chan struct{})
			go func() {
				defer close(copyDone)
				_, _ = d.CopyInput(fixtureWriter(func(raw []byte) (int, error) {
					messages <- append([]byte{}, raw...)
					packet := object(raw)
					if methodOf(packet) == "" && packet["result"] != nil {
						select {
						case <-endReceived:
						case <-ctx.Done():
							return 0, ctx.Err()
						}
						// The peer already consumed the bytes, but stdin has not yet
						// reported whether that write succeeded. Prompt can finish now.
						select {
						case <-d.done:
						case <-time.After(200 * time.Millisecond):
						}
						if writeFails {
							return 0, io.ErrShortWrite
						}
					}
					return len(raw), nil
				}))
			}()
			state := &fixtureState{}
		loop:
			for {
				select {
				case raw := <-messages:
					p := object(raw)
					switch methodOf(p) {
					case "session/prompt":
						state.promptID = p["id"]
						b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "permission-rpc", "method": "session/request_permission", "params": map[string]any{"sessionId": "fixture-session", "toolCall": map[string]any{"toolCallId": "tool:1", "title": "Edit file", "kind": "edit", "rawInput": map[string]any{"path": "owned"}}, "options": []any{map[string]any{"optionId": "allow", "name": "Allow once", "kind": "allow_once"}}}})
						_, _ = d.Write(append(b, '\n'))
					case "":
						update(d, "fixture-session", "tool_call_update", map[string]any{"toolCallId": "tool:1", "status": "completed"})
						sendResult(d, state.promptID, map[string]any{"stopReason": "end_turn"})
						close(endReceived)
					default:
						basicReply(d, p, state)
					}
				case <-copyDone:
					break loop
				case <-ctx.Done():
					t.Error("owned reply writer did not finish")
					break loop
				}
			}
			r := d.Finish()
			_ = d.Close()
			cancel()
			if !r.Evidence.PromptCompleted || !writeFails && r.Terminal != "completed" || writeFails && r.Terminal != "unknown" {
				t.Errorf("write failure=%t: Prompt result raced actual reply write: terminal=%s", writeFails, r.Terminal)
			}
		}
	})
}
func TestDisabledHostCapabilitiesAreActuallyRejected(t *testing.T) {
	r, s := runFixture(t, Options{}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
		m := methodOf(p)
		if m == "initialize" {
			params := object(p["params"])
			caps := object(params["clientCapabilities"])
			if string(caps["terminal"]) != "false" || string(object(caps["fs"])["writeTextFile"]) != "false" {
				t.Error("host capabilities not explicitly false")
			}
		}
		if m == "session/prompt" {
			s.promptID = p["id"]
			raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "forbidden-write", "method": "fs/write_text_file", "params": map[string]any{"sessionId": "fixture-session", "path": "Z:/never-owned/no-file", "content": "forbidden"}})
			_, _ = d.Write(append(raw, '\n'))
			return
		}
		if m == "" {
			if p["error"] == nil {
				t.Error("unsupported write acknowledged")
			}
			sendResult(d, s.promptID, map[string]any{"stopReason": "end_turn"})
			return
		}
		basicReply(d, p, s)
	})
	_ = s
	if r.Terminal == "completed" {
		t.Fatal("unadvertised host request disappeared")
	}
}
func TestCancellationAndEOFNeverCertifyHostStop(t *testing.T) {
	for _, mode := range []string{"cancel", "truncated_eof", "local_close"} {
		t.Run(mode, func(t *testing.T) {
			r, s := runFixture(t, Options{}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
				m := methodOf(p)
				if m == "session/prompt" {
					s.promptID = p["id"]
					switch mode {
					case "cancel":
						if e := d.Cancel(); e != nil {
							t.Error(e)
						}
					case "truncated_eof":
						_, _ = d.Write([]byte(`{"jsonrpc":`))
						d.Finish()
					case "local_close":
						d.Close()
					}
					return
				}
				if m == "session/cancel" {
					sendResult(d, s.promptID, map[string]any{"stopReason": "cancelled"})
					return
				}
				basicReply(d, p, s)
			})
			if r.Terminal != "unknown" || r.Evidence.HostProcessStopped || r.Evidence.HostProcessTreeStopped {
				t.Fatal("cancel/EOF fabricated stop")
			}
			if mode == "cancel" && !r.Evidence.CancelRequested {
				t.Fatal("cancel intent lost")
			}
			if mode == "local_close" {
				s.mu.Lock()
				defer s.mu.Unlock()
				for _, m := range s.methods {
					if m == "session/cancel" {
						t.Fatal("local Close sent Cancel")
					}
				}
			}
		})
	}
}
