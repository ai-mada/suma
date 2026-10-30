package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func cloudNode(a Assignment) bool {
	return a.Type == wbc.NodeType && a.Config["plugin"] == wbc.Plugin && a.Config["workflow"] == wbc.Protocol
}

func cloudReserved(a Assignment) bool { return a.Config["plugin"] == wbc.Plugin }

func cloneCloudAssignment(a Assignment) Assignment {
	if a.WorkBuddyCloud == nil {
		return a
	}
	b := *a.WorkBuddyCloud
	a.WorkBuddyCloud = &b
	raw, err := json.Marshal(a.Config)
	if err == nil {
		var config map[string]any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if d.Decode(&config) == nil {
			a.Config = config
		}
	}
	return a
}

// Capabilities describe native protocol support, never supplier credentials.
func runnerCapabilities(cfg ExecConfig, inventory []string) []string {
	out := append([]string{}, inventory...)
	for _, capability := range ExternalCapabilities(cfg.External) {
		if capability != "external_workflow:"+wbc.Plugin {
			out = append(out, capability)
		}
	}
	return append(out, "external_workflow:"+wbc.Plugin)
}

// Keep the strict execution client unchanged. Only a matching current carrier
// may be copied back to the persisted original lease/state for original-body replay.
func originalCloudClientScope(c *Client, a Assignment) (*Client, error) {
	if c == nil {
		return nil, fmt.Errorf("cloud control client unavailable")
	}
	copy := *c
	if c.Pool == nil {
		if a.PoolID != "" {
			return nil, fmt.Errorf("cloud assignment scope changed")
		}
		return &copy, nil
	}
	if c.Pool.Assignment == nil || !wbc.SameAssignmentIdentity(candidateRef(*c.Pool.Assignment), candidateRef(a)) {
		return nil, fmt.Errorf("cloud assignment scope changed")
	}
	return c.forPoolAssignment(a)
}

func cloudFailureCause(err error, fallback string) string {
	var status *workBuddyCloudControlError
	if errors.As(err, &status) && (status.StatusCode == 401 || status.StatusCode == 403 || status.StatusCode == 404) {
		return "authorization_changed"
	}
	var runnerStatus *runnerHTTPStatusError
	if errors.As(err, &runnerStatus) && (runnerStatus.Status == 401 || runnerStatus.Status == 403 || runnerStatus.Status == 404) {
		return "authorization_changed"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "interrupted"
	}
	return fallback
}

func cloudWait(a Assignment, checkpoint cloudExecutionCheckpoint, cause string) Result {
	e := &wbc.CloudWaitEvidence{Protocol: wbc.Protocol, ClaimID: a.DispatchID, Assignment: checkpoint.Create.Assignment,
		Binding: checkpoint.Binding, CreateOperationID: checkpoint.Create.IdempotencyKey, Cause: cause}
	if checkpoint.Submission != nil {
		e.SubmissionID, e.SubmissionVersion = checkpoint.Submission.ID, checkpoint.Submission.Version
	}
	return Result{Assignment: a, ExitCode: -1, Error: "cloud execution requires authoritative observation: " + cause,
		OperationID: checkpoint.ResultOperationID, OutcomeUnknown: true, Stopped: false, WorkBuddyCloudWait: e}
}

func cloudObservationCause(s wbc.CloudSubmission) string {
	if s.State != "task_known" || s.Task == nil {
		return "remote_unknown"
	}
	switch strings.ToLower(s.Task.Status) {
	case "completed":
		return "delivery_required"
	case "failed", "cancelled":
		return "remote_unknown"
	case "waiting_input":
		return "waiting_input"
	default:
		return ""
	}
}

func executeWorkBuddyCloud(ctx context.Context, a Assignment, cfg ExecConfig) Result {
	a = cloneCloudAssignment(a)
	invalid := Result{Assignment: a, ExitCode: -1, Error: "cloud execution requires its protected original assignment and durable journal"}
	if !cloudNode(a) || a.WorkBuddyCloud == nil || a.WorkBuddyCloud.Validate() != nil || len(a.Command) != 0 || a.DispatchID == "" || cfg.Journal == nil || cfg.CloudClient == nil {
		return invalid
	}
	client, err := originalCloudClientScope(cfg.CloudClient, a)
	if err != nil {
		return invalid
	}
	p, err := cfg.Journal.beginCloudExecution(client, a)
	if err != nil {
		invalid.Error = "cloud checkpoint unavailable; original execution retained"
		return invalid
	}
	finish := func(cause string) Result { return cloudWait(a, p, cause) }
	if ctx == nil || ctx.Err() != nil {
		return finish("interrupted")
	}
	window := cfg.CloudObserveWindow
	if window <= 0 || window > 5*time.Minute {
		window = 30 * time.Second
	}
	if a.TimeoutSeconds > 0 && time.Duration(a.TimeoutSeconds)*time.Second < window {
		window = time.Duration(a.TimeoutSeconds) * time.Second
	}
	bounded, cancel := context.WithTimeout(ctx, window)
	defer cancel()
	lookup, err := client.LookupWorkBuddyCloud(bounded, wbc.CloudLookupRequest{ProjectID: p.ProjectID, DelegationID: p.Binding.DelegationID, Assignment: p.Create.Assignment})
	if err != nil {
		return finish(cloudFailureCause(err, "creation_unconfirmed"))
	}
	if lookup.Submission != nil {
		if err = cfg.Journal.recordCloudSubmission(a, *lookup.Submission, false); err != nil {
			return finish("creation_unconfirmed")
		}
		p, err = cfg.Journal.cloudExecution(a)
		if err != nil {
			return invalid
		}
	}
	if lookup.State == "prepared" || lookup.State == "unknown" {
		return finish("creation_unconfirmed")
	}
	if lookup.State == "not_found" {
		if p.Submission != nil {
			return finish("remote_unknown")
		}
		// Absence is not authority: the original dispatch must still be live.
		stop, authorityErr := client.Heartbeat(bounded, a)
		if authorityErr != nil {
			return finish(cloudFailureCause(authorityErr, "creation_unconfirmed"))
		}
		if stop {
			return finish("interrupted")
		}
		s, createErr := client.CreateWorkBuddyCloud(bounded, p.Create)
		if createErr != nil {
			return finish(cloudFailureCause(createErr, "creation_unconfirmed"))
		}
		if err = cfg.Journal.recordCloudSubmission(a, s, false); err != nil {
			return finish("creation_unconfirmed")
		}
		p.Submission = &s
	}
	if p.Submission == nil || p.Submission.State != "task_known" {
		return finish("remote_unknown")
	}
	lastProgress := ""
	for {
		if p.Refresh == nil {
			if cause := cloudObservationCause(*p.Submission); cause != "" {
				return finish(cause)
			}
		}
		if cfg.Progress != nil {
			fact := fmt.Sprintf("cloud task observation: submission=%s version=%d", p.Submission.ID, p.Submission.Version)
			if fact != lastProgress {
				if err = cfg.Progress(fact); err != nil {
					return finish("observation_unconfirmed")
				}
				lastProgress = fact
			}
		}
		poll := cfg.CloudPollInterval
		if poll <= 0 || poll > 30*time.Second {
			poll = 2 * time.Second
		}
		if p.Refresh == nil && !wait(bounded, poll) {
			return finish("interrupted")
		}
		next, refreshPrepareErr := cfg.Journal.beginCloudRefresh(a)
		if refreshPrepareErr != nil {
			return finish("observation_unconfirmed")
		}
		p = next
		s, refreshErr := client.RefreshWorkBuddyCloud(bounded, p.Refresh.Original, p.Refresh.Request)
		if refreshErr != nil {
			return finish(cloudFailureCause(refreshErr, "observation_unconfirmed"))
		}
		if err = cfg.Journal.recordCloudSubmission(a, s, true); err != nil {
			return finish("observation_unconfirmed")
		}
		p, err = cfg.Journal.cloudExecution(a)
		if err != nil {
			return invalid
		}
	}
}
