package workbuddycloudcontract

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/workbuddy"
)

func testDelegate() CloudDelegateRequest {
	return CloudDelegateRequest{ProjectID: "project", ConnectionID: "connection", ExpectedConnectionVersion: 1, GrantGeneration: 1, RunID: "run", NodeID: "node", ExpectedRunVersion: 1, NodeSpecSHA256: strings.Repeat("a", 64), RequestSHA256: strings.Repeat("b", 64), ExpiresAt: "2026-10-30T10:00:00Z", MaxCreates: 1, IdempotencyKey: "delegate-operation"}
}
func testAssignment() cc.AssignmentRef {
	return cc.AssignmentRef{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1}
}

func TestCloudNodeBindingKeepsExactProviderDigestAndRejectsExtraAuthority(t *testing.T) {
	raw := json.RawMessage(`{"connection_id":"connection","grant_generation":2,"request":{"prompt":"private-prompt\n准确正文","name":"任务"}}`)
	b, err := ParseNodeBinding(raw)
	if err != nil || b.ConnectionID != "connection" || b.GrantGeneration != 2 {
		t.Fatal("valid binding rejected", err)
	}
	want, _ := workbuddy.CreateRequestDigest(b.Request)
	got, err := b.RequestDigest()
	if err != nil || got != want {
		t.Fatal("digest differs from provider request bytes")
	}
	for _, invalid := range []string{
		strings.Replace(string(raw), `"grant_generation":2`, `"grant_generation":0`, 1),
		strings.Replace(string(raw), `"connection_id":"connection"`, `"connection_id":null`, 1),
		strings.Replace(string(raw), `"grant_generation":2`, `"grant_generation":2,"grant_generation":3`, 1),
		strings.Replace(string(raw), `"request":{`, `"endpoint":"https://elsewhere.invalid","request":{`, 1),
		strings.Replace(string(raw), `"name":"任务"`, `"name":"任务","token":"private-token"`, 1),
		`{"connection_id":"connection","grant_generation":2,"request":{"prompt":" "}}`,
	} {
		if _, err := ParseNodeBinding(json.RawMessage(invalid)); err == nil || strings.Contains(fmt.Sprint(err), "private") {
			t.Fatal("unsafe binding accepted or diagnostic exposed source")
		}
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", b, b), "private-prompt") {
		t.Fatal("binding formatting exposed prompt")
	}
}

func TestCloudDelegateUsesExplicitDatabaseTimeAndOneCreate(t *testing.T) {
	r := testDelegate()
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	if r.Validate() != nil || r.ValidateAt(now) != nil {
		t.Fatal("valid delegation rejected")
	}
	r.ExpiresAt = now.Add(MaxDelegationLifetime).Format(time.RFC3339Nano)
	if r.ValidateAt(now) != nil {
		t.Fatal("inclusive configured lifetime rejected")
	}
	for _, mutate := range []func(*CloudDelegateRequest){
		func(r *CloudDelegateRequest) { r.ExpiresAt = now.Format(time.RFC3339Nano) },
		func(r *CloudDelegateRequest) {
			r.ExpiresAt = now.Add(MaxDelegationLifetime + time.Nanosecond).Format(time.RFC3339Nano)
		},
		func(r *CloudDelegateRequest) { r.MaxCreates = 2 },
		func(r *CloudDelegateRequest) { r.ExpectedConnectionVersion = MaxVersion },
		func(r *CloudDelegateRequest) { r.NodeSpecSHA256 = strings.Repeat("A", 64) },
		func(r *CloudDelegateRequest) { r.RequestSHA256 = "" },
		func(r *CloudDelegateRequest) { r.IdempotencyKey = "short" },
	} {
		bad := testDelegate()
		mutate(&bad)
		if bad.ValidateAt(now) == nil {
			t.Fatal("invalid delegation admitted")
		}
	}
	if r.ValidateAt(time.Time{}) == nil {
		t.Fatal("missing authority clock admitted")
	}
}

func TestCloudMachineScopeCannotBecomePromptOrPartialPool(t *testing.T) {
	r := CloudCreateRequest{ProjectID: "project", DelegationID: "delegation", ExpectedVersion: 1, Assignment: testAssignment(), IdempotencyKey: "create-operation"}
	if r.Validate() != nil {
		t.Fatal("project assignment rejected")
	}
	raw, _ := json.Marshal(r)
	for _, extra := range []string{`"prompt":"replacement"`, `"task_id":"other"`, `"url":"https://elsewhere.invalid"`} {
		injected := append(append([]byte{}, raw[:len(raw)-1]...), []byte(","+extra+"}")...)
		var out CloudCreateRequest
		if Decode(injected, &out) == nil {
			t.Fatal("machine request accepted caller-selected provider input")
		}
	}
	r.Assignment.PoolID = "pool"
	if r.Validate() == nil {
		t.Fatal("partial pool scope accepted")
	}
	r.Assignment.TenantID, r.Assignment.ProjectID = "tenant", "project"
	r.Assignment.PoolRunnerID, r.Assignment.PoolSlotID = "poolrunner", "slot"
	r.Assignment.PoolGrantID, r.Assignment.PoolGrantVersion = "grant", 1
	r.Assignment.PoolCredentialID, r.Assignment.PoolCredentialVersion = "credential", 1
	r.Assignment.DispatchID, r.Assignment.DispatchState, r.Assignment.LeaseUntil = "dispatch", "running", "2026-10-30T10:00:00Z"
	if r.Validate() != nil {
		t.Fatal("complete pool scope rejected")
	}
	r.Assignment.ProjectID = "other"
	if r.Validate() == nil {
		t.Fatal("cross-project pool scope accepted")
	}
	r.Assignment = testAssignment()
	r.Assignment.Fence = 9007199254740992
	if r.Validate() == nil {
		t.Fatal("unsafe JSON fence admitted")
	}
}

func TestCloudVersionAndObservationStatesDoNotManufactureFacts(t *testing.T) {
	stamp := "2026-10-30T09:00:00Z"
	d := CloudDelegation{ID: "delegation", ProjectID: "project", OwnerID: "owner", ConnectionID: "connection", Version: MaxVersion, RunID: "run", NodeID: "node", NodeSpecSHA256: strings.Repeat("a", 64), ConnectionVersionAtIssue: 1, GrantGeneration: 1, ClientBindingSHA256: strings.Repeat("b", 64), RequestSHA256: strings.Repeat("c", 64), ExpiresAt: "2026-10-30T10:00:00Z", MaxCreates: 1, CreatesConsumed: 1, State: "revoked", CreatedAt: stamp, UpdatedAt: stamp}
	if d.Validate() != nil {
		t.Fatal("final version tombstone cannot be read")
	}
	if (CloudRevokeRequest{ProjectID: "project", ExpectedVersion: MaxVersion, IdempotencyKey: "revoke-operation"}).Validate() == nil {
		t.Fatal("exhausted version accepted for a new mutation")
	}
	s := CloudSubmission{ID: "submission", ProjectID: "project", DelegationID: "delegation", Version: MaxVersion, RunID: "run", NodeID: "node", Assignment: testAssignment(), RequestSHA256: strings.Repeat("a", 64), State: "unknown", UsageState: "unreported", LastError: "effect_unknown", CreatedAt: stamp, UpdatedAt: stamp}
	if s.Validate() != nil {
		t.Fatal("unknown final-version fact rejected")
	}
	s.State = "task_known"
	if s.Validate() == nil {
		t.Fatal("known task without supplier identity accepted")
	}
	s.Task = &workbuddy.Task{ID: "task_one", Status: "future_vendor_state"}
	s.ObservedAt = stamp
	if s.Validate() != nil {
		t.Fatal("unknown vendor status must be retained without success inference")
	}
	raw, _ := json.Marshal(s)
	var received CloudSubmission
	if Decode(raw, &received) != nil || received.Validate() != nil {
		t.Fatal("valid observation failed strict response round trip")
	}
	for _, unsafe := range []string{
		strings.Replace(string(raw), `"task_id":"task_one"`, `"task_id":"task_one","acp_token":"private-token"`, 1),
		strings.Replace(string(raw), `"version":2147483647`, `"version":null`, 1),
		strings.Replace(string(raw), `"state":"task_known"`, `"state":"task_known","state":"unknown"`, 1),
	} {
		if Decode([]byte(unsafe), &received) == nil {
			t.Fatal("unsafe control response passed strict decoder")
		}
	}
	s.State = "succeeded"
	if s.Validate() == nil {
		t.Fatal("provider observation became platform completion")
	}
	s.State, s.UsageState = "task_known", "complete"
	if s.Validate() == nil {
		t.Fatal("unobserved cloud use became billed complete usage")
	}
}

func TestCloudNodeViewBindsReopenedSourceAndSeparatesHumanObservation(t *testing.T) {
	stamp := "2026-10-30T09:00:00Z"
	binding := NodeBinding{ConnectionID: "connection", GrantGeneration: 1, Request: workbuddy.CreateRequest{Prompt: "original private prompt"}}
	requestHash, _ := binding.RequestDigest()
	view := CloudNodeView{ProjectID: "project", RunID: "run", NodeID: "node", RunVersion: 2, NodeSpecSHA256: strings.Repeat("a", 64), RequestSHA256: requestHash, Binding: binding, AllowedActions: []string{"delegate"}}
	if view.Validate() != nil {
		t.Fatal("exact original node view rejected")
	}
	d := CloudDelegation{ID: "delegation", ProjectID: "project", OwnerID: "owner", ConnectionID: "connection", Version: 1, RunID: "run", NodeID: "node", NodeSpecSHA256: view.NodeSpecSHA256, ConnectionVersionAtIssue: 1, GrantGeneration: 1, ClientBindingSHA256: strings.Repeat("b", 64), RequestSHA256: requestHash, ExpiresAt: "2026-10-30T10:00:00Z", MaxCreates: 1, CreatesConsumed: 1, State: "active", CreatedAt: stamp, UpdatedAt: stamp}
	s := CloudSubmission{ID: "submission", ProjectID: "project", DelegationID: d.ID, Version: 2, RunID: "run", NodeID: "node", Assignment: testAssignment(), RequestSHA256: requestHash, State: "task_known", Task: &workbuddy.Task{ID: "task_one", Status: "running"}, ObservedAt: stamp, UsageState: "unreported", CreatedAt: stamp, UpdatedAt: stamp}
	view.Delegation, view.Submission, view.AllowedActions = &d, &s, []string{"revoke", "observe"}
	if view.Validate() != nil {
		t.Fatal("bound related records rejected")
	}
	for _, change := range []func(*CloudNodeView){
		func(v *CloudNodeView) { v.Binding.Request.Prompt = "replacement" },
		func(v *CloudNodeView) { v.Submission.ProjectID = "other" },
		func(v *CloudNodeView) { v.Submission.DelegationID = "other" },
		func(v *CloudNodeView) { v.Delegation.GrantGeneration++ },
		func(v *CloudNodeView) { v.Delegation.State = "revoked" },
		func(v *CloudNodeView) { v.AllowedActions = []string{"observe", "observe"} },
		func(v *CloudNodeView) { v.AllowedActions = []string{"create"} },
		func(v *CloudNodeView) { v.Submission = nil },
	} {
		bad, dc, sc := view, d, s
		bad.Delegation, bad.Submission = &dc, &sc
		change(&bad)
		if bad.Validate() == nil {
			t.Fatal("reopened view accepted mismatched source or impossible action")
		}
	}
	view.Delegation, view.AllowedActions = nil, []string{}
	if view.Validate() != nil {
		t.Fatal("team projection required another owner's private delegation")
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", view, view), "private prompt") {
		t.Fatal("view diagnostic exposed original prompt")
	}
	r := CloudObserveRequest{ProjectID: "project", ExpectedVersion: 2, IdempotencyKey: "observe-operation"}
	if r.Validate() != nil {
		t.Fatal("explicit human observe request rejected")
	}
	raw, _ := json.Marshal(r)
	injected := append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"assignment":{"run_id":"run"}}`)...)
	if Decode(injected, &r) == nil {
		t.Fatal("human observe accepted machine assignment impersonation")
	}
	r.ExpectedVersion = MaxVersion
	if r.Validate() == nil {
		t.Fatal("observation mutation lacks version room")
	}
}
