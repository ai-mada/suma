package platform

// Application code scopes reference a governed Git target. Target.Version is
// intentionally absent: ordinary integrations advance it without changing the
// repository identity. Commands use a fresh expected target version for CAS.
type ApplicationCodeScope struct {
	ID           string `json:"id"`
	TargetID     string `json:"target_id"`
	ProviderID   string `json:"provider_id"`
	TargetAlias  string `json:"target_alias"`
	TargetRef    string `json:"target_ref"`
	ConfigSHA256 string `json:"config_sha256"`
	RelativeRoot string `json:"relative_root"`
	Shared       bool   `json:"shared"`
	Reason       string `json:"reason"`
}
type ApplicationCodeScopeInput struct {
	ID                    string `json:"id"`
	TargetID              string `json:"target_id"`
	ExpectedTargetVersion int    `json:"expected_target_version"`
	RelativeRoot          string `json:"relative_root"`
	Shared                bool   `json:"shared"`
	Reason                string `json:"reason"`
}
type ProjectApplication struct {
	ID                         string                 `json:"id"`
	ProjectID                  string                 `json:"project_id"`
	Version                    int                    `json:"version"`
	Key                        string                 `json:"key"`
	Name                       string                 `json:"name"`
	Description                string                 `json:"description"`
	Kind                       string                 `json:"kind"`
	ParentApplicationID        string                 `json:"parent_application_id"`
	OwnerTeamID                string                 `json:"owner_team_id"`
	State                      string                 `json:"state"`
	CodeScopes                 []ApplicationCodeScope `json:"code_scopes"`
	CurrentRegisteredVersionID string                 `json:"current_registered_version_id"`
	CreatedBy                  string                 `json:"created_by"`
	CreatedAt                  string                 `json:"created_at"`
	UpdatedAt                  string                 `json:"updated_at"`
}
type ApplicationWriteRequest struct {
	ProjectID           string                      `json:"project_id"`
	ExpectedVersion     int                         `json:"expected_version"`
	Key                 string                      `json:"key"`
	Name                string                      `json:"name"`
	Description         string                      `json:"description"`
	Kind                string                      `json:"kind"`
	ParentApplicationID string                      `json:"parent_application_id"`
	OwnerTeamID         string                      `json:"owner_team_id"`
	State               string                      `json:"state"`
	CodeScopes          []ApplicationCodeScopeInput `json:"code_scopes"`
	Reason              string                      `json:"reason"`
	IdempotencyKey      string                      `json:"idempotency_key"`
}
type ProjectApplicationView struct {
	Application            ProjectApplication `json:"application"`
	RegisteredVersionCount int                `json:"registered_version_count"`
	AllowedActions         []string           `json:"allowed_actions"`
	Blockers               []string           `json:"blockers"`
}
type ProjectApplicationPage struct {
	Items      []ProjectApplication `json:"items"`
	Total      int                  `json:"total"`
	NextCursor string               `json:"next_cursor"`
	CanCreate  bool                 `json:"can_create"`
}
type ApplicationTeamOption struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Name    string `json:"name"`
}
type ApplicationOptions struct {
	ProjectID        string                        `json:"project_id"`
	Teams            []ApplicationTeamOption       `json:"teams"`
	Targets          []RepositoryIntegrationTarget `json:"targets"`
	Applications     []ProjectApplication          `json:"applications"`
	TeamTotal        int                           `json:"team_total"`
	TeamTotalExact   bool                          `json:"team_total_exact"`
	TargetTotal      int                           `json:"target_total"`
	ApplicationTotal int                           `json:"application_total"`
	Complete         bool                          `json:"complete"`
	Limit            int                           `json:"limit"`
}

// Provider inspection and parsers produce these immutable bounded facts. Raw
// manifest bytes/base64, scripts, URLs and credentials never enter this DTO.
type ApplicationManifestDependency struct {
	Name                string `json:"name"`
	DeclaredRange       string `json:"declared_range"`
	Kind                string `json:"kind"`
	RangeState          string `json:"range_state"`
	Compatibility       string `json:"compatibility"`
	RangeRedacted       bool   `json:"range_redacted"`
	DeclaredRangeSHA256 string `json:"declared_range_sha256"`
}
type ApplicationManifestObservation struct {
	Path            string                          `json:"path"`
	BlobOID         string                          `json:"blob_oid"`
	SHA256          string                          `json:"sha256"`
	Parser          string                          `json:"parser"`
	ParserVersion   string                          `json:"parser_version"`
	Ecosystem       string                          `json:"ecosystem"`
	PackageName     string                          `json:"package_name"`
	DeclaredVersion string                          `json:"declared_version"`
	Dependencies    []ApplicationManifestDependency `json:"dependencies"`
	Complete        bool                            `json:"complete"`
	Reason          string                          `json:"reason"`
}
type ApplicationSourceCoverage struct {
	Complete      bool   `json:"complete"`
	ManifestTotal int    `json:"manifest_total"`
	ManifestCount int    `json:"manifest_count"`
	MaxManifests  int    `json:"max_manifests"`
	Reason        string `json:"reason"`
}
type ApplicationSourceInspection struct {
	SnapshotSHA256 string                           `json:"snapshot_sha256"`
	Commit         string                           `json:"commit"`
	TreeOID        string                           `json:"tree_oid"`
	RelativeRoot   string                           `json:"relative_root"`
	ScopeTreeOID   string                           `json:"scope_tree_oid"`
	ParserSHA256   string                           `json:"parser_sha256"`
	Manifests      []ApplicationManifestObservation `json:"manifests"`
	Coverage       ApplicationSourceCoverage        `json:"coverage"`
}
type ApplicationVersionSource struct {
	Scope                  ApplicationCodeScope        `json:"scope"`
	IntegrationPlanID      string                      `json:"integration_plan_id"`
	IntegrationPlanVersion int                         `json:"integration_plan_version"`
	OperationID            string                      `json:"operation_id"`
	ReceiptSHA256          string                      `json:"receipt_sha256"`
	ReceiptOID             string                      `json:"receipt_oid"`
	Commit                 string                      `json:"commit"`
	TaskBasis              ReleaseTaskBasis            `json:"task_basis"`
	SourceNodeID           string                      `json:"source_node_id"`
	Inspection             ApplicationSourceInspection `json:"inspection"`
}
type ApplicationVersionSourceInput struct {
	ScopeID                    string `json:"scope_id"`
	IntegrationPlanID          string `json:"integration_plan_id"`
	ExpectedIntegrationVersion int    `json:"expected_integration_version"`
}
type ApplicationDependencyObservation struct {
	ID                      string   `json:"id"`
	ScopeID                 string   `json:"scope_id"`
	ManifestPath            string   `json:"manifest_path"`
	ManifestSHA256          string   `json:"manifest_sha256"`
	Parser                  string   `json:"parser"`
	ParserVersion           string   `json:"parser_version"`
	Ecosystem               string   `json:"ecosystem"`
	Name                    string   `json:"name"`
	DeclaredRange           string   `json:"declared_range"`
	Kind                    string   `json:"kind"`
	MatchState              string   `json:"match_state"`
	RangeState              string   `json:"range_state"`
	Compatibility           string   `json:"compatibility"`
	RangeRedacted           bool     `json:"range_redacted"`
	DeclaredRangeSHA256     string   `json:"declared_range_sha256"`
	CandidateApplicationIDs []string `json:"candidate_application_ids"`
}
type ApplicationDependencyInput struct {
	TargetApplicationID string `json:"target_application_id"`
	TargetVersionID     string `json:"target_version_id"`
	TargetVersionSHA256 string `json:"target_version_sha256"`
	ObservationID       string `json:"observation_id"`
	DeclaredRange       string `json:"declared_range"`
	Reason              string `json:"reason"`
}
type ApplicationDependency struct {
	ApplicationDependencyInput
	ObservedDeclaredRange string `json:"observed_declared_range"`
	Compatibility         string `json:"compatibility"`
}
type ApplicationCodeVersion struct {
	ID                         string                             `json:"id"`
	ProjectID                  string                             `json:"project_id"`
	ApplicationID              string                             `json:"application_id"`
	Sequence                   int                                `json:"sequence"`
	Label                      string                             `json:"label"`
	Application                ProjectApplication                 `json:"application"`
	Sources                    []ApplicationVersionSource         `json:"sources"`
	Observations               []ApplicationDependencyObservation `json:"observations"`
	Dependencies               []ApplicationDependency            `json:"dependencies"`
	DependencyCoverageComplete bool                               `json:"dependency_coverage_complete"`
	DependencyGapsAcknowledged bool                               `json:"dependency_gaps_acknowledged"`
	SnapshotSHA256             string                             `json:"snapshot_sha256"`
	RegisteredPreviewSHA256    string                             `json:"registered_preview_sha256"`
	Reason                     string                             `json:"reason"`
	RegisteredBy               string                             `json:"registered_by"`
	RegisteredAt               string                             `json:"registered_at"`
}
type ApplicationVersionPreviewRequest struct {
	ProjectID                  string                          `json:"project_id"`
	ExpectedApplicationVersion int                             `json:"expected_application_version"`
	Label                      string                          `json:"label"`
	Sources                    []ApplicationVersionSourceInput `json:"sources"`
	Dependencies               []ApplicationDependencyInput    `json:"dependencies"`
	AcknowledgeDependencyGaps  bool                            `json:"acknowledge_dependency_gaps"`
	Reason                     string                          `json:"reason"`
}
type ApplicationVersionPreview struct {
	Version       ApplicationCodeVersion `json:"version"`
	PreviewSHA256 string                 `json:"preview_sha256"`
	CanRegister   bool                   `json:"can_register"`
	Blockers      []string               `json:"blockers"`
}
type ApplicationVersionRegisterRequest struct {
	ApplicationVersionPreviewRequest
	ExpectedPreviewSHA256 string `json:"expected_preview_sha256"`
	IdempotencyKey        string `json:"idempotency_key"`
}
type ApplicationVersionView struct {
	Version      ApplicationCodeVersion `json:"version"`
	BasisCurrent bool                   `json:"basis_current"`
	Blockers     []string               `json:"blockers"`
}
type ApplicationVersionPage struct {
	Items      []ApplicationCodeVersion `json:"items"`
	Total      int                      `json:"total"`
	NextCursor string                   `json:"next_cursor"`
}
type ApplicationIntegrationOption struct {
	ID       string `json:"id"`
	Version  int    `json:"version"`
	TaskID   string `json:"task_id"`
	TargetID string `json:"target_id"`
	Commit   string `json:"commit"`
	State    string `json:"state"`
}
type ApplicationVersionOptions struct {
	ProjectID             string                         `json:"project_id"`
	ApplicationID         string                         `json:"application_id"`
	Integrations          []ApplicationIntegrationOption `json:"integrations"`
	Applications          []ProjectApplication           `json:"applications"`
	Versions              []ApplicationVersionOption     `json:"versions"`
	IntegrationTotal      int                            `json:"integration_total"`
	IntegrationTotalExact bool                           `json:"integration_total_exact"`
	VersionTotal          int                            `json:"version_total"`
	Complete              bool                           `json:"complete"`
	Limit                 int                            `json:"limit"`
}
type ApplicationVersionOption struct {
	ID             string `json:"id"`
	ApplicationID  string `json:"application_id"`
	Sequence       int    `json:"sequence"`
	Label          string `json:"label"`
	SnapshotSHA256 string `json:"snapshot_sha256"`
}
type ApplicationGraphNode struct {
	ApplicationID              string `json:"application_id"`
	Name                       string `json:"name"`
	Kind                       string `json:"kind"`
	State                      string `json:"state"`
	ParentApplicationID        string `json:"parent_application_id"`
	OwnerTeamID                string `json:"owner_team_id"`
	VersionID                  string `json:"version_id"`
	VersionLabel               string `json:"version_label"`
	DependencyCoverageComplete bool   `json:"dependency_coverage_complete"`
}
type ApplicationGraphEdge struct {
	SourceApplicationID string `json:"source_application_id"`
	SourceVersionID     string `json:"source_version_id"`
	TargetApplicationID string `json:"target_application_id"`
	TargetVersionID     string `json:"target_version_id"`
	TargetVersionSHA256 string `json:"target_version_sha256"`
	ObservationID       string `json:"observation_id"`
	State               string `json:"state"`
}
type ApplicationDependencyGraph struct {
	ProjectID        string                 `json:"project_id"`
	SHA256           string                 `json:"sha256"`
	Nodes            []ApplicationGraphNode `json:"nodes"`
	Edges            []ApplicationGraphEdge `json:"edges"`
	Cycles           [][]string             `json:"cycles"`
	ApplicationTotal int                    `json:"application_total"`
	EdgeTotal        int                    `json:"edge_total"`
	EdgeTotalExact   bool                   `json:"edge_total_exact"`
	Complete         bool                   `json:"complete"`
	MaxApplications  int                    `json:"max_applications"`
	MaxEdges         int                    `json:"max_edges"`
}
type ApplicationImpactRequest struct {
	ProjectID           string `json:"project_id"`
	ApplicationID       string `json:"application_id"`
	ExpectedGraphSHA256 string `json:"expected_graph_sha256"`
}
type ApplicationImpactItem struct {
	ApplicationID string   `json:"application_id"`
	VersionID     string   `json:"version_id"`
	OwnerTeamID   string   `json:"owner_team_id"`
	Depth         int      `json:"depth"`
	Path          []string `json:"path"`
}
type ApplicationImpact struct {
	Graph         ApplicationDependencyGraph `json:"graph"`
	ApplicationID string                     `json:"application_id"`
	Items         []ApplicationImpactItem    `json:"items"`
	Complete      bool                       `json:"complete"`
	MaxDepth      int                        `json:"max_depth"`
}
