package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
)

const pairingTestControl = "https://control.example.test/exact/path"

func pairingFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.handler = (&Server{Service: f.s, Engine: f.e, PublicURL: pairingTestControl}).Handler()
	return f
}

func pairingCreate(t *testing.T, f *fixture) map[string]any {
	t.Helper()
	return f.call("POST", "/runner-pairings", f.body(map[string]any{}), 201)
}

func pairingPropose(t *testing.T, f *fixture, p map[string]any, token, key string, want int) map[string]any {
	t.Helper()
	return f.bearer("POST", "/runner-pairings/"+p["id"].(string)+"/proposal", token, map[string]any{"project_id": f.project, "device_name": "Reviewed remote machine", "credential_sha256": digest(token), "idempotency_key": key}, want)
}

func pairingStatus(t *testing.T, f *fixture, p map[string]any, token, key string, want int) map[string]any {
	t.Helper()
	return f.bearer("POST", "/runner-pairings/"+p["id"].(string)+"/status", token, map[string]any{"project_id": f.project, "idempotency_key": key}, want)
}

func pairingApprove(t *testing.T, f *fixture, p map[string]any) map[string]any {
	t.Helper()
	return f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", f.body(map[string]any{"expected_version": p["version"], "action": "approve"}), 200)
}

func TestRunnerPairingOriginalDigestApprovalAndIndependentCurrentIdentity(t *testing.T) {
	f := pairingFixture(t)
	create := f.body(map[string]any{})
	p := f.call("POST", "/runner-pairings", create, 201)
	if p["state"] != "waiting" || p["version"] != float64(1) || p["control_url"] != pairingTestControl || p["can_approve"] != false {
		t.Fatal("creation claimed approval or changed operator destination", p)
	}
	token, operation := strings.Repeat("a", 64), newID()
	proposal := pairingPropose(t, f, p, token, operation, 200)
	if proposal["state"] != "proposed" || proposal["version"] != float64(2) || proposal["can_approve"] != false || domainACount(t, f.s, "suma_release_workers") != 0 || domainACount(t, f.s, "suma_credential_grants") != 0 {
		t.Fatal("a machine proposal issued execution authority", proposal)
	}
	if _, err := f.s.AuthenticateActor(context.Background(), token); err == nil {
		t.Fatal("unapproved local secret became a Runner")
	}
	view := f.call("GET", "/runner-pairings/"+p["id"].(string)+queryProject(f.project), nil, 200)
	if view["can_approve"] != true || view["credential_sha256"] != digest(token) {
		t.Fatal("browser did not receive the exact digest for explicit review", view)
	}
	approve := f.body(map[string]any{"expected_version": 2, "action": "approve"})
	approved := f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", approve, 200)
	worker := approved["worker_id"].(string)
	if approved["state"] != "approved" || approved["version"] != float64(3) || !strings.HasPrefix(worker, "remote-") || approved["can_approve"] != false || strings.Contains(stringify(approved), token) || strings.Contains(stringify(approved), `"token"`) {
		t.Fatal("approval lost identity or exposed the secret", approved)
	}
	actor, err := f.s.AuthenticateActor(context.Background(), token)
	if err != nil || actor.kind != actorProjectRunner || actor.credential.projectID != f.project {
		t.Fatal("approved digest did not enter original credential authority", err)
	}
	var profiles string
	var deployment int
	if err = f.s.DB.QueryRow(`SELECT profiles,deployment FROM suma_release_workers WHERE worker_id=?`, worker).Scan(&profiles, &deployment); err != nil || profiles != "[]" || deployment != 0 {
		t.Fatal("registration granted release or deployment capabilities", profiles, deployment, err)
	}
	var count int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations WHERE response LIKE ?`, "%"+token+"%").Scan(&count); err != nil || count != 0 {
		t.Fatal("raw machine secret entered receipts", count, err)
	}
	if got := pairingStatus(t, f, p, token, operation, 200); got["worker_id"] != worker || got["can_approve"] != false {
		t.Fatal("machine did not recover its exact original identity", got)
	}
	if got := pairingPropose(t, f, p, token, operation, 200); got["worker_id"] != worker {
		t.Fatal("lost proposal ACK changed its identity", got)
	}
	if got := f.call("POST", "/runner-pairings", create, 201); stringify(got) != stringify(p) {
		t.Fatal("lost create ACK did not recover its original waiting snapshot", got)
	}
	if got := f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", approve, 200); got["worker_id"] != worker || domainACount(t, f.s, "suma_release_workers") != 1 || domainACount(t, f.s, "suma_credential_grants") != 1 {
		t.Fatal("lost approval ACK issued another credential", got)
	}
	// A short pairing TTL does not revoke an already approved independent Grant.
	if _, err = f.s.DB.Exec(`UPDATE suma_runner_pairings SET expires_at='2000-01-01T00:00:00Z',payload=json_set(payload,'$.expires_at','2000-01-01T00:00:00Z') WHERE id=?`, p["id"]); err != nil {
		t.Fatal(err)
	}
	pairingStatus(t, f, p, token, operation, 200)
	if _, err = f.s.DB.Exec(`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, digest(token)); err != nil {
		t.Fatal(err)
	}
	pairingStatus(t, f, p, token, operation, 401)
	if _, err = f.s.DB.Exec(`UPDATE suma_sessions SET expires_at=? WHERE token_hash=?`, approved["credential_expires_at"], digest(token)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_release_workers SET status='revoked',version=version+1 WHERE worker_id=?`, worker); err != nil {
		t.Fatal(err)
	}
	pairingStatus(t, f, p, token, operation, 403)
	if domainACount(t, f.s, "dag_runs") != 0 || domainACount(t, f.s, "suma_tool_profiles") != 0 {
		t.Fatal("pairing started a task or configured a tool")
	}
}

func queryProject(project string) string { return "?project_id=" + project }

func TestRunnerPairingProposalIsExactAndApprovalUsesOriginalCAS(t *testing.T) {
	f, token, op := pairingFixture(t), strings.Repeat("b", 64), newID()
	p := pairingCreate(t, f)
	pairingPropose(t, f, p, token, op, 200)
	pairingPropose(t, f, p, strings.Repeat("c", 64), newID(), 409)
	f.bearer("POST", "/runner-pairings/"+p["id"].(string)+"/proposal", token, map[string]any{"project_id": f.project, "device_name": "Changed machine", "credential_sha256": digest(token), "idempotency_key": op}, 409)
	pairingStatus(t, f, p, token, newID(), 403)
	pairingStatus(t, f, p, strings.Repeat("c", 64), op, 403)
	f.bearer("POST", "/runner-pairings/"+p["id"].(string)+"/status", token, map[string]any{"project_id": newID(), "idempotency_key": op}, 403)
	f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", f.body(map[string]any{"expected_version": 1, "action": "approve"}), 409)
	approve := f.body(map[string]any{"expected_version": 2, "action": "approve"})
	f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", approve, 200)
	approve["action"] = "reject"
	f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", approve, 409)
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='member' WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	approve["action"] = "approve"
	f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", approve, 403)
	if domainACount(t, f.s, "suma_release_workers") != 1 {
		t.Fatal("stale authority changed the original registered identity")
	}
}

func TestRunnerPairingExpiredRejectedAndChangedControlCannotIssue(t *testing.T) {
	for _, mode := range []string{"expired", "rejected", "changed-control"} {
		t.Run(mode, func(t *testing.T) {
			f, token, op := pairingFixture(t), strings.Repeat("d", 64), newID()
			p := pairingCreate(t, f)
			pairingPropose(t, f, p, token, op, 200)
			switch mode {
			case "expired":
				if _, err := f.s.DB.Exec(`UPDATE suma_runner_pairings SET expires_at='2000-01-01T00:00:00Z',payload=json_set(payload,'$.expires_at','2000-01-01T00:00:00Z') WHERE id=?`, p["id"]); err != nil {
					t.Fatal(err)
				}
			case "rejected":
				f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", f.body(map[string]any{"expected_version": 2, "action": "reject"}), 200)
			case "changed-control":
				f.handler = (&Server{Service: f.s, Engine: f.e, PublicURL: pairingTestControl + "/different"}).Handler()
			}
			pairingPropose(t, f, p, token, op, 409)
			pairingStatus(t, f, p, token, op, 409)
			f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", f.body(map[string]any{"expected_version": 2, "action": "approve"}), 409)
			if domainACount(t, f.s, "suma_release_workers") != 0 || domainACount(t, f.s, "suma_credential_grants") != 0 {
				t.Fatal("invalid pairing created a credential")
			}
		})
	}
}

func TestRunnerPairingProjectCapAndSafeOperatorURL(t *testing.T) {
	f := pairingFixture(t)
	for i := 0; i < 7; i++ {
		pairingCreate(t, f)
	}
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		body, _ := json.Marshal(f.body(map[string]any{}))
		go func() {
			req := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/runner-pairings", bytes.NewReader(body))
			req.AddCookie(f.cookie)
			req.Header.Set("X-Suma-Expected-User", f.user.ID)
			req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, req)
			results <- w.Code
		}()
	}
	first, second := <-results, <-results
	if !((first == 201 && second == 409) || (first == 409 && second == 201)) || domainACount(t, f.s, "suma_runner_pairings") != 8 {
		t.Fatalf("concurrent create exceeded project cap: %d/%d", first, second)
	}
	f.call("POST", "/runner-pairings", f.body(map[string]any{}), 409)
	if _, err := f.s.DB.Exec(`UPDATE suma_runner_pairings SET expires_at='2000-01-01T00:00:00Z',payload=json_set(payload,'$.expires_at','2000-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	pairingCreate(t, f)
	for _, control := range []string{"", "http://remote.example.test", "https://user:secret@example.test", "https://control.example.test/?secret=x", "https://control.example.test/#fragment"} {
		f.handler = (&Server{Service: f.s, Engine: f.e, PublicURL: control}).Handler()
		f.call("POST", "/runner-pairings", f.body(map[string]any{}), 503)
	}
}

func TestRunnerPairingHTTPRequiresBrowserViewAndStrictMachineBearer(t *testing.T) {
	f := pairingFixture(t)
	api, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner-pairings", api.Token, f.body(map[string]any{}), 403)
	p := pairingCreate(t, f)
	path := "/api/v1/runner-pairings/" + p["id"].(string)
	for _, test := range []struct {
		suffix, raw, authorization string
		cookie                     bool
		want                       int
	}{
		{"/proposal", `{"project_id":"` + f.project + `","device_name":"Machine","credential_sha256":"` + digest(strings.Repeat("e", 64)) + `","idempotency_key":"original-proposal184"}`, "", true, 401},
		{"/proposal", `{"project_id":"` + f.project + `","project_id":"` + f.project + `"}`, "Bearer " + strings.Repeat("e", 64), false, 400},
		{"/proposal", `{"project_id":"` + f.project + `","device_name":"Machine","credential_sha256":"` + digest(strings.Repeat("e", 64)) + `","idempotency_key":"original-proposal184","token":"injected"}`, "Bearer " + strings.Repeat("e", 64), false, 400},
		{"/commands", `{"project_id":"` + f.project + `","expected_version":1,"action":"approve","idempotency_key":"original-approve184"}`, "", true, 428},
	} {
		req := httptest.NewRequest("POST", path+test.suffix, bytes.NewBufferString(test.raw))
		if test.cookie {
			req.AddCookie(f.cookie)
		}
		if test.authorization != "" {
			req.Header.Set("Authorization", test.authorization)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != test.want {
			t.Fatalf("%s status=%d want=%d: %s", test.suffix, w.Code, test.want, w.Body.String())
		}
	}
	if BrowserCommandRequiresView(http.MethodPost, "runner-pairings/"+p["id"].(string)+"/status") || !BrowserCommandRequiresView(http.MethodPost, "runner-pairings/"+p["id"].(string)+"/proposal") {
		t.Fatal("status was classified as a write or proposal as a read")
	}
}

func TestRunnerPairingApprovalRollsBackItsGrantAndOriginalPendingState(t *testing.T) {
	f, token, op := pairingFixture(t), strings.Repeat("f", 64), newID()
	p := pairingCreate(t, f)
	pairingPropose(t, f, p, token, op, 200)
	if _, err := f.s.DB.Exec(`CREATE TRIGGER pairing_activity_failure BEFORE INSERT ON suma_activity WHEN NEW.kind='runner_pairing.approved' BEGIN SELECT RAISE(ABORT,'controlled approval failure'); END`); err != nil {
		t.Fatal(err)
	}
	body := f.body(map[string]any{"expected_version": 2, "action": "approve"})
	f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", body, 500)
	if domainACount(t, f.s, "suma_release_workers") != 0 || domainACount(t, f.s, "suma_credential_grants") != 0 {
		t.Fatal("failed approval left partial authority")
	}
	got := pairingStatus(t, f, p, token, op, 200)
	if got["state"] != "proposed" || got["version"] != float64(2) {
		t.Fatal("failed approval consumed original pending state", got)
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER pairing_activity_failure`); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", body, 200)
}

func TestRunnerPairingInvalidMachineProofRejectsWhileSQLiteWriterIsHeld(t *testing.T) {
	f, token, operation := pairingFixture(t), strings.Repeat("a", 64), newID()
	p := pairingCreate(t, f)
	pairingPropose(t, f, p, token, operation, 200)
	other, err := store.Open("sqlite", authorityDatabasePath(t, f.s))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	holder, err := other.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback()
	if _, err = holder.Exec(`UPDATE suma_authority_sqlite_guard SET sequence=sequence+1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, action, locator, bearer, key string
		want                               int
	}{
		{"unknown-proposal", "proposal", newID(), token, operation, 404},
		{"unknown-status", "status", newID(), token, operation, 403},
		{"wrong-token-proposal", "proposal", p["id"].(string), strings.Repeat("b", 64), operation, 409},
		{"wrong-token-status", "status", p["id"].(string), strings.Repeat("b", 64), operation, 403},
		{"wrong-key-proposal", "proposal", p["id"].(string), token, newID(), 409},
		{"wrong-key-status", "status", p["id"].(string), token, newID(), 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := map[string]any{"project_id": f.project, "idempotency_key": test.key}
			if test.action == "proposal" {
				body["device_name"] = "Reviewed remote machine"
				body["credential_sha256"] = digest(test.bearer)
			}
			raw, _ := json.Marshal(body)
			// A bounded request context exposes a wait-for-writer as an ordinary
			// failure; the assertion is the exact public rejection while the
			// independent transaction stays locked, not an elapsed-time margin.
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()
			req := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/runner-pairings/"+test.locator+"/"+test.action, bytes.NewReader(raw)).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer "+test.bearer)
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, req)
			if w.Code != test.want {
				t.Fatalf("invalid proof should reject without writer: status=%d want=%d body=%s", w.Code, test.want, w.Body.String())
			}
			var held int
			if err := holder.QueryRow(`SELECT sequence FROM suma_authority_sqlite_guard WHERE id=1`).Scan(&held); err != nil {
				t.Fatal("writer holder ended before rejection", err)
			}
		})
	}
}
