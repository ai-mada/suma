package runner

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// dshInput keeps stdin open after execute; EOF cancels the runtime's active run.
// Its only further command is a cancellation of the original host-owned request.
type dshInput = duplexInput

func newDshInput(requestID, cwd, goal string) (*dshInput, error) {
	if strings.TrimSpace(requestID) == "" || len(requestID) > 200 || !utf8.ValidString(requestID) || strings.ContainsAny(requestID, "\x00\r\n") || !filepath.IsAbs(cwd) || !utf8.ValidString(cwd) || strings.ContainsRune(cwd, 0) || strings.TrimSpace(goal) == "" || !utf8.ValidString(goal) || strings.ContainsRune(goal, 0) || len(goal) > 8<<20 {
		return nil, fmt.Errorf("dsh execute requires a host request identity, absolute workspace and bounded goal")
	}
	execute, err := json.Marshal(struct {
		Version    int    `json:"v"`
		Type       string `json:"type"`
		RequestID  string `json:"request_id"`
		Cwd        string `json:"cwd"`
		Prompt     string `json:"prompt"`
		MCPServers []any  `json:"mcp_servers"`
	}{1, "execute", requestID, cwd, goal, []any{}})
	if err != nil || len(execute) > 8<<20 {
		return nil, fmt.Errorf("dsh execute exceeds the protocol command limit")
	}
	cancel, _ := json.Marshal(struct {
		Version   int    `json:"v"`
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
	}{1, "cancel", requestID})
	return newDuplexInput(execute, cancel), nil
}
