package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

var errACPPermission = errors.New("ACP permission requires the original current human decision")

type acpPermissionCheckpoint struct {
	Request      ac.PermissionRequest   `json:"request"`
	PermissionID string                 `json:"permission_id,omitempty"`
	Decision     *ac.PermissionDecision `json:"decision,omitempty"`
}
type acpExecutionCheckpoint struct {
	Profile  tc.Profile                         `json:"profile"`
	Requests map[string]acpPermissionCheckpoint `json:"requests"`
}

func cloneACPCheckpoint(value *acpExecutionCheckpoint) *acpExecutionCheckpoint {
	if value == nil {
		return nil
	}
	raw, _ := json.Marshal(value)
	var result acpExecutionCheckpoint
	_ = json.Unmarshal(raw, &result)
	return &result
}
func validACPCheckpoint(a Assignment, value *acpExecutionCheckpoint) error {
	if value == nil {
		return nil
	}
	if !tc.SupportsACPExecutionProfile(value.Profile) || len(a.Command) != 1 || value.Profile.CommandID != a.Command[0] || a.Type != "agent" || len(value.Requests) > 32 || value.Requests == nil {
		return errACPPermission
	}
	fixed, err := tc.DecodeACPNodeConfiguration(a.Config)
	if err != nil || fixed == nil || *fixed != *value.Profile.ACPExecution || a.Config["tool_profile_sha256"] != value.Profile.ProfileSHA256 {
		return errACPPermission
	}
	for key, request := range value.Requests {
		if key != request.Request.IdempotencyKey || ac.ValidatePermissionRequest(request.Request) != nil || request.Decision != nil && (request.PermissionID == "" || !acpDecisionMatches(request.Request, *request.Decision)) {
			return errACPPermission
		}
	}
	return nil
}
func (j *Journal) beginACPExecution(a Assignment, p tc.Profile) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	entry, ok := j.entries[k]
	if !ok || entry.Result != nil || entry.ACP != nil || candidateRef(a) != candidateRef(entry.Assignment) || a.DispatchID != entry.Assignment.DispatchID {
		return errACPPermission
	}
	checkpoint := &acpExecutionCheckpoint{Profile: tc.CloneProfile(p), Requests: map[string]acpPermissionCheckpoint{}}
	if validACPCheckpoint(entry.Assignment, checkpoint) != nil {
		return errACPPermission
	}
	before := entry
	entry.ACP = checkpoint
	j.entries[k] = entry
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}
func (j *Journal) recordACPPermission(a Assignment, request ac.PermissionRequest) error {
	if ac.ValidatePermissionRequest(request) != nil {
		return errACPPermission
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	entry, ok := j.entries[k]
	if !ok || entry.Result != nil || entry.ACP == nil || candidateRef(a) != candidateRef(entry.Assignment) || a.DispatchID != entry.Assignment.DispatchID {
		return errACPPermission
	}
	before := entry
	entry.ACP = cloneACPCheckpoint(entry.ACP)
	if original, exists := entry.ACP.Requests[request.IdempotencyKey]; exists {
		first, _ := json.Marshal(original.Request)
		next, _ := json.Marshal(request)
		if !bytes.Equal(first, next) {
			return errACPPermission
		}
		return nil
	}
	if len(entry.ACP.Requests) >= 32 {
		return errACPPermission
	}
	raw, _ := json.Marshal(request)
	var safe ac.PermissionRequest
	_ = json.Unmarshal(raw, &safe)
	entry.ACP.Requests[request.IdempotencyKey] = acpPermissionCheckpoint{Request: safe}
	j.entries[k] = entry
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}
func (j *Journal) observeACPPermission(a Assignment, permission ac.Permission) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	entry, ok := j.entries[k]
	if !ok || entry.Result != nil || entry.ACP == nil || candidateRef(a) != candidateRef(entry.Assignment) || a.DispatchID != entry.Assignment.DispatchID {
		return errACPPermission
	}
	original, ok := entry.ACP.Requests[permission.Request.IdempotencyKey]
	if !ok || !acpPermissionSource(a, entry.ACP.Profile.ProfileSHA256, permission, original.Request, permission.ProjectID) || original.PermissionID != "" && original.PermissionID != permission.ID {
		return errACPPermission
	}
	before := entry
	entry.ACP = cloneACPCheckpoint(entry.ACP)
	original.PermissionID = permission.ID
	// Observed is a local database/HTTP fact, never a provider adoption ACK.
	if permission.State == "decided" {
		if permission.Decision == nil || !acpDecisionMatches(original.Request, *permission.Decision) {
			return errACPPermission
		}
		copy := *permission.Decision
		original.Decision = &copy
	}
	entry.ACP.Requests[permission.Request.IdempotencyKey] = original
	j.entries[k] = entry
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}
func acpDecisionMatches(request ac.PermissionRequest, decision ac.PermissionDecision) bool {
	if ac.ValidatePermissionDecision(decision) != nil || decision.Kind != "allow_once" && decision.Kind != "reject_once" || decision.Kind == "allow_once" && !request.DetailsComplete {
		return false
	}
	for _, option := range request.Options {
		if option.ID == decision.OptionID && option.Kind == decision.Kind {
			return true
		}
	}
	return false
}
func safeACPPermission(request ac.PermissionRequest, secrets []string) (ac.PermissionRequest, error) {
	if ac.ValidatePermissionRequest(request) != nil {
		return ac.PermissionRequest{}, errACPPermission
	}
	clean := func(value string) string {
		out := newOutput(64<<10, secrets)
		_, _ = out.Write([]byte(value))
		return out.String()
	}
	for _, opaque := range []string{request.IdempotencyKey, request.SessionID, request.ToolCallID, request.Kind} {
		if clean(opaque) != opaque {
			return ac.PermissionRequest{}, errACPPermission
		}
	}
	before := request
	request.Title, request.Details = clean(request.Title), clean(request.Details)
	request.Options = append([]ac.PermissionOption{}, request.Options...)
	if request.Title != before.Title || request.Details != before.Details {
		request.DetailsComplete = false
	}
	for i := range request.Options {
		if clean(request.Options[i].ID) != request.Options[i].ID {
			return ac.PermissionRequest{}, errACPPermission
		}
		name := clean(request.Options[i].Name)
		if name != request.Options[i].Name {
			request.DetailsComplete = false
		}
		request.Options[i].Name = name
	}
	if ac.ValidatePermissionRequest(request) != nil {
		return ac.PermissionRequest{}, errACPPermission
	}
	return request, nil
}
func acpPermissionSource(a Assignment, profile string, p ac.Permission, request ac.PermissionRequest, project string) bool {
	first, _ := json.Marshal(request)
	received, _ := json.Marshal(p.Request)
	return ac.ValidatePermission(p) == nil && p.ProjectID == project && p.RunID == a.RunID && p.NodeID == a.NodeID && p.Attempt == a.Attempt && p.Fence == a.Fence && p.DispatchID == a.DispatchID && p.WorkerID == a.WorkerID && p.ProfileSHA256 == profile && bytes.Equal(first, received)
}
func (c *Client) OpenToolPermission(ctx context.Context, a Assignment, request ac.PermissionRequest) (ac.Permission, error) {
	var result ac.Permission
	if ac.ValidatePermissionRequest(request) != nil {
		return result, errACPPermission
	}
	err := c.post(ctx, "tool-permissions", map[string]any{"project_id": c.ProjectID, "assignment": a, "request": request}, &result)
	if err == nil {
		profile, _ := a.Config["tool_profile_sha256"].(string)
		if !acpPermissionSource(a, profile, result, request, c.ProjectID) {
			return ac.Permission{}, errACPPermission
		}
	}
	return result, err
}
func (c *Client) PollToolPermission(ctx context.Context, a Assignment, id string) (ac.Permission, error) {
	var result ac.Permission
	if !acpLocalText(id, 64) || strings.ContainsAny(id, "/\\") {
		return result, errACPPermission
	}
	err := c.post(ctx, "tool-permissions/poll", map[string]any{"project_id": c.ProjectID, "assignment": a, "permission_id": id}, &result)
	if err == nil {
		profile, _ := a.Config["tool_profile_sha256"].(string)
		if result.ID != id || !acpPermissionSource(a, profile, result, result.Request, c.ProjectID) {
			return ac.Permission{}, errACPPermission
		}
	}
	return result, err
}
func (w *Worker) waitToolPermission(ctx context.Context, a Assignment, request ac.PermissionRequest, secrets []string) (ac.PermissionDecision, error) {
	if w.Journal == nil || w.Client == nil {
		return ac.PermissionDecision{}, errACPPermission
	}
	request, err := safeACPPermission(request, secrets)
	if err != nil {
		return ac.PermissionDecision{}, err
	}
	if err = w.Journal.recordACPPermission(a, request); err != nil {
		return ac.PermissionDecision{}, err
	}
	var permission ac.Permission
	for attempt := 0; attempt < 3; attempt++ {
		permission, err = w.Client.OpenToolPermission(ctx, a, request)
		if err == nil {
			break
		}
		var status *runnerHTTPStatusError
		if errors.As(err, &status) && status.Status >= 400 && status.Status < 500 && status.Status != 429 {
			return ac.PermissionDecision{}, errACPPermission
		}
		if attempt == 2 || !wait(ctx, 100*time.Millisecond) {
			return ac.PermissionDecision{}, errACPPermission
		}
	}
	for {
		if err = w.Journal.observeACPPermission(a, permission); err != nil {
			return ac.PermissionDecision{}, errACPPermission
		}
		switch permission.State {
		case "decided":
			if permission.Decision != nil && acpDecisionMatches(request, *permission.Decision) {
				return *permission.Decision, nil
			}
			return ac.PermissionDecision{}, errACPPermission
		case "expired", "superseded":
			return ac.PermissionDecision{}, errACPPermission
		case "pending":
		default:
			return ac.PermissionDecision{}, errACPPermission
		}
		if !wait(ctx, 250*time.Millisecond) {
			return ac.PermissionDecision{}, errACPPermission
		}
		next, pollErr := w.Client.PollToolPermission(ctx, a, permission.ID)
		if pollErr != nil {
			return ac.PermissionDecision{}, errACPPermission
		}
		if !acpPermissionSource(a, permission.ProfileSHA256, next, request, w.Client.ProjectID) || next.ID != permission.ID || next.Version < permission.Version {
			return ac.PermissionDecision{}, errACPPermission
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
