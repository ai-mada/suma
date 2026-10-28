package platform

// A release set is a reviewed association of immutable application code
// versions and independently proven release candidates. It does not invent an
// application-scoped build attestation or replace existing deployment authority.
type ApplicationReleaseSetMemberInput struct {
	ApplicationID            string `json:"application_id"`
	ApplicationVersionID     string `json:"application_version_id"`
	ApplicationVersionSHA256 string `json:"application_version_sha256"`
	CandidateID              string `json:"candidate_id"`
}
type ApplicationReleaseSetMember struct {
	ApplicationReleaseSetMemberInput
	ApplicationName         string             `json:"application_name"`
	ApplicationVersionLabel string             `json:"application_version_label"`
	CandidateName           string             `json:"candidate_name"`
	TargetID                string             `json:"target_id"`
	SourceCommit            string             `json:"source_commit"`
	ArtifactID              string             `json:"artifact_id"`
	ArtifactSHA256          string             `json:"artifact_sha256"`
	CandidateSHA256         string             `json:"candidate_sha256"`
	BuildProof              ArtifactProof      `json:"build_proof"`
	PackageTestProof        ArtifactProof      `json:"package_test_proof"`
	SourceTasks             []ReleaseTaskBasis `json:"source_tasks"`
	CandidateTasks          []ReleaseTaskBasis `json:"candidate_tasks"`
}
type ApplicationReleaseSetDependency struct {
	ApplicationID        string `json:"application_id"`
	ApplicationVersionID string `json:"application_version_id"`
	TargetApplicationID  string `json:"target_application_id"`
	TargetVersionID      string `json:"target_version_id"`
	TargetVersionSHA256  string `json:"target_version_sha256"`
	Compatibility        string `json:"compatibility"`
}
type ApplicationReleaseSetBlocker struct {
	Code          string `json:"code"`
	ApplicationID string `json:"application_id"`
	RefID         string `json:"ref_id"`
}
type ApplicationReleaseSet struct {
	ID                                  string                            `json:"id"`
	ProjectID                           string                            `json:"project_id"`
	Label                               string                            `json:"label"`
	Reason                              string                            `json:"reason"`
	Members                             []ApplicationReleaseSetMember     `json:"members"`
	Dependencies                        []ApplicationReleaseSetDependency `json:"dependencies"`
	Cycles                              [][]string                        `json:"cycles"`
	DependencyClosureSHA256             string                            `json:"dependency_closure_sha256"`
	UnverifiedCompatibilityAcknowledged bool                              `json:"unverified_compatibility_acknowledged"`
	SnapshotSHA256                      string                            `json:"snapshot_sha256"`
	RegisteredPreviewSHA256             string                            `json:"registered_preview_sha256"`
	CreatedBy                           string                            `json:"created_by"`
	CreatedAt                           string                            `json:"created_at"`
}
type ApplicationReleaseSetPreviewRequest struct {
	ProjectID                          string                             `json:"project_id"`
	Label                              string                             `json:"label"`
	Members                            []ApplicationReleaseSetMemberInput `json:"members"`
	AcknowledgeUnverifiedCompatibility bool                               `json:"acknowledge_unverified_compatibility"`
	Reason                             string                             `json:"reason"`
}
type ApplicationReleaseSetPreview struct {
	Set           ApplicationReleaseSet          `json:"set"`
	PreviewSHA256 string                         `json:"preview_sha256"`
	CanRegister   bool                           `json:"can_register"`
	Blockers      []ApplicationReleaseSetBlocker `json:"blockers"`
}
type ApplicationReleaseSetRegisterRequest struct {
	ApplicationReleaseSetPreviewRequest
	ExpectedPreviewSHA256 string `json:"expected_preview_sha256"`
	IdempotencyKey        string `json:"idempotency_key"`
}
type ApplicationReleaseSetPlanBinding struct {
	ApplicationID            string `json:"application_id"`
	ApplicationVersionID     string `json:"application_version_id"`
	ApplicationVersionSHA256 string `json:"application_version_sha256"`
	CandidateID              string `json:"candidate_id"`
	ArtifactID               string `json:"artifact_id"`
	ArtifactSHA256           string `json:"artifact_sha256"`
	EnvironmentID            string `json:"environment_id"`
	PlanID                   string `json:"plan_id"`
	PlanSHA256               string `json:"plan_sha256"`
}
type ApplicationReleaseSetPlanGroup struct {
	ID                    string                             `json:"id"`
	ProjectID             string                             `json:"project_id"`
	ReleaseSetID          string                             `json:"release_set_id"`
	ReleaseSetSHA256      string                             `json:"release_set_sha256"`
	Bindings              []ApplicationReleaseSetPlanBinding `json:"bindings"`
	PreparedPreviewSHA256 string                             `json:"prepared_preview_sha256"`
	SnapshotSHA256        string                             `json:"snapshot_sha256"`
	Reason                string                             `json:"reason"`
	CreatedBy             string                             `json:"created_by"`
	CreatedAt             string                             `json:"created_at"`
}
type ApplicationReleaseSetRolloutItem struct {
	Binding               ApplicationReleaseSetPlanBinding `json:"binding"`
	PlanVersion           int                              `json:"plan_version"`
	PlanState             string                           `json:"plan_state"`
	DeploymentOperationID string                           `json:"deployment_operation_id"`
	DeploymentState       string                           `json:"deployment_state"`
	EffectState           string                           `json:"effect_state"`
	HealthState           string                           `json:"health_state"`
}
type ApplicationReleaseSetPlanGroupView struct {
	Group ApplicationReleaseSetPlanGroup     `json:"group"`
	Items []ApplicationReleaseSetRolloutItem `json:"items"`
}
type ApplicationReleaseSetView struct {
	Set                ApplicationReleaseSet                `json:"set"`
	BasisCurrent       bool                                 `json:"basis_current"`
	Blockers           []ApplicationReleaseSetBlocker       `json:"blockers"`
	AllowedActions     []string                             `json:"allowed_actions"`
	PlanGroups         []ApplicationReleaseSetPlanGroupView `json:"plan_groups"`
	PlanGroupTotal     int                                  `json:"plan_group_total"`
	PlanGroupsComplete bool                                 `json:"plan_groups_complete"`
}
type ApplicationReleaseSetSummary struct {
	ID             string `json:"id"`
	ProjectID      string `json:"project_id"`
	Label          string `json:"label"`
	SnapshotSHA256 string `json:"snapshot_sha256"`
	MemberCount    int    `json:"member_count"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
}
type ApplicationReleaseSetPage struct {
	Items      []ApplicationReleaseSetSummary `json:"items"`
	Total      int                            `json:"total"`
	NextCursor string                         `json:"next_cursor"`
	CanCreate  bool                           `json:"can_create"`
}
type ApplicationReleaseSetCandidateOption struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	SourceCommit        string `json:"source_commit"`
	ArtifactID          string `json:"artifact_id"`
	ArtifactSHA256      string `json:"artifact_sha256"`
	BuildWorkerID       string `json:"build_worker_id"`
	PackageTestWorkerID string `json:"package_test_worker_id"`
}
type ApplicationReleaseSetOptions struct {
	ProjectID        string                                   `json:"project_id"`
	Applications     []ApplicationReleaseSetApplicationOption `json:"applications"`
	Versions         []ApplicationVersionOption               `json:"versions"`
	Candidates       []ApplicationReleaseSetCandidateOption   `json:"candidates"`
	Environments     []ReleaseEnvironment                     `json:"environments"`
	ApplicationTotal int                                      `json:"application_total"`
	VersionTotal     int                                      `json:"version_total"`
	CandidateTotal   int                                      `json:"candidate_total"`
	EnvironmentTotal int                                      `json:"environment_total"`
	Complete         bool                                     `json:"complete"`
	Limit            int                                      `json:"limit"`
}
type ApplicationReleaseSetApplicationOption struct {
	ID                         string `json:"id"`
	Name                       string `json:"name"`
	State                      string `json:"state"`
	CurrentRegisteredVersionID string `json:"current_registered_version_id"`
}
type ApplicationReleaseSetEnvironmentInput struct {
	ApplicationID                 string `json:"application_id"`
	EnvironmentID                 string `json:"environment_id"`
	ExpectedEnvironmentGeneration int64  `json:"expected_environment_generation"`
}
type ApplicationReleaseSetDeploymentPreviewRequest struct {
	ProjectID         string                                  `json:"project_id"`
	ExpectedSetSHA256 string                                  `json:"expected_set_sha256"`
	Environments      []ApplicationReleaseSetEnvironmentInput `json:"environments"`
	Reason            string                                  `json:"reason"`
}
type ApplicationReleaseSetDeploymentPreviewItem struct {
	ApplicationID string             `json:"application_id"`
	CandidateID   string             `json:"candidate_id"`
	Preview       ReleasePlanPreview `json:"preview"`
}
type ApplicationReleaseSetDeploymentPreview struct {
	ReleaseSetID     string                                       `json:"release_set_id"`
	ReleaseSetSHA256 string                                       `json:"release_set_sha256"`
	Items            []ApplicationReleaseSetDeploymentPreviewItem `json:"items"`
	PreviewSHA256    string                                       `json:"preview_sha256"`
	CanPrepare       bool                                         `json:"can_prepare"`
	Blockers         []ApplicationReleaseSetBlocker               `json:"blockers"`
}
type ApplicationReleaseSetDeploymentPrepareRequest struct {
	ApplicationReleaseSetDeploymentPreviewRequest
	ExpectedPreviewSHA256 string `json:"expected_preview_sha256"`
	IdempotencyKey        string `json:"idempotency_key"`
}
