package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type PoolRunnerSession struct {
	Runner struct {
		ID                  string   `json:"id"`
		PoolID              string   `json:"pool_id"`
		WorkerID            string   `json:"worker_id"`
		MaxSlots            int      `json:"max_slots"`
		CredentialVersion   int      `json:"credential_version"`
		Name                string   `json:"name,omitempty"`
		Version             int      `json:"version,omitempty"`
		Status              string   `json:"status,omitempty"`
		CredentialExpiresAt string   `json:"credential_expires_at,omitempty"`
		LastSeenAt          string   `json:"last_seen_at,omitempty"`
		AllowedActions      []string `json:"allowed_actions,omitempty"`
		CreatedAt           string   `json:"created_at,omitempty"`
	} `json:"runner"`
	PoolActive bool `json:"pool_active"`
	Scopes     []struct {
		TenantID      string `json:"tenant_id"`
		ProjectID     string `json:"project_id"`
		PoolID        string `json:"pool_id,omitempty"`
		GrantID       string `json:"grant_id,omitempty"`
		GrantVersion  int    `json:"grant_version,omitempty"`
		ProjectWeight int    `json:"project_weight,omitempty"`
		Status        string `json:"status,omitempty"`
	} `json:"scopes"`
}

func (c *Client) PoolSession(ctx context.Context) (PoolRunnerSession, error) {
	var out PoolRunnerSession
	if c.Pool == nil || c.Pool.RunnerID == "" || c.Pool.PoolID == "" || c.Token == "" {
		return out, fmt.Errorf("explicit pool machine identity required")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host == "" || u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return out, fmt.Errorf("pool runner requires a secure control origin")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/api/v1/pool-runner/session?runner_id="+url.QueryEscape(c.Pool.RunnerID), nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * 1e9, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	r, err := hc.Do(req)
	if err != nil {
		return out, fmt.Errorf("pool session unavailable")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return out, &runnerHTTPStatusError{Path: "pool-runner/session", Status: r.StatusCode}
	}
	raw, readErr := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
	if readErr != nil || len(raw) > 2<<20 {
		return out, fmt.Errorf("invalid bounded Pool session response")
	}
	out, err = decodePoolSession(raw)
	if err != nil {
		return out, fmt.Errorf("invalid pool session response")
	}
	if !validPoolLocalID(out.Runner.ID) || !validPoolLocalID(out.Runner.PoolID) || out.Runner.ID != c.Pool.RunnerID || out.Runner.PoolID != c.Pool.PoolID || out.Runner.WorkerID != "pool-"+out.Runner.ID || out.Runner.MaxSlots < 1 || out.Runner.MaxSlots > 128 || out.Runner.CredentialVersion < 1 || len(out.Scopes) > 256 {
		return out, fmt.Errorf("pool session identity or capacity mismatch")
	}
	return out, nil
}

func (c *Client) ClaimPool(ctx context.Context, request PoolClaimRequest) (*Assignment, error) {
	if c.Pool == nil || c.Pool.Assignment != nil || request.PoolID != c.Pool.PoolID || request.RunnerID != c.Pool.RunnerID {
		return nil, fmt.Errorf("pool claim identity mismatch")
	}
	var response struct {
		Assignment *Assignment `json:"assignment"`
	}
	if err := c.post(ctx, "claim", request, &response); err != nil {
		return nil, err
	}
	if a := response.Assignment; a != nil {
		if err := validPoolAssignment(*a); err != nil {
			return nil, err
		}
		if a.PoolID != request.PoolID || a.PoolRunnerID != request.RunnerID || a.PoolSlotID != request.SlotID || a.WorkerID != request.WorkerID {
			return nil, fmt.Errorf("pool dispatch identity differs from pending claim")
		}
		found := false
		for _, scope := range request.Scopes {
			if scope.TenantID == a.TenantID && scope.ProjectID == a.ProjectID {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("pool dispatch outside declared local scopes")
		}
	}
	return response.Assignment, nil
}

func (c *Client) forPoolAssignment(a Assignment) (*Client, error) {
	if c.Pool == nil || a.PoolID != c.Pool.PoolID || a.PoolRunnerID != c.Pool.RunnerID {
		return nil, fmt.Errorf("pool checkpoint belongs to another execution identity")
	}
	if err := validPoolAssignment(a); err != nil {
		return nil, err
	}
	copy := *c
	pool := *c.Pool
	pool.Assignment = &a
	copy.Pool = &pool
	copy.ProjectID = a.ProjectID
	return &copy, nil
}

func replayPool(ctx context.Context, c *Client, j *Journal) error {
	if err := ReplaySteering(ctx, c, j); err != nil {
		return err
	}
	if err := ReplayToolInputObservations(ctx, c, j); err != nil {
		return err
	}
	if err := ReplayTerminalExchanges(ctx, c, j); err != nil {
		return err
	}
	if err := drainWorkspaceProgress(ctx, j, func(ctx context.Context, p Progress) error {
		scoped, err := c.forPoolAssignment(p.Assignment)
		if err != nil {
			return err
		}
		return scoped.Progress(ctx, p)
	}); err != nil {
		return err
	}
	if err := ReplayCandidates(ctx, c, j); err != nil {
		return err
	}
	if len(j.pendingArtifacts()) != 0 || len(j.pendingReleases()) != 0 {
		return fmt.Errorf("pool artifact/deployment recovery requires an explicit independent capability")
	}
	if err := replayStandardProgress(ctx, j, func(ctx context.Context, p Progress) error {
		scoped, err := c.forPoolAssignment(p.Assignment)
		if err != nil {
			return err
		}
		return scoped.Progress(ctx, p)
	}); err != nil {
		return err
	}
	return replayResults(ctx, j, func(ctx context.Context, r Result) error {
		scoped, err := c.forPoolAssignment(r.Assignment)
		if err != nil {
			return err
		}
		return scoped.Complete(ctx, r)
	})
}
