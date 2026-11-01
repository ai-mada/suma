package acpstdio

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Cursor's reviewed ACP implementation omits agentInfo and returns models in
// addition to configOptions. Only the latter's exact ACK authorizes Prompt.
func TestCursorNativeSessionKeepsConfigACKAuthority(t *testing.T) {
	for _, mode := range []string{"complete", "missing_config", "bad_models", "unsupported_extension"} {
		t.Run(mode, func(t *testing.T) {
			r, state := runFixture(t, Options{Runtime: "cursor"}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
				switch methodOf(p) {
				case "initialize":
					sendResult(d, p["id"], map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{map[string]any{"id": "cursor_login", "name": "Cursor Login"}}})
				case "session/new":
					models := map[string]any{"currentModelId": "lite", "availableModels": []any{map[string]any{"modelId": "lite", "name": "Lite"}, map[string]any{"modelId": "performance", "name": "Performance"}}}
					if mode == "bad_models" {
						models["CurrentModelId"] = "performance"
					}
					result := map[string]any{"sessionId": "fixture-session", "configOptions": modelOptions("lite"), "models": models}
					if mode == "missing_config" {
						delete(result, "configOptions")
					}
					sendResult(d, p["id"], result)
				case "session/set_config_option":
					sendResult(d, p["id"], map[string]any{"configOptions": modelOptions("performance")})
				case "session/prompt":
					if mode == "unsupported_extension" {
						_, _ = d.Write([]byte(`{"jsonrpc":"2.0","id":"cursor-question","method":"cursor/ask_question","params":{"sessionId":"fixture-session","questions":[]}}` + "\n"))
						return
					}
					update(d, "fixture-session", "available_commands_update", map[string]any{"availableCommands": []any{}})
					sendResult(d, p["id"], map[string]any{"stopReason": "end_turn"})
				}
			})
			_ = state
			if mode == "complete" {
				if r.Terminal != "completed" || !r.Evidence.ModelSelectionAcknowledged || r.Evidence.SelectedModel != "performance" || r.Evidence.AgentName != "" || r.Evidence.AgentVersion != "" {
					t.Fatalf("Cursor config ACK was not preserved: %s %v", r.Terminal, r.Error)
				}
			} else if r.Terminal != "unknown" || mode != "unsupported_extension" && r.Evidence.PromptStarted {
				t.Fatal("unsupported or unacknowledged Cursor reached a successful prompt")
			}
		})
	}
}

func TestCursorModelDiscoveryKeepsNativeConfigSourceAndNoWork(t *testing.T) {
	for _, mode := range []string{"native_config", "models_without_config"} {
		t.Run(mode, func(t *testing.T) {
			calls := []string{}
			o := discoveryFixture(t, nil, 2*time.Second, func(d *ModelDiscovery, p map[string]json.RawMessage) {
				method := methodOf(p)
				calls = append(calls, method)
				switch method {
				case "initialize":
					discoveryReply(d, p["id"], map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{map[string]any{"id": "cursor_login", "name": "Cursor Login"}}})
				case "session/new":
					result := map[string]any{"sessionId": "cursor-discovery189", "configOptions": modelOptions("lite"), "models": map[string]any{"currentModelId": "lite", "availableModels": []any{map[string]any{"modelId": "lite", "name": "Lite"}}}}
					if mode == "models_without_config" {
						delete(result, "configOptions")
					}
					discoveryReply(d, p["id"], result)
					if mode == "native_config" {
						update(d.d, "cursor-discovery189", "available_commands_update", map[string]any{"availableCommands": []any{}})
					}
				default:
					t.Error("discovery sent work/auth/config RPC", method)
				}
			}, "cursor")
			if strings.Join(calls, ",") != "initialize,session/new" {
				t.Fatal("Cursor discovery sent unexpected work")
			}
			if mode == "native_config" {
				if o.ErrorCode != "" || o.Catalogue == nil || o.Catalogue.Source != "acp_session_config" || o.AgentName != "" || o.Evidence.UnexpectedActivity != 0 {
					t.Fatal("native Cursor catalogue lost exact source", o.ErrorCode)
				}
			} else if o.Catalogue != nil || o.ErrorCode == "" {
				t.Fatal("legacy models fabricated config authority")
			}
		})
	}
}

func TestCursorExtensionNotificationsAreObservationsOnly(t *testing.T) {
	for _, request := range []bool{false, true} {
		t.Run(map[bool]string{false: "notices", true: "task_request_refused"}[request], func(t *testing.T) {
			notices := 0
			r, _ := runFixture(t, Options{Runtime: "cursor", Secrets: []string{"owned-secret189"}, Emit: func(e Event) error {
				if e.Kind == "notice" {
					notices++
					if strings.Contains(e.Text, "owned-secret189") {
						t.Error("extension secret escaped")
					}
				}
				return nil
			}}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
				if methodOf(p) != "session/prompt" {
					basicReply(d, p, s)
					return
				}
				for _, method := range []string{"cursor/update_todos", "cursor/generate_image", "cursor/task"} {
					packet := map[string]any{"jsonrpc": "2.0", "method": method, "params": map[string]any{"toolCallId": "tool189", "details": "owned-secret189"}}
					if request && method == "cursor/task" {
						packet["id"] = "must-not-dispatch"
					}
					raw, _ := json.Marshal(packet)
					_, _ = d.Write(append(raw, '\n'))
				}
				if !request {
					sendResult(d, p["id"], map[string]any{"stopReason": "end_turn"})
				}
			})
			if !request && (r.Terminal != "completed" || notices != 3) {
				t.Fatal("Cursor observations failed normal turn", r.Error, notices)
			}
			if request && r.Terminal != "unknown" {
				t.Fatal("ID-bearing task fabricated dispatch or completion")
			}
		})
	}
}
