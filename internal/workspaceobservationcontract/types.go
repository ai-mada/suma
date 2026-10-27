// Package workspaceobservationcontract defines bounded, read-only observations
// of an exact live execution workspace. It contains no host paths or secrets.
package workspaceobservationcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const Protocol = "workspace-observation-v1"
const MaxResultBytes = 64 << 10
const RequestTTLSeconds = 60
const LeaseSeconds = 10

type WorkspaceObservationScope struct {
	RunID         string `json:"run_id"`
	NodeID        string `json:"node_id"`
	Attempt       int    `json:"attempt"`
	Fence         int64  `json:"fence"`
	WorkerID      string `json:"worker_id"`
	ToolProfileID string `json:"tool_profile_id"`
	ProfileSHA256 string `json:"profile_sha256"`
}
type WorkspaceObservationTarget struct {
	WorkspaceObservationScope
	RunName  string `json:"run_name"`
	NodeName string `json:"node_name"`
	PoolID   string `json:"pool_id,omitempty"`
}
type WorkspaceObservationTargetPage struct {
	ProjectID  string                       `json:"project_id"`
	Items      []WorkspaceObservationTarget `json:"items"`
	Total      int                          `json:"total"`
	Limit      int                          `json:"limit"`
	Truncated  bool                         `json:"truncated"`
	ObservedAt string                       `json:"observed_at"`
}
type WorkspaceObservationArguments struct {
	Directory string `json:"directory,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Path      string `json:"path,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	MaxLines  int    `json:"max_lines,omitempty"`
}
type WorkspaceObservationCreateRequest struct {
	ProjectID      string                        `json:"project_id"`
	Scope          WorkspaceObservationScope     `json:"scope"`
	Operation      string                        `json:"operation"`
	Arguments      WorkspaceObservationArguments `json:"arguments"`
	ExpectedSHA256 string                        `json:"expected_sha256,omitempty"`
	IdempotencyKey string                        `json:"idempotency_key"`
}
type WorkspaceObservationEntry struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}
type WorkspaceObservationList struct {
	Entries    []WorkspaceObservationEntry `json:"entries"`
	NextCursor string                      `json:"next_cursor"`
	Complete   bool                        `json:"complete"`
}
type WorkspaceObservationRead struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	SHA256    string `json:"sha256"`
	Bytes     int    `json:"bytes"`
	StartLine int    `json:"start_line"`
	NextLine  int    `json:"next_line,omitempty"`
	Complete  bool   `json:"complete"`
}
type WorkspaceObservationResult struct {
	List *WorkspaceObservationList `json:"list,omitempty"`
	Read *WorkspaceObservationRead `json:"read,omitempty"`
}
type WorkspaceObservation struct {
	ID                    string                        `json:"id"`
	ProjectID             string                        `json:"project_id"`
	Version               int                           `json:"version"`
	State                 string                        `json:"state"`
	Scope                 WorkspaceObservationScope     `json:"scope"`
	Operation             string                        `json:"operation"`
	Arguments             WorkspaceObservationArguments `json:"arguments"`
	ExpectedSHA256        string                        `json:"expected_sha256,omitempty"`
	WorkspacePolicySHA256 string                        `json:"workspace_policy_sha256,omitempty"`
	CreatedAt             string                        `json:"created_at"`
	ExpiresAt             string                        `json:"expires_at"`
	ObservedAt            string                        `json:"observed_at,omitempty"`
	Result                *WorkspaceObservationResult   `json:"result,omitempty"`
	ErrorCode             string                        `json:"error_code,omitempty"`
}
type WorkspaceObservationClaimRequest struct {
	ProjectID             string                    `json:"project_id"`
	WorkerID              string                    `json:"worker_id"`
	Scope                 WorkspaceObservationScope `json:"scope"`
	WorkspacePolicySHA256 string                    `json:"workspace_policy_sha256"`
	OperationID           string                    `json:"operation_id"`
}
type WorkspaceObservationLease struct {
	Observation WorkspaceObservation `json:"observation"`
	LeaseFence  int64                `json:"lease_fence"`
	LeaseUntil  string               `json:"lease_until"`
}
type WorkspaceObservationResultRequest struct {
	ProjectID             string                    `json:"project_id"`
	WorkerID              string                    `json:"worker_id"`
	RequestID             string                    `json:"request_id"`
	Scope                 WorkspaceObservationScope `json:"scope"`
	LeaseFence            int64                     `json:"lease_fence"`
	WorkspacePolicySHA256 string                    `json:"workspace_policy_sha256"`
	ObservedAt            string                    `json:"observed_at"`
	// This name deliberately does not collide with the existing DAG/Pool result.
	ObservationResult *WorkspaceObservationResult `json:"observation_result,omitempty"`
	ErrorCode         string                      `json:"error_code,omitempty"`
	ResultSHA256      string                      `json:"result_sha256"`
	OperationID       string                      `json:"operation_id"`
}
type WorkspaceObservationReceipt struct {
	RequestID    string `json:"request_id"`
	LeaseFence   int64  `json:"lease_fence"`
	ResultSHA256 string `json:"result_sha256"`
	Acknowledged bool   `json:"acknowledged"`
}

type Scope = WorkspaceObservationScope
type Target = WorkspaceObservationTarget
type TargetPage = WorkspaceObservationTargetPage
type Arguments = WorkspaceObservationArguments
type CreateRequest = WorkspaceObservationCreateRequest
type Entry = WorkspaceObservationEntry
type List = WorkspaceObservationList
type Read = WorkspaceObservationRead
type Result = WorkspaceObservationResult
type Observation = WorkspaceObservation
type ClaimRequest = WorkspaceObservationClaimRequest
type Lease = WorkspaceObservationLease
type ResultRequest = WorkspaceObservationResultRequest
type Receipt = WorkspaceObservationReceipt

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,149}$`)

func ValidDigest(value string) bool { return digestPattern.MatchString(value) }
func Digest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ResultDigest binds either unchanged source observations or a closed error.
// Request identity, assignment, policy and lease fence are checked separately.
func ResultDigest(result *Result, code string) string {
	return Digest(struct {
		ObservationResult *Result `json:"observation_result"`
		ErrorCode         string  `json:"error_code"`
	}{result, code})
}
func ValidateScope(s Scope) error {
	if !identifierPattern.MatchString(s.RunID) || !identifierPattern.MatchString(s.NodeID) || !identifierPattern.MatchString(s.WorkerID) || !identifierPattern.MatchString(s.ToolProfileID) || s.Attempt < 1 || s.Fence < 1 || !ValidDigest(s.ProfileSHA256) {
		return fmt.Errorf("invalid workspace observation scope")
	}
	return nil
}

// Lexical validation is an early rejection only. Runner must still use its
// active os.Root handle, native path policy and identity checks for every read.
func ValidPath(value string, empty bool) bool {
	if value == "" {
		return empty
	}
	if len(value) > 1024 || !utf8.ValidString(value) || strings.ContainsAny(value, "\\:\x00") || strings.HasPrefix(value, "/") || path.Clean(value) != value || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
	}
	return true
}
func ValidateOperation(operation string, a Arguments, expected string) error {
	if expected != "" && !ValidDigest(expected) {
		return fmt.Errorf("invalid workspace observation arguments")
	}
	switch operation {
	case "list":
		if !ValidPath(a.Directory, true) || a.Path != "" || a.StartLine != 0 || a.MaxLines != 0 || expected != "" || a.Limit < 0 || a.Limit > 200 || len(a.Cursor) > 4096 || !utf8.ValidString(a.Cursor) || strings.IndexFunc(a.Cursor, unicode.IsControl) >= 0 {
			return fmt.Errorf("invalid workspace observation arguments")
		}
	case "read":
		if !ValidPath(a.Path, false) || a.Directory != "" || a.Cursor != "" || a.Limit != 0 || a.StartLine < 0 || a.StartLine > 1048577 || a.MaxLines < 0 || a.MaxLines > 1000 {
			return fmt.Errorf("invalid workspace observation arguments")
		}
	default:
		return fmt.Errorf("invalid workspace observation operation")
	}
	return nil
}
func ValidError(code string) bool {
	switch code {
	case "workspace_closed", "workspace_unavailable", "path_not_allowed", "file_missing", "text_required", "output_limit", "cursor_stale", "file_changed", "sensitive_content", "request_invalid":
		return true
	}
	return false
}
func ValidateResult(o Observation, r *Result, code string) error {
	if code != "" {
		if r != nil || !ValidError(code) {
			return fmt.Errorf("invalid workspace observation result")
		}
		return nil
	}
	if r == nil || (r.List == nil) == (r.Read == nil) {
		return fmt.Errorf("invalid workspace observation result")
	}
	if o.Operation == "list" {
		if r.List == nil || len(r.List.Entries) > 200 || r.List.Entries == nil || len(r.List.NextCursor) > 4096 || r.List.Complete != (r.List.NextCursor == "") {
			return fmt.Errorf("invalid workspace observation result")
		}
		seen := map[string]bool{}
		for _, e := range r.List.Entries {
			if !ValidPath(e.Path, false) || (e.Kind != "file" && e.Kind != "directory") || seen[e.Path] {
				return fmt.Errorf("invalid workspace observation result")
			}
			seen[e.Path] = true
		}
	} else if o.Operation == "read" {
		x := r.Read
		if x == nil || x.Path != o.Arguments.Path || !ValidDigest(x.SHA256) || x.Bytes < 0 || x.Bytes > 1<<20 || !utf8.ValidString(x.Content) || len(x.Content) > 48<<10 || x.StartLine < 1 || x.StartLine != max(1, o.Arguments.StartLine) || x.NextLine < 0 || (!x.Complete && x.NextLine <= x.StartLine) || (x.Complete && x.NextLine != 0) || (o.ExpectedSHA256 != "" && x.SHA256 != o.ExpectedSHA256) {
			return fmt.Errorf("invalid workspace observation result")
		}
	} else {
		return fmt.Errorf("invalid workspace observation result")
	}
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > MaxResultBytes {
		return fmt.Errorf("invalid workspace observation result")
	}
	return nil
}
