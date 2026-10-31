package toolpermissioncontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/strictjson"
)

var ErrInvalid = errors.New("invalid tool permission contract")
var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,149}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

const MaxInteger int64 = 9007199254740991

func token(s string, maximum int) bool {
	if s == "" || len(s) > maximum || !utf8.ValidString(s) || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func operationKey(s string) bool { return len(s) >= 8 && token(s, 128) }
func utc(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil && strings.HasSuffix(s, "Z")
}

func ValidateRequest(r Request) error {
	if !operationKey(r.IdempotencyKey) {
		return ErrInvalid
	}
	switch r.Protocol {
	case ac.Protocol:
		if r.ACP == nil || r.Codex != nil || ac.ValidatePermissionSource(*r.ACP) != nil {
			return ErrInvalid
		}
	case cc.Protocol:
		if r.Codex == nil || r.ACP != nil || cc.ValidateApprovalRequest(*r.Codex) != nil {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	encoded, err := json.Marshal(r)
	if err != nil || len(encoded) > 64<<10 {
		return ErrInvalid
	}
	return nil
}

func ValidateChoice(c Choice) error {
	switch c.Protocol {
	case ac.Protocol:
		if c.ACP != nil && c.Codex == nil {
			return ac.ValidatePermissionChoice(*c.ACP)
		}
	case cc.Protocol:
		if c.Codex != nil && c.ACP == nil {
			return cc.ValidateApprovalChoice(*c.Codex)
		}
	}
	return ErrInvalid
}

func IsGrant(c Choice) bool {
	return ValidateChoice(c) == nil && (c.ACP != nil && c.ACP.Kind == "allow_once" || c.Codex != nil && c.Codex.Decision == "accept")
}
func ChoiceMatchesRequest(r Request, c Choice) bool {
	if ValidateRequest(r) != nil || ValidateChoice(c) != nil || r.Protocol != c.Protocol {
		return false
	}
	if r.ACP != nil {
		return ac.PermissionChoiceMatchesSource(*r.ACP, *c.ACP)
	}
	return cc.ApprovalChoiceMatchesRequest(*r.Codex, *c.Codex)
}
func ValidateDecision(d Decision) error {
	_, validTime := utc(d.DecidedAt)
	if ValidateChoice(d.Choice) != nil || !identifier.MatchString(d.ActorID) || !validTime {
		return ErrInvalid
	}
	return nil
}
func DecisionMatchesRequest(r Request, d Decision) bool {
	return ValidateDecision(d) == nil && ChoiceMatchesRequest(r, d.Choice)
}
func ValidateDecisionRequest(d DecisionRequest) error {
	if !identifier.MatchString(d.ProjectID) || d.ExpectedVersion < 1 || d.ExpectedVersion >= MaxInteger || !operationKey(d.IdempotencyKey) || ValidateChoice(d.Choice) != nil {
		return ErrInvalid
	}
	return nil
}

// Native representability is an upper bound; actor, lease and policy may only
// remove actions. Opaque ACP options never become Codex decision identifiers.
func AllowedActions(r Request) []string {
	out := []string{}
	if ValidateRequest(r) != nil {
		return out
	}
	if r.ACP != nil {
		for _, kind := range []string{"allow_once", "reject_once"} {
			for _, option := range r.ACP.Options {
				if option.Kind == kind && (kind != "allow_once" || r.ACP.DetailsComplete) {
					out = append(out, kind)
					break
				}
			}
		}
		return out
	}
	if r.Codex.DetailsComplete && r.Codex.ScopeSupported {
		out = append(out, "accept")
	}
	return append(out, "decline", "cancel")
}

func ValidatePermission(p Permission) error {
	start, startOK := utc(p.CreatedAt)
	end, endOK := utc(p.ExpiresAt)
	if !identifier.MatchString(p.ID) || !identifier.MatchString(p.ProjectID) || !identifier.MatchString(p.RunID) || !identifier.MatchString(p.NodeID) || p.Attempt < 1 || p.Fence < 1 || p.Fence > MaxInteger || !identifier.MatchString(p.DispatchID) || !identifier.MatchString(p.WorkerID) || !digestPattern.MatchString(p.ProfileSHA256) || p.Version < 1 || p.Version > MaxInteger || ValidateRequest(p.Request) != nil || !startOK || !endOK || !end.After(start) || p.AllowedActions == nil {
		return ErrInvalid
	}
	if p.State != "pending" && p.State != "decided" && p.State != "expired" && p.State != "superseded" || p.State == "pending" && p.Decision != nil || p.State == "decided" && p.Decision == nil || p.Decision != nil && !DecisionMatchesRequest(p.Request, *p.Decision) {
		return ErrInvalid
	}
	terminalObservation := false
	if p.Observation != nil {
		o := p.Observation
		if p.Request.Codex == nil || cc.ValidateApprovalObservation(*o) != nil || o.ParamsSHA256 != p.Request.Codex.ParamsSHA256 || o.DecisionObserved && (p.Decision == nil || p.Decision.Codex == nil || o.Decision != p.Decision.Codex.Decision) {
			return ErrInvalid
		}
		terminalObservation = o.RequestResolved || o.ItemStatus != "" && o.ItemStatus != "inProgress"
	}
	allowed := AllowedActions(p.Request)
	last := -1
	for _, action := range p.AllowedActions {
		at := -1
		for i, candidate := range allowed {
			if action == candidate {
				at = i
				break
			}
		}
		if at <= last || p.State != "pending" || terminalObservation {
			return ErrInvalid
		}
		last = at
	}
	return nil
}
func ValidatePermissionList(p List) error {
	if p.Items == nil || len(p.Items) > 32 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, item := range p.Items {
		if ValidatePermission(item) != nil || seen[item.ID] {
			return ErrInvalid
		}
		seen[item.ID] = true
	}
	return nil
}

func sourceDigest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Index identities, not display content: another RPC callback for the same
// item is distinct, but reusing an ID with changed body still collides.
func RequestSourceDigests(r Request) (string, string, error) {
	if ValidateRequest(r) != nil {
		return "", "", ErrInvalid
	}
	if r.ACP != nil {
		return sourceDigest([]string{r.Protocol, r.ACP.SessionID}), sourceDigest([]string{r.Protocol, r.ACP.ToolCallID}), nil
	}
	return sourceDigest([]string{r.Protocol, r.Codex.ThreadID}), sourceDigest(struct {
		Protocol string               `json:"protocol"`
		ID       cc.CodexRPCRequestID `json:"id"`
	}{r.Protocol, r.Codex.RequestID}), nil
}

func (p *ToolPermissionRequest) UnmarshalJSON(raw []byte) error {
	type wire ToolPermissionRequest
	var v wire
	if strictjson.Decode(raw, &v, 64<<10) != nil || ValidateRequest(Request(v)) != nil {
		return ErrInvalid
	}
	*p = ToolPermissionRequest(v)
	return nil
}
func (p *ToolPermissionDecision) UnmarshalJSON(raw []byte) error {
	type wire ToolPermissionDecision
	var v wire
	if strictjson.Decode(raw, &v, 8<<10) != nil || ValidateDecision(Decision(v)) != nil {
		return ErrInvalid
	}
	*p = ToolPermissionDecision(v)
	return nil
}
func (p *ToolPermissionDecisionRequest) UnmarshalJSON(raw []byte) error {
	type wire ToolPermissionDecisionRequest
	var v wire
	if strictjson.Decode(raw, &v, 8<<10) != nil || ValidateDecisionRequest(DecisionRequest(v)) != nil {
		return ErrInvalid
	}
	*p = ToolPermissionDecisionRequest(v)
	return nil
}
func (p *ToolPermission) UnmarshalJSON(raw []byte) error {
	type wire ToolPermission
	var v wire
	if strictjson.Decode(raw, &v, 128<<10) != nil || ValidatePermission(Permission(v)) != nil {
		return ErrInvalid
	}
	*p = ToolPermission(v)
	return nil
}
func (p *ToolPermissionList) UnmarshalJSON(raw []byte) error {
	type wire ToolPermissionList
	var v wire
	if strictjson.Decode(raw, &v, 3<<20) != nil || ValidatePermissionList(List(v)) != nil {
		return ErrInvalid
	}
	*p = ToolPermissionList(v)
	return nil
}
