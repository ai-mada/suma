package runner

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	term "github.com/ai-mada/suma/internal/terminalcontract"
)

// TerminalExchange is an authenticated outgoing Runner request. Client.post
// retains the same private/Pool namespace, credential scope and TLS rules used
// by original assignment heartbeats; it never exposes a listening PTY port.
func (c *Client) TerminalExchange(ctx context.Context, request term.ExchangeRequest) (term.ExchangeResponse, error) {
	var reply term.ExchangeResponse
	if request.SessionID == "" || len(request.SessionID) > 128 || strings.ContainsAny(request.SessionID, "/\\\x00") || request.OperationID == "" {
		return reply, fmt.Errorf("Terminal exchange identity is invalid")
	}
	err := c.post(ctx, "terminal-sessions/"+url.PathEscape(request.SessionID)+"/exchange", request, &reply)
	if err == nil && (reply.SessionID != request.SessionID || reply.ControlEpoch < 1 || reply.OutputAckSequence < 0 || len(reply.Inputs) > 32 || reply.Columns < 2 || reply.Columns > 512 || reply.Rows < 1 || reply.Rows > 256) {
		err = fmt.Errorf("Terminal exchange response is invalid")
	}
	return reply, err
}
