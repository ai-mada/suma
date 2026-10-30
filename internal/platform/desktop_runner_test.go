package platform

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func desktopRunnerFixture(t *testing.T) (*fixture, string) {
	t.Helper()
	f, root := desktopWorkspaceFixture(t)
	workspaceGitTestCommand(t, root, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, root, ".gitignore", ".cache/\n")
	workspaceGitTestCommand(t, root, "add", "--", "main.go", ".gitignore")
	workspaceGitTestCommand(t, root, "commit", "-m", "Desktop runner fixture")
	if _, err := f.s.BindDesktopWorkspace(context.Background(), f.actor, f.project, desktopWorkspaceTestRequest(f, root, "")); err != nil {
		t.Fatal(err)
	}
	return f, root
}

func desktopRunnerContextBody(f *fixture) map[string]any {
	return map[string]any{"project_id": f.project, "user_id": f.user.ID, "tenant_id": f.user.TenantID}
}

// Independent transport signer fixes the cross-language field order.
func desktopRunnerTestRequest(f *fixture, action string, body map[string]any) *http.Request {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4310/api/v1/desktop-runners/"+action, bytes.NewReader(raw))
	r.RemoteAddr = "127.0.0.1:33001"
	r.AddCookie(f.cookie)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Suma-Expected-User", f.user.ID)
	r.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
	nonce, expires := newID(), time.Now().UTC().Add(60*time.Second).Format(time.RFC3339Nano)
	fields := []string{"suma-desktop-runner-v1", action}
	names := []string{"project_id", "user_id", "tenant_id"}
	if action == "register" {
		names = append(names, "worker_id", "adapter_id", "instance_id", "expected_binding_sha256", "base_commit", "credential_sha256", "idempotency_key")
	}
	for _, name := range names {
		value, _ := body[name].(string)
		fields = append(fields, value)
	}
	fields = append(fields, nonce, expires)
	mac := hmac.New(sha256.New, f.s.DesktopWorkspaceKey)
	mac.Write([]byte(strings.Join(fields, "\x00")))
	r.Header.Set("X-Suma-Desktop-Nonce", nonce)
	r.Header.Set("X-Suma-Desktop-Expires", expires)
	r.Header.Set("X-Suma-Desktop-Signature", hex.EncodeToString(mac.Sum(nil)))
	return r
}

func desktopRunnerResign(f *fixture, r *http.Request, action string, body map[string]any) {
	fields := []string{"suma-desktop-runner-v1", action}
	names := []string{"project_id", "user_id", "tenant_id"}
	if action == "register" {
		names = append(names, "worker_id", "adapter_id", "instance_id", "expected_binding_sha256", "base_commit", "credential_sha256", "idempotency_key")
	}
	for _, name := range names {
		value, _ := body[name].(string)
		fields = append(fields, value)
	}
	fields = append(fields, r.Header.Get("X-Suma-Desktop-Nonce"), r.Header.Get("X-Suma-Desktop-Expires"))
	mac := hmac.New(sha256.New, f.s.DesktopWorkspaceKey)
	mac.Write([]byte(strings.Join(fields, "\x00")))
	r.Header.Set("X-Suma-Desktop-Signature", hex.EncodeToString(mac.Sum(nil)))
}

func desktopRunnerTestCall(t *testing.T, f *fixture, r *http.Request, want int) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("desktop runner request => %d, want %d: %s", w.Code, want, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func desktopRunnerRegistration(f *fixture, local map[string]any, token string) map[string]any {
	return map[string]any{"project_id": f.project, "user_id": f.user.ID, "tenant_id": f.user.TenantID, "worker_id": "desktop-" + newID(), "adapter_id": "codex-jsonl", "instance_id": local["instance_id"], "expected_binding_sha256": local["binding_sha256"], "base_commit": local["base_commit"], "credential_sha256": digest(token), "idempotency_key": "desktop-runner-" + newID()}
}

func TestDesktopRunnerContextRequiresNativeProofAndExactPrincipal(t *testing.T) {
	f, root := desktopRunnerFixture(t)
	body := desktopRunnerContextBody(f)
	before := domainACount(t, f.s, "suma_operations")
	out := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", body), 200)
	if out["project_id"] != f.project || out["user_id"] != f.user.ID || out["tenant_id"] != f.user.TenantID || out["repository_path"] != root || out["base_commit"] != workspaceGitTestCommand(t, root, "rev-parse", "HEAD") || out["repository_identity"] == "" || len(out["instance_id"].(string)) != 64 || len(out["binding_sha256"].(string)) != 64 {
		t.Fatalf("context does not bind actual local facts: %#v", out)
	}
	if n := domainACount(t, f.s, "suma_operations"); n != before {
		t.Fatal("context created a business operation")
	}
	body["user_id"] = newID()
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", body), 403)
	body["user_id"] = f.user.ID
	r := desktopRunnerTestRequest(f, "context", body)
	r.RemoteAddr = "192.0.2.1:33001"
	desktopRunnerTestCall(t, f, r, 403)
	f.s.DesktopWorkspaceKey = nil
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", body), 503)
}

func TestDesktopRunnerRegistrationPersistsOnlyDigestAndReplaysOriginal(t *testing.T) {
	f, _ := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	token := strings.Repeat("ab", 32)
	body := desktopRunnerRegistration(f, local, token)
	out := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	worker, ok := out["worker"].(map[string]any)
	if !ok || worker["worker_id"] != body["worker_id"] || worker["status"] != "active" || worker["deployment"] != false || out["operation_id"] != body["idempotency_key"] || strings.Contains(stringify(out), token) || strings.Contains(stringify(out), `"token"`) {
		t.Fatalf("invalid or sensitive registration result: %#v", out)
	}
	actor, err := f.s.AuthenticateActor(context.Background(), token)
	if err != nil || actor.kind != actorProjectRunner || actor.credential.projectID != f.project {
		t.Fatalf("pre-persisted credential is not a real project runner: %v", err)
	}
	var stored, receipt string
	if err = f.s.DB.QueryRow(`SELECT token_hash FROM suma_release_workers WHERE worker_id=?`, body["worker_id"]).Scan(&stored); err != nil || stored != digest(token) {
		t.Fatal("registry did not persist the exact digest", err)
	}
	if err = f.s.DB.QueryRow(`SELECT response FROM suma_operations WHERE operation_id=?`, body["idempotency_key"]).Scan(&receipt); err != nil || strings.Contains(receipt, token) || strings.Contains(receipt, `"token"`) {
		t.Fatal("secret entered durable receipt", err)
	}
	replayed := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	if stringify(out) != stringify(replayed) || domainACount(t, f.s, "suma_release_workers") != 1 || domainACount(t, f.s, "suma_credential_grants") != 1 {
		t.Fatal("original operation recovery duplicated or changed issuance")
	}
}

func TestDesktopRunnerRecoveryRechecksAuthorityAndRejectsChangedPayload(t *testing.T) {
	f, root := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	body := desktopRunnerRegistration(f, local, strings.Repeat("cd", 32))
	original := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	workspaceGitTestWrite(t, root, "main.go", "next committed contents")
	workspaceGitTestCommand(t, root, "add", "--", "main.go")
	workspaceGitTestCommand(t, root, "commit", "-m", "Later head")
	replayed := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	if stringify(original) != stringify(replayed) {
		t.Fatal("later HEAD replaced the original receipt")
	}
	body["adapter_id"] = "codebuddy-cli"
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 409)
	body["adapter_id"] = "codex-jsonl"
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 403)
}

func TestDesktopRunnerRejectedRegistrationLeavesNoCredential(t *testing.T) {
	f, _ := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	for _, tc := range []struct {
		field, value string
		status       int
	}{
		{"adapter_id", "workbuddy-cloud", 400}, {"worker_id", "ordinary-shadow", 400}, {"worker_id", "desktop-short", 400}, {"instance_id", strings.Repeat("0", 64), 409}, {"expected_binding_sha256", strings.Repeat("0", 64), 409}, {"base_commit", strings.Repeat("0", 40), 409}, {"credential_sha256", "not-a-hash", 400},
	} {
		t.Run(tc.field, func(t *testing.T) {
			body := desktopRunnerRegistration(f, local, newID()+newID())
			body[tc.field] = tc.value
			desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), tc.status)
		})
	}
	if domainACount(t, f.s, "suma_release_workers") != 0 || domainACount(t, f.s, "suma_credential_grants") != 0 {
		t.Fatal("rejected registration issued a credential")
	}
}

func TestDesktopRunnerRegistrationRollsBackCredentialWithActivityFailure(t *testing.T) {
	f, _ := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	body := desktopRunnerRegistration(f, local, strings.Repeat("ef", 32))
	if _, err := f.s.DB.Exec(`CREATE TRIGGER desktop_runner_activity_failure BEFORE INSERT ON suma_activity WHEN NEW.kind='release_worker.registered' BEGIN SELECT RAISE(ABORT,'fixture activity failure'); END`); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 500)
	for _, table := range []string{"suma_release_workers", "suma_credential_grants", "suma_token_scopes"} {
		if domainACount(t, f.s, table) != 0 {
			t.Fatalf("rollback left %s", table)
		}
	}
	_, err := f.s.AuthenticateActor(context.Background(), strings.Repeat("ef", 32))
	requireAuthorityStatus(t, err, 401)
}

func TestDesktopRunnerOrdinaryWorkerStillReturnsSecretOnlyOnce(t *testing.T) {
	f := newFixture(t)
	body := f.body(map[string]any{"worker_id": "ordinary-worker153", "profiles": []any{}, "deployment": false})
	first := f.call("POST", "/release-workers", body, 201)
	token, ok := first["token"].(string)
	if !ok || len(token) != 64 {
		t.Fatal("ordinary issuer lost one-time token")
	}
	second := f.call("POST", "/release-workers", body, 201)
	if _, exists := second["token"]; exists || second["id"] != first["id"] {
		t.Fatal("ordinary replay returned or reissued secret")
	}
}

func TestDesktopRunnerProofAndClosedHTTPEnvelope(t *testing.T) {
	f, _ := desktopRunnerFixture(t)
	body := desktopRunnerContextBody(f)
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{"missing proof", func(r *http.Request) { r.Header.Del("X-Suma-Desktop-Signature") }, 403},
		{"invalid MAC", func(r *http.Request) { r.Header.Set("X-Suma-Desktop-Signature", strings.Repeat("0", 64)) }, 403},
		{"duplicate nonce", func(r *http.Request) { r.Header.Add("X-Suma-Desktop-Nonce", newID()) }, 403},
		{"expired signed proof", func(r *http.Request) {
			r.Header.Set("X-Suma-Desktop-Expires", time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano))
			desktopRunnerResign(f, r, "context", body)
		}, 403},
		{"far future signed proof", func(r *http.Request) {
			r.Header.Set("X-Suma-Desktop-Expires", time.Now().UTC().Add(180*time.Second).Format(time.RFC3339Nano))
			desktopRunnerResign(f, r, "context", body)
		}, 403},
		{"action separation", func(r *http.Request) { desktopRunnerResign(f, r, "register", body) }, 403},
		{"remote host", func(r *http.Request) { r.Host = "example.test:4310" }, 403},
		{"query alias", func(r *http.Request) { r.URL.RawQuery = "project_id=" + newID() }, 400},
		{"duplicate JSON", func(r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(strings.TrimSuffix(stringify(body), "}") + `,"project_id":"` + f.project + `"}`))
		}, 400},
		{"case alias", func(r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(strings.Replace(stringify(body), `"project_id"`, `"Project_ID"`, 1)))
		}, 400},
		{"unknown key", func(r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(strings.TrimSuffix(stringify(body), "}") + `,"extra":"value"}`))
		}, 400},
		{"null scalar", func(r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(strings.Replace(stringify(body), `"project_id":"`+f.project+`"`, `"project_id":null`, 1)))
		}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := desktopRunnerTestRequest(f, "context", body)
			tc.mutate(r)
			desktopRunnerTestCall(t, f, r, tc.status)
		})
	}
}

func TestDesktopRunnerScopedCredentialCannotUseMachineProof(t *testing.T) {
	f, _ := desktopRunnerFixture(t)
	issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", Label: "desktop fixture", IdempotencyKey: "desktop-api-denied153"})
	if err != nil {
		t.Fatal(err)
	}
	r := desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f))
	r.Header.Del("Cookie")
	r.Header.Set("Authorization", "Bearer "+issued.Token)
	desktopRunnerTestCall(t, f, r, 403)
}

func TestDesktopRunnerRejectsUnbornAndUnsafeRepository(t *testing.T) {
	f, root := desktopWorkspaceFixture(t)
	workspaceGitTestCommand(t, root, "init", "--initial-branch=main")
	if _, err := f.s.BindDesktopWorkspace(context.Background(), f.actor, f.project, desktopWorkspaceTestRequest(f, root, "")); err != nil {
		t.Fatal(err)
	}
	body := desktopRunnerContextBody(f)
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", body), 422)
	workspaceGitTestCommand(t, root, "add", "--", "main.go")
	workspaceGitTestCommand(t, root, "commit", "-m", "Initial fixture")
	workspaceGitTestCommand(t, root, "config", "core.fsmonitor", "untrusted-hook")
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", body), 422)
	if domainACount(t, f.s, "suma_release_workers") != 0 {
		t.Fatal("invalid repository registered a worker")
	}
}

func TestDesktopRunnerFreshRegistrationCannotAdoptLaterHEAD(t *testing.T) {
	f, root := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	body := desktopRunnerRegistration(f, local, strings.Repeat("12", 32))
	workspaceGitTestWrite(t, root, "main.go", "later explicit change")
	workspaceGitTestCommand(t, root, "add", "--", "main.go")
	workspaceGitTestCommand(t, root, "commit", "-m", "Change after preview")
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 409)
	if domainACount(t, f.s, "suma_release_workers") != 0 || domainACount(t, f.s, "suma_credential_grants") != 0 {
		t.Fatal("changed HEAD left an issued credential")
	}
}
