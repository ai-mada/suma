package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func manualRotationPath(id any) string {
	return "/release-workers/" + id.(string) + "/rotate-credential"
}

func TestManualRunnerRotationPreservesBindingAndRecoversOnlyMetadata(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	created := f.call("POST", "/release-workers", f.body(map[string]any{
		"worker_id": "manual-rotation", "deployment": true,
		"profiles": []ReleaseProfileRegistration{{ID: "build", Kind: "build", SHA256: strings.Repeat("a", 64)}},
	}), 201)
	oldSecret := created["token"].(string)
	oldDigest := digest(oldSecret)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	for _, query := range []string{
		`UPDATE suma_sessions SET expires_at=? WHERE token_hash=?`,
		`UPDATE suma_credential_slots SET expires_at=? WHERE current_digest=?`,
		`UPDATE suma_credential_grants SET expires_at=? WHERE id IN (SELECT grant_id FROM suma_credential_authority WHERE token_hash=?)`,
		`UPDATE suma_release_workers SET expires_at=? WHERE token_hash=?`,
	} {
		if _, err := f.s.DB.Exec(query, past, oldDigest); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.DB.Exec(`INSERT INTO suma_tool_inventories(tenant_id,project_id,worker_id,observed_at,items)VALUES(?,?,?,?,?)`, f.user.TenantID, f.project, created["worker_id"], now(), "[]"); err != nil {
		t.Fatal(err)
	}

	body := f.body(map[string]any{"expected_version": 1, "acknowledge_stopped": true})
	path := manualRotationPath(created["id"])
	result := f.call("POST", path, body, 200)
	newSecret, ok := result["token"].(string)
	if !ok || len(newSecret) != 64 || newSecret == oldSecret {
		t.Fatal("successful new operation did not reveal one fresh secret")
	}
	worker := result["worker"].(map[string]any)
	for _, field := range []string{"id", "project_id", "worker_id", "created_at", "profiles", "deployment", "status"} {
		if !reflect.DeepEqual(created[field], worker[field]) {
			t.Errorf("rotation changed %s", field)
		}
	}
	if result["operation_id"] != body["idempotency_key"] || worker["version"] != float64(2) {
		t.Fatal("rotation receipt did not identify the exact operation and version")
	}
	deadline, err := time.Parse(time.RFC3339Nano, worker["expires_at"].(string))
	if err != nil || time.Until(deadline) < 23*time.Hour || time.Until(deadline) > 25*time.Hour {
		t.Fatal("new credential did not receive the standard lifetime", err)
	}
	actor, err := f.s.AuthenticateActor(ctx, newSecret)
	if err != nil || actor.kind != actorProjectRunner || actor.credential.projectID != f.project || actor.credential.subjectID != f.user.ID {
		t.Fatal("new credential changed principal or scope", err)
	}
	_, err = f.s.AuthenticateActor(ctx, oldSecret)
	requireAuthorityStatus(t, err, 401)
	var revoked, inventory int
	if err = f.s.DB.QueryRow(`SELECT revoked FROM suma_credential_authority WHERE token_hash=?`, oldDigest).Scan(&revoked); err != nil || revoked != 1 {
		t.Fatal("old authority was not explicitly revoked", err)
	}
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, worker["worker_id"]).Scan(&inventory); err != nil || inventory != 0 {
		t.Fatal("old observation remained current", err)
	}

	req := httptest.NewRequest("POST", "http://suma.test/api/v1"+path, strings.NewReader(stringify(body)))
	req.AddCookie(f.cookie)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Suma-Expected-User", f.user.ID)
	req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
	reply := httptest.NewRecorder()
	f.handler.ServeHTTP(reply, req)
	if reply.Code != 200 || reply.Header().Get("Cache-Control") != "no-store" || strings.Contains(reply.Body.String(), newSecret) {
		t.Fatal("replay was not a no-store metadata receipt", reply.Code)
	}
	var replay map[string]any
	if json.Unmarshal(reply.Body.Bytes(), &replay) != nil {
		t.Fatal("replay did not return JSON")
	}
	delete(result, "token")
	if !reflect.DeepEqual(result, replay) || domainACount(t, f.s, "suma_credential_issuance") != 3 {
		t.Fatal("replay created a new issuance or returned a different receipt")
	}
	body["expected_version"] = 2
	f.call("POST", path, body, 409)
	f.call("POST", path, f.body(map[string]any{"expected_version": 1, "acknowledge_stopped": true}), 409)
	list := f.call("GET", "/release-workers?project_id="+f.project, nil, 200)
	listed := list["items"].([]any)[0].(map[string]any)
	if listed["manual_rotation_allowed"] != true || strings.Contains(stringify(list), newSecret) {
		t.Fatal("current manual node action was missing or list exposed the secret")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	body["expected_version"] = 1
	f.call("POST", path, body, 403)
	f.call("GET", "/release-workers?project_id="+f.project, nil, 403)
}

func TestManualRunnerRotationRejectsManagedCredentialsAndInvalidRequests(t *testing.T) {
	t.Run("another project administrator", func(t *testing.T) {
		f := newFixture(t)
		worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "manual-owned", "profiles": []any{}}), 201)
		invite := governanceInvite(f, "other-admin@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "admin"}})
		governanceAccept(f, invite, "other-admin@example.test")
		list := f.call("GET", "/release-workers?project_id="+f.project, nil, 200)
		if list["items"].([]any)[0].(map[string]any)["manual_rotation_allowed"] != nil {
			t.Fatal("unrelated administrator saw original holder's rotation action")
		}
		f.call("POST", manualRotationPath(worker["id"]), f.body(map[string]any{"expected_version": 1, "acknowledge_stopped": true}), 403)
	})
	t.Run("remote pairing", func(t *testing.T) {
		f := pairingFixture(t)
		pair := pairingCreate(t, f)
		proposal := pairingPropose(t, f, pair, strings.Repeat("a", 64), newID(), 200)
		pairingApprove(t, f, proposal)
		list := f.call("GET", "/release-workers?project_id="+f.project, nil, 200)
		worker := list["items"].([]any)[0].(map[string]any)
		if worker["manual_rotation_allowed"] != nil {
			t.Fatal("remote paired identity exposed a manual rotation action")
		}
		f.call("POST", manualRotationPath(worker["id"]), f.body(map[string]any{"expected_version": 1, "acknowledge_stopped": true}), 403)
	})
	t.Run("desktop registration", func(t *testing.T) {
		f, _, _, _ := desktopRenewalFixture(t)
		list := f.call("GET", "/release-workers?project_id="+f.project, nil, 200)
		worker := list["items"].([]any)[0].(map[string]any)
		if worker["manual_rotation_allowed"] != nil {
			t.Fatal("desktop managed identity exposed a manual rotation action")
		}
		f.call("POST", manualRotationPath(worker["id"]), f.body(map[string]any{"expected_version": 1, "acknowledge_stopped": true}), 403)
	})
	t.Run("closed request and revoked credential", func(t *testing.T) {
		f := newFixture(t)
		created := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "manual-invalid", "profiles": []any{}}), 201)
		path := manualRotationPath(created["id"])
		for _, req := range []map[string]any{
			{"expected_version": 1, "acknowledge_stopped": false},
			{"expected_version": 1},
			{"expected_version": 1, "acknowledge_stopped": nil},
			{"expected_version": 2147483647, "acknowledge_stopped": true},
			{"expected_version": json.Number("1.0"), "acknowledge_stopped": true},
			{"expected_version": json.Number("1.00000000000000000001"), "acknowledge_stopped": true},
			{"expected_version": 1, "acknowledge_stopped": true, "worker_id": "invented"},
		} {
			f.call("POST", path, f.body(req), 400)
		}
		body := f.body(map[string]any{"expected_version": 1, "acknowledge_stopped": true})
		f.call("POST", path+"/", body, 404)
		f.call("POST", path+"?project_id="+f.project, body, 400)
		f.bearer("POST", path, created["token"].(string), body, 403)
		f.call("PATCH", "/release-workers/"+created["id"].(string), f.body(map[string]any{"expected_version": 1, "status": "revoked"}), 200)
		body["expected_version"] = 2
		f.call("POST", path, body, 409)
		list := f.call("GET", "/release-workers?project_id="+f.project, nil, 200)
		if list["items"].([]any)[0].(map[string]any)["manual_rotation_allowed"] != nil || domainACount(t, f.s, "suma_credential_issuance") != 2 {
			t.Fatal("denied rotations exposed an action or issued a credential")
		}
	})
}

func TestManualRunnerRotationWaitsForPendingEffectsAndRollsBack(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	worker := "drain-before-rotation"
	created := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker}), 201)
	oldSecret := created["token"].(string)
	oldDigest := digest(oldSecret)
	body := f.body(map[string]any{"expected_version": 1, "acknowledge_stopped": true})
	path := manualRotationPath(created["id"])
	waiting := func() {
		t.Helper()
		result := f.call("POST", path, body, 409)
		if result["error"].(map[string]any)["code"] != "worker_rotation_pending" {
			t.Fatal("expected a recoverable pending rotation decision")
		}
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.s.DB.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	workflow := dag.Workflow{ID: "drain-rotation-fixture", Name: "Rotation drain fixture", Nodes: []dag.Node{{ID: "command", Type: "command", Command: []string{"fixture"}, Idempotent: true}}}
	if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, workflow); err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, workflow.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE dag_nodes SET worker_id=?,state='reconciling',attempt=1,fence=1 WHERE run_id=? AND id='command'`, worker, run.ID)
	waiting()
	exec(`UPDATE dag_nodes SET state='failed' WHERE run_id=? AND id='command'`, run.ID)
	// Claimed or uncertain tool operations block rotation, unclaimed work remains eligible.
	{
		preparation, inspection, scan := newID(), newID(), newID()
		exec(`INSERT INTO suma_tool_preparations(id,tenant_id,project_id,worker_id,state,version,created_at,payload)VALUES(?,?,?,?,'awaiting_inventory',1,?,'{}')`, preparation, f.user.TenantID, f.project, worker, now())
		waiting()
		exec(`UPDATE suma_tool_preparations SET state='pending' WHERE id=?`, preparation)
		exec(`INSERT INTO suma_tool_inspections(id,tenant_id,project_id,worker_id,profile_id,state,version,created_at,payload)VALUES(?,?,?,?,?,'unknown',1,?,'{}')`, inspection, f.user.TenantID, f.project, worker, newID(), now())
		waiting()
		exec(`UPDATE suma_tool_inspections SET state='pending' WHERE id=?`, inspection)
		exec(`INSERT INTO suma_tool_installation_scans(id,tenant_id,project_id,worker_id,state,version,created_at,payload)VALUES(?,?,?,?,'claimed',1,?,'{}')`, scan, f.user.TenantID, f.project, worker, now())
		waiting()
		exec(`UPDATE suma_tool_installation_scans SET state='pending' WHERE id=?`, scan)
	}
	{
		op, requestSHA := newID(), strings.Repeat("a", 64)
		exec(`INSERT INTO suma_release_authorizations(id,tenant_id,project_id,operation_id,ticket_hash,expires_at,state,assignment,request_sha256,effect_grant_id,grant_data,actor_id,worker_token_hash)VALUES(?,?,?,?,?,?,'consumed','{}',?,?,'{}',?,?)`,
			newID(), f.user.TenantID, f.project, op, digest(newID()), "2000-01-01T00:00:00Z", requestSHA, newID(), f.user.ID, oldDigest)
		waiting()
		deployment := ReleaseDeployment{OperationID: op, RequestSHA256: requestSHA, Generation: 1, Assignment: &rc.AssignmentRef{WorkerID: worker}, Observation: &rc.DeploymentObservation{OperationID: op, RequestSHA256: requestSHA, Generation: 1, FenceGeneration: 1, State: "failed", Effect: rc.DeploymentEffect{State: "not_started"}}}
		exec(`INSERT INTO suma_release_deployments(operation_id,tenant_id,project_id,version,run_id,node_id,approved_by,executed_by,data)VALUES(?,?,?,1,?,?,?,?,?)`,
			op, f.user.TenantID, f.project, run.ID, "command", f.user.ID, f.user.ID, stringify(deployment))
	}
	{
		op, requestSHA := newID(), strings.Repeat("b", 64)
		grant := newID()
		exec(`INSERT INTO suma_repository_authorizations(id,tenant_id,project_id,operation_id,ticket_hash,expires_at,state,assignment,request_sha256,grant_data,effect_grant_id,worker_token_hash)VALUES(?,?,?,?,?,?,'consumed','{}',?,'{}',?,?)`,
			newID(), f.user.TenantID, f.project, op, digest(newID()), "2000-01-01T00:00:00Z", requestSHA, grant, oldDigest)
		waiting()
		execution := ri.RepositoryExecution{OperationID: op, Generation: 1}
		record := repositoryOperation{Execution: execution, RequestSHA256: requestSHA, WorkerTokenHash: oldDigest, Assignment: &cc.CandidateAssignmentRef{WorkerID: worker}, Observation: &ri.RepositoryObservation{OperationID: op, RequestSHA256: requestSHA, Execution: execution, Phase: "apply", State: "applied", EffectGrantID: grant, Receipt: &ri.RepositoryReceipt{}}}
		exec(`INSERT INTO suma_repository_operations(operation_id,tenant_id,project_id,plan_id,run_id,node_id,version,data)VALUES(?,?,?,?,?,?,1,?)`,
			op, f.user.TenantID, f.project, newID(), run.ID, "command", stringify(record))
	}

	exec(`INSERT INTO suma_tool_inventories(tenant_id,project_id,worker_id,observed_at,items)VALUES(?,?,?,?,?)`, f.user.TenantID, f.project, worker, now(), "[]")
	exec(`CREATE TRIGGER manual_rotation_activity_failure BEFORE INSERT ON suma_activity WHEN NEW.kind='release_worker.credential_rotated' BEGIN SELECT RAISE(ABORT,'fixture failure'); END`)
	f.call("POST", path, body, 500)
	if _, err = f.s.AuthenticateActor(ctx, oldSecret); err != nil {
		t.Fatal("failed transaction revoked the old secret", err)
	}
	var registryVersion int
	if err = f.s.DB.QueryRow(`SELECT version FROM suma_release_workers WHERE id=?`, created["id"]).Scan(&registryVersion); err != nil || registryVersion != 1 {
		t.Fatal("failed transaction changed the registry", err)
	}
	if domainACount(t, f.s, "suma_credential_issuance") != 2 || domainACount(t, f.s, "suma_tool_inventories") != 1 {
		t.Fatal("failed transaction changed issuances or current observation")
	}
	exec(`DROP TRIGGER manual_rotation_activity_failure`)
	rotated := f.call("POST", path, body, 200)["worker"].(map[string]any)
	if profiles, ok := rotated["profiles"].([]any); !ok || len(profiles) != 0 {
		t.Fatal("rotation returned an invalid empty capability projection")
	}
}
