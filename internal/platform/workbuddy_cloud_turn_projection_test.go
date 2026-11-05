package platform

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/workbuddyacp"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestCloudTurnProjectionSplitSecretAndUnicode(t *testing.T) {
	filter := newCloudTurnTextFilter([]string{"private-ticket-169", "private-oauth-169"})
	var all strings.Builder
	for _, part := range []string{"回复 priv", "ate-ticket-", "169 与 private-oa", "uth-169 完成。"} {
		text, _ := filter.Push(part)
		if !utf8.ValidString(text) || strings.Contains(text, "private-ticket-169") || strings.Contains(text, "private-oauth-169") {
			t.Fatal("a persisted chunk exposed a secret or broken Unicode")
		}
		all.WriteString(text)
	}
	last, _ := filter.Flush()
	all.WriteString(last)
	if strings.Contains(all.String(), "private-ticket-169") || strings.Contains(all.String(), "private-oauth-169") || !strings.Contains(all.String(), "回复") || !strings.Contains(all.String(), "完成") {
		t.Fatal("cross-chunk concatenation exposed credentials or lost ordinary text")
	}
}

func TestCloudTurnProjectionIncompleteSecretTailIsHidden(t *testing.T) {
	filter := newCloudTurnTextFilter([]string{"private-ticket-169"})
	first, _ := filter.Push("answer private-tic")
	last, changed := filter.Flush()
	if !changed || strings.Contains(first+last, "private-tic") || !strings.Contains(first+last, "answer") {
		t.Fatal("closing an incomplete stream exposed a pending credential prefix")
	}
}

func TestCloudTurnProjectionToolLedgerRejectsOrphansAndUnfinished(t *testing.T) {
	ledger := newCloudTurnToolLedger()
	ledger.Observe("one", "pending", true)
	if ledger.Complete() {
		t.Fatal("pending tool became completed")
	}
	ledger.Observe("one", "", false)
	ledger.Observe("one", "completed", false)
	if !ledger.Complete() {
		t.Fatal("a fully observed terminal tool remained incomplete")
	}
	ledger.Observe("orphan", "completed", false)
	if ledger.Complete() {
		t.Fatal("orphan terminal update erased missing tool source")
	}
}

func TestCloudTurnProjectionPermissionShowsCompleteInputAndOriginalOptions(t *testing.T) {
	packet := cloudTurnProjectionPermissionPacket(t, `{"toolCallId":"tool-one","title":"Run local tests","kind":"execute","rawInput":{"command":"go test ./internal/platform"}}`)
	permission, err := cloudTurnProjectPermission(packet, "permission-one", "turn-one", nil)
	if err != nil || !permission.ApproveAllowed || len(permission.Options) != 4 || !strings.Contains(permission.Details, "go test ./internal/platform") || permission.DetailsSHA256 != cloudTurnProjectionSHA(permission.Details) {
		t.Fatal("complete operation or accurate original options were lost")
	}
	for _, scope := range []string{"request", "option"} {
		t.Run(scope, func(t *testing.T) {
			raw := string(packet.Raw.Data)
			if scope == "request" {
				raw = strings.Replace(raw, `"toolCall":`, `"unshownScope":{"extra":true},"toolCall":`, 1)
			} else {
				raw = strings.Replace(raw, `"kind":"allow_once"`, `"kind":"allow_once","unshownScope":{"extra":true}`, 1)
			}
			unknown, decodeErr := workbuddyacp.DecodeFrame(workbuddyacp.Frame{Data: []byte(raw)}, "task169")
			if decodeErr != nil {
				t.Fatal("controlled extended permission did not decode")
			}
			projected, projectErr := cloudTurnProjectPermission(unknown, "permission-one", "turn-one", nil)
			if projectErr != nil || projected.ApproveAllowed {
				t.Fatal("undisplayed permission details remained approvable")
			}
		})
	}
}

func TestCloudTurnProjectionPermissionRedactionDisablesApproval(t *testing.T) {
	packet := cloudTurnProjectionPermissionPacket(t, `{"toolCallId":"tool-one","title":"Run local tests","kind":"execute","rawInput":{"command":"echo private-ticket-169"},"rawOutput":{"ignored":"untrusted output"}}`)
	permission, err := cloudTurnProjectPermission(packet, "permission-one", "turn-one", []string{"private-ticket-169"})
	if err != nil || permission.ApproveAllowed || strings.Contains(permission.Details, "private-ticket-169") || strings.Contains(permission.Details, "untrusted output") {
		t.Fatal("redacted or unavailable full details were presented as approvable")
	}
}

func cloudTurnProjectionPermissionPacket(t *testing.T, tool string) workbuddyacp.Packet {
	t.Helper()
	raw := `{"jsonrpc":"2.0","id":"permission-rpc","method":"session/request_permission","params":{"sessionId":"task169","toolCall":` + tool + `,"options":[{"optionId":"once","name":"Allow once","kind":"allow_once"},{"optionId":"reject","name":"Reject once","kind":"reject_once"},{"optionId":"always","name":"Always","kind":"allow_always"},{"optionId":"never","name":"Never","kind":"reject_always"}]}}`
	packet, err := workbuddyacp.DecodeFrame(workbuddyacp.Frame{Data: []byte(raw)}, "task169")
	if err != nil {
		t.Fatal("controlled permission fixture is invalid")
	}
	return packet
}

func TestCloudTurnProjectionArtifactPreservesOnlySafeObservation(t *testing.T) {
	const uri = "agent:///artifacts/private-plan.md"
	const ticket = "private-ticket-169"
	const download = "https://private.example/artifact"
	for _, test := range []struct {
		name, kind, event string
		text              any
		want              string
	}{
		{"plan preview", "plan", "created", "计划\n" + ticket + " · " + uri + " · " + download, "计划\n" + cloudTurnHidden + " · " + cloudTurnHidden + " · " + cloudTurnHidden},
		{"overview preview", "overview", "updated", "Summary", "Summary"},
		{"deleted plan", "plan", "deleted", "stale private text", ""},
		{"tasks metadata", "tasks", "created", "private task text", ""},
		{"media metadata", "media", "updated", "private media text", ""},
		{"missing text", "overview", "updated", nil, ""},
		{"nontext payload", "plan", "created", map[string]any{"url": download}, ""},
		{"invalid controls", "overview", "updated", "invalid\x00\x7f", ""},
		{"oversized input", "plan", "created", strings.Repeat("x", wc.CloudTurnArtifactPreviewMaxBytes+1), ""},
		{"redaction exceeds limit", "overview", "updated", strings.Repeat("xyz", wc.CloudTurnArtifactPreviewMaxBytes/3), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			artifact := map[string]any{"type": test.kind, "uri": uri, "url": download, "previousText": "private previous text"}
			if test.text != nil {
				artifact["text"] = test.text
			}
			raw, err := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"method":  "_codebuddy.ai/artifact",
				"params":  map[string]any{"sessionId": "task169", "event": test.event, "artifact": artifact},
			})
			if err != nil {
				t.Fatal("could not encode artifact fixture")
			}
			packet, err := workbuddyacp.DecodeFrame(workbuddyacp.Frame{Data: raw}, "task169")
			if err != nil || packet.Kind != "artifact" {
				t.Fatal("controlled artifact fixture did not decode")
			}
			secrets := []string{ticket, "xyz"}
			ledger := newCloudTurnToolLedger()
			projected, err := cloudTurnProjectUpdate(packet, newCloudTurnTextFilter(secrets), ledger, secrets)
			if err != nil || projected.Incomplete || projected.ToolID != "" || !ledger.Complete() || len(projected.Entries) != 1 {
				t.Fatal("artifact observation affected the turn or tool state")
			}
			entry := projected.Entries[0]
			entry.Sequence = 1
			if entry.Kind != "artifact" || entry.ArtifactType != test.kind || entry.ArtifactEvent != test.event || entry.Text != test.want || entry.Validate() != nil {
				t.Fatal("artifact observation metadata or safe preview did not match")
			}
			public, err := json.Marshal(entry)
			if err != nil || strings.Contains(string(public), ticket) || strings.Contains(string(public), uri) || strings.Contains(string(public), download) || strings.Contains(string(public), "private previous text") {
				t.Fatal("artifact observation exposed private input")
			}
		})
	}
}
