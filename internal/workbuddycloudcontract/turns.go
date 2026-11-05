package workbuddycloudcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const CloudTurnMaxEntries = 1000
const CloudTurnMaxTextBytes = 1 << 20
const CloudTurnMaxEntryBytes = 32 << 10
const CloudTurnMaxPermissions = 32
const CloudTurnArtifactPreviewMaxBytes = 8 << 10

type CloudTurnSource struct {
	Collaboration    CloudCollaborationSource `json:"collaboration"`
	Attempt          int                      `json:"attempt"`
	Fence            int64                    `json:"fence"`
	DispatchID       string                   `json:"dispatch_id"`
	AssignmentSHA256 string                   `json:"assignment_sha256"`
}
type CloudTurnPromptRequest struct {
	ProjectID                 string `json:"project_id"`
	ExpectedSubmissionVersion int    `json:"expected_submission_version"`
	ExpectedRunVersion        int    `json:"expected_run_version"`
	ExpectedVersion           int    `json:"expected_version"`
	Prompt                    string `json:"prompt"`
	IdempotencyKey            string `json:"idempotency_key"`
}
type CloudTurn struct {
	ID                 string          `json:"id"`
	Version            int             `json:"version"`
	OperationID        string          `json:"operation_id"`
	HeadVersionAtStart int             `json:"head_version_at_start"`
	Source             CloudTurnSource `json:"source"`
	OwnerID            string          `json:"owner_id"`
	State              string          `json:"state"`
	PromptSHA256       string          `json:"prompt_sha256"`
	StartedAt          string          `json:"started_at"`
	DeadlineAt         string          `json:"deadline_at"`
	Overdue            bool            `json:"overdue"`
	FinishedAt         string          `json:"finished_at,omitempty"`
	StopReason         string          `json:"stop_reason,omitempty"`
	ErrorCode          string          `json:"error_code,omitempty"`
	UsageState         string          `json:"usage_state"`
	Usage              *tc.Usage       `json:"usage,omitempty" wire:"nonnull"`
}
type CloudTurnView struct {
	Source         CloudTurnSource `json:"source"`
	Version        int             `json:"version"`
	LatestTurn     *CloudTurn      `json:"latest_turn,omitempty" wire:"nonnull"`
	AllowedActions []string        `json:"allowed_actions"`
}
type CloudTurnEntry struct {
	Sequence      int    `json:"sequence"`
	Kind          string `json:"kind"`
	Text          string `json:"text"`
	ToolCallID    string `json:"tool_call_id,omitempty"`
	ToolState     string `json:"tool_state,omitempty"`
	ArtifactType  string `json:"artifact_type,omitempty"`
	ArtifactEvent string `json:"artifact_event,omitempty"`
}
type CloudTurnPermissionOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}
type CloudTurnPermission struct {
	ID             string                      `json:"id"`
	TurnID         string                      `json:"turn_id"`
	Version        int                         `json:"version"`
	ToolCallID     string                      `json:"tool_call_id"`
	Title          string                      `json:"title"`
	Details        string                      `json:"details"`
	DetailsSHA256  string                      `json:"details_sha256"`
	ApproveAllowed bool                        `json:"approve_allowed"`
	State          string                      `json:"state"`
	Options        []CloudTurnPermissionOption `json:"options"`
}
type CloudTurnCommand struct {
	ID           string `json:"id"`
	TurnID       string `json:"turn_id"`
	OperationID  string `json:"operation_id"`
	Kind         string `json:"kind"`
	State        string `json:"state"`
	PermissionID string `json:"permission_id,omitempty"`
	OptionID     string `json:"option_id,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
}
type CloudTurnPermissionReplyRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	OptionID        string `json:"option_id"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type CloudTurnControlRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type CloudTurnPage struct {
	Turn        CloudTurn             `json:"turn"`
	Items       []CloudTurnEntry      `json:"items"`
	Permissions []CloudTurnPermission `json:"permissions"`
	Commands    []CloudTurnCommand    `json:"commands"`
	NextAfter   int                   `json:"next_after"`
	HasMore     bool                  `json:"has_more"`
}

func (s CloudTurnSource) Validate() error {
	if s.Collaboration.Validate() != nil || !validVersion(s.Attempt) || s.Fence < 1 || s.Fence > tc.MaxSafeUsage || !validID(s.DispatchID, 128) || !cc.ValidHash(s.AssignmentSHA256) {
		return ErrInvalid
	}
	return nil
}
func (r CloudTurnPromptRequest) Validate() error {
	if !validID(r.ProjectID, 64) || !expectedVersion(r.ExpectedSubmissionVersion) || !expectedVersion(r.ExpectedRunVersion) || r.ExpectedVersion < 0 || r.ExpectedVersion > MaxVersion-2 || strings.TrimSpace(r.Prompt) == "" || !cloudTurnText(r.Prompt, 32000) || !operationID(r.IdempotencyKey) {
		return ErrInvalid
	}
	return nil
}
func (r CloudTurnControlRequest) Validate() error {
	if !validID(r.ProjectID, 64) || !expectedVersion(r.ExpectedVersion) || !operationID(r.IdempotencyKey) {
		return ErrInvalid
	}
	return nil
}
func (r CloudTurnPermissionReplyRequest) Validate() error {
	if !validID(r.ProjectID, 64) || !expectedVersion(r.ExpectedVersion) || !cloudTurnProviderToken(r.OptionID, 128) || !operationID(r.IdempotencyKey) {
		return ErrInvalid
	}
	return nil
}
func cloudTurnCode(s string) bool {
	switch s {
	case "", "workbuddy_cloud_turn_unknown", "workbuddy_cloud_turn_interrupted", "workbuddy_cloud_turn_limited", "workbuddy_cloud_turn_authorization_changed", "workbuddy_cloud_turn_rejected", "workbuddy_cloud_turn_unavailable":
		return true
	}
	return false
}
func cloudTurnProviderToken(s string, max int) bool {
	if !validText(s, max, false) || strings.TrimSpace(s) != s || s == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Conversation text, unlike identifiers, preserves normal code/newline layout.
// Reuse the existing history contract's allowed whitespace without changing
// the validation of any other cloud request or identifier.
func cloudTurnText(s string, max int) bool {
	if !utf8.ValidString(s) || len(s) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
func cloudTurnStop(s string) bool {
	switch s {
	case "end_turn", "max_tokens", "max_turn_requests", "refusal", "cancelled":
		return true
	}
	return false
}
func (t CloudTurn) Validate() error {
	started, startOK := parseTime(t.StartedAt)
	deadline, endOK := parseTime(t.DeadlineAt)
	if !validID(t.ID, 64) || !validVersion(t.Version) || !operationID(t.OperationID) || t.HeadVersionAtStart < 0 || t.HeadVersionAtStart > MaxVersion-2 || t.Source.Validate() != nil || !validID(t.OwnerID, 64) || !cc.ValidHash(t.PromptSHA256) || !startOK || !endOK || !deadline.After(started) || deadline.Sub(started) > 15*time.Minute || !cloudTurnCode(t.ErrorCode) || t.StopReason != "" && !cloudTurnStop(t.StopReason) {
		return ErrInvalid
	}
	switch t.UsageState {
	case tc.UsageUnreported:
		if t.Usage != nil {
			return ErrInvalid
		}
	case tc.UsagePartial:
		if t.Usage == nil {
			return ErrInvalid
		}
		raw, err := json.Marshal(t.Usage)
		if err != nil || string(raw) == "{}" {
			return ErrInvalid
		}
		var checked tc.Usage
		if json.Unmarshal(raw, &checked) != nil {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	switch t.State {
	case "prepared", "running", "waiting_permission":
		if t.FinishedAt != "" || t.StopReason != "" || t.ErrorCode != "" {
			return ErrInvalid
		}
	case "completed", "unknown", "rejected":
		finished, ok := parseTime(t.FinishedAt)
		if !ok || finished.Before(started) || t.Overdue {
			return ErrInvalid
		}
		if t.State == "completed" && (!cloudTurnStop(t.StopReason) || t.ErrorCode != "") {
			return ErrInvalid
		}
		if t.State != "completed" && t.ErrorCode == "" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
func sameCloudTurnSource(a, b CloudTurnSource) bool {
	a.Collaboration.SubmissionVersion = 0
	b.Collaboration.SubmissionVersion = 0
	return a == b
}
func (t CloudTurnView) Validate() error {
	if t.Source.Validate() != nil || t.Version < 0 || t.Version > MaxVersion || t.AllowedActions == nil {
		return ErrInvalid
	}
	last := -1
	for _, action := range t.AllowedActions {
		at := -1
		switch action {
		case "prompt":
			at = 0
		case "cancel":
			at = 1
		case "reconcile":
			at = 2
		}
		if at <= last {
			return ErrInvalid
		}
		last = at
	}
	if t.LatestTurn == nil {
		if t.Version != 0 {
			return ErrInvalid
		}
	} else if t.Version < 1 || t.LatestTurn.Validate() != nil || !sameCloudTurnSource(t.Source, t.LatestTurn.Source) {
		return ErrInvalid
	}
	return nil
}

// CloudTurnArtifactPreviewValid keeps optional provider text inside the same
// display character boundary as a conversation, with a smaller byte limit.
func CloudTurnArtifactPreviewValid(text string) bool {
	return cloudTurnText(text, CloudTurnArtifactPreviewMaxBytes)
}

func validCloudArtifactChange(kind, event string) bool {
	switch kind {
	case "plan", "tasks", "media", "overview":
		return event == "created" || event == "updated" || event == "deleted"
	default:
		return false
	}
}

func (e CloudTurnEntry) Validate() error {
	if e.Sequence < 1 || e.Sequence > CloudTurnMaxEntries || !cloudTurnText(e.Text, CloudTurnMaxEntryBytes) || e.ToolCallID != "" && !cloudTurnProviderToken(e.ToolCallID, 128) {
		return ErrInvalid
	}
	if e.Kind == "artifact" {
		if e.ToolCallID != "" || e.ToolState != "" || !validCloudArtifactChange(e.ArtifactType, e.ArtifactEvent) {
			return ErrInvalid
		}
		if e.ArtifactEvent == "deleted" || (e.ArtifactType != "plan" && e.ArtifactType != "overview") {
			if e.Text != "" {
				return ErrInvalid
			}
		} else if !CloudTurnArtifactPreviewValid(e.Text) {
			return ErrInvalid
		}
		return nil
	}
	if e.ArtifactType != "" || e.ArtifactEvent != "" {
		return ErrInvalid
	}
	switch e.Kind {
	case "assistant_text", "plan_summary", "notice", "unsupported":
		if e.ToolState != "" {
			return ErrInvalid
		}
	case "tool_state":
		if !cloudTurnProviderToken(e.ToolCallID, 128) || e.ToolState != "pending" && e.ToolState != "in_progress" && e.ToolState != "completed" && e.ToolState != "failed" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
func (o CloudTurnPermissionOption) Validate() error {
	if !cloudTurnProviderToken(o.ID, 128) || !validText(o.Name, 512, false) || strings.TrimSpace(o.Name) == "" {
		return ErrInvalid
	}
	switch o.Kind {
	case "allow_once", "allow_always", "reject_once", "reject_always":
		return nil
	}
	return ErrInvalid
}
func (p CloudTurnPermission) Validate() error {
	if !validID(p.ID, 64) || !validID(p.TurnID, 64) || !validVersion(p.Version) || !cloudTurnProviderToken(p.ToolCallID, 128) || !cloudTurnText(p.Title, 4096) || !cloudTurnText(p.Details, CloudTurnMaxEntryBytes) || !cc.ValidHash(p.DetailsSHA256) || p.Options == nil || len(p.Options) == 0 || len(p.Options) > 32 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, o := range p.Options {
		if o.Validate() != nil || seen[o.ID] {
			return ErrInvalid
		}
		seen[o.ID] = true
	}
	if p.ApproveAllowed {
		hash := sha256.Sum256([]byte(p.Details))
		if hex.EncodeToString(hash[:]) != p.DetailsSHA256 {
			return ErrInvalid
		}
	}
	switch p.State {
	case "pending", "prepared", "sent", "acknowledged", "unknown", "rejected":
		return nil
	}
	return ErrInvalid
}
func (c CloudTurnCommand) Validate() error {
	if !validID(c.ID, 64) || !validID(c.TurnID, 64) || !operationID(c.OperationID) || !cloudTurnCode(c.ErrorCode) {
		return ErrInvalid
	}
	switch c.State {
	case "prepared", "sent", "acknowledged", "unknown", "rejected":
	default:
		return ErrInvalid
	}
	switch c.Kind {
	case "permission":
		if !validID(c.PermissionID, 64) || !cloudTurnProviderToken(c.OptionID, 128) {
			return ErrInvalid
		}
	case "cancel", "reconcile":
		if c.PermissionID != "" || c.OptionID != "" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
func (p CloudTurnPage) Validate() error {
	if p.Turn.Validate() != nil || p.Items == nil || p.Permissions == nil || p.Commands == nil || len(p.Items) > 200 || p.NextAfter < 0 || p.NextAfter > CloudTurnMaxEntries || len(p.Permissions) > CloudTurnMaxPermissions || len(p.Commands) > 256 {
		return ErrInvalid
	}
	previous := 0
	for _, item := range p.Items {
		if item.Validate() != nil || item.Sequence <= previous || item.Sequence > p.NextAfter {
			return ErrInvalid
		}
		previous = item.Sequence
	}
	if len(p.Items) > 0 && previous != p.NextAfter {
		return ErrInvalid
	}
	ids := map[string]bool{}
	for _, permission := range p.Permissions {
		if permission.Validate() != nil || permission.TurnID != p.Turn.ID || ids[permission.ID] {
			return ErrInvalid
		}
		ids[permission.ID] = true
	}
	ids = map[string]bool{}
	for _, command := range p.Commands {
		if command.Validate() != nil || command.TurnID != p.Turn.ID || ids[command.ID] {
			return ErrInvalid
		}
		ids[command.ID] = true
	}
	return nil
}

func (r *CloudTurnPromptRequest) UnmarshalJSON(raw []byte) error {
	type wire CloudTurnPromptRequest
	var out wire
	if decodeCollaborationRequired(raw, &out, "project_id", "expected_submission_version", "expected_run_version", "expected_version", "prompt", "idempotency_key") != nil {
		return ErrInvalid
	}
	*r = CloudTurnPromptRequest(out)
	return nil
}
func (r *CloudTurnControlRequest) UnmarshalJSON(raw []byte) error {
	type wire CloudTurnControlRequest
	var out wire
	if decodeCollaborationRequired(raw, &out, "project_id", "expected_version", "idempotency_key") != nil {
		return ErrInvalid
	}
	*r = CloudTurnControlRequest(out)
	return nil
}
func (r *CloudTurnPermissionReplyRequest) UnmarshalJSON(raw []byte) error {
	type wire CloudTurnPermissionReplyRequest
	var out wire
	if decodeCollaborationRequired(raw, &out, "project_id", "expected_version", "option_id", "idempotency_key") != nil {
		return ErrInvalid
	}
	*r = CloudTurnPermissionReplyRequest(out)
	return nil
}
func (r *CloudTurnSource) UnmarshalJSON(raw []byte) error {
	type wire CloudTurnSource
	var out wire
	if decodeCollaborationRequired(raw, &out, "collaboration", "attempt", "fence", "dispatch_id", "assignment_sha256") != nil {
		return ErrInvalid
	}
	*r = CloudTurnSource(out)
	return nil
}
func (r *CloudTurn) UnmarshalJSON(raw []byte) error {
	type wire CloudTurn
	var out wire
	if decodeCollaborationRequired(raw, &out, "id", "version", "operation_id", "head_version_at_start", "source", "owner_id", "state", "prompt_sha256", "started_at", "deadline_at", "overdue", "usage_state") != nil {
		return ErrInvalid
	}
	*r = CloudTurn(out)
	return nil
}
func (r *CloudTurnView) UnmarshalJSON(raw []byte) error {
	type wire CloudTurnView
	var out wire
	if decodeCollaborationRequired(raw, &out, "source", "version", "allowed_actions") != nil {
		return ErrInvalid
	}
	*r = CloudTurnView(out)
	return nil
}
func (r *CloudTurnEntry) UnmarshalJSON(raw []byte) error {
	type wire CloudTurnEntry
	var out wire
	if decodeCollaborationRequired(raw, &out, "sequence", "kind", "text") != nil {
		return ErrInvalid
	}
	*r = CloudTurnEntry(out)
	return nil
}
func (r *CloudTurnPermissionOption) UnmarshalJSON(raw []byte) error {
	type wire CloudTurnPermissionOption
	var out wire
	if decodeCollaborationRequired(raw, &out, "id", "name", "kind") != nil {
		return ErrInvalid
	}
	*r = CloudTurnPermissionOption(out)
	return nil
}
func (r *CloudTurnPermission) UnmarshalJSON(raw []byte) error {
	type wire CloudTurnPermission
	var out wire
	if decodeCollaborationRequired(raw, &out, "id", "turn_id", "version", "tool_call_id", "title", "details", "details_sha256", "approve_allowed", "state", "options") != nil {
		return ErrInvalid
	}
	*r = CloudTurnPermission(out)
	return nil
}
func (r *CloudTurnCommand) UnmarshalJSON(raw []byte) error {
	type wire CloudTurnCommand
	var out wire
	if decodeCollaborationRequired(raw, &out, "id", "turn_id", "operation_id", "kind", "state") != nil {
		return ErrInvalid
	}
	*r = CloudTurnCommand(out)
	return nil
}
func (r *CloudTurnPage) UnmarshalJSON(raw []byte) error {
	type wire CloudTurnPage
	var out wire
	if decodeCollaborationRequired(raw, &out, "turn", "items", "permissions", "commands", "next_after", "has_more") != nil {
		return ErrInvalid
	}
	*r = CloudTurnPage(out)
	return nil
}
func (CloudTurnPromptRequest) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "CloudTurnPromptRequest{redacted}")
}
func (CloudTurnEntry) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "CloudTurnEntry{redacted}")
}
func (CloudTurnPermission) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "CloudTurnPermission{redacted}")
}
func (CloudTurnPage) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "CloudTurnPage{redacted}")
}
