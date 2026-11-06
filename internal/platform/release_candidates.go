package platform

import "github.com/ai-mada/suma/internal/releasecontract"

type ArtifactProof struct {
	ID               string                                 `json:"id"`
	ProjectID        string                                 `json:"project_id"`
	Assignment       releasecontract.AssignmentRef          `json:"assignment"`
	Kind             string                                 `json:"kind"`
	ArtifactID       string                                 `json:"artifact_id"`
	ManifestSHA256   string                                 `json:"manifest_sha256"`
	SourceCommit     string                                 `json:"source_commit"`
	ProfileID        string                                 `json:"profile_id"`
	ProfileSHA256    string                                 `json:"profile_sha256"`
	WorkspaceSHA256  string                                 `json:"workspace_sha256"`
	CreatedAt        string                                 `json:"created_at"`
	ApplicationBuild *releasecontract.ApplicationBuildBasis `json:"application_build,omitempty"`
}
type ReleaseTaskBasis struct {
	ID       string `json:"id"`
	Version  int    `json:"version"`
	RunID    string `json:"run_id"`
	ReviewID string `json:"review_id"`
	SHA256   string `json:"sha256"`
}
type ReleaseCandidate struct {
	ID                       string                     `json:"id"`
	ProjectID                string                     `json:"project_id"`
	Name                     string                     `json:"name"`
	ArtifactID               string                     `json:"artifact_id"`
	ArtifactSHA256           string                     `json:"artifact_sha256"`
	SourceCommit             string                     `json:"source_commit"`
	BuildAttestationID       string                     `json:"build_attestation_id"`
	PackageTestAttestationID string                     `json:"package_test_attestation_id"`
	AcceptedTaskBasisSHA256  string                     `json:"accepted_task_basis_sha256"`
	BuildProof               ArtifactProof              `json:"build_proof"`
	PackageTestProof         ArtifactProof              `json:"package_test_proof"`
	Tasks                    []ReleaseTaskBasis         `json:"tasks"`
	CreatedAt                string                     `json:"created_at"`
	RepairHandoff            *IssueRepairReleaseHandoff `json:"repair_handoff,omitempty"`
}

type ReleaseArtifact struct {
	ID             string                           `json:"id"`
	ProjectID      string                           `json:"project_id"`
	ManifestSHA256 string                           `json:"manifest_sha256"`
	Manifest       releasecontract.ArtifactManifest `json:"manifest"`
	SizeBytes      int64                            `json:"size_bytes"`
	State          string                           `json:"state"`
	CreatedAt      string                           `json:"created_at"`
	Proofs         []ArtifactProof                  `json:"proofs"`
}
