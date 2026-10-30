package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	"github.com/ai-mada/suma/internal/workbuddy"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

type cloudFixture struct {
	f                *fixture
	w                *WorkBuddyConnections
	delegation       wc.CloudDelegation
	request          wc.CloudCreateRequest
	actor            Actor
	poolToken        string
	claimToken       string
	poolClaim        dag.PoolClaimRequest
	creates, queries atomic.Int32
}

func newCloudFixture(t *testing.T, provider func(http.ResponseWriter, *http.Request)) *cloudFixture {
	return newCloudFixtureMode(t, provider, false)
}
func newCloudFixtureMode(t *testing.T, provider func(http.ResponseWriter, *http.Request), pooled bool, unclaimed ...bool) *cloudFixture {
	t.Helper()
	h := &cloudFixture{}
	h.f, h.w = workBuddyCommandFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/openapi/v2/tasks" {
			h.creates.Add(1)
			if provider != nil {
				provider(w, r)
				return
			}
			w.WriteHeader(201)
			fmt.Fprint(w, `{"task_id":"cloud-task","status":"running","name":"Cloud task"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/openapi/v2/tasks/") {
			h.queries.Add(1)
			if r.URL.Path != "/openapi/v2/tasks/cloud-task" {
				t.Error("queried another task")
			}
			fmt.Fprint(w, `{"task_id":"cloud-task","status":"provider_future_state","name":"Cloud task"}`)
			return
		}
		workBuddyCommandResponse(w, "open-one", "user.task.readable user.task.invokable", "private-refresh")
	})
	if err := h.f.s.InitializeWorkBuddyCloud(context.Background()); err != nil {
		t.Fatal(err)
	}
	auth, _ := workBuddyCommandStart(t, h.f, h.w)
	connection, err := h.w.CompleteAuthorization(context.Background(), h.f.actor, h.f.project, auth.ID, workBuddyCommandComplete(auth))
	if err != nil {
		t.Fatal(err)
	}
	var poolID, poolRunnerID, poolWorkerID string
	if pooled {
		pool, grant := dispatchAuthorizedPool(t, h.f)
		dispatchSetRoute(t, h.f, 0, &grant)
		credential, e := h.f.s.RegisterExecutionPoolRunner(context.Background(), h.f.actor, pool.ID, ExecutionPoolRunnerCreateRequest{Name: "WorkBuddy fixture", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		poolID, poolRunnerID, poolWorkerID, h.poolToken = pool.ID, credential.Runner.ID, credential.Runner.WorkerID, credential.Token
	}
	binding := wc.NodeBinding{ConnectionID: connection.ID, GrantGeneration: connection.GrantGeneration, Request: workbuddy.CreateRequest{Prompt: "fixed cloud prompt", Name: "Cloud task"}}
	workflow, err := h.f.e.Publish(context.Background(), h.f.user.TenantID, h.f.project, dag.Workflow{Name: "Cloud fixture", Nodes: []dag.Node{{ID: "cloud", Type: "external_workflow", Config: map[string]any{"plugin": "workbuddy-cloud", "workflow": "workbuddy-cloud/v1", "workbuddy_cloud": binding}}}})
	if err != nil {
		t.Fatal(err)
	}
	var run dag.Run
	if pooled {
		created := h.f.call("POST", "/runs", h.f.body(map[string]any{"workflow_id": workflow.ID, "workflow_version": workflow.Version, "idempotency_key": newID()}), 201)
		if err = json.Unmarshal([]byte(stringify(created)), &run); err != nil || run.ID == "" {
			t.Fatal("create bound run", err)
		}
	} else {
		run, err = h.f.e.CreateRun(context.Background(), h.f.user.TenantID, h.f.project, workflow.ID, newID())
		if err != nil {
			t.Fatal(err)
		}
	}
	var raw string
	var runVersion int
	if err = h.f.s.DB.QueryRow(`SELECT n.spec,r.version FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE n.run_id=? AND n.id='cloud'`, run.ID).Scan(&raw, &runVersion); err != nil {
		t.Fatal(err)
	}
	var spec dag.Node
	if err = json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatal(err)
	}
	requestSHA, err := workbuddy.CreateRequestDigest(binding.Request)
	if err != nil {
		t.Fatal(err)
	}
	h.delegation, err = h.w.DelegateCloud(context.Background(), h.f.actor, h.f.project, wc.CloudDelegateRequest{ProjectID: h.f.project, ConnectionID: connection.ID, ExpectedConnectionVersion: connection.Version, GrantGeneration: connection.GrantGeneration, RunID: run.ID, NodeID: "cloud", ExpectedRunVersion: runVersion, NodeSpecSHA256: digest(stringify(spec)), RequestSHA256: requestSHA, ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), MaxCreates: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	token := h.f.call("POST", "/auth/tokens", h.f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	h.actor = domainCActor(t, h.f.s, token)
	h.claimToken = token
	h.request = wc.CloudCreateRequest{ProjectID: h.f.project, DelegationID: h.delegation.ID, ExpectedVersion: h.delegation.Version, IdempotencyKey: newID()}
	if pooled {
		h.poolClaim = dag.PoolClaimRequest{PoolID: poolID, RunnerID: poolRunnerID, WorkerID: poolWorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: h.f.user.TenantID, ProjectID: h.f.project, Capabilities: []string{"external_workflow:workbuddy-cloud"}}}}
	}
	if len(unclaimed) > 0 && unclaimed[0] {
		return h
	}
	var assigned *dag.Assignment
	cloud := h.f.s.newCloudClaimAuthority(h.w)
	if pooled {
		claim := h.poolClaim
		assigned, err = h.f.e.ClaimPoolWithOperation(context.Background(), claim, func(tx *sql.Tx) (dag.PoolClaimAuthority, error) {
			_, authority, e := cloud.beforePool(context.Background(), tx, h.poolToken, claim)
			return authority, e
		}, func(tx *sql.Tx, scope dag.PoolScope, runID, nodeID string) (bool, error) {
			_, allowed, e := cloud.eligibleTx(context.Background(), tx, scope.TenantID, scope.ProjectID, runID, nodeID)
			return allowed, e
		}, cloud.runnableHook())
	} else {
		authority := h.f.s.engineAuthority(context.Background(), h.actor, commandTarget{tenantID: h.f.user.TenantID, projectID: h.f.project}, purposeRunnerWrite)
		defer authority.close()
		operation := newID()
		caps := []string{"external_workflow:workbuddy-cloud"}
		assigned, err = h.f.e.ClaimWithOperation(authority.ctx, h.f.user.TenantID, h.f.project, "cloud-worker", operation, caps, func(tx *sql.Tx) error { return cloud.beforePrivate(authority, tx, "cloud-worker", operation, caps) }, func(tx *sql.Tx, runID, nodeID string) (bool, error) {
			_, allowed, e := cloud.eligibleTx(authority.ctx, tx, h.f.user.TenantID, h.f.project, runID, nodeID)
			return allowed, e
		}, cloud.runnableHook())
	}
	if err != nil || assigned == nil {
		t.Fatal("claim", err)
	}
	h.request = wc.CloudCreateRequest{ProjectID: h.f.project, DelegationID: h.delegation.ID, ExpectedVersion: h.delegation.Version, Assignment: repositoryTestRef(*assigned), IdempotencyKey: newID()}
	return h
}

func TestWorkBuddyCloudActualCreateReplayAndExactRefresh(t *testing.T) {
	h := newCloudFixture(t, nil)
	ctx := context.Background()
	created, err := h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request)
	if err != nil || created.State != "task_known" || created.Task == nil || created.Task.ID != "cloud-task" || created.UsageState != "unreported" || h.creates.Load() != 1 {
		t.Fatal("create", err)
	}
	replay, err := h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request)
	if err != nil || replay.ID != created.ID || h.creates.Load() != 1 {
		t.Fatal("replay resent", err)
	}
	changed := h.request
	changed.IdempotencyKey = newID()
	if _, err = h.w.CreateCloudTask(ctx, h.actor, h.f.project, changed); err == nil || h.creates.Load() != 1 {
		t.Fatal("second operation created another cloud task")
	}
	refreshed, err := h.w.RefreshCloudTask(ctx, h.actor, h.f.project, created.ID, wc.CloudRefreshRequest{ProjectID: h.f.project, ExpectedVersion: created.Version, Assignment: h.request.Assignment, IdempotencyKey: newID()})
	if err != nil || refreshed.Task.Status != "provider_future_state" || h.queries.Load() != 1 {
		t.Fatal("exact refresh", err)
	}
	var stored string
	if err = h.f.s.DB.QueryRow(`SELECT payload FROM suma_workbuddy_cloud_submissions WHERE id=?`, created.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-access", "private-refresh", "fixed cloud prompt"} {
		if strings.Contains(stored, secret) {
			t.Fatal("secret persisted in submission")
		}
	}
}

func TestWorkBuddyCloudTwoServicesPrepareOnceAndUnknownNeverRetries(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	h := newCloudFixture(t, func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; w.WriteHeader(503) })
	otherService := &Service{DB: h.f.s.DB, Dialect: h.f.s.Dialect, DataDir: h.f.s.DataDir}
	var sequence int
	var databaseName, databaseFile string
	if err := h.f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&sequence, &databaseName, &databaseFile); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", databaseFile)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	otherService.DB = db
	other, err := NewWorkBuddyConnections(otherService, h.w.app, h.w.key)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := h.w.CreateCloudTask(context.Background(), h.actor, h.f.project, h.request)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("no provider create")
	}
	if _, err = other.CreateCloudTask(context.Background(), h.actor, h.f.project, h.request); err == nil {
		t.Fatal("durable prepared allowed replay")
	}
	once.Do(func() { close(release) })
	if err = <-result; err == nil {
		t.Fatal("unknown effect reported success")
	}
	if _, err = other.CreateCloudTask(context.Background(), h.actor, h.f.project, h.request); err == nil || h.creates.Load() != 1 {
		t.Fatal("unknown repeated create")
	}
}

func TestWorkBuddyCloudCurrentAuthorityAndBudgetRejectBeforeWire(t *testing.T) {
	for _, kind := range []string{"paused", "fence", "cross_project", "owner_removed", "scope", "grant_changed", "organization_enforce", "project_enforce"} {
		t.Run(kind, func(t *testing.T) {
			h := newCloudFixture(t, nil)
			ctx := context.Background()
			request := h.request
			switch kind {
			case "paused":
				var version int
				if err := h.f.s.DB.QueryRow(`SELECT version FROM dag_runs WHERE id=?`, request.Assignment.RunID).Scan(&version); err != nil {
					t.Fatal(err)
				}
				if _, err := h.f.e.Command(ctx, h.f.user.TenantID, h.f.project, request.Assignment.RunID, "pause", newID(), version, nil); err != nil {
					t.Fatal(err)
				}
			case "fence":
				request.Assignment.Fence++
			case "cross_project":
				request.ProjectID = "other-project"
			case "owner_removed":
				if _, err := h.f.s.DB.Exec(`DELETE FROM suma_org_members WHERE user_id=? AND tenant_id=?`, h.f.user.ID, h.f.user.TenantID); err != nil {
					t.Fatal(err)
				}
			case "scope":
				h.w.app.Scopes = []string{"user.task.readable"}
			case "grant_changed":
				c, err := h.w.Get(ctx, h.f.actor, h.f.project, h.delegation.ConnectionID)
				if err != nil {
					t.Fatal(err)
				}
				a, err := h.w.StartAuthorization(ctx, h.f.actor, h.f.project, WorkBuddyAuthorizationRequest{ProjectID: h.f.project, ConnectionID: c.ID, ExpectedVersion: c.Version, Scopes: c.Scopes, IdempotencyKey: newID()})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = h.w.CompleteAuthorization(ctx, h.f.actor, h.f.project, a.ID, workBuddyCommandComplete(a)); err != nil {
					t.Fatal(err)
				}
			default:
				scope := strings.TrimSuffix(kind, "_enforce")
				if _, err := h.f.s.WriteUsagePolicy(ctx, h.f.actor, h.f.project, UsagePolicyWriteRequest{ProjectID: h.f.project, Scope: scope, ExpectedVersion: 0, Mode: "enforce", Currency: "USD", AcknowledgeEnforcement: true, Reason: "explicit strict policy", IdempotencyKey: newID()}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := h.w.CreateCloudTask(ctx, h.actor, request.ProjectID, request); err == nil || h.creates.Load() != 0 {
				t.Fatal("current authority failure sent cloud request")
			}
			var count int
			if err := h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workbuddy_cloud_submissions`).Scan(&count); err != nil || count != 0 {
				t.Fatal("rejected request consumed prepare", err)
			}
		})
	}
}

func TestWorkBuddyCloudSameGrantTokenRotationRemainsUsable(t *testing.T) {
	h := newCloudFixture(t, nil)
	ctx := context.Background()
	c, err := h.w.Get(ctx, h.f.actor, h.f.project, h.delegation.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := h.w.Refresh(ctx, h.f.actor, h.f.project, c.ID, WorkBuddyRefreshRequest{ProjectID: h.f.project, ExpectedVersion: c.Version, IdempotencyKey: newID()})
	if err != nil || refreshed.GrantGeneration != c.GrantGeneration || refreshed.TokenGeneration <= c.TokenGeneration {
		t.Fatal("refresh", err)
	}
	sub, err := h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request)
	if err != nil {
		t.Fatal("ordinary token rotation invalidated durable grant", err)
	}
	var generation int
	if err = h.f.s.DB.QueryRow(`SELECT token_generation FROM suma_workbuddy_cloud_submissions WHERE id=?`, sub.ID).Scan(&generation); err != nil || generation != refreshed.TokenGeneration {
		t.Fatal("actual token generation not recorded", err)
	}
}

func TestWorkBuddyCloudPrepareAtomicityAndLateObservedFact(t *testing.T) {
	t.Run("prepare_rollback", func(t *testing.T) {
		h := newCloudFixture(t, nil)
		if _, err := h.f.s.DB.Exec(`CREATE TRIGGER reject_cloud_prepare BEFORE INSERT ON suma_workbuddy_cloud_submissions BEGIN SELECT RAISE(ABORT,'fixture rejection'); END`); err != nil {
			t.Fatal(err)
		}
		if _, err := h.w.CreateCloudTask(context.Background(), h.actor, h.f.project, h.request); err == nil || h.creates.Load() != 0 {
			t.Fatal("failed prepare made external request")
		}
		d, err := h.w.GetCloudDelegation(context.Background(), h.f.actor, h.f.project, h.delegation.ID)
		if err != nil || d.Version != 1 || d.CreatesConsumed != 0 {
			t.Fatal("failed transaction consumed grant", err)
		}
	})
	t.Run("revoke_during_wire", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		var once sync.Once
		defer once.Do(func() { close(release) })
		h := newCloudFixture(t, func(w http.ResponseWriter, r *http.Request) {
			close(entered)
			<-release
			w.WriteHeader(201)
			fmt.Fprint(w, `{"task_id":"cloud-task","status":"running"}`)
		})
		finished := make(chan error, 1)
		go func() {
			_, err := h.w.CreateCloudTask(context.Background(), h.actor, h.f.project, h.request)
			finished <- err
		}()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("no external request")
		}
		d, err := h.w.GetCloudDelegation(context.Background(), h.f.actor, h.f.project, h.delegation.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.w.RevokeCloudDelegation(context.Background(), h.f.actor, h.f.project, d.ID, wc.CloudRevokeRequest{ProjectID: h.f.project, ExpectedVersion: d.Version, IdempotencyKey: newID()}); err != nil {
			t.Fatal(err)
		}
		once.Do(func() { close(release) })
		if err = <-finished; err == nil {
			t.Fatal("revoked caller received privileged response")
		}
		var id string
		if err = h.f.s.DB.QueryRow(`SELECT id FROM suma_workbuddy_cloud_submissions`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		sub, err := h.w.GetCloudSubmission(context.Background(), h.f.actor, h.f.project, id)
		if err != nil || sub.State != "task_known" || sub.Task == nil || sub.Task.ID != "cloud-task" {
			t.Fatal("late precise fact lost after revocation", err)
		}
		if _, err = h.w.CreateCloudTask(context.Background(), h.actor, h.f.project, h.request); err == nil || h.creates.Load() != 1 {
			t.Fatal("revocation bypassed by receipt")
		}
	})
}

func TestWorkBuddyCloudPoolUsesExactRealMachineScope(t *testing.T) {
	h := newCloudFixtureMode(t, nil, true)
	ctx := context.Background()
	forged := h.request
	forged.Assignment.PoolGrantVersion++
	if _, err := h.w.CreatePoolCloudTask(ctx, h.poolToken, forged); err == nil || h.creates.Load() != 0 {
		t.Fatal("forged pool grant version admitted")
	}
	stripped := h.request
	stripped.Assignment = repositoryTestRef(dag.Assignment{RunID: h.request.Assignment.RunID, NodeID: h.request.Assignment.NodeID, Attempt: h.request.Assignment.Attempt, Fence: h.request.Assignment.Fence, WorkerID: h.request.Assignment.WorkerID})
	if _, err := h.w.CreateCloudTask(ctx, h.actor, h.f.project, stripped); err == nil {
		t.Fatal("project token impersonated pool route")
	}
	sub, err := h.w.CreatePoolCloudTask(ctx, h.poolToken, h.request)
	if err != nil || sub.Task == nil || h.creates.Load() != 1 {
		t.Fatal("valid pool machine rejected", err)
	}
	if _, err = h.f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET version=version+1 WHERE id=?`, h.request.Assignment.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.w.RefreshPoolCloudTask(ctx, h.poolToken, sub.ID, wc.CloudRefreshRequest{ProjectID: h.f.project, ExpectedVersion: sub.Version, Assignment: h.request.Assignment, IdempotencyKey: newID()}); err == nil || h.queries.Load() != 0 {
		t.Fatal("changed pool grant allowed fresh external read")
	}
}

func TestWorkBuddyCloudSchemaRejectsPartialDomainWithoutRepair(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "cloud.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := &Service{DB: db, Dialect: "sqlite"}
	if _, err := db.Exec(`CREATE TABLE suma_workbuddy_cloud_delegations(id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeWorkBuddyCloud(context.Background()); err == nil {
		t.Fatal("partial domain accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='suma_workbuddy_cloud_submissions'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("schema rejection wrote another table", err)
	}
}
