// Package candidatecontract defines the versioned transfer of one scheduler-
// pinned Git candidate. A bundle is code input, never a test or review result.
package candidatecontract

import (
	"fmt"
	"regexp"
	"strings"
)

const SchemaVersion = 1
const MaxBundleBytes int64 = 64 << 20

var hash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var commit = regexp.MustCompile(`^[a-f0-9]{40}([a-f0-9]{24})?$`)

// AssignmentRef carries only the immutable callback identity. Pool callbacks
// retain the entire dispatch scope; no human role is derived from a machine.
type CandidateAssignmentRef struct {
	RunID                 string `json:"run_id"`
	NodeID                string `json:"node_id"`
	Attempt               int    `json:"attempt"`
	Fence                 int64  `json:"fence"`
	WorkerID              string `json:"worker_id"`
	TenantID              string `json:"tenant_id,omitempty"`
	ProjectID             string `json:"project_id,omitempty"`
	PoolID                string `json:"pool_id,omitempty"`
	PoolRunnerID          string `json:"pool_runner_id,omitempty"`
	PoolSlotID            string `json:"pool_slot_id,omitempty"`
	PoolGrantID           string `json:"pool_grant_id,omitempty"`
	PoolGrantVersion      int    `json:"pool_grant_version,omitempty"`
	PoolCredentialID      string `json:"pool_credential_id,omitempty"`
	PoolCredentialVersion int    `json:"pool_credential_version,omitempty"`
	DispatchID            string `json:"dispatch_id,omitempty"`
	DispatchState         string `json:"dispatch_state,omitempty"`
	LeaseUntil            string `json:"lease_until,omitempty"`
}

func (a CandidateAssignmentRef) Valid() bool {
	return validID(a.RunID, 100) && validID(a.NodeID, 150) && validID(a.WorkerID, 100) && a.Attempt > 0 && a.Fence > 0
}

// Pool-specific validity and current grants are additionally checked by the
// existing pool authority module against its durable dispatch, not these fields.
func validID(s string, n int) bool {
	return len(s) > 0 && len(s) <= n && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "/\\\x00\r\n")
}
func ValidHash(s string) bool   { return hash.MatchString(s) }
func ValidCommit(s string) bool { return commit.MatchString(s) }

type CandidateTransferStartRequest struct {
	ProjectID        string        `json:"project_id"`
	TenantID         string        `json:"tenant_id,omitempty"`
	RunnerID         string        `json:"runner_id,omitempty"`
	Assignment       AssignmentRef `json:"assignment"`
	OperationID      string        `json:"operation_id"`
	BaseCommit       string        `json:"base_commit"`
	BundleBaseCommit string        `json:"bundle_base_commit,omitempty"`
	HeadCommit       string        `json:"head_commit"`
	EvidenceSHA256   string        `json:"evidence_sha256"`
	BundleSHA256     string        `json:"bundle_sha256"`
	SizeBytes        int64         `json:"size_bytes"`
}

func (r CandidateTransferStartRequest) Validate() error {
	if !r.Assignment.Valid() || !validID(r.ProjectID, 64) || !validID(r.OperationID, 128) || !ValidCommit(r.BaseCommit) || !ValidCommit(r.HeadCommit) || !ValidCommit(r.BundleBase()) || len(r.BaseCommit) != len(r.HeadCommit) || len(r.BundleBase()) != len(r.HeadCommit) || r.BundleBase() == r.HeadCommit || !ValidHash(r.EvidenceSHA256) || !ValidHash(r.BundleSHA256) || r.SizeBytes < 1 || r.SizeBytes > MaxBundleBytes {
		return fmt.Errorf("invalid candidate transfer manifest or assignment")
	}
	return nil
}

// BundleBase is the pack prerequisite anchor. BaseCommit remains the selected
// workspace input; inherited no-delta code can therefore transfer B..M honestly.
func (r CandidateTransferStartRequest) BundleBase() string {
	if r.BundleBaseCommit != "" {
		return r.BundleBaseCommit
	}
	return r.BaseCommit
}

type CandidateTransferCommandRequest struct {
	ProjectID   string        `json:"project_id"`
	TenantID    string        `json:"tenant_id,omitempty"`
	RunnerID    string        `json:"runner_id,omitempty"`
	Assignment  AssignmentRef `json:"assignment"`
	OperationID string        `json:"operation_id"`
}

// Binding is a sealed producer identity. Resolve still requires a current
// consumer lease, frozen DAG candidate and successful matching source evidence.
type CandidateBinding struct {
	SchemaVersion    int           `json:"schema_version"`
	TransferID       string        `json:"transfer_id"`
	ProjectID        string        `json:"project_id"`
	State            string        `json:"state"`
	Source           AssignmentRef `json:"source"`
	BaseCommit       string        `json:"base_commit"`
	BundleBaseCommit string        `json:"bundle_base_commit,omitempty"`
	HeadCommit       string        `json:"head_commit"`
	EvidenceSHA256   string        `json:"evidence_sha256"`
	BundleSHA256     string        `json:"bundle_sha256"`
	SizeBytes        int64         `json:"size_bytes"`
}

func (b CandidateBinding) ValidSealed() bool {
	return b.SchemaVersion == SchemaVersion && validID(b.TransferID, 64) && b.State == "sealed" && b.Source.Valid() && validID(b.ProjectID, 64) && ValidCommit(b.BaseCommit) && ValidCommit(b.HeadCommit) && ValidCommit(b.BundleBase()) && len(b.BaseCommit) == len(b.HeadCommit) && len(b.BundleBase()) == len(b.HeadCommit) && b.BundleBase() != b.HeadCommit && ValidHash(b.EvidenceSHA256) && ValidHash(b.BundleSHA256) && b.SizeBytes > 0 && b.SizeBytes <= MaxBundleBytes
}

func (b CandidateBinding) BundleBase() string {
	if b.BundleBaseCommit != "" {
		return b.BundleBaseCommit
	}
	return b.BaseCommit
}

type CandidateImportReceipt struct {
	Binding        Binding `json:"binding"`
	Imported       bool    `json:"imported"`
	TargetWorkerID string  `json:"target_worker_id"`
}

// Short package-local spellings retain qualified names in generated public schemas.
type AssignmentRef = CandidateAssignmentRef
type StartRequest = CandidateTransferStartRequest
type CommandRequest = CandidateTransferCommandRequest
type Binding = CandidateBinding
type ImportReceipt = CandidateImportReceipt
