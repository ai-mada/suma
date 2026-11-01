package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"

	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

type cloudRefreshCheckpoint struct {
	Original wbc.CloudSubmission     `json:"original"`
	Request  wbc.CloudRefreshRequest `json:"request"`
	Body     string                  `json:"body"`
}

// Only original control-plane requests and safe observations are stored here.
// Supplier credentials, ACP tickets and a second prompt are never checkpointed.
type cloudExecutionCheckpoint struct {
	Version           int                        `json:"version"`
	ControlOrigin     string                     `json:"control_origin"`
	ProjectID         string                     `json:"project_id"`
	AssignmentSHA256  string                     `json:"assignment_sha256"`
	RequestSHA256     string                     `json:"request_sha256"`
	Binding           wbc.CloudAssignmentBinding `json:"binding"`
	Create            wbc.CloudCreateRequest     `json:"create"`
	CreateBody        string                     `json:"create_body"`
	ResultOperationID string                     `json:"result_operation_id"`
	Submission        *wbc.CloudSubmission       `json:"submission,omitempty"`
	Refresh           *cloudRefreshCheckpoint    `json:"refresh,omitempty"`
}

func (p *cloudExecutionCheckpoint) UnmarshalJSON(raw []byte) error {
	type plain cloudExecutionCheckpoint
	var out plain
	if len(raw) > 64<<10 || wbc.Decode(raw, &out) != nil {
		return fmt.Errorf("invalid cloud checkpoint encoding")
	}
	*p = cloudExecutionCheckpoint(out)
	return nil
}

func cloudControlOrigin(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || len(value) > 2048 ||
		(u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return "", fmt.Errorf("invalid cloud control origin")
	}
	return strings.TrimRight(value, "/"), nil
}

func cloudAssignmentSHA(a Assignment) string { raw, _ := json.Marshal(a); return digest(raw) }

func cloudMixed(e entry) bool {
	return developmentContinuationInput(e.Assignment) != nil || e.Assignment.DevelopmentContinuation != nil || e.Terminal != nil || e.ModelGateway != nil || e.WorkspaceTrace != nil || e.ArtifactPending != nil || e.CandidatePending != nil || e.Assignment.ReleaseExecution != nil || e.Assignment.RepositoryIntegration != nil || e.Assignment.CodeBaseline != nil || e.Assignment.CandidateComposition != nil || e.Assignment.CandidateConflictRepair != nil || hasCandidate(e.Assignment)
}

func validCloudExecution(a Assignment, p cloudExecutionCheckpoint) error {
	if !cloudNode(a) || a.WorkBuddyCloud == nil || a.WorkBuddyCloud.Validate() != nil || len(a.Command) != 0 || a.DispatchID == "" || p.Version != 1 || p.AssignmentSHA256 != cloudAssignmentSHA(a) || p.Binding != *a.WorkBuddyCloud || p.Create.Validate() != nil || p.ProjectID != p.Create.ProjectID || p.Create.Assignment != candidateRef(a) || p.Create.DelegationID != p.Binding.DelegationID || p.Create.ExpectedVersion != p.Binding.DelegationVersion || len(p.ResultOperationID) != 32 {
		return fmt.Errorf("cloud checkpoint differs from original assignment")
	}
	if origin, err := cloudControlOrigin(p.ControlOrigin); err != nil || origin != p.ControlOrigin {
		return fmt.Errorf("cloud checkpoint origin changed")
	}
	node, err := wbc.ParseNodeBinding(a.Config["workbuddy_cloud"])
	if err != nil {
		return err
	}
	requestSHA, err := node.RequestDigest()
	if err != nil || requestSHA != p.RequestSHA256 {
		return fmt.Errorf("cloud original request digest changed")
	}
	createBody, err := json.Marshal(p.Create)
	if err != nil || string(createBody) != p.CreateBody {
		return fmt.Errorf("cloud original create bytes changed")
	}
	if p.Submission != nil && !cloudSubmissionMatches(p, *p.Submission) {
		return fmt.Errorf("cloud original submission changed")
	}
	if p.Refresh != nil {
		r := p.Refresh
		body, err := json.Marshal(r.Request)
		if err != nil || string(body) != r.Body || r.Request.Validate() != nil || !cloudSubmissionMatches(p, r.Original) || r.Original.State != "task_known" || p.Submission == nil || p.Submission.ID != r.Original.ID || p.Submission.Version < r.Original.Version || p.Submission.Task == nil || r.Original.Task == nil || p.Submission.Task.ID != r.Original.Task.ID || r.Request.ProjectID != p.ProjectID || r.Request.Assignment != p.Create.Assignment || r.Request.ExpectedVersion != r.Original.Version {
			return fmt.Errorf("cloud original observation request changed")
		}
	}
	return nil
}

func cloudSubmissionMatches(p cloudExecutionCheckpoint, s wbc.CloudSubmission) bool {
	return s.Validate() == nil && s.ProjectID == p.ProjectID && s.DelegationID == p.Binding.DelegationID && s.RunID == p.Create.Assignment.RunID && s.NodeID == p.Create.Assignment.NodeID && s.Assignment == p.Create.Assignment && s.RequestSHA256 == p.RequestSHA256
}

func validCloudResult(e entry, r Result) bool {
	if e.Cloud == nil || r.WorkBuddyCloudWait == nil || r.WorkBuddyCloudWait.Validate() != nil || r.WorkBuddyCloudWait.Assignment != e.Cloud.Create.Assignment || r.WorkBuddyCloudWait.Binding != e.Cloud.Binding || r.WorkBuddyCloudWait.CreateOperationID != e.Cloud.Create.IdempotencyKey || r.WorkBuddyCloudWait.ClaimID != e.Assignment.DispatchID || r.OperationID != e.Cloud.ResultOperationID || !r.OutcomeUnknown || r.Stopped || r.ExitCode == 0 || r.ToolEvidence != nil || r.WorkspaceEvidence != nil || r.ArtifactEvidence != nil || r.ReleaseEvidence != nil || r.TestReport != nil || r.IntegrationEvidence != nil || r.StartupFailure != nil {
		return false
	}
	proof := r.WorkBuddyCloudWait
	if e.Cloud.Submission == nil {
		return proof.SubmissionID == "" && proof.SubmissionVersion == 0
	}
	return proof.SubmissionID == e.Cloud.Submission.ID && proof.SubmissionVersion == e.Cloud.Submission.Version
}

func cloneCloudCheckpoint(p cloudExecutionCheckpoint) cloudExecutionCheckpoint {
	copy := p
	if p.Submission != nil {
		s := *p.Submission
		if s.Task != nil {
			task := *s.Task
			s.Task = &task
		}
		copy.Submission = &s
	}
	if p.Refresh != nil {
		r := *p.Refresh
		if r.Original.Task != nil {
			task := *r.Original.Task
			r.Original.Task = &task
		}
		copy.Refresh = &r
	}
	return copy
}

// Match memory to the actual durable inbox before authorizing any transport.
func (j *Journal) cloudEntryLocked(a Assignment) (entry, error) {
	if j.unlock == nil {
		return entry{}, fmt.Errorf("cloud journal closed")
	}
	e, ok := j.entries[key(a)]
	if !ok || cloudAssignmentSHA(a) != cloudAssignmentSHA(e.Assignment) || cloudMixed(e) {
		return entry{}, fmt.Errorf("cloud inbox scope unavailable")
	}
	raw, err := os.ReadFile(j.path)
	if err != nil {
		return entry{}, err
	}
	var disk map[string]entry
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&disk) != nil || d.Decode(new(any)) != io.EOF {
		return entry{}, fmt.Errorf("cloud inbox is not durable")
	}
	stored, ok := disk[key(a)]
	if !ok {
		return entry{}, fmt.Errorf("cloud inbox is not durable")
	}
	left, _ := json.Marshal(e)
	right, _ := json.Marshal(stored)
	if !bytes.Equal(left, right) {
		return entry{}, fmt.Errorf("cloud inbox differs from durable facts")
	}
	if e.Cloud != nil && validCloudExecution(e.Assignment, *e.Cloud) != nil {
		return entry{}, fmt.Errorf("invalid cloud checkpoint")
	}
	return e, nil
}

func (j *Journal) beginCloudExecution(c *Client, a Assignment) (cloudExecutionCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, err := j.cloudEntryLocked(a)
	if err != nil {
		return cloudExecutionCheckpoint{}, err
	}
	origin, err := cloudControlOrigin(c.BaseURL)
	if err != nil {
		return cloudExecutionCheckpoint{}, err
	}
	if e.Cloud != nil {
		if e.Cloud.ControlOrigin != origin || e.Cloud.ProjectID != c.ProjectID || e.Result != nil || e.Acked {
			return cloudExecutionCheckpoint{}, fmt.Errorf("cloud original scope is final or changed")
		}
		return cloneCloudCheckpoint(*e.Cloud), nil
	}
	if e.Result != nil || e.Acked || a.WorkBuddyCloud == nil {
		return cloudExecutionCheckpoint{}, fmt.Errorf("cloud inbox unavailable")
	}
	node, err := wbc.ParseNodeBinding(a.Config["workbuddy_cloud"])
	if err != nil {
		return cloudExecutionCheckpoint{}, err
	}
	requestSHA, err := node.RequestDigest()
	if err != nil {
		return cloudExecutionCheckpoint{}, err
	}
	p := cloudExecutionCheckpoint{Version: 1, ControlOrigin: origin, ProjectID: c.ProjectID, AssignmentSHA256: cloudAssignmentSHA(a), RequestSHA256: requestSHA, Binding: *a.WorkBuddyCloud, ResultOperationID: operationID()}
	p.Create = wbc.CloudCreateRequest{ProjectID: c.ProjectID, DelegationID: p.Binding.DelegationID, ExpectedVersion: p.Binding.DelegationVersion, Assignment: candidateRef(a), IdempotencyKey: operationID()}
	body, _ := json.Marshal(p.Create)
	p.CreateBody = string(body)
	if err = validCloudExecution(a, p); err != nil {
		return cloudExecutionCheckpoint{}, err
	}
	previous := e
	e.Cloud = &p
	j.entries[key(a)] = e
	if err = j.save(); err != nil {
		j.entries[key(a)] = previous
		return cloudExecutionCheckpoint{}, err
	}
	return cloneCloudCheckpoint(p), nil
}

func (j *Journal) cloudExecution(a Assignment) (cloudExecutionCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, err := j.cloudEntryLocked(a)
	if err != nil || e.Cloud == nil {
		return cloudExecutionCheckpoint{}, fmt.Errorf("cloud checkpoint unavailable")
	}
	return cloneCloudCheckpoint(*e.Cloud), nil
}

func (j *Journal) recordCloudSubmission(a Assignment, s wbc.CloudSubmission, acknowledgeRefresh bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, err := j.cloudEntryLocked(a)
	if err != nil || e.Cloud == nil || e.Result != nil || e.Acked {
		return fmt.Errorf("cloud original submission unavailable")
	}
	p := cloneCloudCheckpoint(*e.Cloud)
	if !cloudSubmissionMatches(p, s) {
		return fmt.Errorf("cloud submission identity changed")
	}
	if p.Submission != nil && (s.ID != p.Submission.ID || p.Submission.Task != nil && (s.Task == nil || s.Task.ID != p.Submission.Task.ID)) {
		return fmt.Errorf("cloud task identity changed")
	}
	if acknowledgeRefresh {
		if p.Refresh == nil || s.Version <= p.Refresh.Request.ExpectedVersion {
			return fmt.Errorf("cloud observation receipt does not match original request")
		}
		p.Refresh = nil
	}
	if p.Submission == nil || s.Version >= p.Submission.Version {
		p.Submission = &s
	}
	if err = validCloudExecution(a, p); err != nil {
		return err
	}
	previous := e
	e.Cloud = &p
	j.entries[key(a)] = e
	if err = j.save(); err != nil {
		j.entries[key(a)] = previous
		return err
	}
	return nil
}

func (j *Journal) beginCloudRefresh(a Assignment) (cloudExecutionCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, err := j.cloudEntryLocked(a)
	if err != nil || e.Cloud == nil || e.Result != nil || e.Acked {
		return cloudExecutionCheckpoint{}, fmt.Errorf("cloud observation unavailable")
	}
	p := cloneCloudCheckpoint(*e.Cloud)
	if p.Refresh != nil {
		return p, nil
	}
	if p.Submission == nil || p.Submission.State != "task_known" {
		return cloudExecutionCheckpoint{}, fmt.Errorf("cloud original task unavailable")
	}
	r := &cloudRefreshCheckpoint{Original: *p.Submission, Request: wbc.CloudRefreshRequest{ProjectID: p.ProjectID, Assignment: p.Create.Assignment, ExpectedVersion: p.Submission.Version, IdempotencyKey: operationID()}}
	body, _ := json.Marshal(r.Request)
	r.Body = string(body)
	p.Refresh = r
	if err = validCloudExecution(a, p); err != nil {
		return cloudExecutionCheckpoint{}, err
	}
	previous := e
	e.Cloud = &p
	j.entries[key(a)] = e
	if err = j.save(); err != nil {
		j.entries[key(a)] = previous
		return cloudExecutionCheckpoint{}, err
	}
	return p, nil
}

func (j *Journal) pendingCloudExecutions() []Assignment {
	j.mu.Lock()
	defer j.mu.Unlock()
	keys := []string{}
	for k, e := range j.entries {
		if e.Result == nil && cloudNode(e.Assignment) && e.Assignment.WorkBuddyCloud != nil {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]Assignment, 0, len(keys))
	for _, k := range keys {
		out = append(out, cloneCloudAssignment(j.entries[k].Assignment))
	}
	return out
}
