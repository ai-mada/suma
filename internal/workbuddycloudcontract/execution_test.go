package workbuddycloudcontract

import (
	"strings"
	"testing"
)

func TestCloudBindingPinsAuthorityAcrossConsumption(t *testing.T) {
	d := CloudDelegation{ID: "delegation", ProjectID: "project", OwnerID: "owner", ConnectionID: "connection", Version: 1, RunID: "run", NodeID: "node", NodeSpecSHA256: strings.Repeat("a", 64), ConnectionVersionAtIssue: 1, GrantGeneration: 1, ClientBindingSHA256: strings.Repeat("b", 64), RequestSHA256: strings.Repeat("c", 64), ExpiresAt: "2026-10-30T10:00:00Z", MaxCreates: 1, State: "active", CreatedAt: "2026-10-30T09:00:00Z", UpdatedAt: "2026-10-30T09:00:00Z"}
	first, err := d.AssignmentBinding()
	if err != nil || first.Validate() != nil {
		t.Fatal(first, err)
	}
	d.Version, d.CreatesConsumed, d.State, d.UpdatedAt = 2, 1, "revoked", "2026-10-30T09:01:00Z"
	consumed, err := d.AssignmentBinding()
	if err != nil || first.DelegationSHA256 != consumed.DelegationSHA256 || consumed.DelegationVersion != 2 {
		t.Fatal("mutable consumption changed authority identity", err)
	}
	d.OwnerID = "different-owner"
	replaced, err := d.AssignmentBinding()
	if err != nil || replaced.DelegationSHA256 == first.DelegationSHA256 {
		t.Fatal("replacement owner retained original authority", err)
	}
}

func TestCloudWaitProofDoesNotBroadenOriginalAssignment(t *testing.T) {
	a := testAssignment()
	b := a
	b.LeaseUntil, b.DispatchState = "2026-10-30T10:01:00Z", "reconciling"
	if !SameAssignmentIdentity(a, b) {
		t.Fatal("heartbeat observation changed identity")
	}
	b.Fence++
	if SameAssignmentIdentity(a, b) {
		t.Fatal("late fence accepted")
	}
	e := CloudWaitEvidence{Protocol: Protocol, ClaimID: "claim", Assignment: a, Binding: CloudAssignmentBinding{"delegation", 1, strings.Repeat("a", 64)}, CreateOperationID: "original-create", Cause: "delivery_required"}
	if e.Validate() != nil {
		t.Fatal("valid original wait rejected")
	}
	e.SubmissionVersion = 1
	if e.Validate() == nil {
		t.Fatal("partial submission identity accepted")
	}
	e.SubmissionID = "submission"
	if e.Validate() != nil {
		t.Fatal("valid submission proof rejected")
	}
	e.Cause = "succeeded"
	if e.Validate() == nil {
		t.Fatal("machine can promote vendor status into success")
	}
}
