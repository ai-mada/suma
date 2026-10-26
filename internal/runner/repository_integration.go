package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

func repositoryRequest(ctx context.Context, spec ExternalSpec, method, path string, body any) (rc.RepositoryObservation, int, error) {
	var o rc.RepositoryObservation
	base, err := externalURL(spec)
	if err != nil {
		return o, 0, err
	}
	token := os.Getenv(spec.TokenEnv)
	if len(token) < 32 {
		return o, 0, fmt.Errorf("repository provider credential is unavailable")
	}
	var data io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return o, 0, e
		}
		data = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, data)
	if err != nil {
		return o, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	c := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return o, 0, fmt.Errorf("repository provider request unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return o, resp.StatusCode, fmt.Errorf("repository provider rejected request")
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&o); err != nil {
		return o, resp.StatusCode, fmt.Errorf("invalid repository provider observation")
	}
	return o, resp.StatusCode, nil
}
func executeRepositoryIntegration(ctx context.Context, a Assignment, cfg ExecConfig) Result {
	r := Result{Assignment: a, ExitCode: -1}
	fail := func(message string, unknown bool) Result { r.Error = message; r.OutcomeUnknown = unknown; return r }
	if a.RepositoryIntegration == nil || a.ReleaseExecution != nil || a.Type != "external_workflow" || len(a.Command) != 0 || a.Config["purpose"] != "repository_integration" || cfg.Artifacts == nil {
		return fail("repository integration requires its managed execution binding", false)
	}
	execution := *a.RepositoryIntegration
	request := rc.RepositoryRequest{RepositoryExecution: execution, Assignment: candidateRef(a)}
	hash, err := rc.RepositoryRequestDigest(request)
	if err != nil {
		return fail("repository integration binding is invalid", false)
	}
	spec, ok := cfg.External[execution.ProviderID]
	if !ok || spec.Protocol != rc.RepositoryProtocol {
		return fail("repository provider protocol is not configured", false)
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
	path := "/operations/" + url.PathEscape(execution.OperationID)
	o, status, err := repositoryRequest(runCtx, spec, "GET", path, nil)
	if err != nil && status != http.StatusNotFound {
		return fail("repository inspect unavailable; reconcile original operation", true)
	}
	if status != http.StatusNotFound && err == nil && (o.State == "unknown" || o.State == "applying" || execution.Phase == "apply" && o.State != "applied" && o.State != "not_applied") {
		return fail("repository operation requires original-operation reconciliation", true)
	}
	if status == http.StatusNotFound {
		var ticket rc.RepositoryAuthorization
		err = cfg.Artifacts.post(runCtx, "repository-integrations/authorize", rc.RepositoryAuthorizationRequest{ProjectID: execution.ProjectID, Assignment: request.Assignment, OperationID: execution.OperationID}, &ticket)
		if err != nil {
			return fail("repository authorization unavailable", execution.Phase == "apply")
		}
		expires, e := time.Parse(time.RFC3339Nano, ticket.ExpiresAt)
		if e != nil || !expires.After(time.Now()) || ticket.AuthorizationTicket == "" || ticket.OperationID != execution.OperationID || ticket.RequestSHA256 != hash {
			return fail("repository authorization binding is invalid", execution.Phase == "apply")
		}
		request.AuthorizationTicket = ticket.AuthorizationTicket
	}
	// POST also recovers/publishes the same original Git proof before completion.
	// Provider never repeats effects for an existing durable authorized intent.
	o, _, err = repositoryRequest(runCtx, spec, "POST", "/operations", request)
	request.AuthorizationTicket = ""
	if err != nil {
		o, _, err = repositoryRequest(runCtx, spec, "GET", path, nil)
		if err != nil {
			return fail("repository dispatch outcome unknown; reconcile original operation", true)
		}
		return fail("repository proof persistence requires original-operation reconciliation", true)
	}
	if err = rc.RepositoryValidateObservation(o, execution, hash); err != nil {
		return fail("repository observation proof is invalid", true)
	}
	r.IntegrationEvidence = &rc.RepositoryResult{OperationID: o.OperationID, RequestSHA256: o.RequestSHA256, ObservationSHA256: o.ObservationSHA256, Version: o.Version}
	r.Output = fmt.Sprintf("repository operation %s: %s", o.OperationID, o.State)
	switch o.State {
	case "prepared":
		if execution.Phase != "prepare" {
			return fail("repository operation phase mismatch", true)
		}
		r.ExitCode = 0
		return r
	case "applied":
		if execution.Phase != "apply" {
			return fail("repository operation phase mismatch", true)
		}
		r.ExitCode = 0
		return r
	case "not_applied":
		r.ExitCode = 1
		r.Error = "repository operation was confirmed not applied"
		return r
	case "blocked":
		if execution.Phase == "prepare" {
			r.ExitCode = 1
			r.Error = "repository preparation was blocked"
			return r
		}
	}
	return fail("repository operation requires reconciliation", true)
}
