package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL, Token, ProjectID string
	HTTP                      *http.Client
	Pool                      *PoolContext
}
type runnerHTTPStatusError struct {
	Path   string
	Status int
}

func (e *runnerHTTPStatusError) Error() string {
	return fmt.Sprintf("runner %s rejected: HTTP %d", e.Path, e.Status)
}

func (c *Client) post(ctx context.Context, path string, payload, reply any) error {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("invalid control URL")
	}
	if u.Scheme == "http" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
		return fmt.Errorf("remote runner requires HTTPS")
	}
	if strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("runner bearer token required")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	namespace := "runner"
	if c.Pool != nil {
		namespace = "pool-runner"
		if b, err = c.poolPayload(b); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/api/v1/"+namespace+"/"+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &runnerHTTPStatusError{Path: path, Status: resp.StatusCode}
	}
	if reply != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(reply)
	}
	return nil
}
func (c *Client) Claim(ctx context.Context, worker string, capabilities []string) (*Assignment, error) {
	return c.ClaimWithOperation(ctx, worker, capabilities, operationID())
}
func (c *Client) ClaimWithOperation(ctx context.Context, worker string, capabilities []string, operation string) (*Assignment, error) {
	if strings.TrimSpace(operation) == "" {
		return nil, fmt.Errorf("claim operation is required")
	}
	var r struct {
		Assignment *Assignment `json:"assignment"`
	}
	err := c.post(ctx, "claim", map[string]any{"project_id": c.ProjectID, "worker_id": worker, "capabilities": capabilities, "idempotency_key": operation}, &r)
	return r.Assignment, err
}
func (c *Client) Heartbeat(ctx context.Context, a Assignment) (bool, error) {
	var r struct {
		CancelRequested bool `json:"cancel_requested"`
	}
	err := c.post(ctx, "heartbeat", map[string]any{"project_id": c.ProjectID, "assignment": a}, &r)
	return r.CancelRequested, err
}
func (c *Client) Complete(ctx context.Context, r Result) error {
	return c.post(ctx, "results", resultCallbackPayload(c.ProjectID, r), nil)
}
func (c *Client) Progress(ctx context.Context, p Progress) error {
	if p.HostOperation != nil {
		if p.Message != "" || p.ToolEvent != nil {
			return fmt.Errorf("workspace operation cannot mix SDK progress")
		}
		return c.post(ctx, "progress", map[string]any{"project_id": c.ProjectID, "assignment": p.Assignment, "operation_id": p.OperationID, "host_operation": p.HostOperation}, nil)
	}
	return c.post(ctx, "progress", map[string]any{"project_id": c.ProjectID, "assignment": p.Assignment, "operation_id": p.OperationID, "message": p.Message, "tool_event": p.ToolEvent}, nil)
}
func ReplayProgress(ctx context.Context, c *Client, j *Journal) error {
	if err := drainWorkspaceProgress(ctx, j, c.Progress); err != nil {
		return err
	}
	for _, p := range j.PendingProgress() {
		if p.HostOperation != nil {
			continue
		}
		if err := c.Progress(ctx, p); err != nil {
			return err
		}
		if err := j.AckProgress(p.OperationID); err != nil {
			return err
		}
	}
	return nil
}
func Replay(ctx context.Context, c *Client, j *Journal) error {
	if err := ReplaySteering(ctx, c, j); err != nil {
		return err
	}
	if err := ReplayToolInputObservations(ctx, c, j); err != nil {
		return err
	}
	if err := ReplayToolPermissionObservations(ctx, c, j); err != nil {
		return err
	}
	if err := ReplayTerminalExchanges(ctx, c, j); err != nil {
		return err
	}
	if err := drainWorkspaceProgress(ctx, j, c.Progress); err != nil {
		return err
	}
	if err := ReplayCandidates(ctx, c, j); err != nil {
		return err
	}
	if err := ReplayArtifacts(ctx, c, j); err != nil {
		return err
	}
	if err := ReplayProgress(ctx, c, j); err != nil {
		return err
	}
	for _, r := range j.Pending() {
		if err := c.Complete(ctx, r); err != nil {
			return err
		}
		if err := j.Ack(r.OperationID); err != nil {
			return err
		}
	}
	return nil
}
