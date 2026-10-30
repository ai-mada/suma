package workbuddycloudcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

// CloudAssignmentBinding fixes the human grant selected in the claim transaction.
// The immutable digest survives the single permitted consumption (v1 -> v2);
// current authority must still be checked separately before each external action.
type CloudAssignmentBinding struct {
	DelegationID      string `json:"delegation_id"`
	DelegationVersion int    `json:"delegation_version"`
	DelegationSHA256  string `json:"delegation_sha256"`
}

func (b CloudAssignmentBinding) Validate() error {
	if !validID(b.DelegationID, 64) || !validVersion(b.DelegationVersion) || !cc.ValidHash(b.DelegationSHA256) {
		return ErrInvalid
	}
	return nil
}

// AssignmentBinding hashes authorization identity, never mutable consumption or
// display metadata. It cannot by itself establish that a grant is active/current.
func (d CloudDelegation) AssignmentBinding() (CloudAssignmentBinding, error) {
	if d.Validate() != nil {
		return CloudAssignmentBinding{}, ErrInvalid
	}
	identity := struct {
		ID, ProjectID, OwnerID, ConnectionID                                         string
		ConnectionVersionAtIssue, GrantGeneration                                    int
		ClientBindingSHA256, RunID, NodeID, NodeSpecSHA256, RequestSHA256, ExpiresAt string
		MaxCreates                                                                   int
	}{d.ID, d.ProjectID, d.OwnerID, d.ConnectionID, d.ConnectionVersionAtIssue,
		d.GrantGeneration, d.ClientBindingSHA256, d.RunID, d.NodeID,
		d.NodeSpecSHA256, d.RequestSHA256, d.ExpiresAt, d.MaxCreates}
	raw, err := json.Marshal(identity)
	if err != nil {
		return CloudAssignmentBinding{}, ErrInvalid
	}
	sum := sha256.Sum256(raw)
	return CloudAssignmentBinding{d.ID, d.Version, hex.EncodeToString(sum[:])}, nil
}

// SameAssignmentIdentity compares a current carrier with its original proof.
// Heartbeats may change only the lease/state observations. Callers must retain
// the original complete ref for stored-body hashes and operation replay.
func SameAssignmentIdentity(a, b cc.AssignmentRef) bool {
	a.LeaseUntil, b.LeaseUntil = "", ""
	a.DispatchState, b.DispatchState = "", ""
	return a == b
}

// CloudWaitEvidence requests durable reconciliation of an existing cloud effect.
// It is not a success, stop receipt or authority to create another task. The
// control plane must resolve it against the original claim and stored submission.
type CloudWaitEvidence struct {
	Protocol          string                 `json:"protocol"`
	ClaimID           string                 `json:"claim_id"`
	Assignment        cc.AssignmentRef       `json:"assignment"`
	Binding           CloudAssignmentBinding `json:"binding"`
	CreateOperationID string                 `json:"create_operation_id"`
	SubmissionID      string                 `json:"submission_id,omitempty"`
	SubmissionVersion int                    `json:"submission_version,omitempty"`
	Cause             string                 `json:"cause"`
}

func (e CloudWaitEvidence) Validate() error {
	if e.Protocol != Protocol || !validID(e.ClaimID, 100) || e.Binding.Validate() != nil ||
		!operationID(e.CreateOperationID) || !validAssignment(e.Assignment.ProjectID, e.Assignment) {
		return ErrInvalid
	}
	if e.SubmissionID == "" {
		if e.SubmissionVersion != 0 {
			return ErrInvalid
		}
	} else if !validID(e.SubmissionID, 64) || !validVersion(e.SubmissionVersion) {
		return ErrInvalid
	}
	switch e.Cause {
	case "creation_unconfirmed", "remote_unknown", "waiting_input", "delivery_required", "interrupted", "authorization_changed", "observation_unconfirmed":
		return nil
	default:
		return ErrInvalid
	}
}
