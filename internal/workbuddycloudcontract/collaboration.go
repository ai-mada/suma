package workbuddycloudcontract

import (
	"encoding/json"
	"fmt"
	"io"
	"unicode"
	"unicode/utf8"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

const (
	CloudHistoryMaxEntries    = 500
	CloudHistoryMaxTextBytes  = 1 << 20
	CloudHistoryMaxEntryBytes = 64 << 10
)

// CloudCollaborationSource identifies the original task. SubmissionVersion is
// the observation at load admission, not a supplier history cursor.
type CloudCollaborationSource struct {
	ProjectID         string `json:"project_id"`
	RunID             string `json:"run_id"`
	NodeID            string `json:"node_id"`
	SubmissionID      string `json:"submission_id"`
	SubmissionVersion int    `json:"submission_version"`
	TaskID            string `json:"task_id"`
	NodeSpecSHA256    string `json:"node_spec_sha256"`
	RequestSHA256     string `json:"request_sha256"`
}

func (s CloudCollaborationSource) Validate() error {
	if !validID(s.ProjectID, 64) || !validID(s.RunID, 100) || !validID(s.NodeID, 150) || !validID(s.SubmissionID, 64) || !validVersion(s.SubmissionVersion) || !taskIDPattern.MatchString(s.TaskID) || !cc.ValidHash(s.NodeSpecSHA256) || !cc.ValidHash(s.RequestSHA256) {
		return ErrInvalid
	}
	return nil
}

type CloudHistoryLoadRequest struct {
	ProjectID                 string `json:"project_id"`
	ExpectedSubmissionVersion int    `json:"expected_submission_version"`
	ExpectedVersion           int    `json:"expected_version"`
	IdempotencyKey            string `json:"idempotency_key"`
}

// Zero is the initial head version; unlike an omitted precondition it is an
// explicit concurrency assertion. The alias preserves the closed JSON decoder.
func (r *CloudHistoryLoadRequest) UnmarshalJSON(raw []byte) error {
	type wire CloudHistoryLoadRequest
	var out wire
	if decodeCollaborationRequired(raw, &out, "project_id", "expected_submission_version", "expected_version", "idempotency_key") != nil {
		return ErrInvalid
	}
	*r = CloudHistoryLoadRequest(out)
	return nil
}

func decodeCollaborationRequired(raw []byte, out any, required ...string) error {
	if Decode(raw, out) != nil {
		return ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return ErrInvalid
	}
	for _, field := range required {
		if fields[field] == nil {
			return ErrInvalid
		}
	}
	return nil
}

func (r CloudHistoryLoadRequest) Validate() error {
	if !validID(r.ProjectID, 64) || !expectedVersion(r.ExpectedSubmissionVersion) || r.ExpectedVersion < 0 || r.ExpectedVersion >= MaxVersion || !operationID(r.IdempotencyKey) {
		return ErrInvalid
	}
	return nil
}

type CloudHistoryLoad struct {
	ID                 string                   `json:"id"`
	Version            int                      `json:"version"`
	OperationID        string                   `json:"operation_id"`
	HeadVersionAtStart int                      `json:"head_version_at_start"`
	Source             CloudCollaborationSource `json:"source"`
	State              string                   `json:"state"`
	Coverage           string                   `json:"coverage"`
	StartedAt          string                   `json:"started_at"`
	DeadlineAt         string                   `json:"deadline_at"`
	Overdue            bool                     `json:"overdue"`
	FinishedAt         string                   `json:"finished_at,omitempty"`
	EntryCount         int                      `json:"entry_count"`
	UnsupportedCount   int                      `json:"unsupported_count"`
	TextBytes          int                      `json:"text_bytes"`
	ErrorCode          string                   `json:"error_code,omitempty"`
}

func (l *CloudHistoryLoad) UnmarshalJSON(raw []byte) error {
	type wire CloudHistoryLoad
	var out wire
	if decodeCollaborationRequired(raw, &out, "id", "version", "operation_id", "head_version_at_start", "source", "state", "coverage", "started_at", "deadline_at", "overdue", "entry_count", "unsupported_count", "text_bytes") != nil {
		return ErrInvalid
	}
	*l = CloudHistoryLoad(out)
	return nil
}

func (l CloudHistoryLoad) Validate() error {
	started, startedOK := parseTime(l.StartedAt)
	deadline, deadlineOK := parseTime(l.DeadlineAt)
	if !validID(l.ID, 64) || !validVersion(l.Version) || !operationID(l.OperationID) || l.HeadVersionAtStart < 0 || l.HeadVersionAtStart >= MaxVersion || l.Source.Validate() != nil || l.Coverage != "unverified" || !startedOK || !deadlineOK || !deadline.After(started) || l.EntryCount < 0 || l.EntryCount > CloudHistoryMaxEntries || l.UnsupportedCount < 0 || l.UnsupportedCount > l.EntryCount || l.TextBytes < 0 || l.TextBytes > CloudHistoryMaxTextBytes || l.EntryCount == 0 && l.TextBytes != 0 {
		return ErrInvalid
	}
	if l.State == "loading" {
		if l.FinishedAt != "" || l.ErrorCode != "" {
			return ErrInvalid
		}
		return nil
	}
	finished, finishedOK := parseTime(l.FinishedAt)
	if !finishedOK || finished.Before(started) || l.Overdue {
		return ErrInvalid
	}
	if l.State == "loaded" {
		if l.ErrorCode != "" {
			return ErrInvalid
		}
		return nil
	}
	if l.State != "interrupted" && l.State != "limited" && l.State != "rejected" {
		return ErrInvalid
	}
	switch l.ErrorCode {
	case "workbuddy_cloud_history_unavailable", "workbuddy_cloud_history_interrupted", "workbuddy_cloud_history_limited", "workbuddy_cloud_history_rejected":
		return nil
	default:
		return ErrInvalid
	}
}

type CloudHistoryEntry struct {
	Sequence int    `json:"sequence"`
	Kind     string `json:"kind"`
	Text     string `json:"text,omitempty"`
}

func (CloudHistoryEntry) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddycloudcontract.CloudHistoryEntry{text:redacted}")
}

func (e CloudHistoryEntry) Validate() error {
	if e.Sequence < 1 || e.Sequence > CloudHistoryMaxEntries || !utf8.ValidString(e.Text) || len(e.Text) > CloudHistoryMaxEntryBytes {
		return ErrInvalid
	}
	for _, r := range e.Text {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return ErrInvalid
		}
	}
	switch e.Kind {
	case "user_text", "assistant_text", "tool_summary", "plan_summary":
		return nil
	case "unsupported":
		if e.Text == "" {
			return nil
		}
	}
	return ErrInvalid
}

type CloudCollaborationView struct {
	Source         CloudCollaborationSource `json:"source"`
	Version        int                      `json:"version"`
	LatestLoad     *CloudHistoryLoad        `json:"latest_load,omitempty" wire:"nonnull"`
	LastLoaded     *CloudHistoryLoad        `json:"last_loaded,omitempty" wire:"nonnull"`
	AllowedActions []string                 `json:"allowed_actions"`
}

func (v *CloudCollaborationView) UnmarshalJSON(raw []byte) error {
	type wire CloudCollaborationView
	var out wire
	if decodeCollaborationRequired(raw, &out, "source", "version", "allowed_actions") != nil {
		return ErrInvalid
	}
	*v = CloudCollaborationView(out)
	return nil
}

func sameCollaborationSource(a, b CloudCollaborationSource) bool {
	a.SubmissionVersion, b.SubmissionVersion = 0, 0
	return a == b
}

func (v CloudCollaborationView) Validate() error {
	if v.Source.Validate() != nil || v.Version < 0 || v.Version > MaxVersion || v.AllowedActions == nil || len(v.AllowedActions) > 1 || len(v.AllowedActions) == 1 && v.AllowedActions[0] != "load_history" {
		return ErrInvalid
	}
	if v.Version == 0 && (v.LatestLoad != nil || v.LastLoaded != nil) || v.Version > 0 && v.LatestLoad == nil {
		return ErrInvalid
	}
	for _, load := range []*CloudHistoryLoad{v.LatestLoad, v.LastLoaded} {
		if load != nil && (load.Validate() != nil || !sameCollaborationSource(v.Source, load.Source)) {
			return ErrInvalid
		}
	}
	if v.LastLoaded != nil && v.LastLoaded.State != "loaded" {
		return ErrInvalid
	}
	return nil
}

type CloudHistoryPage struct {
	Load      CloudHistoryLoad    `json:"load"`
	Items     []CloudHistoryEntry `json:"items"`
	Total     int                 `json:"total"`
	NextAfter int                 `json:"next_after"`
	HasMore   bool                `json:"has_more"`
}

func (p *CloudHistoryPage) UnmarshalJSON(raw []byte) error {
	type wire CloudHistoryPage
	var out wire
	if decodeCollaborationRequired(raw, &out, "load", "items", "total", "next_after", "has_more") != nil {
		return ErrInvalid
	}
	*p = CloudHistoryPage(out)
	return nil
}

func (CloudHistoryPage) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddycloudcontract.CloudHistoryPage{text:redacted}")
}

func (p CloudHistoryPage) Validate() error {
	if p.Load.Validate() != nil || p.Items == nil || len(p.Items) > 100 || p.Total != p.Load.EntryCount || len(p.Items) > p.Total || p.NextAfter < 0 || p.NextAfter > CloudHistoryMaxEntries || p.HasMore != (p.NextAfter < p.Total) || p.HasMore && len(p.Items) == 0 {
		return ErrInvalid
	}
	previous := 0
	for _, item := range p.Items {
		if item.Validate() != nil || item.Sequence > p.Total || previous != 0 && item.Sequence != previous+1 {
			return ErrInvalid
		}
		previous = item.Sequence
	}
	if previous != 0 && p.NextAfter != previous {
		return ErrInvalid
	}
	return nil
}
