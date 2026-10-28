package runner

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

func newOpenCodeInput(requestID, goal string, bridge NativeWorkspaceBridge) (*duplexInput, error) {
	return newHostAgentInput(requestID, goal, bridge)
}

// Shared bounded host carrier; each SDK retains its own protocol decoder.
func newHostAgentInput(requestID, goal string, bridge NativeWorkspaceBridge) (*duplexInput, error) {
	if requestID == "" || !utf8.ValidString(requestID) || len(requestID) > 128 || strings.ContainsAny(requestID, "\x00\r\n") || bridge.RequestID != requestID || bridge.ConflictRepairProtocol != "" || strings.TrimSpace(goal) == "" || !utf8.ValidString(goal) || strings.ContainsRune(goal, 0) || len(goal) > 32000 {
		return nil, fmt.Errorf("SDK input requires its host-owned request and bounded goal")
	}
	execute, err := json.Marshal(struct {
		Version         int                   `json:"v"`
		Type            string                `json:"type"`
		RequestID       string                `json:"request_id"`
		Goal            string                `json:"goal"`
		WorkspaceBridge NativeWorkspaceBridge `json:"workspace_bridge"`
	}{1, "execute", requestID, goal, bridge})
	if err != nil {
		return nil, err
	}
	cancel, _ := json.Marshal(struct {
		Version   int    `json:"v"`
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
	}{1, "cancel", requestID})
	return newDuplexInput(execute, cancel), nil
}
