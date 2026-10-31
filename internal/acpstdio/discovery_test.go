package acpstdio

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func discoveryFixture(t *testing.T, secrets []string, timeout time.Duration, reply func(*ModelDiscovery, map[string]json.RawMessage)) DiscoveryObservation {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	d, err := NewModelDiscovery(ctx, DiscoveryOptions{Cwd: t.TempDir(), Secrets: secrets})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	packets := make(chan []byte, 64)
	copied := make(chan struct{})
	go func() {
		defer close(copied)
		_, _ = d.CopyInput(fixtureWriter(func(b []byte) (int, error) {
			select {
			case packets <- append([]byte{}, b...):
				return len(b), nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}))
	}()
	for {
		select {
		case b := <-packets:
			reply(d, object(b))
		case <-copied:
			// Reading the provisional catalogue does not launch another stack;
			// mutating the caller's copy cannot change the final observation.
			catalogue, err := d.WaitCatalogue(ctx)
			if err == nil && len(catalogue.Models) != 0 {
				catalogue.Models[0].ID = "caller-mutated-copy"
			}
			final := d.Finish()
			again := d.Finish()
			if final.ErrorCode != again.ErrorCode || (final.Catalogue == nil) != (again.Catalogue == nil) {
				t.Error("re-reading Finish changed the original observation")
			}
			if final.Catalogue != nil && final.Catalogue.Models[0].ID == "caller-mutated-copy" {
				t.Error("catalogue snapshot was shared with caller")
			}
			return final
		case <-ctx.Done():
			_ = d.Close()
			<-copied
			return d.Finish()
		}
	}
}
func discoveryReply(d *ModelDiscovery, id json.RawMessage, result any) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	_, _ = d.Write(append(b, '\n'))
}
func discoveryInitialize(d *ModelDiscovery, p map[string]json.RawMessage, close bool) {
	caps := map[string]any{}
	if close {
		caps["sessionCapabilities"] = map[string]any{"close": map[string]any{}}
	}
	discoveryReply(d, p["id"], map[string]any{"protocolVersion": 1, "agentCapabilities": caps, "agentInfo": map[string]any{"name": "Owned discovery fixture", "version": "1"}})
}
func TestModelDiscoveryNeverStartsWork(t *testing.T) {
	for _, mode := range []string{"flat", "grouped", "missing_current", "unlisted_current", "close"} {
		t.Run(mode, func(t *testing.T) {
			methods := []string{}
			o := discoveryFixture(t, nil, 3*time.Second, func(d *ModelDiscovery, p map[string]json.RawMessage) {
				m := methodOf(p)
				methods = append(methods, m)
				switch m {
				case "initialize":
					caps := object(object(p["params"])["clientCapabilities"])
					if string(caps["terminal"]) != "false" || string(object(caps["fs"])["readTextFile"]) != "false" {
						t.Error("discovery advertised host I/O")
					}
					discoveryInitialize(d, p, mode == "close")
				case "session/new":
					options := modelOptions("lite")
					if mode == "missing_current" {
						delete(options[0].(map[string]any), "currentValue")
					}
					if mode == "unlisted_current" {
						options[0].(map[string]any)["currentValue"] = "not-listed"
					}
					if mode == "grouped" {
						options[0].(map[string]any)["options"] = []any{map[string]any{"group": "models", "name": "Models", "options": options[0].(map[string]any)["options"]}}
					}
					discoveryReply(d, p["id"], map[string]any{"sessionId": "discovery-session", "configOptions": options})
				case "session/close":
					discoveryReply(d, p["id"], map[string]any{})
				default:
					t.Error("discovery sent work/auth/configuration request")
				}
			})
			want := "initialize,session/new"
			if mode == "close" {
				want += ",session/close"
			}
			if strings.Join(methods, ",") != want || o.ErrorCode != "" || o.Catalogue == nil || len(o.Catalogue.Models) != 2 || !o.Evidence.SessionCreated || o.Evidence.PromptRequests != 0 || o.Evidence.ConfigurationWrites != 0 || o.Evidence.HostProcessStopped || o.Evidence.HostProcessTreeStopped {
				t.Fatal("no-work catalogue observation missing")
			}
			if mode == "missing_current" {
				if o.Catalogue.CurrentValue != "" {
					t.Error("unverified current model invented")
				}
			}
			if mode == "unlisted_current" && o.Catalogue.CurrentValue != "not-listed" {
				t.Error("safe unlisted current model observation was changed")
			}
			if mode == "close" && (!o.Evidence.SessionCloseRequested || !o.Evidence.SessionCloseAcknowledged) {
				t.Error("close facts missing")
			}
		})
	}
}
func TestModelDiscoveryRejectsUnsafeDirectory(t *testing.T) {
	for _, mode := range []string{"ambiguous", "duplicate", "secret_id", "alias", "limit", "secret_name"} {
		t.Run(mode, func(t *testing.T) {
			o := discoveryFixture(t, []string{"secret-value"}, 3*time.Second, func(d *ModelDiscovery, p map[string]json.RawMessage) {
				if methodOf(p) == "initialize" {
					discoveryInitialize(d, p, false)
					return
				}
				if methodOf(p) != "session/new" {
					t.Error("unsafe directory caused extra request")
					return
				}
				options := modelOptions("lite")
				option := options[0].(map[string]any)
				values := option["options"].([]any)
				switch mode {
				case "ambiguous":
					options = append(options, options[0])
				case "duplicate":
					option["options"] = append(values, values[0])
				case "secret_id":
					values[0].(map[string]any)["value"] = "secret-value"
				case "alias":
					option["Category"] = "model"
				case "limit":
					for len(values) < 257 {
						values = append(values, map[string]any{"name": "Model", "value": "value"})
					}
					option["options"] = values
				case "secret_name":
					values[0].(map[string]any)["name"] = "secret-value"
				}
				discoveryReply(d, p["id"], map[string]any{"sessionId": "discovery-session", "configOptions": options})
			})
			if mode == "secret_name" {
				if o.ErrorCode != "" || o.Catalogue == nil || strings.Contains(o.Catalogue.Models[0].Name, "secret-value") {
					t.Fatal("name was not safely sanitized")
				}
				return
			}
			if o.ErrorCode == "" || o.Catalogue != nil {
				t.Fatal("unadoptable directory escaped")
			}
		})
	}
}
func TestModelDiscoveryRejectsActivityAndUnknownLifecycle(t *testing.T) {
	for _, mode := range []string{"text", "tool", "permission", "filesystem", "terminal", "timeout", "close_error", "late", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			var closeID json.RawMessage
			refusals := 0
			timeout := 3 * time.Second
			if mode == "timeout" {
				timeout = 30 * time.Millisecond
			}
			o := discoveryFixture(t, nil, timeout, func(d *ModelDiscovery, p map[string]json.RawMessage) {
				switch methodOf(p) {
				case "initialize":
					if mode != "timeout" {
						discoveryInitialize(d, p, true)
					}
				case "session/new":
					discoveryReply(d, p["id"], map[string]any{"sessionId": "discovery-session", "configOptions": modelOptions("lite")})
				case "session/close":
					closeID = p["id"]
					switch mode {
					case "close_error":
						_, _ = d.Write([]byte(`{"jsonrpc":"2.0","id":` + string(p["id"]) + `,"error":{"code":-32603,"message":"private provider diagnostic"}}` + "\n"))
					case "truncated":
						_, _ = d.Write([]byte(`{"jsonrpc":`))
						_ = d.Close()
					case "late":
						discoveryReply(d, p["id"], map[string]any{})
						discoveryReply(d, p["id"], map[string]any{})
					default:
						method := "session/update"
						params := map[string]any{"sessionId": "discovery-session", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "unexpected body"}}}
						if mode == "tool" {
							params["update"] = map[string]any{"sessionUpdate": "tool_call", "toolCallId": "tool:1", "title": "Unexpected tool", "status": "pending"}
						}
						packet := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
						if mode == "permission" {
							packet["id"] = "unexpected-permission"
							packet["method"] = "session/request_permission"
							packet["params"] = map[string]any{"sessionId": "discovery-session", "toolCall": map[string]any{"toolCallId": "tool:1", "title": "Unexpected edit", "kind": "edit", "rawInput": map[string]any{}}, "options": []any{map[string]any{"optionId": "allow", "name": "Allow once", "kind": "allow_once"}}}
						}
						if mode == "filesystem" || mode == "terminal" {
							packet["id"] = "unexpected-io"
							if mode == "filesystem" {
								packet["method"] = "fs/read_text_file"
								packet["params"] = map[string]any{"sessionId": "discovery-session", "path": "Z:/never-read"}
							} else {
								packet["method"] = "terminal/create"
								packet["params"] = map[string]any{"sessionId": "discovery-session", "command": "never-run"}
							}
						}
						b, _ := json.Marshal(packet)
						_, _ = d.Write(append(b, '\n'))
					}
				case "":
					refusals++
					if mode == "permission" && stringField(object(object(p["result"])["outcome"]), "outcome") != "cancelled" {
						t.Error("unexpected permission not cancelled")
					}
					if (mode == "filesystem" || mode == "terminal") && p["error"] == nil {
						t.Error("unexpected host capability was acknowledged")
					}
					discoveryReply(d, closeID, map[string]any{})
				default:
					t.Error("unexpected work request")
				}
			})
			if o.ErrorCode == "" || o.Catalogue != nil || o.Evidence.HostProcessStopped || o.Evidence.HostProcessTreeStopped {
				t.Fatal("unexpected/unfinished discovery became adoptable")
			}
			if (mode == "permission" || mode == "filesystem" || mode == "terminal") && (refusals != 1 || o.Evidence.UnexpectedActivity != 1) {
				t.Error("unexpected request did not receive exactly one real SDK refusal")
			}
			if mode == "permission" && o.Evidence.PermissionRequests != 1 {
				t.Error("permission request fact missing")
			}
		})
	}
}
