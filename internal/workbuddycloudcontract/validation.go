package workbuddycloudcontract

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/workbuddy"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

var ErrInvalid = errors.New("invalid WorkBuddy cloud execution contract")
var taskIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)
var cloudDeliveryTaskIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
var errorCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)

// Decode rejects unknown/case-aliased fields, duplicate keys, invalid Unicode
// and null scalar values. Call the typed Validate method afterwards; this
// decoder does not establish assignment binding or current authorization.
func Decode(raw []byte, out any) error {
	if woc.Decode(raw, out) != nil {
		return ErrInvalid
	}
	return nil
}

func validText(value string, max int, empty bool) bool {
	if !utf8.ValidString(value) || len(value) > max || (!empty && strings.TrimSpace(value) == "") {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func validID(value string, max int) bool {
	return validText(value, max, false) && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "/\\")
}
func validVersion(version int) bool    { return version >= 1 && version <= MaxVersion }
func expectedVersion(version int) bool { return version >= 1 && version < MaxVersion }
func operationID(value string) bool    { return len(value) >= 8 && validID(value, 128) }
func parseTime(value string) (time.Time, bool) {
	if len(value) > 40 || !strings.HasSuffix(value, "Z") {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	return t, err == nil && !t.IsZero()
}

func (b NodeBinding) Validate() error {
	if !validID(b.ConnectionID, 64) || !validVersion(b.GrantGeneration) {
		return ErrInvalid
	}
	_, err := workbuddy.CreateRequestDigest(b.Request)
	if err != nil {
		return ErrInvalid
	}
	return nil
}
func (b NodeBinding) RequestDigest() (string, error) {
	if b.Validate() != nil {
		return "", ErrInvalid
	}
	return workbuddy.CreateRequestDigest(b.Request)
}

// ParseNodeBinding preserves the existing strict nested JSON contract. The
// caller must separately verify node Type, plugin, workflow and current scope.
func ParseNodeBinding(raw any) (NodeBinding, error) {
	var out NodeBinding
	encoded, err := json.Marshal(raw)
	if err != nil || len(encoded) > 2<<20 || Decode(encoded, &out) != nil || out.Validate() != nil {
		return NodeBinding{}, ErrInvalid
	}
	return out, nil
}

func (r CloudDelegateRequest) Validate() error {
	_, timeOK := parseTime(r.ExpiresAt)
	if !validID(r.ProjectID, 64) || !validID(r.ConnectionID, 64) || !expectedVersion(r.ExpectedConnectionVersion) || !validVersion(r.GrantGeneration) || !validID(r.RunID, 100) || !validID(r.NodeID, 150) || !expectedVersion(r.ExpectedRunVersion) || !cc.ValidHash(r.NodeSpecSHA256) || !cc.ValidHash(r.RequestSHA256) || !timeOK || r.MaxCreates != 1 || !operationID(r.IdempotencyKey) {
		return ErrInvalid
	}
	return nil
}

// ValidateAt requires the caller's authoritative database time; there is no
// process-clock authorization decision hidden in this reusable wire package.
func (r CloudDelegateRequest) ValidateAt(dbNow time.Time) error {
	if r.Validate() != nil || dbNow.IsZero() {
		return ErrInvalid
	}
	expires, _ := parseTime(r.ExpiresAt)
	if !expires.After(dbNow) || expires.Sub(dbNow) > MaxDelegationLifetime {
		return ErrInvalid
	}
	return nil
}

func validAssignment(project string, a cc.AssignmentRef) bool {
	if !a.Valid() || !validID(a.RunID, 100) || !validID(a.NodeID, 150) || !validID(a.WorkerID, 100) || !validVersion(a.Attempt) || a.Fence > 9007199254740991 {
		return false
	}
	pool := a.TenantID != "" || a.ProjectID != "" || a.PoolID != "" || a.PoolRunnerID != "" || a.PoolSlotID != "" || a.PoolGrantID != "" || a.PoolGrantVersion != 0 || a.PoolCredentialID != "" || a.PoolCredentialVersion != 0 || a.DispatchID != "" || a.DispatchState != "" || a.LeaseUntil != ""
	if !pool {
		return true
	}
	_, timeOK := parseTime(a.LeaseUntil)
	return validID(a.TenantID, 64) && a.ProjectID == project && validID(a.PoolID, 64) && validID(a.PoolRunnerID, 100) && validID(a.PoolSlotID, 100) && validID(a.PoolGrantID, 64) && validVersion(a.PoolGrantVersion) && validID(a.PoolCredentialID, 64) && validVersion(a.PoolCredentialVersion) && validID(a.DispatchID, 100) && validID(a.DispatchState, 40) && timeOK
}
func (r CloudCreateRequest) Validate() error {
	if !validID(r.ProjectID, 64) || !validID(r.DelegationID, 64) || !expectedVersion(r.ExpectedVersion) || !operationID(r.IdempotencyKey) || !validAssignment(r.ProjectID, r.Assignment) {
		return ErrInvalid
	}
	return nil
}
func (r CloudRefreshRequest) Validate() error {
	if !validID(r.ProjectID, 64) || !expectedVersion(r.ExpectedVersion) || !operationID(r.IdempotencyKey) || !validAssignment(r.ProjectID, r.Assignment) {
		return ErrInvalid
	}
	return nil
}
func (r CloudRevokeRequest) Validate() error {
	if !validID(r.ProjectID, 64) || !expectedVersion(r.ExpectedVersion) || !operationID(r.IdempotencyKey) {
		return ErrInvalid
	}
	return nil
}

func (r CloudObserveRequest) Validate() error {
	if !validID(r.ProjectID, 64) || !expectedVersion(r.ExpectedVersion) || !operationID(r.IdempotencyKey) {
		return ErrInvalid
	}
	return nil
}

func (link CloudTaskDeliveryLink) Validate() error {
	if !cloudDeliveryTaskIDPattern.MatchString(link.TaskID) || link.TaskID == "." || link.TaskID == ".." || !validVersion(link.TaskVersion) {
		return ErrInvalid
	}
	switch link.RunRelation {
	case "current", "changed", "unbound":
		return nil
	default:
		return ErrInvalid
	}
}

func (v CloudNodeView) Validate() error {
	if !validID(v.ProjectID, 64) || !validID(v.RunID, 100) || !validID(v.NodeID, 150) || !validVersion(v.RunVersion) || !cc.ValidHash(v.NodeSpecSHA256) || !cc.ValidHash(v.RequestSHA256) || v.AllowedActions == nil {
		return ErrInvalid
	}
	requestHash, err := v.Binding.RequestDigest()
	if err != nil || requestHash != v.RequestSHA256 {
		return ErrInvalid
	}
	if d := v.Delegation; d != nil {
		if d.Validate() != nil || d.ProjectID != v.ProjectID || d.RunID != v.RunID || d.NodeID != v.NodeID || d.NodeSpecSHA256 != v.NodeSpecSHA256 || d.RequestSHA256 != v.RequestSHA256 || d.ConnectionID != v.Binding.ConnectionID || d.GrantGeneration != v.Binding.GrantGeneration {
			return ErrInvalid
		}
	}
	if s := v.Submission; s != nil {
		if s.Validate() != nil || s.ProjectID != v.ProjectID || s.RunID != v.RunID || s.NodeID != v.NodeID || s.RequestSHA256 != v.RequestSHA256 || v.Delegation != nil && s.DelegationID != v.Delegation.ID {
			return ErrInvalid
		}
	}
	if link := v.TaskDelivery; link != nil && link.Validate() != nil {
		return ErrInvalid
	}
	previous := 0
	for _, action := range v.AllowedActions {
		rank := 0
		switch action {
		case "delegate":
			rank = 1
		case "revoke":
			if v.Delegation == nil || v.Delegation.State != "active" {
				return ErrInvalid
			}
			rank = 2
		case "observe":
			if v.Submission == nil || v.Submission.State != "task_known" {
				return ErrInvalid
			}
			rank = 3
		default:
			return ErrInvalid
		}
		if rank <= previous {
			return ErrInvalid
		}
		previous = rank
	}
	return nil
}
func (d CloudDelegation) Validate() error {
	_, expires := parseTime(d.ExpiresAt)
	_, created := parseTime(d.CreatedAt)
	_, updated := parseTime(d.UpdatedAt)
	if !validID(d.ID, 64) || !validID(d.ProjectID, 64) || !validID(d.OwnerID, 64) || !validID(d.ConnectionID, 64) || !validVersion(d.Version) || !validID(d.RunID, 100) || !validID(d.NodeID, 150) || !cc.ValidHash(d.NodeSpecSHA256) || !validVersion(d.ConnectionVersionAtIssue) || !validVersion(d.GrantGeneration) || !cc.ValidHash(d.ClientBindingSHA256) || !cc.ValidHash(d.RequestSHA256) || !expires || !created || !updated || d.MaxCreates != 1 || d.CreatesConsumed < 0 || d.CreatesConsumed > 1 || d.State != "active" && d.State != "revoked" {
		return ErrInvalid
	}
	return nil
}
func (s CloudSubmission) Validate() error {
	_, created := parseTime(s.CreatedAt)
	_, updated := parseTime(s.UpdatedAt)
	if !validID(s.ID, 64) || !validID(s.ProjectID, 64) || !validID(s.DelegationID, 64) || !validVersion(s.Version) || !validAssignment(s.ProjectID, s.Assignment) || s.RunID != s.Assignment.RunID || s.NodeID != s.Assignment.NodeID || !cc.ValidHash(s.RequestSHA256) || !created || !updated || s.UsageState != "unreported" || s.LastError != "" && !errorCodePattern.MatchString(s.LastError) {
		return ErrInvalid
	}
	switch s.State {
	case "prepared", "unknown":
		if s.Task != nil || s.ObservedAt != "" {
			return ErrInvalid
		}
	case "task_known":
		if _, ok := parseTime(s.ObservedAt); !ok || s.Task == nil || !taskIDPattern.MatchString(s.Task.ID) || !validText(s.Task.Status, 512, false) || utf8.RuneCountInString(s.Task.Status) > 128 || !validText(s.Task.Name, 2048, true) || utf8.RuneCountInString(s.Task.Name) > 512 {
			return ErrInvalid
		}
		for _, stamp := range []string{s.Task.CreatedAt, s.Task.UpdatedAt} {
			if stamp != "" {
				if len(stamp) > 64 {
					return ErrInvalid
				}
				if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
					return ErrInvalid
				}
			}
		}
	default:
		return ErrInvalid
	}
	return nil
}
