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

// The Runner talks only to Suma with its original machine identity. OAuth and
// ACP credentials are not part of this protocol, and no local Create fallback
// is permitted if the control plane cannot confirm its durable operation.
type workBuddyCloudControlError struct {
	Code           string
	StatusCode     int
	OutcomeUnknown bool
}

func (e *workBuddyCloudControlError) Error() string {
	return "WorkBuddy cloud control request could not be confirmed"
}

func cloudControlFailure(code string, status int, unknown bool) error {
	return &workBuddyCloudControlError{Code: code, StatusCode: status, OutcomeUnknown: unknown}
}

func (c *Client) cloudRequestScope(project string, a cc.AssignmentRef) bool {
	if c == nil || project != c.ProjectID || !a.Valid() {
		return false
	}
	if c.Pool != nil {
		fixed := c.Pool.Assignment
		return fixed != nil && c.Pool.PoolID == fixed.PoolID && c.Pool.RunnerID == fixed.PoolRunnerID && validPoolAssignment(*fixed) == nil && candidateRef(*fixed) == a && project == fixed.ProjectID
	}
	// Private project callbacks must never strip a submitted Pool identity and
	// then use the more permissive private Runner namespace.
	return a.TenantID == "" && a.ProjectID == "" && a.PoolID == "" && a.PoolRunnerID == "" && a.PoolSlotID == "" && a.PoolGrantID == "" && a.PoolGrantVersion == 0 && a.PoolCredentialID == "" && a.PoolCredentialVersion == 0 && a.DispatchID == "" && a.DispatchState == "" && a.LeaseUntil == ""
}

func (c *Client) workBuddyCloudPost(ctx context.Context, path string, request any) (wbc.CloudSubmission, error) {
	var empty wbc.CloudSubmission
	if c == nil || ctx == nil || ctx.Err() != nil {
		return empty, cloudControlFailure("not_sent", 0, false)
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) || strings.TrimSpace(c.Token) == "" {
		return empty, cloudControlFailure("configuration", 0, false)
	}
	body, err := json.Marshal(request)
	if err != nil {
		return empty, cloudControlFailure("invalid_request", 0, false)
	}
	namespace := "runner"
	if c.Pool != nil {
		namespace = "pool-runner"
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(bounded, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/api/v1/"+namespace+"/"+path, bytes.NewReader(body))
	if err != nil {
		return empty, cloudControlFailure("not_sent", 0, false)
	}
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	hc := http.Client{}
	if c.HTTP != nil {
		hc = *c.HTTP
	}
	hc.Jar = nil
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := hc.Do(req)
	if err != nil {
		return empty, cloudControlFailure("transport", 0, true)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		// A rejected/unknown command never grants a second create permit. Read
		// the original control-plane record before deciding any next action.
		return empty, cloudControlFailure("http_status", response.StatusCode, true)
	}
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return empty, cloudControlFailure("invalid_response", 0, true)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return empty, cloudControlFailure("invalid_response", 0, true)
	}
	var out wbc.CloudSubmission
	if wbc.Decode(raw, &out) != nil || out.Validate() != nil || out.State != "task_known" {
		return empty, cloudControlFailure("invalid_response", 0, true)
	}
	return out, nil
}

// CreateWorkBuddyCloud submits one stable command to the authoritative control
// plane. It does not call the vendor, retry or turn a provider status into a
// successful Runner result. All returned identities must match this command.
func (c *Client) CreateWorkBuddyCloud(ctx context.Context, request wbc.CloudCreateRequest) (wbc.CloudSubmission, error) {
	if request.Validate() != nil || !c.cloudRequestScope(request.ProjectID, request.Assignment) {
		return wbc.CloudSubmission{}, cloudControlFailure("invalid_request", 0, false)
	}
	out, err := c.workBuddyCloudPost(ctx, "workbuddy-cloud-submissions", request)
	if err != nil {
		return wbc.CloudSubmission{}, err
	}
	if out.ProjectID != request.ProjectID || out.DelegationID != request.DelegationID || out.Assignment != request.Assignment || out.RunID != request.Assignment.RunID || out.NodeID != request.Assignment.NodeID {
		return wbc.CloudSubmission{}, cloudControlFailure("identity_mismatch", 0, true)
	}
	return out, nil
}

// RefreshWorkBuddyCloud reads only the task already known in original. The
// provider task ID is never sent as authority or substituted from a response.
func (c *Client) RefreshWorkBuddyCloud(ctx context.Context, original wbc.CloudSubmission, request wbc.CloudRefreshRequest) (wbc.CloudSubmission, error) {
	if original.Validate() != nil || original.State != "task_known" || request.Validate() != nil || request.ExpectedVersion != original.Version || request.ProjectID != original.ProjectID || request.Assignment != original.Assignment || !c.cloudRequestScope(request.ProjectID, request.Assignment) {
		return wbc.CloudSubmission{}, cloudControlFailure("invalid_request", 0, false)
	}
	out, err := c.workBuddyCloudPost(ctx, "workbuddy-cloud-submissions/"+url.PathEscape(original.ID)+"/refresh", request)
	if err != nil {
		return wbc.CloudSubmission{}, err
	}
	if out.ID != original.ID || out.ProjectID != original.ProjectID || out.DelegationID != original.DelegationID || out.Assignment != original.Assignment || out.RequestSHA256 != original.RequestSHA256 || out.RunID != original.RunID || out.NodeID != original.NodeID || out.Task == nil || original.Task == nil || out.Task.ID != original.Task.ID || out.Version <= original.Version {
		return wbc.CloudSubmission{}, cloudControlFailure("identity_mismatch", 0, true)
	}
	return out, nil
}
