package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ai-mada/suma/internal/releasecontract"
)

func releaseRequest(ctx context.Context, spec ExternalSpec, method, path string, body any) (releasecontract.DeploymentObservation, int, error) {
	var observation releasecontract.DeploymentObservation
	base, err := externalURL(spec)
	if err != nil {
		return observation, 0, err
	}
	token := os.Getenv(spec.TokenEnv)
	if token == "" {
		return observation, 0, fmt.Errorf("release provider worker credential is not configured")
	}
	var payload io.Reader
	if body != nil {
		data, e := json.Marshal(body)
		if e != nil {
			return observation, 0, e
		}
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, payload)
	if err != nil {
		return observation, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return observation, 0, fmt.Errorf("release provider request unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return observation, resp.StatusCode, fmt.Errorf("release provider rejected request: HTTP %d", resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&observation); err != nil {
		return observation, resp.StatusCode, fmt.Errorf("release provider returned invalid observation")
	}
	return observation, resp.StatusCode, nil
}

func validReleaseObservation(o releasecontract.DeploymentObservation, execution releasecontract.ReleaseExecution, requestHash string) error {
	hash, err := releasecontract.ObservationDigest(o)
	if err != nil || hash != o.EvidenceSHA256 || o.Protocol != execution.Protocol || o.OperationID != execution.OperationID || o.RequestSHA256 != requestHash || o.ProviderID != execution.ProviderID || o.PlanID != execution.PlanID || o.PlanRevision != execution.PlanRevision || o.PlanSHA256 != execution.PlanSHA256 || o.EnvironmentID != execution.EnvironmentID || o.Generation != execution.Generation || o.Version < 1 {
		return fmt.Errorf("release provider observation binding or digest mismatch")
	}
	return nil
}

func executeRelease(ctx context.Context, a Assignment, cfg ExecConfig) Result {
	r := Result{Assignment: a, ExitCode: -1}
	fail := func(message string, unknown bool) Result { r.Error = message; r.OutcomeUnknown = unknown; return r }
	if a.Type != "external_workflow" || len(a.Command) != 0 || cfg.Artifacts == nil {
		return fail("release execution requires a managed external assignment and control client", false)
	}
	execution := *a.ReleaseExecution
	request := releasecontract.DeploymentRequest{ReleaseExecution: execution, Assignment: assignmentRef(a)}
	requestHash, err := releasecontract.DeploymentRequestDigest(request)
	if err != nil {
		return fail(err.Error(), false)
	}
	spec, ok := cfg.External[execution.ProviderID]
	if !ok || spec.Protocol != releasecontract.ReleaseProtocol {
		return fail("release provider v2 is not configured", false)
	}
	if _, err = externalURL(spec); err != nil {
		return fail(err.Error(), false)
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
	path := "/v2/operations/" + url.PathEscape(execution.OperationID)
	observation, status, err := releaseRequest(runCtx, spec, http.MethodGet, path, nil)
	if err != nil && status != http.StatusNotFound {
		return fail("release inspect unavailable; original operation requires reconciliation", true)
	}
	authorizationRequired := status == http.StatusOK && observation.State == "received" && observation.Error != nil && observation.Error.Code == "authorization_required"
	if authorizationRequired {
		if err = validReleaseObservation(observation, execution, requestHash); err != nil {
			return fail(err.Error(), true)
		}
	}
	if status == http.StatusNotFound || authorizationRequired {
		var ticket releasecontract.ReleaseAuthorization
		err = cfg.Artifacts.post(runCtx, "release-authorizations", releasecontract.ReleaseAuthorizationRequest{ProjectID: cfg.Artifacts.ProjectID, Assignment: assignmentRef(a), OperationID: execution.OperationID}, &ticket)
		if err != nil {
			return fail("release authorization unavailable or rejected", false)
		}
		expires, e := time.Parse(time.RFC3339Nano, ticket.ExpiresAt)
		if e != nil || !expires.After(time.Now()) || ticket.AuthorizationTicket == "" || ticket.OperationID != execution.OperationID || ticket.RequestSHA256 != requestHash {
			return fail("release authorization binding is invalid", false)
		}
		request.AuthorizationTicket = ticket.AuthorizationTicket
		observation, _, err = releaseRequest(runCtx, spec, http.MethodPost, "/v2/deployments", request)
		request.AuthorizationTicket = ""
		ticket.AuthorizationTicket = ""
		if err != nil {
			observation, _, err = releaseRequest(runCtx, spec, http.MethodGet, path, nil)
			if err != nil {
				return fail("release dispatch outcome unknown; inspect original operation "+execution.OperationID, true)
			}
		}
	}
	lastVersion := 0
	lastHash := ""
	terminal := func(o releasecontract.DeploymentObservation) (bool, Result) {
		if e := validReleaseObservation(o, execution, requestHash); e != nil {
			return true, fail(e.Error(), true)
		}
		if o.Version < lastVersion || (o.Version == lastVersion && o.EvidenceSHA256 != lastHash) {
			return true, fail("release observation version conflict", true)
		}
		if o.Version > lastVersion && cfg.Progress != nil {
			if e := cfg.Progress(fmt.Sprintf("release %s: state=%s effect=%s health=%s generation=%d", execution.OperationID, o.State, o.Effect.State, o.Health.State, o.Generation)); e != nil {
				return true, fail("durable release progress unavailable", true)
			}
		}
		lastVersion = o.Version
		lastHash = o.EvidenceSHA256
		r.ReleaseEvidence = &releasecontract.ReleaseResult{OperationID: o.OperationID, RequestSHA256: o.RequestSHA256, EvidenceSHA256: o.EvidenceSHA256, Version: o.Version}
		r.Output = fmt.Sprintf("release operation %s: %s; effect=%s health=%s generation=%d", o.OperationID, o.State, o.Effect.State, o.Health.State, o.Generation)
		switch o.State {
		case "succeeded":
			if o.Effect.State != "applied" || o.Effect.ActiveGeneration != execution.Generation || o.Effect.ActiveArtifactSHA256 != execution.ArtifactSHA256 || o.Health.State != "passed" || o.Health.Generation != execution.Generation || o.Health.ArtifactSHA256 != execution.ArtifactSHA256 || o.FenceGeneration < execution.Generation {
				return true, fail("release success lacks matching active and health evidence", true)
			}
			r.ExitCode = 0
			return true, r
		case "failed":
			r.ExitCode = 1
			r.Error = "release provider operation failed"
			if o.Effect.State == "unknown" {
				r.OutcomeUnknown = true
			}
			return true, r
		case "cancelled":
			if o.Effect.State != "not_started" || o.FenceGeneration < execution.Generation {
				return true, fail("release cancellation has no confirmed fence", true)
			}
			r.Stopped = true
			r.Error = "release provider operation cancelled"
			return true, r
		case "reconciling":
			return true, fail("release provider outcome requires reconciliation", true)
		case "received", "authorized", "staging", "applying", "checking_health":
			return false, r
		default:
			return true, fail("release provider returned unknown state", true)
		}
	}
	poll := time.Duration(spec.PollMilliseconds) * time.Millisecond
	if poll <= 0 {
		poll = time.Second
	}
	if poll > 30*time.Second {
		poll = 30 * time.Second
	}
	for {
		if done, result := terminal(observation); done {
			return result
		}
		if !wait(runCtx, poll) {
			break
		}
		observation, _, err = releaseRequest(runCtx, spec, http.MethodGet, path, nil)
		if err != nil {
			return fail("release status unavailable; inspect original operation "+execution.OperationID, true)
		}
	}
	grace := time.Duration(spec.CancelWaitMilliseconds) * time.Millisecond
	if grace <= 0 {
		grace = 5 * time.Second
	}
	if grace > 30*time.Second {
		grace = 30 * time.Second
	}
	stopCtx, stop := context.WithTimeout(context.Background(), grace)
	defer stop()
	_, _, _ = releaseRequest(stopCtx, spec, http.MethodPost, path+"/cancel", map[string]string{"operation_id": artifactOperation(a, "release-cancel")})
	for stopCtx.Err() == nil {
		observation, _, err = releaseRequest(stopCtx, spec, http.MethodGet, path, nil)
		if err == nil {
			if done, result := terminal(observation); done {
				return result
			}
		}
		if !wait(stopCtx, poll) {
			break
		}
	}
	return fail("release cancellation unconfirmed; inspect original operation "+execution.OperationID, true)
}
