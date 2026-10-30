// Package workbuddyacp provides a bounded, explicit ACP transport. It does not
// grant execution authority, retry requests or infer vendor completion.
package workbuddyacp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/workbuddy"
)

const maxFrameBytes = 1 << 20
const headerWait = 10 * time.Second
const postWait = 30 * time.Second

// Frame is transient provider data, never a safe diagnostic or execution fact.
type Frame struct {
	Data      []byte `json:"-"`
	Event, ID string `json:"-"`
	source    *Connection
}

func (Frame) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddyacp.Frame{payload:redacted}")
}

type Connection struct {
	ctx                            context.Context
	cancel                         context.CancelFunc
	client                         *http.Client
	body                           io.ReadCloser
	endpoint, ticket, connectionID string
	expiresAt                      *time.Time
	frames                         chan Frame
	done                           chan struct{}
	closeOnce                      sync.Once
	mu                             sync.Mutex
	closed                         bool
	terminal                       error
}

func (*Connection) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddyacp.Connection{credentials:redacted}")
}

func safeID(id string) bool {
	if len(id) == 0 || len(id) > 200 {
		return false
	}
	for _, b := range []byte(id) {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-') {
			return false
		}
	}
	return true
}
func safeTicket(ticket string) bool {
	if len(ticket) == 0 || len(ticket) > 8192 {
		return false
	}
	base := strings.TrimRight(ticket, "=")
	if base == "" {
		return false
	}
	for _, b := range []byte(base) {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("-._~+/", rune(b))) {
			return false
		}
	}
	return true
}
func singleConnectionID(header http.Header) (string, bool) {
	values := header.Values("Acp-Connection-Id")
	if len(values) != 1 || len(values[0]) == 0 || len(values[0]) > 512 {
		return "", false
	}
	for _, b := range []byte(values[0]) {
		if b < 33 || b > 126 || b == ',' {
			return "", false
		}
	}
	return values[0], true
}
func transportClient(supplied *http.Client) *http.Client {
	client := http.Client{}
	if supplied != nil {
		client = *supplied
	}
	client.Timeout = 0 // SSE lifetime belongs to the caller, not a whole-response timer.
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if tr, ok := base.(*http.Transport); ok {
		copy := tr.Clone()
		copy.ResponseHeaderTimeout = headerWait
		copy.TLSHandshakeTimeout = headerWait
		client.Transport = copy
	} else {
		client.Transport = base
	}
	return &client
}

// Connect accepts only the exact documented gateway URL. A trusted injected
// RoundTripper may provide a local fixture without broadening this allowlist.
func Connect(ctx context.Context, access workbuddy.SessionAccess, client *http.Client) (*Connection, error) {
	if ctx == nil || !access.Available || !safeID(access.Task.ID) || access.SessionID != access.Task.ID || access.ACPURL != "https://acp.workbuddy.cn/sessions/"+access.Task.ID || !safeTicket(access.Ticket) || access.ExpiresAt != nil && !time.Now().Before(*access.ExpiresAt) {
		return nil, failure("invalid_access", 0, false)
	}
	if ctx.Err() != nil {
		return nil, failure("cancelled", 0, false)
	}
	lifetime, cancel := context.WithCancel(ctx)
	c := &Connection{ctx: lifetime, cancel: cancel, client: transportClient(client), endpoint: access.ACPURL, ticket: access.Ticket, frames: make(chan Frame, 32), done: make(chan struct{})}
	if access.ExpiresAt != nil {
		expiry := *access.ExpiresAt
		c.expiresAt = &expiry
	}
	req, err := http.NewRequestWithContext(lifetime, http.MethodGet, c.endpoint, nil)
	if err != nil {
		cancel()
		return nil, failure("invalid_access", 0, false)
	}
	req.Header.Set("Authorization", "Bearer "+c.ticket)
	req.Header.Set("Accept", "text/event-stream")
	timer := time.AfterFunc(headerWait, cancel)
	response, err := c.client.Do(req)
	timer.Stop()
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		cancel()
		return nil, failure("transport", 0, false)
	}
	media, _, parseErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	id, valid := singleConnectionID(response.Header)
	if response.StatusCode != http.StatusOK || parseErr != nil || media != "text/event-stream" || !valid || response.Body == nil || lifetime.Err() != nil || c.expiresAt != nil && !time.Now().Before(*c.expiresAt) {
		if response.Body != nil {
			response.Body.Close()
		}
		cancel()
		code := "invalid_response"
		if response.StatusCode != http.StatusOK {
			code = "http_status"
		}
		return nil, failure(code, response.StatusCode, false)
	}
	c.body = response.Body
	c.connectionID = id
	go c.readFrames()
	return c, nil
}

func (c *Connection) unavailable() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return failure("closed", 0, false)
	}
	if c.terminal != nil {
		return c.terminal
	}
	if c.ctx.Err() != nil {
		return failure("cancelled", 0, false)
	}
	if c.expiresAt != nil && !time.Now().Before(*c.expiresAt) {
		return failure("invalid_access", 0, false)
	}
	return nil
}

// send means HTTP acceptance only. Matching JSON-RPC replies arrive via Receive.
// No automatic retries, cancellation RPC, token refresh or reconnect is issued.
func (c *Connection) send(ctx context.Context, rawJSON []byte) error {
	if c == nil || ctx == nil || len(rawJSON) == 0 || len(rawJSON) > maxFrameBytes || !utf8.Valid(rawJSON) || !json.Valid(rawJSON) {
		return failure("invalid_request", 0, false)
	}
	if ctx.Err() != nil {
		return failure("cancelled", 0, false)
	}
	if err := c.unavailable(); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, postWait)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	// An opaque read-closer deliberately prevents net/http from constructing a
	// GetBody replay hook for the POST. Never attach an idempotency header.
	copy := bytes.Clone(rawJSON)
	req, err := http.NewRequestWithContext(bounded, http.MethodPost, c.endpoint, io.NopCloser(bytes.NewReader(copy)))
	if err != nil {
		return failure("invalid_request", 0, false)
	}
	req.ContentLength = int64(len(copy))
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+c.ticket)
	req.Header.Set("Acp-Connection-Id", c.connectionID)
	req.Header.Set("Content-Type", "application/json")
	if bounded.Err() != nil {
		return failure("cancelled", 0, false)
	}
	response, err := c.client.Do(req)
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		return failure("transport", 0, true)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return failure("http_status", response.StatusCode, true)
	}
	return nil
}

func (c *Connection) Receive(ctx context.Context) (Frame, error) {
	if c == nil || ctx == nil {
		return Frame{}, failure("invalid_request", 0, false)
	}
	if ctx.Err() != nil {
		c.Close()
		return Frame{}, failure("cancelled", 0, false)
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return Frame{}, failure("closed", 0, false)
	}
	select {
	case <-ctx.Done():
		c.Close()
		return Frame{}, failure("cancelled", 0, false)
	case <-c.ctx.Done():
		c.Close()
		return Frame{}, failure("cancelled", 0, false)
	case frame, ok := <-c.frames:
		if ok {
			return frame, nil
		}
		c.mu.Lock()
		err := c.terminal
		c.mu.Unlock()
		if err == nil {
			err = failure("stream_incomplete", 0, false)
		}
		return Frame{}, err
	}
}

func (c *Connection) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() { c.mu.Lock(); c.closed = true; c.mu.Unlock(); c.cancel(); c.body.Close() })
	<-c.done
	return nil
}

// splitSSELine handles LF, CRLF and bare CR, including a CR at a buffer edge.
func splitSSELine(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' {
			return i + 1, data[:i], nil
		}
		if b == '\r' {
			if i+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			end := i + 1
			if end < len(data) && data[end] == '\n' {
				end++
			}
			return end, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
func (c *Connection) readFrames() {
	terminal := failure("stream_incomplete", 0, false)
	defer func() {
		c.body.Close()
		c.mu.Lock()
		c.terminal = terminal
		c.mu.Unlock()
		close(c.frames)
		close(c.done)
	}()
	scanner := bufio.NewScanner(c.body)
	scanner.Buffer(make([]byte, 4096), maxFrameBytes+2)
	scanner.Split(splitSSELine)
	var data []byte
	event, id := "", ""
	hasData, first := false, true
	for scanner.Scan() {
		line := scanner.Bytes()
		if !utf8.Valid(line) {
			terminal = failure("frame_invalid", 0, false)
			return
		}
		if first {
			line = bytes.TrimPrefix(line, []byte{0xef, 0xbb, 0xbf})
			first = false
		}
		if len(line) == 0 {
			if hasData {
				name := event
				if name == "" {
					name = "message"
				}
				frame := Frame{Data: bytes.Clone(data[:len(data)-1]), Event: name, ID: id, source: c}
				select {
				case c.frames <- frame:
				case <-c.ctx.Done():
					terminal = failure("cancelled", 0, false)
					return
				}
			}
			data = nil
			event = ""
			hasData = false
			continue
		}
		if line[0] == ':' {
			continue
		}
		field, value, found := bytes.Cut(line, []byte{':'})
		if !found {
			value = nil
		}
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
		switch string(field) {
		case "data":
			if len(data)+len(value)+1 > maxFrameBytes {
				terminal = failure("frame_limit", 0, false)
				return
			}
			data = append(data, value...)
			data = append(data, '\n')
			hasData = true
		case "event":
			if len(value) > 4096 {
				terminal = failure("frame_limit", 0, false)
				return
			}
			event = string(value)
		case "id":
			if bytes.IndexByte(value, 0) < 0 {
				if len(value) > 4096 {
					terminal = failure("frame_limit", 0, false)
					return
				}
				id = string(value)
			}
		}
		size := len(data) + len(event) + len(id)
		if hasData {
			size-- // The final data newline is removed at dispatch.
			if event == "" {
				size += len("message")
			}
		}
		if size > maxFrameBytes {
			terminal = failure("frame_limit", 0, false)
			return
		}
	}
	if scanner.Err() != nil {
		terminal = failure("stream_incomplete", 0, false)
	}
}
