package workspaceoperationcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/toolcontract"
)

var ErrInvalid = errors.New("invalid workspace operation event")
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var operationPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var errorPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)

func validIdentityText(value string, max int) bool {
	if value == "" || len(value) > max || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, c := range value {
		if c < 32 || c >= 127 && c <= 159 {
			return false
		}
	}
	return true
}

func ValidateIdentity(i Identity) error {
	if !validIdentityText(i.TenantID, 100) || !validIdentityText(i.ProjectID, 100) || !validIdentityText(i.RunID, 100) || !validIdentityText(i.NodeID, 150) || !validIdentityText(i.WorkerID, 100) || i.Attempt < 1 || i.Attempt > 2147483647 || i.Fence < 1 || i.Fence > toolcontract.MaxSafeUsage {
		return ErrInvalid
	}
	return nil
}

// CanonicalEventID binds transport identity to the original assignment and the
// immutable invocation transition; changed bodies are still rejected by receipt
// fingerprints. No SDK-provided identifier can create a new assignment scope.
func CanonicalEventID(i Identity, e Event) string {
	raw, _ := json.Marshal(struct {
		Identity    Identity `json:"identity"`
		Sequence    int      `json:"sequence"`
		OperationID string   `json:"operation_id"`
		Revision    int      `json:"revision"`
	}{i, e.Sequence, e.OperationID, e.Revision})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func ValidPath(value string) bool {
	if !validIdentityText(value, 1024) || value == "." || path.IsAbs(value) || path.Clean(value) != value || strings.ContainsAny(value, `\:`) {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || part == "" || strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}

func validTime(value string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, value)
	return t, err == nil && strings.HasSuffix(value, "Z") && len(value) <= 35
}

func ValidateEvent(e Event) error {
	if e.Protocol != Protocol || e.Source != Source || e.SchemaVersion != 1 || !digestPattern.MatchString(e.EventID) || e.Sequence < 1 || e.Sequence > MaxRecords || !operationPattern.MatchString(e.OperationID) || e.EffectOperationID != "" && !operationPattern.MatchString(e.EffectOperationID) || !digestPattern.MatchString(e.RequestSHA256) || !digestPattern.MatchString(e.PolicySHA256) || !digestPattern.MatchString(e.ProfileSHA256) || e.GenerationBefore < 0 || e.GenerationAfter < e.GenerationBefore || e.GenerationAfter > MaxGeneration {
		return ErrInvalid
	}
	switch e.Action {
	case "list", "read", "write", "edit", "remove", "check", "merge_status", "merge_continue", "seal":
	default:
		return ErrInvalid
	}
	start, ok := validTime(e.StartedAt)
	if !ok {
		return ErrInvalid
	}
	recorded, ok := validTime(e.RecordedAt)
	if !ok || recorded.Before(start) {
		return ErrInvalid
	}
	switch e.Phase {
	case "started":
		if e.Revision != 1 || e.CompletedAt != "" || e.GenerationBefore != e.GenerationAfter || e.ExitCode != nil || e.Stopped != nil || e.ErrorCode != "" || e.Replayed || e.ResultSHA256 != "" {
			return ErrInvalid
		}
	case "completed", "failed":
		completed, ok := validTime(e.CompletedAt)
		if e.Revision != 2 || !ok || completed.Before(start) || recorded.Before(completed) {
			return ErrInvalid
		}
		if e.Phase == "failed" && e.ErrorCode == "" {
			return ErrInvalid
		}
	case "unknown":
		if e.Revision != 2 || e.CompletedAt != "" || e.ErrorCode == "" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if e.Path != "" && !ValidPath(e.Path) || e.CheckID != "" && !operationPattern.MatchString(e.CheckID) || e.ResultSHA256 != "" && !digestPattern.MatchString(e.ResultSHA256) || e.ErrorCode != "" && !errorPattern.MatchString(e.ErrorCode) || e.Replayed && e.Phase != "completed" && e.Phase != "failed" {
		return ErrInvalid
	}
	if e.ExitCode != nil && (*e.ExitCode < -2147483648 || *e.ExitCode > 2147483647) {
		return ErrInvalid
	}
	raw, err := json.Marshal(e)
	if err != nil || len(raw) > MaxEventBytes {
		return ErrInvalid
	}
	return nil
}

func ValidateBoundEvent(i Identity, e Event) error {
	if ValidateIdentity(i) != nil || ValidateEvent(e) != nil || CanonicalEventID(i, e) != e.EventID {
		return ErrInvalid
	}
	return nil
}

// UnmarshalJSON rejects duplicate, unknown and explicit null fields before a Go
// struct can erase them. Numeric fields retain integer-only JSON semantics.
func (e *WorkspaceOperationEvent) UnmarshalJSON(raw []byte) error {
	if len(raw) > MaxEventBytes || !utf8.Valid(raw) || !toolcontract.ValidJSONUnicode(raw) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return ErrInvalid
		}
	}
	if _, err = d.Token(); err != nil {
		return ErrInvalid
	}
	if _, err = d.Token(); err != io.EOF {
		return ErrInvalid
	}
	for _, key := range []string{"protocol", "source", "schema_version", "event_id", "sequence", "operation_id", "revision", "action", "phase", "generation_before", "generation_after", "started_at", "recorded_at", "request_sha256", "policy_sha256", "profile_sha256"} {
		if !seen[key] {
			return ErrInvalid
		}
	}
	type wire Event
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decoded) != nil {
		return ErrInvalid
	}
	if ValidateEvent(Event(decoded)) != nil {
		return ErrInvalid
	}
	*e = Event(decoded)
	return nil
}

func DecodeEvent(raw []byte) (Event, error) {
	var out Event
	err := json.Unmarshal(raw, &out)
	return out, err
}

func CloneEvent(e Event) Event {
	if e.ExitCode != nil {
		value := *e.ExitCode
		e.ExitCode = &value
	}
	if e.Stopped != nil {
		value := *e.Stopped
		e.Stopped = &value
	}
	return e
}

func ValidateCoverage(c Coverage) error {
	if c.MaxInvocations != MaxCalls || c.MaxRecords != MaxRecords || c.InvocationCount < 0 || c.InvocationCount > MaxCalls || c.RecordCount < c.InvocationCount || c.RecordCount > MaxRecords || c.PendingCount < 0 || c.PendingCount > c.InvocationCount || c.RecordCount != 2*c.InvocationCount-c.PendingCount {
		return ErrInvalid
	}
	switch c.State {
	case "complete":
		if c.Reason != "" {
			return ErrInvalid
		}
	case "partial", "unknown":
		if !errorPattern.MatchString(c.Reason) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func (c *WorkspaceOperationCoverage) UnmarshalJSON(raw []byte) error {
	if len(raw) > 1024 || !utf8.Valid(raw) || !toolcontract.ValidJSONUnicode(raw) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return ErrInvalid
		}
	}
	if _, err = d.Token(); err != nil {
		return ErrInvalid
	}
	if _, err = d.Token(); err != io.EOF {
		return ErrInvalid
	}
	for _, key := range []string{"invocation_count", "record_count", "pending_count", "max_invocations", "max_records", "state"} {
		if !seen[key] {
			return ErrInvalid
		}
	}
	type wire WorkspaceOperationCoverage
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decoded) != nil || ValidateCoverage(Coverage(decoded)) != nil {
		return ErrInvalid
	}
	*c = Coverage(decoded)
	return nil
}
