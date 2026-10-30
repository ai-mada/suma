package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// Lookup compares immutable execution identity against the current Pool
// context. Lease and dispatch state are observations that may have advanced;
// the original request still carries both exactly as recorded in its journal.
// Create and Refresh retain their original strict cloudRequestScope checks.
func (c *Client) cloudLookupScope(project string, original cc.AssignmentRef) bool {
	if c == nil || project != c.ProjectID || !original.Valid() {
		return false
	}
	if c.Pool == nil {
		return c.cloudRequestScope(project, original)
	}
	fixed := c.Pool.Assignment
	if fixed == nil || c.Pool.PoolID != fixed.PoolID || c.Pool.RunnerID != fixed.PoolRunnerID || project != fixed.ProjectID || validPoolAssignment(*fixed) != nil {
		return false
	}
	comparison := candidateRef(*fixed)
	comparison.LeaseUntil = original.LeaseUntil
	comparison.DispatchState = original.DispatchState
	return comparison == original
}

// LookupWorkBuddyCloud reads only the original control-plane submission. No
// supplier request, idempotency key or replacement assignment is synthesized.
// Failure is never a not_found fact or permission to retry cloud creation.
func (c *Client) LookupWorkBuddyCloud(ctx context.Context, request wbc.CloudLookupRequest) (wbc.CloudLookupResult, error) {
	var empty wbc.CloudLookupResult
	if c == nil || ctx == nil || ctx.Err() != nil {
		return empty, cloudControlFailure("not_sent", 0, false)
	}
	if request.Validate() != nil || !c.cloudLookupScope(request.ProjectID, request.Assignment) {
		return empty, cloudControlFailure("invalid_request", 0, false)
	}
	origin, err := url.Parse(c.BaseURL)
	if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || origin.RawFragment != "" || (origin.Scheme != "https" && !(origin.Scheme == "http" && (origin.Hostname() == "localhost" || origin.Hostname() == "127.0.0.1" || origin.Hostname() == "::1"))) || strings.TrimSpace(c.Token) == "" {
		return empty, cloudControlFailure("configuration", 0, false)
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return empty, cloudControlFailure("invalid_request", 0, false)
	}
	namespace := "runner"
	if c.Pool != nil {
		namespace = "pool-runner"
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(bounded, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/api/v1/"+namespace+"/workbuddy-cloud-submissions/lookup", bytes.NewReader(raw))
	if err != nil {
		return empty, cloudControlFailure("not_sent", 0, false)
	}
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	client := http.Client{}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return empty, cloudControlFailure("transport", 0, true)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return empty, cloudControlFailure("http_status", response.StatusCode, true)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return empty, cloudControlFailure("invalid_response", 0, true)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(encoded) > 1<<20 {
		return empty, cloudControlFailure("invalid_response", 0, true)
	}
	var out wbc.CloudLookupResult
	if wbc.Decode(encoded, &out) != nil || out.Validate() != nil {
		return empty, cloudControlFailure("invalid_response", 0, true)
	}
	if out.ProjectID != request.ProjectID || out.DelegationID != request.DelegationID || out.Assignment != request.Assignment {
		return empty, cloudControlFailure("identity_mismatch", 0, true)
	}
	return out, nil
}
