package releasecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const ReleaseProtocol = "suma.release.v1"

// ReleaseProviderCapabilityPrefix identifies a Runner claim's local release
// protocol configuration. It does not establish reachability or permission.
const ReleaseProviderCapabilityPrefix = "release_provider:"

type HealthCheck struct {
	Path           string `json:"path"`
	ExpectedStatus int    `json:"expected_status"`
	ExpectedFile   string `json:"expected_file"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}
type HealthPolicy struct {
	Checks    []HealthCheck `json:"checks"`
	TimeoutMS int           `json:"timeout_ms"`
	Attempts  int           `json:"attempts"`
}
type ReservedExecution struct {
	RunID  string `json:"run_id"`
	NodeID string `json:"node_id"`
}

// ApplicationReleaseSetRef binds the original deployment plan to one immutable
// release-set member. The reference is part of the plan digest and authorization
// grant; it does not grant group approval or permission to deploy other members.
type ApplicationReleaseSetRef struct {
	ReleaseSetID             string `json:"release_set_id"`
	ReleaseSetSHA256         string `json:"release_set_sha256"`
	ApplicationID            string `json:"application_id"`
	ApplicationVersionID     string `json:"application_version_id"`
	ApplicationVersionSHA256 string `json:"application_version_sha256"`
}

var applicationReleaseRefID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

func ValidateApplicationReleaseSetRef(ref *ApplicationReleaseSetRef) error {
	if ref == nil {
		return nil
	}
	if !applicationReleaseRefID.MatchString(ref.ReleaseSetID) || !applicationReleaseRefID.MatchString(ref.ApplicationID) || !applicationReleaseRefID.MatchString(ref.ApplicationVersionID) || !ValidDigest(ref.ReleaseSetSHA256) || !ValidDigest(ref.ApplicationVersionSHA256) {
		return fmt.Errorf("invalid application release-set plan binding")
	}
	return nil
}

type ReleasePlanContent struct {
	ApplicationReleaseSet     *ApplicationReleaseSetRef `json:"application_release_set,omitempty"`
	CandidateID               string                    `json:"candidate_id"`
	Action                    string                    `json:"action"`
	EnvironmentID             string                    `json:"environment_id"`
	ProviderID                string                    `json:"provider_id"`
	SiteID                    string                    `json:"site_id"`
	EnvironmentPolicyVersion  int                       `json:"environment_policy_version"`
	ExpectedGeneration        int64                     `json:"expected_generation"`
	Generation                int64                     `json:"generation"`
	ArtifactID                string                    `json:"artifact_id"`
	ArtifactSHA256            string                    `json:"artifact_sha256"`
	SourceCommit              string                    `json:"source_commit"`
	BuildAttestationID        string                    `json:"build_attestation_id"`
	PackageTestAttestationID  string                    `json:"package_test_attestation_id"`
	AcceptedTaskBasisSHA256   string                    `json:"accepted_task_basis_sha256"`
	ReservedExecution         ReservedExecution         `json:"reserved_execution"`
	Health                    HealthPolicy              `json:"health"`
	RollbackSourceOperationID string                    `json:"rollback_source_operation_id,omitempty"`
}
type ReleaseExecution struct {
	Protocol           string `json:"protocol"`
	OperationID        string `json:"operation_id"`
	PlanID             string `json:"plan_id"`
	PlanRevision       int    `json:"plan_revision"`
	PlanSHA256         string `json:"plan_sha256"`
	EnvironmentID      string `json:"environment_id"`
	ProviderID         string `json:"provider_id"`
	ExpectedGeneration int64  `json:"expected_generation"`
	Generation         int64  `json:"generation"`
	ArtifactID         string `json:"artifact_id"`
	ArtifactSHA256     string `json:"artifact_sha256"`
}
type ReleaseAuthorizationRequest struct {
	ProjectID   string        `json:"project_id"`
	Assignment  AssignmentRef `json:"assignment"`
	OperationID string        `json:"operation_id"`
}
type ReleaseAuthorization struct {
	AuthorizationID     string `json:"authorization_id"`
	AuthorizationTicket string `json:"authorization_ticket"`
	ExpiresAt           string `json:"expires_at"`
	OperationID         string `json:"operation_id"`
	RequestSHA256       string `json:"request_sha256"`
}

// DeploymentRequest's secret is deliberately excluded by DeploymentRequestDigest.
// Providers persist only NonSecretDeploymentRequest before consuming the ticket.
type DeploymentRequest struct {
	ReleaseExecution
	Assignment          AssignmentRef `json:"assignment"`
	AuthorizationTicket string        `json:"authorization_ticket"`
}
type NonSecretDeploymentRequest struct {
	ReleaseExecution
	Assignment AssignmentRef `json:"assignment"`
}
type ConsumeDeploymentAuthorization struct {
	ProviderID          string        `json:"provider_id"`
	AuthorizationTicket string        `json:"authorization_ticket"`
	OperationID         string        `json:"operation_id"`
	Assignment          AssignmentRef `json:"assignment"`
	CallerWorkerID      string        `json:"caller_worker_id"`
	RequestSHA256       string        `json:"request_sha256"`
}
type InspectDeploymentAuthorization struct {
	OperationID   string        `json:"operation_id"`
	Assignment    AssignmentRef `json:"assignment"`
	RequestSHA256 string        `json:"request_sha256"`
}
type DeploymentAuthorizationGrant struct {
	AuthorizationID string             `json:"authorization_id"`
	OperationID     string             `json:"operation_id"`
	ProviderID      string             `json:"provider_id"`
	Assignment      AssignmentRef      `json:"assignment"`
	Plan            ReleasePlanContent `json:"plan"`
	RequestSHA256   string             `json:"request_sha256"`
	State           string             `json:"state"`
}
type BeginDeploymentEffect struct {
	AuthorizationID string        `json:"authorization_id"`
	OperationID     string        `json:"operation_id"`
	Assignment      AssignmentRef `json:"assignment"`
	RequestSHA256   string        `json:"request_sha256"`
	Generation      int64         `json:"generation"`
}
type DeploymentEffectGrant struct {
	EffectGrantID string `json:"effect_grant_id"`
	OperationID   string `json:"operation_id"`
	Generation    int64  `json:"generation"`
	State         string `json:"state"`
}
type DeploymentEffect struct {
	State                    string `json:"state"`
	PreviousActiveGeneration int64  `json:"previous_active_generation"`
	ActiveGeneration         int64  `json:"active_generation"`
	ActiveArtifactSHA256     string `json:"active_artifact_sha256"`
	ObservedAt               string `json:"observed_at"`
}
type HealthObservation struct {
	State          string              `json:"state"`
	Generation     int64               `json:"generation"`
	ArtifactSHA256 string              `json:"artifact_sha256"`
	Checks         []HealthCheckResult `json:"checks"`
}
type HealthCheckResult struct {
	Path       string `json:"path"`
	StatusCode int    `json:"status_code"`
	BodySHA256 string `json:"body_sha256"`
	ObservedAt string `json:"observed_at"`
}
type DeploymentError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type DeploymentObservation struct {
	Protocol        string            `json:"protocol"`
	OperationID     string            `json:"operation_id"`
	RequestSHA256   string            `json:"request_sha256"`
	ProviderID      string            `json:"provider_id"`
	PlanID          string            `json:"plan_id"`
	PlanRevision    int               `json:"plan_revision"`
	PlanSHA256      string            `json:"plan_sha256"`
	EnvironmentID   string            `json:"environment_id"`
	Generation      int64             `json:"generation"`
	FenceGeneration int64             `json:"fence_generation"`
	State           string            `json:"state"`
	Effect          DeploymentEffect  `json:"effect"`
	Health          HealthObservation `json:"health"`
	EvidenceSHA256  string            `json:"evidence_sha256"`
	Error           *DeploymentError  `json:"error"`
	Version         int               `json:"version"`
}

// ReleaseResult contains references, never a client supplied successful conclusion.
type ReleaseResult struct {
	OperationID    string `json:"operation_id"`
	RequestSHA256  string `json:"request_sha256"`
	EvidenceSHA256 string `json:"evidence_sha256"`
	Version        int    `json:"version"`
}

var operationPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{7,127}$`)

func ValidOperationID(id string) bool { return operationPattern.MatchString(id) }
func CanonicalDigest(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var normalized any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err = d.Decode(&normalized); err != nil {
		return "", err
	}
	b, err = json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func DeploymentRequestDigest(r DeploymentRequest) (string, error) {
	if r.Protocol != ReleaseProtocol || !ValidOperationID(r.OperationID) || !r.Assignment.Valid() || r.PlanID == "" || r.PlanRevision < 1 || !ValidDigest(r.PlanSHA256) || r.Generation != r.ExpectedGeneration+1 || r.ExpectedGeneration < 0 || !ValidDigest(r.ArtifactSHA256) || r.ProviderID == "" || r.EnvironmentID == "" || r.ArtifactID == "" {
		return "", fmt.Errorf("invalid deployment request binding")
	}
	return CanonicalDigest(NonSecretDeploymentRequest{ReleaseExecution: r.ReleaseExecution, Assignment: r.Assignment})
}
func ObservationDigest(o DeploymentObservation) (string, error) {
	o.EvidenceSHA256 = ""
	return CanonicalDigest(o)
}
func ValidateHealthPolicy(h HealthPolicy, frozen bool) error {
	if len(h.Checks) < 1 || len(h.Checks) > 8 || h.TimeoutMS < 100 || h.TimeoutMS > 10000 || h.Attempts < 1 || h.Attempts > 5 {
		return fmt.Errorf("invalid health policy limits")
	}
	seen := map[string]bool{}
	for _, c := range h.Checks {
		portablePath := strings.TrimSuffix(strings.TrimPrefix(c.Path, "/"), "/")
		if c.Path != "/" && !ValidPath(portablePath) {
			return fmt.Errorf("health paths must be portable site-relative paths")
		}
		if !strings.HasPrefix(c.Path, "/") || strings.ContainsAny(c.Path, "?#\\%") || strings.Contains(c.Path, "..") || strings.Contains(c.Path, "//") || strings.HasPrefix(c.Path, "/.suma-release") || len(c.Path) > 200 || !ValidPath(c.ExpectedFile) || c.ExpectedStatus != 200 || seen[c.Path] || frozen && !ValidDigest(c.ExpectedSHA256) || !frozen && c.ExpectedSHA256 != "" {
			return fmt.Errorf("invalid fixed health check")
		}
		seen[c.Path] = true
	}
	return nil
}
