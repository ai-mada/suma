package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

var errToolPermission = errors.New("tool permission requires the original current human decision")
var errToolPermissionReconciliation = errors.New("tool permission source requires reconciliation; original journal retained")

type toolPermissionCheckpoint struct {
	Request      pc.Request                   `json:"request"`
	PermissionID string                       `json:"permission_id,omitempty"`
	Decision     *pc.Decision                 `json:"decision,omitempty"`
	Observation  *cc.CodexApprovalObservation `json:"observation,omitempty"`
	Observations []toolPermissionObservation  `json:"observations,omitempty"`
}
type toolPermissionExecutionCheckpoint struct {
	Profile  tc.Profile                          `json:"profile"`
	Requests map[string]toolPermissionCheckpoint `json:"requests"`
}

func cloneToolPermissionCheckpoint(value *toolPermissionExecutionCheckpoint) *toolPermissionExecutionCheckpoint {
	if value == nil {
		return nil
	}
	raw, _ := json.Marshal(value)
	var result toolPermissionExecutionCheckpoint
	_ = json.Unmarshal(raw, &result)
	return &result
}
func samePermissionValue(first, next any) bool {
	a, ae := json.Marshal(first)
	b, be := json.Marshal(next)
	return ae == nil && be == nil && bytes.Equal(a, b)
}
func validToolPermissionCheckpoint(a Assignment, value *toolPermissionExecutionCheckpoint) error {
	if value == nil {
		return nil
	}
	p := value.Profile
	if len(a.Command) != 1 || p.CommandID != a.Command[0] || a.Type != "agent" || a.WorkBuddyCloud != nil || len(value.Requests) > 32 || value.Requests == nil || a.Config["tool_profile_sha256"] != p.ProfileSHA256 {
		return errToolPermission
	}
	switch p.Protocol {
	case ac.Protocol:
		fixed, err := tc.DecodeACPNodeConfiguration(a.Config)
		if err != nil || fixed == nil || !tc.SupportsACPExecutionProfile(p) || *fixed != *p.ACPExecution {
			return errToolPermission
		}
	case cc.Protocol:
		if p.Adapter != "codex" {
			return errToolPermission
		}
		if _, err := cc.DecodeBinding(a.Config["codex_execution"]); err != nil {
			return errToolPermission
		}
	default:
		return errToolPermission
	}
	for key, request := range value.Requests {
		if key != request.Request.IdempotencyKey || request.Request.Protocol != p.Protocol || pc.ValidateRequest(request.Request) != nil || request.Decision != nil && (request.PermissionID == "" || !pc.DecisionMatchesRequest(request.Request, *request.Decision)) {
			return errToolPermission
		}
		if request.Request.Codex != nil {
			binding, err := cc.DecodeBinding(a.Config["codex_execution"])
			if err != nil || request.Request.Codex.BindingSHA256 != binding.BindingSHA256 {
				return errToolPermission
			}
		}
		if err := validToolPermissionObservations(request); err != nil {
			return err
		}
	}
	return nil
}
func (j *Journal) beginToolPermissionExecution(a Assignment, p tc.Profile) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || e.ToolPermissions != nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID {
		return errToolPermission
	}
	checkpoint := &toolPermissionExecutionCheckpoint{Profile: tc.CloneProfile(p), Requests: map[string]toolPermissionCheckpoint{}}
	if validToolPermissionCheckpoint(e.Assignment, checkpoint) != nil {
		return errToolPermission
	}
	before := e
	e.ToolPermissions = checkpoint
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}
func (j *Journal) recordToolPermission(a Assignment, request pc.Request) error {
	if pc.ValidateRequest(request) != nil {
		return errToolPermission
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || e.ToolPermissions == nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID || request.Protocol != e.ToolPermissions.Profile.Protocol {
		return errToolPermission
	}
	if request.Codex != nil {
		binding, err := cc.DecodeBinding(e.Assignment.Config["codex_execution"])
		if err != nil || request.Codex.BindingSHA256 != binding.BindingSHA256 {
			return errToolPermission
		}
	}
	if original, exists := e.ToolPermissions.Requests[request.IdempotencyKey]; exists {
		if !samePermissionValue(original.Request, request) {
			return errToolPermission
		}
		return nil
	}
	if len(e.ToolPermissions.Requests) >= 32 {
		return errToolPermission
	}
	before := e
	e.ToolPermissions = cloneToolPermissionCheckpoint(e.ToolPermissions)
	raw, _ := json.Marshal(request)
	var safe pc.Request
	_ = json.Unmarshal(raw, &safe)
	e.ToolPermissions.Requests[request.IdempotencyKey] = toolPermissionCheckpoint{Request: safe}
	if validNativeRequestCheckpoints(e) != nil {
		return errToolPermission
	}
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}
func (j *Journal) observeToolPermission(a Assignment, permission pc.Permission) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || e.ToolPermissions == nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID {
		return errToolPermission
	}
	original, ok := e.ToolPermissions.Requests[permission.Request.IdempotencyKey]
	if !ok || !toolPermissionSource(a, e.ToolPermissions.Profile.ProfileSHA256, permission, original.Request, permission.ProjectID) || original.PermissionID != "" && original.PermissionID != permission.ID {
		return errToolPermission
	}
	before := e
	e.ToolPermissions = cloneToolPermissionCheckpoint(e.ToolPermissions)
	original = e.ToolPermissions.Requests[permission.Request.IdempotencyKey]
	original.PermissionID = permission.ID
	for i := range original.Observations {
		if original.Observations[i].PermissionID == "" {
			original.Observations[i].PermissionID = permission.ID
		}
	}
	// This is a persisted human decision, never a native response-write ACK.
	if permission.State == "decided" {
		if permission.Decision == nil || !pc.DecisionMatchesRequest(original.Request, *permission.Decision) || original.Decision != nil && !samePermissionValue(original.Decision, permission.Decision) {
			return errToolPermission
		}
		raw, _ := json.Marshal(permission.Decision)
		var d pc.Decision
		_ = json.Unmarshal(raw, &d)
		original.Decision = &d
	}
	e.ToolPermissions.Requests[permission.Request.IdempotencyKey] = original
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}
func (j *Journal) toolPermission(a Assignment, request pc.Request) (toolPermissionCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[key(a)]
	if !ok || e.ToolPermissions == nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID {
		return toolPermissionCheckpoint{}, errToolPermission
	}
	v, ok := cloneToolPermissionCheckpoint(e.ToolPermissions).Requests[request.IdempotencyKey]
	if !ok || !samePermissionValue(v.Request, request) {
		return toolPermissionCheckpoint{}, errToolPermission
	}
	return v, nil
}

func safeToolPermission(request pc.Request, secrets []string) (pc.Request, error) {
	if pc.ValidateRequest(request) != nil {
		return pc.Request{}, errToolPermission
	}
	clean := func(value string) string {
		out := newOutput(64<<10, secrets)
		_, _ = out.Write([]byte(value))
		return out.String()
	}
	if clean(request.IdempotencyKey) != request.IdempotencyKey {
		return pc.Request{}, errToolPermission
	}
	if request.ACP != nil {
		r := *request.ACP
		for _, opaque := range []string{r.SessionID, r.ToolCallID, r.Kind} {
			if clean(opaque) != opaque {
				return pc.Request{}, errToolPermission
			}
		}
		r.Title, r.Details = clean(r.Title), clean(r.Details)
		r.Options = append([]ac.PermissionOption{}, r.Options...)
		if r.Title != request.ACP.Title || r.Details != request.ACP.Details {
			r.DetailsComplete = false
		}
		for i := range r.Options {
			if clean(r.Options[i].ID) != r.Options[i].ID {
				return pc.Request{}, errToolPermission
			}
			name := clean(r.Options[i].Name)
			if name != r.Options[i].Name {
				r.DetailsComplete = false
			}
			r.Options[i].Name = name
		}
		request.ACP = &r
	} else {
		// Driver has already projected relative paths. Never rewrite identities,
		// digests or native detail fields after their exact display was bound.
		r := request.Codex
		values := []string{r.RequestID.Kind, r.RequestID.Value, r.Method, r.ThreadID, r.TurnID, r.ItemID, r.ApprovalID, r.BindingSHA256, r.ParamsSHA256, r.ItemSHA256, r.Title, r.Details, r.UnsupportedReason, r.Command, r.CWD, r.Reason}
		for _, change := range r.Changes {
			values = append(values, change.Path, change.Kind, change.Diff, change.MovePath)
		}
		for _, value := range values {
			if clean(value) != value {
				return pc.Request{}, errToolPermission
			}
		}
	}
	if pc.ValidateRequest(request) != nil {
		return pc.Request{}, errToolPermission
	}
	return request, nil
}
func toolPermissionSource(a Assignment, profile string, p pc.Permission, request pc.Request, project string) bool {
	return pc.ValidatePermission(p) == nil && p.ProjectID == project && p.RunID == a.RunID && p.NodeID == a.NodeID && p.Attempt == a.Attempt && p.Fence == a.Fence && p.DispatchID == a.DispatchID && p.WorkerID == a.WorkerID && p.ProfileSHA256 == profile && samePermissionValue(request, p.Request)
}
func (c *Client) OpenToolPermission(ctx context.Context, a Assignment, request pc.Request) (pc.Permission, error) {
	var result pc.Permission
	if pc.ValidateRequest(request) != nil {
		return result, errToolPermission
	}
	err := c.post(ctx, "tool-permissions", map[string]any{"project_id": c.ProjectID, "assignment": a, "request": request}, &result)
	if err == nil {
		profile, _ := a.Config["tool_profile_sha256"].(string)
		if !toolPermissionSource(a, profile, result, request, c.ProjectID) {
			return pc.Permission{}, errToolPermission
		}
	}
	return result, err
}
func (c *Client) PollToolPermission(ctx context.Context, a Assignment, id string) (pc.Permission, error) {
	var result pc.Permission
	if !acpLocalText(id, 64) || strings.ContainsAny(id, "/\\") {
		return result, errToolPermission
	}
	err := c.post(ctx, "tool-permissions/poll", map[string]any{"project_id": c.ProjectID, "assignment": a, "permission_id": id}, &result)
	if err == nil {
		profile, _ := a.Config["tool_profile_sha256"].(string)
		if result.ID != id || !toolPermissionSource(a, profile, result, result.Request, c.ProjectID) {
			return pc.Permission{}, errToolPermission
		}
	}
	return result, err
}
func (w *Worker) waitToolPermission(ctx context.Context, a Assignment, request pc.Request, secrets []string) (pc.Decision, error) {
	if w.Journal == nil || w.Client == nil {
		return pc.Decision{}, errToolPermission
	}
	request, err := safeToolPermission(request, secrets)
	if err != nil {
		return pc.Decision{}, err
	}
	if err = w.Journal.recordToolPermission(a, request); err != nil {
		return pc.Decision{}, err
	}
	var permission pc.Permission
	for attempt := 0; attempt < 3; attempt++ {
		permission, err = w.Client.OpenToolPermission(ctx, a, request)
		if err == nil {
			break
		}
		var status *runnerHTTPStatusError
		if errors.As(err, &status) && status.Status >= 400 && status.Status < 500 && status.Status != 429 {
			return pc.Decision{}, errToolPermission
		}
		if attempt == 2 || !wait(ctx, 100*time.Millisecond) {
			return pc.Decision{}, errToolPermission
		}
	}
	for {
		if err = w.Journal.observeToolPermission(a, permission); err != nil {
			return pc.Decision{}, errToolPermission
		}
		switch permission.State {
		case "decided":
			if permission.Decision != nil && pc.DecisionMatchesRequest(request, *permission.Decision) {
				return *permission.Decision, nil
			}
			return pc.Decision{}, errToolPermission
		case "pending":
		default:
			return pc.Decision{}, errToolPermission
		}
		if !wait(ctx, 250*time.Millisecond) {
			return pc.Decision{}, errToolPermission
		}
		next, pollErr := w.Client.PollToolPermission(ctx, a, permission.ID)
		if pollErr != nil {
			return pc.Decision{}, errToolPermission
		}
		if !toolPermissionSource(a, permission.ProfileSHA256, next, request, w.Client.ProjectID) || next.ID != permission.ID || next.Version < permission.Version {
			return pc.Decision{}, errToolPermission
		}
		permission = next
	}
}
func cloneACPEvidence(value *ac.Evidence) *ac.Evidence {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
