package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	rc "github.com/ai-mada/suma/internal/releasecontract"
	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

type ExternalSpec struct {
	URL                    string `json:"url"`
	TokenEnv               string `json:"token_env"`
	PollMilliseconds       int    `json:"poll_milliseconds,omitempty"`
	CancelWaitMilliseconds int    `json:"cancel_wait_milliseconds,omitempty"`
	Protocol               string `json:"protocol,omitempty"`
}
type externalRun struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Output string `json:"output"`
	Error  string `json:"error"`
}

func externalURL(spec ExternalSpec) (string, error) {
	u, err := url.Parse(spec.URL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid external plugin URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return "", fmt.Errorf("external plugin requires HTTPS or loopback")
	}
	return strings.TrimRight(spec.URL, "/"), nil
}
func ExternalCapabilities(specs map[string]ExternalSpec) []string {
	caps := []string{}
	for name, spec := range specs {
		if _, err := externalURL(spec); err == nil && spec.TokenEnv != "" && os.Getenv(spec.TokenEnv) != "" {
			caps = append(caps, "external_workflow:"+name)
			if name != wbc.Plugin && rc.ValidProfileID(name) && spec.Protocol == rc.ReleaseProtocol {
				caps = append(caps, rc.ReleaseProviderCapabilityPrefix+name)
			}
		}
	}
	sort.Strings(caps)
	return caps
}
func externalRequest(ctx context.Context, spec ExternalSpec, method, path string, body any) (externalRun, error) {
	var result externalRun
	base, err := externalURL(spec)
	if err != nil {
		return result, err
	}
	token := os.Getenv(spec.TokenEnv)
	if token == "" {
		return result, fmt.Errorf("external plugin token is not configured")
	}
	var data io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return result, e
		}
		data = bytes.NewReader(b)
	}
	request, err := http.NewRequestWithContext(ctx, method, base+path, data)
	if err != nil {
		return result, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return result, fmt.Errorf("external plugin request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("external plugin HTTP %d", response.StatusCode)
	}
	if strings.HasSuffix(path, "/cancel") {
		return result, nil
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&result); err != nil {
		return result, fmt.Errorf("external plugin returned invalid JSON")
	}
	if result.ID == "" || len(result.ID) > 256 {
		return result, fmt.Errorf("external plugin returned invalid run ID")
	}
	return result, nil
}
func executeExternal(ctx context.Context, a Assignment, cfg ExecConfig) Result {
	result := Result{Assignment: a, ExitCode: -1}
	plugin, _ := a.Config["plugin"].(string)
	workflow, _ := a.Config["workflow"].(string)
	spec, ok := cfg.External[plugin]
	if !ok {
		result.Error = "external workflow plugin is not configured"
		return result
	}
	if spec.Protocol != "" {
		result.Error = "external release provider requires a server-managed release execution binding"
		return result
	}
	if workflow == "" || len(workflow) > 512 || len(a.Command) != 0 {
		result.Error = "external workflow requires workflow config and no local command"
		return result
	}
	if _, err := externalURL(spec); err != nil {
		result.Error = err.Error()
		return result
	}
	if spec.TokenEnv == "" || os.Getenv(spec.TokenEnv) == "" {
		result.Error = "external plugin token is not configured"
		return result
	}
	if ctx.Err() != nil {
		result.Error = ctx.Err().Error()
		return result
	}
	seconds := a.TimeoutSeconds
	if seconds <= 0 {
		seconds = 600
	}
	if seconds > 86400 {
		seconds = 86400
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	h := sha256.Sum256([]byte(key(a)))
	dispatch := hex.EncodeToString(h[:])
	state, err := externalRequest(runCtx, spec, "POST", "/runs", map[string]any{"dispatch_id": dispatch, "workflow": workflow, "input": a.Config["input"]})
	if err != nil {
		result.OutcomeUnknown = true
		result.Error = "external dispatch outcome unknown; reconcile by dispatch_id " + dispatch
		return result
	}
	terminal := func(s externalRun) bool {
		out := newOutput(cfg.OutputLimit, append(cfg.Secrets, os.Getenv(spec.TokenEnv)))
		_, _ = out.Write([]byte(s.Output))
		result.Output = out.String()
		switch s.State {
		case "succeeded":
			result.ExitCode = 0
			return true
		case "failed":
			result.ExitCode = 1
			result.Error = "external workflow failed"
			return true
		case "cancelled":
			result.Stopped = true
			result.Error = "external workflow cancelled"
			return true
		case "pending", "running", "queued":
			return false
		default:
			result.OutcomeUnknown = true
			result.Error = "external plugin returned unknown state; reconcile remote run " + s.ID
			return true
		}
	}
	poll := time.Duration(spec.PollMilliseconds) * time.Millisecond
	if poll <= 0 {
		poll = time.Second
	}
	if poll > 30*time.Second {
		poll = 30 * time.Second
	}
	id := state.ID
	for {
		if terminal(state) {
			return result
		}
		if !wait(runCtx, poll) {
			break
		}
		state, err = externalRequest(runCtx, spec, "GET", "/runs/"+url.PathEscape(id), nil)
		if err != nil {
			if runCtx.Err() != nil {
				break
			}
			result.OutcomeUnknown = true
			result.Error = "external status unavailable; reconcile remote run " + id
			return result
		}
		if state.ID != id {
			result.OutcomeUnknown = true
			result.Error = "external status run ID mismatch"
			return result
		}
	}
	// A cancellation acknowledgement alone is not evidence of a stopped workflow.
	grace := time.Duration(spec.CancelWaitMilliseconds) * time.Millisecond
	if grace <= 0 {
		grace = 5 * time.Second
	}
	if grace > 30*time.Second {
		grace = 30 * time.Second
	}
	cancelCtx, stop := context.WithTimeout(context.Background(), grace)
	defer stop()
	_, _ = externalRequest(cancelCtx, spec, "POST", "/runs/"+url.PathEscape(id)+"/cancel", map[string]any{"dispatch_id": dispatch})
	for cancelCtx.Err() == nil {
		state, err = externalRequest(cancelCtx, spec, "GET", "/runs/"+url.PathEscape(id), nil)
		if err == nil && state.ID == id && terminal(state) {
			return result
		}
		if !wait(cancelCtx, poll) {
			break
		}
	}
	result.Stopped = false
	result.OutcomeUnknown = true
	result.Error = "external cancellation unconfirmed; reconcile remote run " + id
	return result
}
