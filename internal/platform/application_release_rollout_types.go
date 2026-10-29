package platform

// ApplicationReleaseRollout freezes an explicitly authorized execution of one
// original plan group. Each member remains an ordinary approved release plan;
// this record grants neither approval nor an alternative deployment protocol.
type ApplicationReleaseRollout struct {
	ID                   string                            `json:"id"`
	ProjectID            string                            `json:"project_id"`
	ReleaseSetID         string                            `json:"release_set_id"`
	ReleaseSetSHA256     string                            `json:"release_set_sha256"`
	PlanGroupID          string                            `json:"plan_group_id"`
	PlanGroupSHA256      string                            `json:"plan_group_sha256"`
	ParentRunID          string                            `json:"parent_run_id"`
	Members              []ApplicationReleaseRolloutMember `json:"members"`
	SnapshotSHA256       string                            `json:"snapshot_sha256"`
	StartedPreviewSHA256 string                            `json:"started_preview_sha256"`
	Reason               string                            `json:"reason"`
	CreatedBy            string                            `json:"created_by"`
	CreatedAt            string                            `json:"created_at"`
}

type ApplicationReleaseRolloutMember struct {
	ApplicationID   string   `json:"application_id"`
	ApplicationName string   `json:"application_name"`
	PlanID          string   `json:"plan_id"`
	PlanRevision    int      `json:"plan_revision"`
	PlanVersion     int      `json:"plan_version"`
	PlanSHA256      string   `json:"plan_sha256"`
	ParentNodeID    string   `json:"parent_node_id"`
	BindingID       string   `json:"binding_id"`
	ChildRunID      string   `json:"child_run_id"`
	ChildNodeID     string   `json:"child_node_id"`
	DependsOn       []string `json:"depends_on"`
}

type ApplicationReleaseRolloutPreviewRequest struct {
	ProjectID               string `json:"project_id"`
	PlanGroupID             string `json:"plan_group_id"`
	ExpectedSetSHA256       string `json:"expected_set_sha256"`
	ExpectedPlanGroupSHA256 string `json:"expected_plan_group_sha256"`
	Reason                  string `json:"reason"`
}

type ApplicationReleaseRolloutPreview struct {
	Rollout       ApplicationReleaseRollout      `json:"rollout"`
	PreviewSHA256 string                         `json:"preview_sha256"`
	CanStart      bool                           `json:"can_start"`
	Blockers      []ApplicationReleaseSetBlocker `json:"blockers"`
}

type ApplicationReleaseRolloutStartRequest struct {
	ApplicationReleaseRolloutPreviewRequest
	ExpectedPreviewSHA256 string `json:"expected_preview_sha256"`
	IdempotencyKey        string `json:"idempotency_key"`
}

type ApplicationReleaseRolloutItem struct {
	ApplicationID         string `json:"application_id"`
	ParentNodeState       string `json:"parent_node_state"`
	ChildRunState         string `json:"child_run_state"`
	DeploymentOperationID string `json:"deployment_operation_id"`
	DeploymentState       string `json:"deployment_state"`
	EffectState           string `json:"effect_state"`
	HealthState           string `json:"health_state"`
}

type ApplicationReleaseRolloutView struct {
	Rollout        ApplicationReleaseRollout       `json:"rollout"`
	PlanGroup      ApplicationReleaseSetPlanGroup  `json:"plan_group"`
	RunVersion     int                             `json:"run_version"`
	State          string                          `json:"state"`
	PauseReason    string                          `json:"pause_reason"`
	BasisCurrent   bool                            `json:"basis_current"`
	Items          []ApplicationReleaseRolloutItem `json:"items"`
	Blockers       []ApplicationReleaseSetBlocker  `json:"blockers"`
	AllowedActions []string                        `json:"allowed_actions"`
}

type ApplicationReleaseRolloutPage struct {
	Items      []ApplicationReleaseRolloutView `json:"items"`
	Total      int                             `json:"total"`
	NextCursor string                          `json:"next_cursor"`
	CanPreview bool                            `json:"can_preview"`
}

type ApplicationReleaseRolloutResumePreviewRequest struct {
	ProjectID             string `json:"project_id"`
	ExpectedRunVersion    int    `json:"expected_run_version"`
	ExpectedRolloutSHA256 string `json:"expected_rollout_sha256"`
}

type ApplicationReleaseRolloutResumePreview struct {
	RolloutID     string                         `json:"rollout_id"`
	RolloutSHA256 string                         `json:"rollout_sha256"`
	RunVersion    int                            `json:"run_version"`
	PreviewSHA256 string                         `json:"preview_sha256"`
	CanResume     bool                           `json:"can_resume"`
	Blockers      []ApplicationReleaseSetBlocker `json:"blockers"`
}

type ApplicationReleaseRolloutCommandRequest struct {
	ProjectID             string `json:"project_id"`
	ExpectedRunVersion    int    `json:"expected_run_version"`
	ExpectedRolloutSHA256 string `json:"expected_rollout_sha256"`
	Action                string `json:"action"`
	ExpectedPreviewSHA256 string `json:"expected_preview_sha256"`
	Reason                string `json:"reason"`
	IdempotencyKey        string `json:"idempotency_key"`
}
