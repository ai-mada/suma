package workbuddycloudcontract

import (
	"encoding/base64"
	"encoding/json"

	"github.com/ai-mada/suma/internal/strictjson"
)

// CloudNodeProgress is the project-readable view for operator and automation
// clients. It carries persisted progress without request, account, assignment,
// conversation, or provider task identifiers.
type CloudNodeProgress struct {
	ProjectID         string `json:"project_id"`
	RunID             string `json:"run_id"`
	NodeID            string `json:"node_id"`
	RunVersion        int    `json:"run_version"`
	RunState          string `json:"run_state"`
	NodeState         string `json:"node_state"`
	NodeReason        string `json:"node_reason,omitempty"`
	Attempt           int    `json:"attempt"`
	SubmissionState   string `json:"submission_state"`
	ProviderStatus    string `json:"provider_status,omitempty"`
	ObservedAt        string `json:"observed_at,omitempty"`
	ObservationFailed bool   `json:"observation_failed"`
}

// DecodeCloudNodeProgress requires the two zero-valued facts to be present as
// well as the fields whose required values are checked by Validate.
func DecodeCloudNodeProgress(raw []byte) (CloudNodeProgress, error) {
	var p CloudNodeProgress
	var fields map[string]json.RawMessage
	if Decode(raw, &p) != nil || json.Unmarshal(raw, &fields) != nil ||
		fields["attempt"] == nil || fields["observation_failed"] == nil || p.Validate() != nil {
		return CloudNodeProgress{}, ErrInvalid
	}
	return p, nil
}

func (p CloudNodeProgress) Validate() error {
	if !validID(p.ProjectID, 64) || !validID(p.RunID, 100) || !validID(p.NodeID, 150) ||
		!validVersion(p.RunVersion) || !errorCodePattern.MatchString(p.RunState) || !errorCodePattern.MatchString(p.NodeState) ||
		p.Attempt < 0 || p.Attempt > MaxVersion || CloudProgressReason(p.NodeReason) != p.NodeReason {
		return ErrInvalid
	}
	switch p.SubmissionState {
	case "not_recorded", "prepared", "unknown":
		if p.ProviderStatus != "" || p.ObservedAt != "" || p.ObservationFailed {
			return ErrInvalid
		}
	case "task_known":
		if _, ok := parseTime(p.ObservedAt); !ok || CloudProgressProviderStatus(p.ProviderStatus) != p.ProviderStatus {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// CloudProgressAttentionFilter selects saved records requiring attention.
const CloudProgressAttentionFilter = "attention"

// NeedsAttention classifies the same public facts displayed in the workbench.
func (p CloudNodeProgress) NeedsAttention() bool {
	if p.NodeReason != "" || p.SubmissionState == "unknown" || p.ObservationFailed {
		return true
	}
	switch p.ProviderStatus {
	case "waiting_input", "failed", "unknown":
		return true
	default:
		return false
	}
}

// CloudNodeProgressPage contains only saved submissions in the current project.
type CloudNodeProgressPage struct {
	ProjectID  string              `json:"project_id"`
	Items      []CloudNodeProgress `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
	Filter     string              `json:"filter,omitempty"`
}

func DecodeCloudNodeProgressPage(raw []byte) (CloudNodeProgressPage, error) {
	var out CloudNodeProgressPage
	if strictjson.Decode(raw, &out, 64<<10) != nil || !validID(out.ProjectID, 64) || out.Items == nil || len(out.Items) > 20 ||
		(out.Filter != "" && out.Filter != CloudProgressAttentionFilter) || len(out.NextCursor) > 1024 ||
		out.Filter == "" && out.NextCursor != "" && len(out.Items) == 0 {
		return CloudNodeProgressPage{}, ErrInvalid
	}
	if _, err := base64.RawURLEncoding.Strict().DecodeString(out.NextCursor); err != nil {
		return CloudNodeProgressPage{}, ErrInvalid
	}
	seen := make(map[[2]string]bool, len(out.Items))
	for _, p := range out.Items {
		key := [2]string{p.RunID, p.NodeID}
		if p.Validate() != nil || p.ProjectID != out.ProjectID || p.SubmissionState == "not_recorded" || seen[key] ||
			out.Filter == CloudProgressAttentionFilter && !p.NeedsAttention() {
			return CloudNodeProgressPage{}, ErrInvalid
		}
		seen[key] = true
	}
	return out, nil
}

// CloudProgressReason projects only platform-defined WorkBuddy outcomes. An
// arbitrary stored node diagnostic must never become a public status code.
func CloudProgressReason(reason string) string {
	switch reason {
	case "cloud.authorization_changed", "cloud.creation_unconfirmed", "cloud.delivery_required",
		"cloud.interrupted", "cloud.not_created", "cloud.observation_unconfirmed", "cloud.remote_unknown",
		"cloud.retry_requires_reconciliation", "cloud.runner_disconnected", "cloud.stop_unconfirmed", "cloud.waiting_input":
		return reason
	default:
		return ""
	}
}

// CloudProgressProviderStatus keeps recognised status codes and projects an
// unfamiliar provider string as unknown, without exposing its original text.
func CloudProgressProviderStatus(status string) string {
	switch status {
	case "pending", "queued", "running", "working", "waiting_input", "completed", "failed", "cancelled", "canceled":
		return status
	default:
		return "unknown"
	}
}
