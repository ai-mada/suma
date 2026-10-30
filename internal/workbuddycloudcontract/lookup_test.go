package workbuddycloudcontract

import (
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/workbuddy"
)

func lookupAssignment() cc.AssignmentRef {
	return cc.AssignmentRef{RunID: "run", NodeID: "node", Attempt: 1, Fence: 2, WorkerID: "worker"}
}

func TestCloudLookupRequestRejectsCommandsAndPartialScope(t *testing.T) {
	r := CloudLookupRequest{ProjectID: "project", DelegationID: "delegation", Assignment: lookupAssignment()}
	if r.Validate() != nil {
		t.Fatal("valid read-only lookup rejected")
	}
	raw, _ := json.Marshal(r)
	for _, extra := range []string{`"prompt":"replacement"`, `"token":"secret"`, `"provider_url":"https://other.invalid"`, `"idempotency_key":"new-operation"`, `"expected_version":1`, `"project_id":"other"`} {
		var out CloudLookupRequest
		bad := string(raw[:len(raw)-1]) + "," + extra + "}"
		if Decode([]byte(bad), &out) == nil {
			t.Fatal("lookup accepted injected command, secret or duplicate scope")
		}
	}
	for _, mutate := range []func(*CloudLookupRequest){
		func(v *CloudLookupRequest) { v.ProjectID = "" },
		func(v *CloudLookupRequest) { v.DelegationID = strings.Repeat("d", 65) },
		func(v *CloudLookupRequest) { v.Assignment.Fence = 0 },
		func(v *CloudLookupRequest) { v.Assignment.PoolID = "pool" },
	} {
		bad := r
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid lookup scope accepted")
		}
	}
}

func TestCloudLookupResultBindsFullOriginalAssignment(t *testing.T) {
	a := lookupAssignment()
	a.TenantID = "tenant"
	a.ProjectID = "project"
	a.PoolID = "pool"
	a.PoolRunnerID = "runner"
	a.PoolSlotID = "slot"
	a.PoolGrantID = "grant"
	a.PoolGrantVersion = 1
	a.PoolCredentialID = "credential"
	a.PoolCredentialVersion = 2
	a.DispatchID = "dispatch"
	a.DispatchState = "active"
	a.LeaseUntil = "2026-10-30T10:00:00Z"
	s := CloudSubmission{ID: "submission", ProjectID: "project", DelegationID: "delegation", Version: 1, RunID: a.RunID, NodeID: a.NodeID, Assignment: a, RequestSHA256: strings.Repeat("a", 64), State: "prepared", UsageState: "unreported", CreatedAt: "2026-10-30T09:00:00Z", UpdatedAt: "2026-10-30T09:00:00Z"}
	r := CloudLookupResult{ProjectID: "project", DelegationID: "delegation", Assignment: a, State: "not_found"}
	if r.Validate() != nil {
		t.Fatal("valid not-found result rejected")
	}
	r.Submission = &s
	if r.Validate() == nil {
		t.Fatal("not-found result carried submission")
	}
	for _, state := range []string{"prepared", "unknown", "task_known"} {
		s.State = state
		r.State = state
		if state == "task_known" {
			s.Task = &workbuddy.Task{ID: "supplier-task", Status: "working"}
			s.ObservedAt = "2026-10-30T09:00:00Z"
		}
		if r.Validate() != nil {
			t.Fatal("valid stored fact rejected", state)
		}
		bad := r
		bad.Submission = nil
		if bad.Validate() == nil {
			t.Fatal("found state lacked stored fact")
		}
	}
	for _, mutate := range []func(*CloudSubmission){
		func(v *CloudSubmission) { v.ProjectID = "other" }, func(v *CloudSubmission) { v.DelegationID = "other" },
		func(v *CloudSubmission) { v.RunID = "other" }, func(v *CloudSubmission) { v.NodeID = "other" },
		func(v *CloudSubmission) { v.State = "unknown" }, func(v *CloudSubmission) { v.Assignment.Fence++ },
		func(v *CloudSubmission) { v.Assignment.Attempt++ }, func(v *CloudSubmission) { v.Assignment.WorkerID = "other" },
		func(v *CloudSubmission) { v.Assignment.PoolGrantVersion++ }, func(v *CloudSubmission) { v.Assignment.PoolCredentialVersion++ },
		func(v *CloudSubmission) { v.Assignment.DispatchID = "other" }, func(v *CloudSubmission) { v.Assignment.LeaseUntil = "2026-10-30T11:00:00Z" },
	} {
		copy := s
		mutate(&copy)
		bad := r
		bad.Submission = &copy
		if bad.Validate() == nil {
			t.Fatal("lookup accepted changed original fact binding")
		}
	}
	bad := r
	bad.State = "completed"
	if bad.Validate() == nil {
		t.Fatal("provider status became lookup lifecycle")
	}
	raw, _ := json.Marshal(r)
	for _, extra := range []string{`"token":"secret"`, `"provider_url":"https://other.invalid"`, `"prompt":"replacement"`, `"idempotency_key":"operation"`} {
		var out CloudLookupResult
		if Decode([]byte(string(raw[:len(raw)-1])+","+extra+"}"), &out) == nil {
			t.Fatal("lookup response accepted extra authority")
		}
	}
}
