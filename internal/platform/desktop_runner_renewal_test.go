package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

func desktopRenewalRequest(f *fixture, body map[string]any) *http.Request {
	r := desktopRunnerTestRequest(f, "context", body)
	r.URL.Path = "/api/v1/desktop-runners/renew"
	fields := []string{"suma-desktop-runner-v1", "renew"}
	for _, name := range []string{"project_id", "user_id", "tenant_id", "worker_id", "adapter_id", "instance_id", "expected_binding_sha256", "base_commit", "registration_operation_id", "expected_worker_version", "previous_credential_sha256", "credential_sha256", "idempotency_key"} {
		fields = append(fields, fmt.Sprint(body[name]))
	}
	fields = append(fields, r.Header.Get("X-Suma-Desktop-Nonce"), r.Header.Get("X-Suma-Desktop-Expires"))
	mac := hmac.New(sha256.New, f.s.DesktopWorkspaceKey)
	mac.Write([]byte(strings.Join(fields, "\x00")))
	r.Header.Set("X-Suma-Desktop-Signature", hex.EncodeToString(mac.Sum(nil)))
	return r
}

func desktopRenewalFixture(t *testing.T) (*fixture, map[string]any, string, string) {
	t.Helper()
	f, _ := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	oldToken, newToken := newID()+newID(), newID()+newID()
	body := desktopRunnerRegistration(f, local, oldToken)
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	body["registration_operation_id"] = body["idempotency_key"]
	body["idempotency_key"] = "desktop-runner-renew-" + newID()
	body["expected_worker_version"] = 1
	body["previous_credential_sha256"] = body["credential_sha256"]
	body["credential_sha256"] = digest(newToken)
	return f, body, oldToken, newToken
}

func desktopRenewalCopy(body map[string]any) map[string]any {
	copy := make(map[string]any, len(body))
	for key, value := range body {
		copy[key] = value
	}
	return copy
}

func TestDesktopRunnerRenewalKeepsIdentityAndOriginalReceipt(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprintf("expired_%v", expired), func(t *testing.T) {
			f, body, oldToken, newToken := desktopRenewalFixture(t)
			if expired {
				past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
				for _, query := range []string{`UPDATE suma_sessions SET expires_at=? WHERE token_hash=?`, `UPDATE suma_credential_slots SET expires_at=? WHERE current_digest=?`, `UPDATE suma_credential_grants SET expires_at=? WHERE id IN (SELECT grant_id FROM suma_credential_authority WHERE token_hash=?)`, `UPDATE suma_release_workers SET expires_at=? WHERE token_hash=?`} {
					if _, err := f.s.DB.Exec(query, past, digest(oldToken)); err != nil {
						t.Fatal(err)
					}
				}
			}
			out := desktopRunnerTestCall(t, f, desktopRenewalRequest(f, body), 200)
			worker := out["worker"].(map[string]any)
			if worker["worker_id"] != body["worker_id"] || worker["version"] != float64(2) || out["operation_id"] != body["idempotency_key"] || out["adapter_id"] != body["adapter_id"] || len(worker["profiles"].([]any)) != 0 || worker["deployment"] != false || strings.Contains(stringify(out), newToken) {
				t.Fatal("renewal changed identity/capabilities or exposed the secret")
			}
			_, err := f.s.AuthenticateActor(context.Background(), oldToken)
			requireAuthorityStatus(t, err, 401)
			actor, err := f.s.AuthenticateActor(context.Background(), newToken)
			if err != nil || actor.kind != actorProjectRunner || actor.credential.projectID != f.project {
				t.Fatalf("new digest is not a current original-project Runner: %v", err)
			}
			deadline, err := time.Parse(time.RFC3339Nano, worker["expires_at"].(string))
			if err != nil || time.Until(deadline) < 23*time.Hour || time.Until(deadline) > 25*time.Hour {
				t.Fatal("renewal did not grant the fixed 24-hour lifetime", err)
			}
			if _, err = f.s.DB.Exec(`UPDATE suma_release_workers SET version=version+1 WHERE worker_id=?`, body["worker_id"]); err != nil {
				t.Fatal(err)
			}
			replay := desktopRunnerTestCall(t, f, desktopRenewalRequest(f, body), 200)
			if stringify(out) != stringify(replay) || domainACount(t, f.s, "suma_credential_issuance") != 3 {
				t.Fatal("receipt recovery issued another secret/digest or used current CAS")
			}
			body["credential_sha256"] = digest(newID() + newID())
			desktopRunnerTestCall(t, f, desktopRenewalRequest(f, body), 409)
		})
	}
}

func TestDesktopRunnerRenewalRejectsChangedSourceAuthorityAndCAS(t *testing.T) {
	f, body, oldToken, _ := desktopRenewalFixture(t)
	t.Run("unsigned_new_digest", func(t *testing.T) {
		r := desktopRenewalRequest(f, body)
		changed := desktopRenewalCopy(body)
		changed["credential_sha256"] = digest(newID() + newID())
		r.Body = io.NopCloser(strings.NewReader(stringify(changed)))
		desktopRunnerTestCall(t, f, r, 403)
	})
	for _, check := range []struct {
		field  string
		value  any
		status int
	}{
		{"expected_worker_version", 2, 409}, {"previous_credential_sha256", strings.Repeat("0", 64), 409}, {"expected_binding_sha256", strings.Repeat("0", 64), 409}, {"adapter_id", "opencode-sdk", 409}, {"user_id", newID(), 403}, {"registration_operation_id", "desktop-runner-" + newID(), 403}, {"provider_id", "forbidden-input", 400},
	} {
		t.Run(check.field, func(t *testing.T) {
			changed := desktopRenewalCopy(body)
			changed[check.field] = check.value
			desktopRunnerTestCall(t, f, desktopRenewalRequest(f, changed), check.status)
		})
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_credential_authority SET revoked=1 WHERE token_hash=?`, digest(oldToken)); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRenewalRequest(f, body), 403)
	if _, err := f.s.DB.Exec(`UPDATE suma_credential_authority SET revoked=0 WHERE token_hash=?`, digest(oldToken)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRenewalRequest(f, body), 403)
	if domainACount(t, f.s, "suma_credential_issuance") != 2 {
		t.Fatal("rejected renewal persisted an issuance")
	}
}

func TestDesktopRunnerRenewalWaitsForOriginalExecutionAndEffectResources(t *testing.T) {
	f, body, _, _ := desktopRenewalFixture(t)
	ctx := context.Background()
	wf := dag.Workflow{ID: "renewal178", Name: "Renewal drain fixture", Nodes: []dag.Node{{ID: "work", Type: "command", Command: []string{"controlled"}, Idempotent: true}}}
	if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, wf); err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, wf.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET worker_id=?,state='reconciling',attempt=1,fence=1 WHERE run_id=? AND id='work'`, body["worker_id"], run.ID); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRenewalRequest(f, body), 409)
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET state='failed' WHERE run_id=? AND id='work'`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO dag_attempts(run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result) VALUES(?,'work',1,1,?,'failed','','','{}')`, run.ID, body["worker_id"]); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO dag_resource_ledger(run_id,node_id,attempt,fence,name,tenant_id,project_id,amount,release_class,state) VALUES(?,'work',1,1,'effect178',?,?,1,'effect','active')`, run.ID, f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRenewalRequest(f, body), 409)
	if _, err = f.s.DB.Exec(`UPDATE dag_resource_ledger SET state='released' WHERE run_id=?`, run.ID); err != nil {
		t.Fatal(err)
	}
	cloudID := newID()
	if _, err = f.s.DB.Exec(`INSERT INTO suma_workbuddy_cloud_submissions(id,tenant_id,project_id,delegation_id,run_id,node_id,version,state,assignment_sha256,worker_credential_hash,token_generation,prepared_nonce,payload) VALUES(?,?,?,?,?,'work',1,'unknown',?,?,1,?,'{}')`, cloudID, f.user.TenantID, f.project, newID(), run.ID, strings.Repeat("a", 64), body["previous_credential_sha256"], newID()); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRenewalRequest(f, body), 409)
	if _, err = f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_submissions SET state='task_known' WHERE id=?`, cloudID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET state='waiting' WHERE run_id=? AND id='work'`, run.ID); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRenewalRequest(f, body), 409)
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET state='failed' WHERE run_id=? AND id='work'`, run.ID); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRenewalRequest(f, body), 200)
}

func TestDesktopRunnerRenewalRollsBackNewGrantAndOldRevocation(t *testing.T) {
	f, body, oldToken, newToken := desktopRenewalFixture(t)
	if _, err := f.s.DB.Exec(`CREATE TRIGGER renewal178_activity_failure BEFORE INSERT ON suma_activity WHEN NEW.kind='desktop_runner.credential_renewed' BEGIN SELECT RAISE(ABORT,'renewal fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRenewalRequest(f, body), 500)
	if _, err := f.s.AuthenticateActor(context.Background(), oldToken); err != nil {
		t.Fatal("rollback revoked the old credential", err)
	}
	_, err := f.s.AuthenticateActor(context.Background(), newToken)
	requireAuthorityStatus(t, err, 401)
	if domainACount(t, f.s, "suma_credential_issuance") != 2 {
		t.Fatal("rollback persisted a new grant/issuance")
	}
}
