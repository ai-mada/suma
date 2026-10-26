package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

// dshInput keeps stdin open after execute; EOF cancels the runtime's active run.
// Its only further command is a cancellation of the original host-owned request.
type dshInput struct {
	mu                sync.Mutex
	changed           *sync.Cond
	pending           []byte
	cancel            []byte
	cancelled, closed bool
}

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
	d := &dshInput{pending: append(execute, '\n'), cancel: append(cancel, '\n')}
	d.changed = sync.NewCond(&d.mu)
	return d, nil
}

func (d *dshInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for len(d.pending) == 0 && !d.closed {
		d.changed.Wait()
	}
	if d.closed {
		return 0, io.EOF
	}
	n := copy(p, d.pending)
	d.pending = d.pending[n:]
	return n, nil
}

func (d *dshInput) Cancel() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return io.ErrClosedPipe
	}
	if !d.cancelled {
		d.cancelled = true
		d.pending = append(d.pending, d.cancel...)
		d.changed.Broadcast()
	}
	return nil
}

func (d *dshInput) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	d.pending = nil
	d.changed.Broadcast()
	return nil
}
