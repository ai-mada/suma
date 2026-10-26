// Package repositoryintegrationcontract binds governed Git effects independently
// of deployment, tool execution and human acceptance.
package repositoryintegrationcontract

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	"strings"
)

const RepositoryProtocol = "suma.repository-integration.v1"
const RepositoryMaxDiffBytes = 512 << 10
const RepositoryMaxFiles = 1000

type RepositoryExecution struct {
	Protocol              string              `json:"protocol"`
	TenantID              string              `json:"tenant_id"`
	ProjectID             string              `json:"project_id"`
	ProviderID            string              `json:"provider_id"`
	TargetID              string              `json:"target_id"`
	TargetAlias           string              `json:"target_alias"`
	TargetRef             string              `json:"target_ref"`
	TargetConfigSHA256    string              `json:"target_config_sha256"`
	PlanID                string              `json:"plan_id"`
	PlanVersion           int                 `json:"plan_version"`
	Phase                 string              `json:"phase"`
	OperationID           string              `json:"operation_id"`
	Source                cc.CandidateBinding `json:"source"`
	ExpectedTargetHead    string              `json:"expected_target_head"`
	ExpectedFenceOID      string              `json:"expected_fence_oid"`
	ExpectedGitGeneration int64               `json:"expected_git_generation"`
	Generation            int64               `json:"generation"`
	PreparedSHA256        string              `json:"prepared_sha256"`
}
type RepositoryRequest struct {
	RepositoryExecution
	Assignment          cc.CandidateAssignmentRef `json:"assignment"`
	AuthorizationTicket string                    `json:"authorization_ticket"`
}
type RepositoryNonSecretRequest struct {
	RepositoryExecution
	Assignment cc.CandidateAssignmentRef `json:"assignment"`
}
type RepositoryTargetObservation struct {
	Alias        string `json:"alias"`
	Ref          string `json:"ref"`
	ConfigSHA256 string `json:"config_sha256"`
	HeadCommit   string `json:"head_commit"`
	ObjectFormat string `json:"object_format"`
	FenceOID     string `json:"fence_oid"`
	Generation   int64  `json:"generation"`
	Available    bool   `json:"available"`
	Reason       string `json:"reason"`
	ObservedAt   string `json:"observed_at"`
}
type RepositoryFileChange struct {
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
	Status  string `json:"status"`
	OldOID  string `json:"old_oid"`
	NewOID  string `json:"new_oid"`
	Binary  bool   `json:"binary"`
}
type RepositoryPreview struct {
	TargetHead         string                 `json:"target_head"`
	CandidateHead      string                 `json:"candidate_head"`
	Files              []RepositoryFileChange `json:"files"`
	Diff               string                 `json:"diff"`
	DiffSHA256         string                 `json:"diff_sha256"`
	SourceBundleSHA256 string                 `json:"source_bundle_sha256"`
	ConfigSHA256       string                 `json:"config_sha256"`
	PreparedSHA256     string                 `json:"prepared_sha256"`
	FastForward        bool                   `json:"fast_forward"`
	BlockedReason      string                 `json:"blocked_reason"`
}
type RepositoryObservation struct {
	Protocol          string              `json:"protocol"`
	OperationID       string              `json:"operation_id"`
	RequestSHA256     string              `json:"request_sha256"`
	Phase             string              `json:"phase"`
	State             string              `json:"state"`
	Version           int                 `json:"version"`
	Execution         RepositoryExecution `json:"execution"`
	Preview           *RepositoryPreview  `json:"preview,omitempty"`
	ReceiptOID        string              `json:"receipt_oid"`
	ReceiptSHA256     string              `json:"receipt_sha256"`
	Receipt           *RepositoryReceipt  `json:"receipt,omitempty"`
	EffectGrantID     string              `json:"effect_grant_id"`
	Reason            string              `json:"reason"`
	ObservedAt        string              `json:"observed_at"`
	ObservationSHA256 string              `json:"observation_sha256"`
}
type RepositoryResult struct {
	OperationID       string `json:"operation_id"`
	RequestSHA256     string `json:"request_sha256"`
	ObservationSHA256 string `json:"observation_sha256"`
	Version           int    `json:"version"`
}
type RepositoryAuthorizationRequest struct {
	ProjectID   string                    `json:"project_id"`
	Assignment  cc.CandidateAssignmentRef `json:"assignment"`
	OperationID string                    `json:"operation_id"`
}
type RepositoryAuthorization struct {
	AuthorizationID     string `json:"authorization_id"`
	AuthorizationTicket string `json:"authorization_ticket"`
	ExpiresAt           string `json:"expires_at"`
	OperationID         string `json:"operation_id"`
	RequestSHA256       string `json:"request_sha256"`
}
type RepositoryConsumeAuthorization struct {
	ProviderID          string                    `json:"provider_id"`
	AuthorizationTicket string                    `json:"authorization_ticket"`
	OperationID         string                    `json:"operation_id"`
	Assignment          cc.CandidateAssignmentRef `json:"assignment"`
	RequestSHA256       string                    `json:"request_sha256"`
}
type RepositoryInspectAuthorization struct {
	OperationID   string                    `json:"operation_id"`
	Assignment    cc.CandidateAssignmentRef `json:"assignment"`
	RequestSHA256 string                    `json:"request_sha256"`
}
type RepositoryAuthorizationGrant struct {
	AuthorizationID string                    `json:"authorization_id"`
	OperationID     string                    `json:"operation_id"`
	ProviderID      string                    `json:"provider_id"`
	Assignment      cc.CandidateAssignmentRef `json:"assignment"`
	Execution       RepositoryExecution       `json:"execution"`
	RequestSHA256   string                    `json:"request_sha256"`
	State           string                    `json:"state"`
}
type RepositoryBeginEffect struct {
	AuthorizationID string                    `json:"authorization_id"`
	OperationID     string                    `json:"operation_id"`
	Assignment      cc.CandidateAssignmentRef `json:"assignment"`
	RequestSHA256   string                    `json:"request_sha256"`
	Generation      int64                     `json:"generation"`
}
type RepositoryEffectGrant struct {
	EffectGrantID string `json:"effect_grant_id"`
	OperationID   string `json:"operation_id"`
	Generation    int64  `json:"generation"`
	State         string `json:"state"`
}
type RepositoryBundleRequest struct {
	AuthorizationID string                    `json:"authorization_id"`
	OperationID     string                    `json:"operation_id"`
	Assignment      cc.CandidateAssignmentRef `json:"assignment"`
	RequestSHA256   string                    `json:"request_sha256"`
}
type RepositoryReconcileRequest struct {
	OperationID          string                      `json:"operation_id"`
	RequestSHA256        string                      `json:"request_sha256"`
	ReconciliationTicket string                      `json:"reconciliation_ticket"`
	Request              *RepositoryNonSecretRequest `json:"request,omitempty"`
}
type RepositoryConsumeReconciliation struct {
	ProviderID           string                    `json:"provider_id"`
	OperationID          string                    `json:"operation_id"`
	RequestSHA256        string                    `json:"request_sha256"`
	ReconciliationTicket string                    `json:"reconciliation_ticket"`
	Execution            RepositoryExecution       `json:"execution"`
	Assignment           cc.CandidateAssignmentRef `json:"assignment"`
}
type RepositoryReconciliationGrant struct {
	ReconciliationID string                    `json:"reconciliation_id"`
	OperationID      string                    `json:"operation_id"`
	RequestSHA256    string                    `json:"request_sha256"`
	Generation       int64                     `json:"generation"`
	State            string                    `json:"state"`
	Execution        RepositoryExecution       `json:"execution"`
	Assignment       cc.CandidateAssignmentRef `json:"assignment"`
}

// RepositoryReceipt is persisted as a Git blob. The receipt reference and target
// fence are created atomically with the target CAS, or with a negative fence.
type RepositoryReceipt struct {
	Protocol           string `json:"protocol"`
	OperationID        string `json:"operation_id"`
	RequestSHA256      string `json:"request_sha256"`
	TargetID           string `json:"target_id"`
	TargetAlias        string `json:"target_alias"`
	TargetRef          string `json:"target_ref"`
	TargetConfigSHA256 string `json:"target_config_sha256"`
	BeforeCommit       string `json:"before_commit"`
	ResultCommit       string `json:"result_commit"`
	SourceBundleSHA256 string `json:"source_bundle_sha256"`
	PreparedSHA256     string `json:"prepared_sha256"`
	Generation         int64  `json:"generation"`
	EffectGrantID      string `json:"effect_grant_id"`
	State              string `json:"state"`
}

func RepositoryCanonicalDigest(v any) (string, error) { return rc.CanonicalDigest(v) }
func RepositoryValidOperationID(id string) bool       { return rc.ValidOperationID(id) }
func RepositoryValidRef(ref string) bool {
	if !strings.HasPrefix(ref, "refs/heads/") || len(ref) > 200 || strings.HasSuffix(ref, "/") || strings.HasSuffix(ref, ".") || strings.Contains(ref, "..") || strings.Contains(ref, "@{") || strings.ContainsAny(ref, " ~^:?*[\\\x00\r\n\t") {
		return false
	}
	for _, part := range strings.Split(ref, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}
func RepositoryValidateExecution(e RepositoryExecution) error {
	for _, id := range []string{e.TenantID, e.ProjectID, e.ProviderID, e.TargetID, e.TargetAlias, e.PlanID} {
		if id == "" || len(id) > 128 || strings.ContainsAny(id, "/\\\x00\r\n") {
			return fmt.Errorf("invalid repository audience identity")
		}
	}
	bound, _ := json.Marshal(e.Source)
	if len(bound) > 16<<10 {
		return fmt.Errorf("repository source binding exceeds limit")
	}
	if e.Protocol != RepositoryProtocol || !RepositoryValidOperationID(e.OperationID) || e.TenantID == "" || e.ProjectID == "" || e.ProviderID == "" || e.TargetID == "" || e.TargetAlias == "" || e.PlanID == "" || e.PlanVersion < 1 || !RepositoryValidRef(e.TargetRef) || !cc.ValidHash(e.TargetConfigSHA256) || !e.Source.ValidSealed() || e.Source.ProjectID != e.ProjectID || !cc.ValidCommit(e.ExpectedTargetHead) || e.ExpectedGitGeneration < 0 || e.Generation != e.ExpectedGitGeneration+1 || (e.ExpectedFenceOID != "" && !cc.ValidCommit(e.ExpectedFenceOID)) || (e.Phase != "prepare" && e.Phase != "apply") || (e.Phase == "apply" && !cc.ValidHash(e.PreparedSHA256)) || (e.Phase == "prepare" && e.PreparedSHA256 != "") {
		return fmt.Errorf("invalid repository integration execution binding")
	}
	return nil
}
func RepositoryRequestDigest(r RepositoryRequest) (string, error) {
	if err := RepositoryValidateExecution(r.RepositoryExecution); err != nil {
		return "", err
	}
	if !r.Assignment.Valid() {
		return "", fmt.Errorf("invalid repository assignment")
	}
	r.Assignment.LeaseUntil = ""
	r.Assignment.DispatchState = ""
	return RepositoryCanonicalDigest(RepositoryNonSecretRequest{r.RepositoryExecution, r.Assignment})
}
func RepositoryPreviewDigest(p RepositoryPreview) (string, error) {
	p.PreparedSHA256 = ""
	return RepositoryCanonicalDigest(p)
}
func RepositoryObservationDigest(o RepositoryObservation) (string, error) {
	o.ObservationSHA256 = ""
	return RepositoryCanonicalDigest(o)
}
func RepositoryExecutionEqual(a, b RepositoryExecution) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func RepositoryStableAssignment(a cc.CandidateAssignmentRef) cc.CandidateAssignmentRef {
	a.LeaseUntil = ""
	a.DispatchState = ""
	return a
}

func RepositoryReceiptDigest(r RepositoryReceipt) (string, error) {
	return RepositoryCanonicalDigest(r)
}
func RepositoryReceiptOID(r RepositoryReceipt, format string) (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	data := append([]byte(fmt.Sprintf("blob %d\x00", len(b))), b...)
	if format == "sha1" {
		h := sha1.Sum(data)
		return hex.EncodeToString(h[:]), nil
	}
	if format == "sha256" {
		h := sha256.Sum256(data)
		return hex.EncodeToString(h[:]), nil
	}
	return "", fmt.Errorf("unsupported Git object format")
}
func RepositoryValidateReceipt(r RepositoryReceipt, e RepositoryExecution, requestSHA string) error {
	if r.Protocol != RepositoryProtocol || r.OperationID != e.OperationID || r.RequestSHA256 != requestSHA || r.TargetID != e.TargetID || r.TargetAlias != e.TargetAlias || r.TargetRef != e.TargetRef || r.TargetConfigSHA256 != e.TargetConfigSHA256 || r.BeforeCommit != e.ExpectedTargetHead || r.SourceBundleSHA256 != e.Source.BundleSHA256 || r.PreparedSHA256 != e.PreparedSHA256 || r.Generation != e.Generation || r.EffectGrantID == "" || (r.State != "applied" && r.State != "not_applied") || (r.State == "applied" && r.ResultCommit != e.Source.HeadCommit) || (r.State == "not_applied" && r.ResultCommit != "") {
		return fmt.Errorf("repository receipt binding mismatch")
	}
	return nil
}
func RepositoryValidateObservation(o RepositoryObservation, e RepositoryExecution, requestSHA string) error {
	hash, err := RepositoryObservationDigest(o)
	if err != nil || hash != o.ObservationSHA256 || o.Protocol != RepositoryProtocol || o.OperationID != e.OperationID || o.RequestSHA256 != requestSHA || o.Phase != e.Phase || o.Version < 1 || !RepositoryExecutionEqual(o.Execution, e) {
		return fmt.Errorf("repository observation binding mismatch")
	}
	switch o.State {
	case "preparing", "prepared", "applying", "applied", "not_applied", "blocked", "unknown":
	default:
		return fmt.Errorf("invalid repository observation state")
	}
	if o.State == "prepared" {
		if o.Preview == nil {
			return fmt.Errorf("repository preview absent")
		}
		p := *o.Preview
		diffHash := sha256.Sum256([]byte(p.Diff))
		if hex.EncodeToString(diffHash[:]) != p.DiffSHA256 {
			return fmt.Errorf("repository preview diff digest mismatch")
		}
		for _, file := range p.Files {
			if file.Binary {
				return fmt.Errorf("binary repository review unsupported")
			}
		}
		h, _ := RepositoryPreviewDigest(p)
		if h != p.PreparedSHA256 || !p.FastForward || p.BlockedReason != "" || len(p.Files) > RepositoryMaxFiles || len(p.Diff) > RepositoryMaxDiffBytes || p.TargetHead != e.ExpectedTargetHead || p.CandidateHead != e.Source.HeadCommit || p.SourceBundleSHA256 != e.Source.BundleSHA256 || p.ConfigSHA256 != e.TargetConfigSHA256 {
			return fmt.Errorf("repository preview binding mismatch")
		}
	}
	if o.State == "applied" || o.State == "not_applied" {
		if e.Phase != "apply" {
			return fmt.Errorf("repository receipt requires apply phase")
		}
		if o.Receipt == nil {
			return fmt.Errorf("repository receipt absent")
		}
		if err := RepositoryValidateReceipt(*o.Receipt, e, requestSHA); err != nil {
			return err
		}
		h, _ := RepositoryReceiptDigest(*o.Receipt)
		format := "sha1"
		if len(o.ReceiptOID) == 64 {
			format = "sha256"
		}
		oid, _ := RepositoryReceiptOID(*o.Receipt, format)
		if o.Receipt.State != o.State || h != o.ReceiptSHA256 || oid != o.ReceiptOID || o.EffectGrantID != o.Receipt.EffectGrantID {
			return fmt.Errorf("repository receipt digest mismatch")
		}
	}
	return nil
}
