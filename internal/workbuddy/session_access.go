package workbuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const acpGatewayOrigin = "https://acp.workbuddy.cn"

// SessionAccess is private connection material read for one exact cloud task.
// Available means only that both validated link and ticket were returned. It
// does not establish current Suma authorization, connectivity, validity at use
// time, task success, or health. Callers must recheck their authority and any
// known expiry before consuming the ticket. A nil ExpiresAt is unknown, never
// an inferred lifetime or a promise that the ticket does not expire.
type SessionAccess struct {
	Task      Task       `json:"task"`
	SessionID string     `json:"session_id"`
	Available bool       `json:"available"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	ACPURL    string     `json:"-"`
	Ticket    string     `json:"-"`
}

func (SessionAccess) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddy.SessionAccess{credentials:redacted}")
}

// GetSessionAccess performs only the existing authenticated, fixed task GET.
// It never contacts a returned endpoint, polls, renews OAuth credentials, or
// makes the task executable. sandboxLink/sandboxDataLink are deliberately not
// interpreted as ACP endpoints or artifact authorities.
func (c *Client) GetSessionAccess(ctx context.Context, taskID string) (SessionAccess, error) {
	if !validTaskID(taskID) {
		return SessionAccess{}, localError("invalid_request")
	}
	raw, err := c.request(ctx, http.MethodGet, "/"+taskID, nil, http.StatusOK)
	if err != nil {
		return SessionAccess{}, err
	}
	task, err := decodeTask(raw)
	if err != nil || task.ID != taskID {
		return SessionAccess{}, localError("invalid_response")
	}
	fields, err := object(raw)
	if err != nil {
		return SessionAccess{}, err
	}
	link, err := stringField(fields, "link", false, 2048)
	if err != nil {
		return SessionAccess{}, err
	}
	ticket, err := stringField(fields, "token", false, 8192)
	if err != nil || ticket != "" && !validToken(ticket) {
		return SessionAccess{}, localError("invalid_response")
	}
	if link != "" {
		if err = validateSessionEndpoint(link, taskID); err != nil {
			return SessionAccess{}, err
		}
	}
	out := SessionAccess{Task: task, SessionID: taskID}
	if rawExpiry, present := fields["expire_at"]; present {
		var seconds int64
		// UTC years 1..9999 are shared by the JSON date representation. This is
		// a local representation boundary, not a guessed vendor ticket lifetime.
		if json.Unmarshal(rawExpiry, &seconds) != nil || seconds <= 0 || seconds > 253402300799 {
			return SessionAccess{}, localError("invalid_response")
		}
		expiry := time.Unix(seconds, 0).UTC()
		out.ExpiresAt = &expiry
	}
	if link != "" && ticket != "" {
		out.Available, out.ACPURL, out.Ticket = true, link, ticket
	}
	return out, nil
}

func validateSessionEndpoint(raw, taskID string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Opaque != "" || u.User != nil || u.Host == "" || u.Port() != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || strings.ContainsAny(raw, "\\ \t\r\n") {
		return localError("invalid_response")
	}
	if u.Host != "acp.workbuddy.cn" {
		// The provider also documents one direct sandbox example, but has not
		// specified its complete trusted origin set. No wildcard is inferred.
		return localError("unsupported_session_endpoint")
	}
	if raw != acpGatewayOrigin+"/sessions/"+taskID {
		return localError("invalid_response")
	}
	return nil
}
