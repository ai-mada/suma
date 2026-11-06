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

const (
	// CloudProgressAttentionFilter selects saved records requiring attention.
	CloudProgressAttentionFilter = "attention"
	// CloudProgressDeliveryFilter selects saved executions waiting for handoff.
	CloudProgressDeliveryFilter = "delivery"
	// CloudProgressInputFilter selects saved executions waiting for input.
	CloudProgressInputFilter = "input"
)

// ValidCloudProgressFilter reports whether a nonempty progress filter is supported.
func ValidCloudProgressFilter(filter string) bool {
	return filter == CloudProgressAttentionFilter || CloudProgressRequiresCurrentTask(filter)
}

// CloudProgressRequiresCurrentTask identifies the queues that exclude historical or independent runs.
func CloudProgressRequiresCurrentTask(filter string) bool {
	return filter == CloudProgressDeliveryFilter || filter == CloudProgressInputFilter
}

// MatchesFilter classifies saved facts. A provider observation is neither an
// input request nor evidence of candidate delivery or human acceptance.
func (p CloudNodeProgress) MatchesFilter(filter string) bool {
	switch filter {
	case "":
		return true
	case CloudProgressAttentionFilter:
		return p.NeedsAttention()
	case CloudProgressDeliveryFilter, CloudProgressInputFilter:
		// Page readers also require an exact, still-current formal task link.
		if p.RunState != "running" || p.NodeState != "waiting" || p.Attempt < 1 || p.SubmissionState != "task_known" {
			return false
		}
		if filter == CloudProgressDeliveryFilter {
			return p.NodeReason == "cloud.delivery_required"
		}
		return p.NodeReason == "cloud.waiting_input" && p.ProviderStatus == "waiting_input" && !p.ObservationFailed
	default:
		return false
	}
}

// ValidCloudProgressTaskID accepts the exact formal task identifiers used in progress links.
func ValidCloudProgressTaskID(id string) bool {
	return cloudDeliveryTaskIDPattern.MatchString(id) && id != "." && id != ".."
}

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

// CloudProgressTaskLink describes the formal task behind an exact progress row.
// The current task binding can differ from the historical cloud execution.
type CloudProgressTaskLink struct {
	RunID       string `json:"run_id"`
	NodeID      string `json:"node_id"`
	TaskID      string `json:"task_id"`
	TaskVersion int    `json:"task_version"`
	RunRelation string `json:"run_relation"`
	TaskTitle   string `json:"task_title,omitempty"`
}

func (link CloudProgressTaskLink) Validate() error {
	target := CloudTaskDeliveryLink{TaskID: link.TaskID, TaskVersion: link.TaskVersion, RunRelation: link.RunRelation}
	if !validID(link.RunID, 100) || !validID(link.NodeID, 150) || !validText(link.TaskTitle, 500, true) {
		return ErrInvalid
	}
	return target.Validate()
}

// CloudNodeProgressPage contains only saved submissions in the current project.
type CloudNodeProgressPage struct {
	ProjectID  string                  `json:"project_id"`
	Items      []CloudNodeProgress     `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
	Filter     string                  `json:"filter,omitempty"`
	TaskID     string                  `json:"task_id,omitempty"`
	TaskLinks  []CloudProgressTaskLink `json:"task_links,omitempty"`
}

func DecodeCloudNodeProgressPage(raw []byte) (CloudNodeProgressPage, error) {
	var wire struct {
		CloudNodeProgressPage
		TaskID *string `json:"task_id,omitempty"`
	}
	if strictjson.Decode(raw, &wire, 64<<10) != nil {
		return CloudNodeProgressPage{}, ErrInvalid
	}
	out := wire.CloudNodeProgressPage
	if wire.TaskID != nil {
		if !ValidCloudProgressTaskID(*wire.TaskID) {
			return CloudNodeProgressPage{}, ErrInvalid
		}
		out.TaskID = *wire.TaskID
	}
	if !validID(out.ProjectID, 64) || out.Items == nil || len(out.Items) > 20 ||
		(out.Filter != "" && !ValidCloudProgressFilter(out.Filter)) || len(out.NextCursor) > 1024 ||
		out.Filter == "" && out.TaskID == "" && out.NextCursor != "" && len(out.Items) == 0 {
		return CloudNodeProgressPage{}, ErrInvalid
	}
	if _, err := base64.RawURLEncoding.Strict().DecodeString(out.NextCursor); err != nil {
		return CloudNodeProgressPage{}, ErrInvalid
	}
	seen := make(map[[2]string]bool, len(out.Items))
	for _, p := range out.Items {
		key := [2]string{p.RunID, p.NodeID}
		if p.Validate() != nil || p.ProjectID != out.ProjectID || p.SubmissionState == "not_recorded" || seen[key] ||
			!p.MatchesFilter(out.Filter) {
			return CloudNodeProgressPage{}, ErrInvalid
		}
		seen[key] = true
	}
	requiresCurrentTask := CloudProgressRequiresCurrentTask(out.Filter)
	for _, link := range out.TaskLinks {
		key := [2]string{link.RunID, link.NodeID}
		if link.Validate() != nil || !seen[key] || out.TaskID != "" && link.TaskID != out.TaskID ||
			requiresCurrentTask && link.RunRelation != "current" {
			return CloudNodeProgressPage{}, ErrInvalid
		}
		delete(seen, key)
	}
	if (out.TaskID != "" || requiresCurrentTask) && len(seen) != 0 {
		return CloudNodeProgressPage{}, ErrInvalid
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
