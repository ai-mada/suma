package codexcontract

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/strictjson"
)

const ApprovalPolicy = "on-request"

// CodexRPCRequestID preserves the direction-specific JSON-RPC ID without a
// float conversion. Number values are canonical signed int64 decimal strings.
type CodexRPCRequestID struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type CodexApprovalFileChange struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Diff     string `json:"diff"`
	MovePath string `json:"move_path,omitempty"`
}

// CodexApprovalRequest is the safe projection of one observed native request
// and item, not an ACP request and not an administrator permission policy.
// Native absolute paths are checked locally and projected relative to the
// attempt's workspace before journal/network persistence.
type CodexApprovalRequest struct {
	RequestID         CodexRPCRequestID         `json:"request_id"`
	Method            string                    `json:"method"`
	ThreadID          string                    `json:"thread_id"`
	TurnID            string                    `json:"turn_id"`
	ItemID            string                    `json:"item_id"`
	StartedAtMS       int64                     `json:"started_at_ms"`
	ApprovalID        string                    `json:"approval_id,omitempty"`
	BindingSHA256     string                    `json:"binding_sha256"`
	ParamsSHA256      string                    `json:"params_sha256"`
	ItemSHA256        string                    `json:"item_sha256"`
	Title             string                    `json:"title"`
	Details           string                    `json:"details"`
	DetailsComplete   bool                      `json:"details_complete"`
	ScopeSupported    bool                      `json:"scope_supported"`
	UnsupportedReason string                    `json:"unsupported_reason,omitempty"`
	Command           string                    `json:"command,omitempty"`
	CWD               string                    `json:"cwd,omitempty"`
	Changes           []CodexApprovalFileChange `json:"changes,omitempty"`
	Reason            string                    `json:"reason,omitempty"`
}

type CodexApprovalChoice struct {
	Decision string `json:"decision"`
}

// CodexApprovalObservation is a cumulative machine observation. Clearing a
// request does not establish successful execution or invent a human decision.
type CodexApprovalObservation struct {
	Sequence         int64  `json:"sequence"`
	ParamsSHA256     string `json:"params_sha256"`
	Decision         string `json:"decision,omitempty"`
	DecisionObserved bool   `json:"decision_observed"`
	ReplyStarted     bool   `json:"reply_started"`
	ReplyWritten     bool   `json:"reply_written"`
	RequestResolved  bool   `json:"request_resolved"`
	ItemStatus       string `json:"item_status,omitempty"`
	FailureCode      string `json:"failure_code,omitempty"`
}

// A pending/open-failed request can have no PermissionID or observation. Such
// evidence is useful for failure diagnosis but never closes a successful turn.
type CodexApprovalEvidence struct {
	PermissionID string                    `json:"permission_id,omitempty"`
	RequestID    CodexRPCRequestID         `json:"request_id"`
	Method       string                    `json:"method"`
	ThreadID     string                    `json:"thread_id"`
	TurnID       string                    `json:"turn_id"`
	ItemID       string                    `json:"item_id"`
	ApprovalID   string                    `json:"approval_id,omitempty"`
	ParamsSHA256 string                    `json:"params_sha256"`
	Observation  *CodexApprovalObservation `json:"observation,omitempty" wire:"nonnull"`
}

func approvalText(value string, maximum int, required, multiline bool) bool {
	if !utf8.ValidString(value) || len(value) > maximum || required && strings.TrimSpace(value) == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
	}
	return true
}

func ValidateRPCRequestID(id CodexRPCRequestID) error {
	if id.Kind == "string" && approvalText(id.Value, 256, true, false) {
		return nil
	}
	if id.Kind == "number" {
		value, err := strconv.ParseInt(id.Value, 10, 64)
		if err == nil && strconv.FormatInt(value, 10) == id.Value {
			return nil
		}
	}
	return fmt.Errorf("invalid exact Codex server request identity")
}

func approvalMethod(method string) bool {
	return method == "item/commandExecution/requestApproval" || method == "item/fileChange/requestApproval"
}

func approvalRelative(value string, root bool) bool {
	if root && value == "." {
		return true
	}
	return value != "" && value != "." && approvalText(value, 2048, true, false) && !strings.ContainsAny(value, "\\:") && !strings.HasPrefix(value, "/") && path.Clean(value) == value && value != ".." && !strings.HasPrefix(value, "../")
}

func ValidateApprovalRequest(r CodexApprovalRequest) error {
	if ValidateRPCRequestID(r.RequestID) != nil || !approvalMethod(r.Method) || !validID(r.ThreadID, false) || !validID(r.TurnID, false) || !validID(r.ItemID, false) || r.StartedAtMS <= 0 || r.StartedAtMS > MaxSafeInteger || !approvalText(r.ApprovalID, 256, false, false) || !hash.MatchString(r.BindingSHA256) || !hash.MatchString(r.ParamsSHA256) || !hash.MatchString(r.ItemSHA256) || !approvalText(r.Title, 4096, true, true) || !approvalText(r.Details, 32<<10, false, true) || !approvalText(r.Reason, 4096, false, true) || !approvalText(r.Command, 32<<10, false, true) || r.UnsupportedReason != "" && !validID(r.UnsupportedReason, false) || !r.ScopeSupported && r.UnsupportedReason == "" {
		return fmt.Errorf("invalid native Codex approval request")
	}
	if r.CWD != "" && !approvalRelative(r.CWD, true) || len(r.Changes) > 128 {
		return fmt.Errorf("invalid native Codex approval workspace")
	}
	seen := map[string]bool{}
	for _, change := range r.Changes {
		if !approvalRelative(change.Path, false) || seen[change.Path] || !approvalText(change.Diff, 32<<10, false, true) || change.MovePath != "" && !approvalRelative(change.MovePath, false) || change.Kind != "add" && change.Kind != "delete" && change.Kind != "update" || change.Kind != "update" && change.MovePath != "" {
			return fmt.Errorf("invalid native Codex approval change")
		}
		seen[change.Path] = true
	}
	if r.Method == "item/commandExecution/requestApproval" && len(r.Changes) != 0 || r.Method == "item/fileChange/requestApproval" && r.Command != "" {
		return fmt.Errorf("mixed native Codex approval details")
	}
	if r.ScopeSupported && r.DetailsComplete && (r.Method == "item/commandExecution/requestApproval" && (strings.TrimSpace(r.Command) == "" || r.CWD == "") || r.Method == "item/fileChange/requestApproval" && len(r.Changes) == 0) {
		return fmt.Errorf("incomplete native Codex approval basis")
	}
	return nil
}

func ValidateApprovalChoice(c CodexApprovalChoice) error {
	if c.Decision != "accept" && c.Decision != "decline" && c.Decision != "cancel" {
		return fmt.Errorf("unsupported native Codex approval choice")
	}
	return nil
}

func ApprovalChoiceMatchesRequest(r CodexApprovalRequest, c CodexApprovalChoice) bool {
	return ValidateApprovalRequest(r) == nil && ValidateApprovalChoice(c) == nil && (c.Decision != "accept" || r.DetailsComplete && r.ScopeSupported)
}

func ValidateApprovalObservation(o CodexApprovalObservation) error {
	if o.Sequence < 1 || o.Sequence > MaxSafeInteger || !hash.MatchString(o.ParamsSHA256) || o.Decision != "" && ValidateApprovalChoice(CodexApprovalChoice{Decision: o.Decision}) != nil || o.DecisionObserved != (o.Decision != "") || o.ReplyStarted && !o.DecisionObserved || o.ReplyWritten && !o.ReplyStarted || o.ItemStatus != "" && o.ItemStatus != "inProgress" && o.ItemStatus != "completed" && o.ItemStatus != "failed" && o.ItemStatus != "declined" || o.FailureCode != "" && !validID(o.FailureCode, false) {
		return fmt.Errorf("invalid native Codex approval observation")
	}
	return nil
}

func ValidateApprovalEvidence(e CodexApprovalEvidence) error {
	if e.PermissionID != "" && !validID(e.PermissionID, false) || ValidateRPCRequestID(e.RequestID) != nil || !approvalMethod(e.Method) || !validID(e.ThreadID, false) || !validID(e.TurnID, false) || !validID(e.ItemID, false) || !approvalText(e.ApprovalID, 256, false, false) || !hash.MatchString(e.ParamsSHA256) || e.Observation != nil && (ValidateApprovalObservation(*e.Observation) != nil || e.Observation.ParamsSHA256 != e.ParamsSHA256 || e.Observation.DecisionObserved && e.PermissionID == "") {
		return fmt.Errorf("invalid native Codex approval evidence")
	}
	return nil
}

func ApprovalEvidenceClosed(e CodexApprovalEvidence) bool {
	if ValidateApprovalEvidence(e) != nil || e.PermissionID == "" || e.Observation == nil {
		return false
	}
	o := e.Observation
	return o.DecisionObserved && o.ReplyStarted && o.ReplyWritten && o.RequestResolved && (o.ItemStatus == "completed" || o.ItemStatus == "failed" || o.ItemStatus == "declined") && o.FailureCode == ""
}

func (r *CodexRPCRequestID) UnmarshalJSON(raw []byte) error {
	type wire CodexRPCRequestID
	var value wire
	if strictjson.Decode(raw, &value, 4096) != nil || ValidateRPCRequestID(CodexRPCRequestID(value)) != nil {
		return fmt.Errorf("invalid exact Codex request identity JSON")
	}
	*r = CodexRPCRequestID(value)
	return nil
}

func (r *CodexApprovalRequest) UnmarshalJSON(raw []byte) error {
	type wire CodexApprovalRequest
	var value wire
	if strictjson.Decode(raw, &value, 64<<10) != nil || ValidateApprovalRequest(CodexApprovalRequest(value)) != nil {
		return fmt.Errorf("invalid native Codex approval request JSON")
	}
	*r = CodexApprovalRequest(value)
	return nil
}

func (r *CodexApprovalChoice) UnmarshalJSON(raw []byte) error {
	type wire CodexApprovalChoice
	var value wire
	if strictjson.Decode(raw, &value, 4096) != nil || ValidateApprovalChoice(CodexApprovalChoice(value)) != nil {
		return fmt.Errorf("invalid native Codex approval choice JSON")
	}
	*r = CodexApprovalChoice(value)
	return nil
}

func (r *CodexApprovalObservation) UnmarshalJSON(raw []byte) error {
	type wire CodexApprovalObservation
	var value wire
	if strictjson.Decode(raw, &value, 8192) != nil || ValidateApprovalObservation(CodexApprovalObservation(value)) != nil {
		return fmt.Errorf("invalid native Codex approval observation JSON")
	}
	*r = CodexApprovalObservation(value)
	return nil
}

func (r *CodexApprovalEvidence) UnmarshalJSON(raw []byte) error {
	type wire CodexApprovalEvidence
	var value wire
	if strictjson.Decode(raw, &value, 16384) != nil || ValidateApprovalEvidence(CodexApprovalEvidence(value)) != nil {
		return fmt.Errorf("invalid native Codex approval evidence JSON")
	}
	*r = CodexApprovalEvidence(value)
	return nil
}
