package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func cloudHistoryAdditionalSubmission(t *testing.T, h *cloudFixture) wc.CloudSubmission {
	t.Helper()
	ctx := context.Background()
	node, err := h.w.GetCloudNode(ctx, h.f.actor, h.f.project, h.delegation.RunID, h.delegation.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	wf, err := h.f.e.Publish(ctx, h.f.user.TenantID, h.f.project, dag.Workflow{Name: "History quota", Nodes: []dag.Node{{ID: "cloud", Type: wc.NodeType, Config: map[string]any{"plugin": wc.Plugin, "workflow": wc.Protocol, "workbuddy_cloud": node.Binding}}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := h.f.e.CreateRun(ctx, h.f.user.TenantID, h.f.project, wf.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	var specJSON string
	var runVersion int
	if err = h.f.s.DB.QueryRow(`SELECT n.spec,r.version FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE n.run_id=? AND n.id='cloud'`, run.ID).Scan(&specJSON, &runVersion); err != nil {
		t.Fatal(err)
	}
	var spec dag.Node
	if err = json.Unmarshal([]byte(specJSON), &spec); err != nil {
		t.Fatal(err)
	}
	connection, err := h.w.Get(ctx, h.f.actor, h.f.project, h.delegation.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	delegation, err := h.w.DelegateCloud(ctx, h.f.actor, h.f.project, wc.CloudDelegateRequest{ProjectID: h.f.project, ConnectionID: connection.ID, ExpectedConnectionVersion: connection.Version, GrantGeneration: connection.GrantGeneration, RunID: run.ID, NodeID: "cloud", ExpectedRunVersion: runVersion, NodeSpecSHA256: digest(stringify(spec)), RequestSHA256: node.RequestSHA256, ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), MaxCreates: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	cloud := h.f.s.newCloudClaimAuthority(h.w)
	issued, err := h.f.s.IssueProjectCredential(ctx, h.f.actor, ProjectCredentialIssueRequest{ProjectID: h.f.project, Scope: "runner", Label: "History quota worker", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	machine := domainCActor(t, h.f.s, issued.Token)
	worker := "history-worker-" + newID()
	authority := h.f.s.engineAuthority(ctx, machine, commandTarget{tenantID: h.f.user.TenantID, projectID: h.f.project}, purposeRunnerWrite)
	operation := newID()
	caps := []string{"external_workflow:workbuddy-cloud"}
	assignment, err := h.f.e.ClaimWithOperation(authority.ctx, h.f.user.TenantID, h.f.project, worker, operation, caps, func(tx *sql.Tx) error { return cloud.beforePrivate(authority, tx, worker, operation, caps) }, func(tx *sql.Tx, runID, nodeID string) (bool, error) {
		_, allowed, e := cloud.eligibleTx(authority.ctx, tx, h.f.user.TenantID, h.f.project, runID, nodeID)
		return allowed, e
	}, cloud.runnableHook())
	authority.close()
	if err != nil || assignment == nil || assignment.RunID != run.ID {
		t.Fatal("new exact cloud assignment missing", err)
	}
	sub, err := h.w.CreateCloudTask(ctx, machine, h.f.project, wc.CloudCreateRequest{ProjectID: h.f.project, DelegationID: delegation.ID, ExpectedVersion: delegation.Version, Assignment: repositoryTestRef(*assignment), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func TestWorkBuddyCloudCollaborationProjectQuotaAcrossServiceInstances(t *testing.T) {
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	released := false
	h, first, posts := cloudHistoryFixture(t, nil, func() { started <- struct{}{}; <-release })
	defer func() {
		if !released {
			close(release)
		}
	}()
	// The scheduler defaults to one running execution. Explicitly provision the
	// five real source attempts before testing the independent history quota.
	tx, err := h.f.s.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := h.f.e.ReadSchedulingPolicyTx(context.Background(), tx, h.f.user.TenantID, h.f.project)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if _, err = h.f.e.WriteSchedulingPolicyTx(context.Background(), tx, h.f.user.TenantID, h.f.project, policy.Version, dag.SchedulingPolicyInput{ConcurrencyLimit: 5}); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	submissions := []wc.CloudSubmission{first}
	for len(submissions) < 5 {
		submissions = append(submissions, cloudHistoryAdditionalSubmission(t, h))
	}
	// A separate service owner shares only the real database and app binding.
	var dbSequence int
	var dbName, dbPath string
	if err = h.f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&dbSequence, &dbName, &dbPath); err != nil {
		t.Fatal(err)
	}
	secondDB, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { secondDB.Close() })
	secondService := &Service{DB: secondDB, Dialect: h.f.s.Dialect, DataDir: h.f.s.DataDir}
	secondConnections := *h.w
	secondConnections.s = secondService
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		sub := submissions[i]
		connections := h.w
		if i%2 == 1 {
			connections = &secondConnections
		}
		go func() {
			_, err := connections.LoadCloudHistory(context.Background(), h.f.actor, h.f.project, sub.ID, cloudHistoryRequest(h, sub))
			results <- err
		}()
	}
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(8 * time.Second):
			t.Fatal("four valid loads failed to start")
		}
	}
	if _, err := secondConnections.LoadCloudHistory(context.Background(), h.f.actor, h.f.project, submissions[4].ID, cloudHistoryRequest(h, submissions[4])); err == nil {
		t.Fatal("fifth project load bypassed quota")
	}
	if posts.Load() != 8 {
		t.Fatal("quota denial made external calls", posts.Load())
	}
	close(release)
	released = true
	for i := 0; i < 4; i++ {
		if err := <-results; err != nil {
			t.Fatal("valid loading failed", err)
		}
	}
}
